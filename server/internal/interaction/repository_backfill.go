package interaction

import (
	"context"
	"fmt"
)

// BackfillRepo 互动缓存回源：点赞判重降级路径。
//
// 拆服务后原 BackfillRepo 中的视频/用户缓存回源已迁到各自服务：
//   - video-svc 的 BackfillVideoCache
//   - interaction 侧的评论用户信息改用 user-svc gRPC（见 CommentLogic）
//
// 本文件只保留 Redis 不可用时的纯 MySQL 点赞判重路径。
type BackfillRepo struct {
	interactionRepo *InteractionRepo
}

func NewBackfillRepo(interactionRepo *InteractionRepo) *BackfillRepo {
	return &BackfillRepo{interactionRepo: interactionRepo}
}

// FallbackCheckLike Redis 不可用时的降级点赞判重路径
// 通过 MySQL VideoLike 表判断用户是否已点赞，如果未点赞则写入点赞记录
//
// 返回值：
//   - created=true：真正新建了记录（首次点赞），调用方应发布 MQ 增量
//   - created=false：记录已存在（幂等），调用方不应发布 MQ 增量
func (r *BackfillRepo) FallbackCheckLike(ctx context.Context, userID string, videoID uint) (bool, error) {
	// 降级查 MySQL：如果已点赞则幂等返回
	status, err := r.interactionRepo.GetUserVideoInteraction(ctx, userID, videoID)
	if err != nil {
		return false, fmt.Errorf("MySQL 查询点赞状态失败: %w", err)
	}
	if status.IsLiked {
		// MySQL 中已有点赞记录，幂等返回
		return false, nil
	}

	// MySQL 中也无点赞记录，写入点赞记录
	created, err := r.interactionRepo.CreateVideoLike(ctx, userID, videoID)
	if err != nil {
		return false, fmt.Errorf("MySQL 写入点赞记录失败: %w", err)
	}

	return created, nil
}
