package interaction

import (
	"context"
	"fmt"

	"fake_tiktok/internal/dto/request"

	"go.uber.org/zap"
)

type InteractionLogic struct {
	deps *LogicDeps
}

func NewInteractionLogic(deps *LogicDeps) *InteractionLogic {
	return &InteractionLogic{
		deps: deps,
	}
}

// ensureVideoExists 通过 video-svc gRPC 校验视频是否存在。
//
// 原实现直接读本地 videos 表；拆库后 videos 在 feed_video，交互前必须
// 先确认目标视频真实存在（否则会留下指向不存在视频的点赞记录）。
func (i *InteractionLogic) ensureVideoExists(ctx context.Context, videoID uint) error {
	video, err := i.deps.VideoClient.GetVideo(ctx, uint64(videoID))
	if err != nil || video == nil {
		return fmt.Errorf("视频不存在")
	}
	return nil
}

// LikeVideo 用户点赞视频
//
// 流程：
//  1. 通过 video-svc 校验视频存在
//  2. Redis SADD 原子判重；Redis 不可用时降级到 MySQL 判重
//  3. 首次点赞时：同步写 MySQL VideoLike → 发布 Kafka 点赞增量 → 更新 Redis 视频动态缓存
//
// 计数链路变更：likes_count 属于 video 库，interaction 只能发 Kafka 增量，
// 由 worker 消费后批量回刷（原实现直接 UPDATE videos，已删除）。
func (i *InteractionLogic) LikeVideo(ctx context.Context, req request.InteractionVideo) error {
	userID := req.UserID
	videoID := req.VideoID

	// ---- 第 0 步：跨服务校验视频存在 ----
	if err := i.ensureVideoExists(ctx, videoID); err != nil {
		return err
	}

	// ---- 第 1 步：Redis SADD 原子判重 ----
	var added bool
	added, redisErr := i.deps.InteractionCacheRepo.AddUserToVideoLikedSet(ctx, userID, videoID)

	if redisErr != nil {
		// Redis 不可用，降级到 MySQL
		i.deps.Logger.Warn("Redis 点赞判重失败，降级到 MySQL",
			zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(redisErr))

		created, mysqlErr := i.deps.BackfillRepo.FallbackCheckLike(ctx, userID, videoID)
		if mysqlErr != nil {
			i.deps.Logger.Error("MySQL 降级点赞失败",
				zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(mysqlErr))
			return fmt.Errorf("点赞服务暂时不可用，请稍后重试")
		}

		// created=false 表示记录已存在（幂等），无需发 MQ 增量
		if !created {
			return nil
		}

		// MySQL 降级路径中真正新建了点赞记录，发布 MQ 增量 + 回填 Redis SET
		i.publishLikeIncrement(ctx, videoID)
		i.backfillLikeCache(ctx, userID, videoID)
		return nil
	}

	// Redis 正常路径：added=false 表示已点赞（幂等返回）
	if !added {
		return nil
	}

	// ---- 第 2 步：同步写入 MySQL VideoLike 表 ----
	created, err := i.deps.InteractionRepo.CreateVideoLike(ctx, userID, videoID)
	if err != nil {
		i.deps.Logger.Error("MySQL 写入点赞记录失败",
			zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
		// MySQL 写入失败时回滚 Redis SET 中的 SADD 操作
		if rollbackErr := i.deps.InteractionCacheRepo.RemoveUserFromVideoLikedSet(ctx, userID, videoID); rollbackErr != nil {
			i.deps.Logger.Error("回滚 Redis SET 失败，可能存在数据不一致",
				zap.String("userID", userID), zap.Uint("videoID", videoID), zap.Error(rollbackErr))
		}
		return fmt.Errorf("点赞失败，请稍后重试")
	}

	// created=false：SET 过期后 SADD 返回 added=true，但 MySQL 中记录已存在
	// 不发 MQ 增量，避免 likes_count 虚高
	if !created {
		i.deps.Logger.Info("SET 过期后重复点赞，MySQL 幂等跳过 MQ 增量",
			zap.String("user_id", userID), zap.Uint("video_id", videoID))
		return nil
	}

	i.publishLikeIncrement(ctx, videoID)
	return nil
}

// publishLikeIncrement 点赞成功后的后续操作：发布 MQ 增量 + 更新 Redis 缓存
func (i *InteractionLogic) publishLikeIncrement(ctx context.Context, videoID uint) {
	if err := i.deps.CounterPublisher.PublishLikeIncrement(ctx, videoID, 1); err != nil {
		i.deps.Logger.Warn("Kafka 发布点赞增量失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}

	// 更新 Redis 视频动态缓存中的 likes_count +1（缓存共享、可重建，不影响库边界）
	if err := i.deps.VideoCacheCounterRepo.IncrementLikeCount(ctx, videoID); err != nil {
		i.deps.Logger.Warn("Redis 更新视频点赞数缓存失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}
}

// backfillLikeCache 降级路径成功后回填 Redis 点赞 SET
func (i *InteractionLogic) backfillLikeCache(ctx context.Context, userID string, videoID uint) {
	if _, err := i.deps.InteractionCacheRepo.AddUserToVideoLikedSet(ctx, userID, videoID); err != nil {
		i.deps.Logger.Warn("降级路径回填 Redis 点赞 SET 失败",
			zap.String("user_id", userID), zap.Uint("video_id", videoID), zap.Error(err))
	}
}

// UnlikeVideo 取消点赞视频
func (i *InteractionLogic) UnlikeVideo(ctx context.Context, req request.InteractionVideo) error {
	userID := req.UserID
	videoID := req.VideoID

	// 1. Redis SREM 移除（即使不存在也不报错）
	_ = i.deps.InteractionCacheRepo.RemoveUserFromVideoLikedSet(ctx, userID, videoID)

	// 2. MySQL 删除
	deleted, err := i.deps.InteractionRepo.DeleteVideoLike(ctx, userID, videoID)
	if err != nil {
		return fmt.Errorf("取消点赞失败，请稍后重试")
	}

	// deleted=false：原本未点赞，幂等返回
	if !deleted {
		return nil
	}

	// 3. 发布 -1 增量 + 更新缓存
	if err := i.deps.CounterPublisher.PublishLikeIncrement(ctx, videoID, -1); err != nil {
		i.deps.Logger.Warn("Kafka 发布取消点赞增量失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}
	if err := i.deps.VideoCacheCounterRepo.DecrementLikeCount(ctx, videoID); err != nil {
		i.deps.Logger.Warn("Redis 更新视频点赞数缓存失败",
			zap.Uint("video_id", videoID), zap.Error(err))
	}
	return nil
}
