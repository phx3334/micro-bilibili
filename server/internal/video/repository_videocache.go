package video

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/pkg/redisx"
	"fake_tiktok/pkg/util"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// VideoCacheRepo 视频缓存仓储（Redis 三服务共享）
type VideoCacheRepo struct {
	client *redisx.RedisClient
	logger *zap.Logger
}

func NewVideoCacheRepo(client *redisx.RedisClient, logger *zap.Logger) *VideoCacheRepo {
	return &VideoCacheRepo{client: client, logger: logger}
}

// GetVideoCache 批量查询视频缓存（静态 Hash + 动态 Hash）
func (r *VideoCacheRepo) GetVideoCache(ctx context.Context, videoIDs []uint) (map[uint]*cache.VideoCacheData, []uint, error) {
	cacheMap := make(map[uint]*cache.VideoCacheData)
	hitMap := make(map[uint]bool)

	// 为 Pipeline 设置超时，防止 Redis 慢查询阻塞业务
	ctxPipe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	pipe := r.client.Pipeline()
	if pipe == nil {
		return cacheMap, videoIDs, redisx.ErrRedisUnavailable
	}

	for _, vid := range videoIDs {
		staticKey := r.client.BuildKey(redisx.VideoStaticHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HGetAll(ctxPipe, staticKey)

		dynamicKey := r.client.BuildKey(redisx.VideoDynamicHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HGetAll(ctxPipe, dynamicKey)

		// 查询空对象标记，防止缓存穿透
		emptyKey := r.client.BuildKey(redisx.VideoEmptyHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HGetAll(ctxPipe, emptyKey)
	}

	cmds, execErr := pipe.Exec(ctxPipe)
	if execErr != nil && len(cmds) == 0 {
		// Pipeline 完全失败（如连接断开），返回错误让上层感知
		return cacheMap, util.CacheMissIDs(hitMap, videoIDs), execErr
	}

	for i, vid := range videoIDs {
		cmdIdx := i * 3 // 每个 videoID 有 3 个命令（static + dynamic + empty）
		if cmdIdx+2 >= len(cmds) {
			break
		}

		data := &cache.VideoCacheData{}
		hasStaticData := false

		// 检查空对象标记
		if emptyCmd, ok := cmds[cmdIdx+2].(*redis.MapStringStringCmd); ok {
			emptyMap := emptyCmd.Val()
			if len(emptyMap) > 0 {
				// 命中空对象标记：视频确实不存在，标记为命中避免穿透到 MySQL
				data.IsEmpty = true
				hitMap[vid] = true
				cacheMap[vid] = data
				continue
			}
		}

		if staticCmd, ok := cmds[cmdIdx].(*redis.MapStringStringCmd); ok {
			staticMap := staticCmd.Val()
			if len(staticMap) > 0 {
				hasStaticData = true
				data.PlayURL = staticMap["play_url"]
				data.CoverURL = staticMap["cover_url"]
				if dur, err := strconv.ParseFloat(staticMap["duration"], 64); err == nil {
					data.Duration = dur
				}
				data.AuthorID = staticMap["author_id"]
				data.Title = staticMap["title"]
				data.Description = staticMap["description"]
				data.Zone = staticMap["zone"]
				data.AuthorName = staticMap["author_name"]
				data.AuthorAvatar = staticMap["author_avatar"]
				if pop, err := strconv.ParseFloat(staticMap["popularity"], 64); err == nil {
					data.Popularity = pop
				}
				if ts, err := strconv.ParseInt(staticMap["created_at"], 10, 64); err == nil {
					data.CreatedAt = time.Unix(ts, 0)
				}
			}
		}

		// 只有静态数据存在时，才认为缓存命中
		if !hasStaticData {
			data.IsEmpty = true
		}
		if hasStaticData {
			if dynCmd, ok := cmds[cmdIdx+1].(*redis.MapStringStringCmd); ok {
				dynMap := dynCmd.Val()
				if pl, err := strconv.ParseInt(dynMap["play_count"], 10, 64); err == nil {
					data.PlayCount = pl
				}
				if cc, err := strconv.ParseInt(dynMap["comment_count"], 10, 64); err == nil {
					data.CommentCnt = cc
				}
				if lc, err := strconv.ParseInt(dynMap["likes_count"], 10, 64); err == nil {
					data.LikesCnt = lc
				}
			}
			hitMap[vid] = true
		}

		cacheMap[vid] = data
	}

	return cacheMap, util.CacheMissIDs(hitMap, videoIDs), nil
}

func (r *VideoCacheRepo) IncrementPlayCount(ctx context.Context, videoID uint) error {
	dynamicKey := r.client.BuildKey(redisx.VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
	_, err := r.client.HIncrBy(ctx, dynamicKey, "play_count", 1)
	return err
}

func (r *VideoCacheRepo) WriteVideoCache(ctx context.Context, items []*cache.VideoCacheData, emptyIDs []uint) {
	if len(items) == 0 && len(emptyIDs) == 0 {
		return
	}

	pipe := r.client.Pipeline()
	if pipe == nil {
		r.logger.Error("[BatchCache] batch write cache failed", zap.Error(redisx.ErrRedisUnavailable))
		return
	}

	ctxPipe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	for _, item := range items {
		vidStr := strconv.FormatUint(uint64(item.VideoID), 10)

		staticKey := r.client.BuildKey(redisx.VideoStaticHashKey, vidStr)
		pipe.HSet(ctxPipe, staticKey,
			"play_url", item.PlayURL,
			"cover_url", item.CoverURL,
			"duration", strconv.FormatFloat(item.Duration, 'f', 2, 64),
			"author_id", item.AuthorID,
			"title", item.Title,
			"description", item.Description,
			"zone", item.Zone,
			"author_name", item.AuthorName,
			"author_avatar", item.AuthorAvatar,
			"popularity", strconv.FormatFloat(item.Popularity, 'f', 2, 64),
			"created_at", item.CreatedAt.Unix(),
		)
		pipe.Expire(ctxPipe, staticKey, redisx.TTLJitter(redisx.VideoStaticHashExpire, 1*time.Hour))

		dynamicKey := r.client.BuildKey(redisx.VideoDynamicHashKey, vidStr)
		// 使用 HSETNX 回填动态计数，避免覆盖 HIncrBy 的并发递增结果
		pipe.HSetNX(ctxPipe, dynamicKey, "play_count", item.PlayCount)
		pipe.HSetNX(ctxPipe, dynamicKey, "comment_count", item.CommentCnt)
		pipe.HSetNX(ctxPipe, dynamicKey, "likes_count", item.LikesCnt)
		// 动态缓存不过期（redisx.VideoDynamicHashExpire=0），由业务写操作实时维护
		if redisx.VideoDynamicHashExpire > 0 {
			pipe.Expire(ctxPipe, dynamicKey, redisx.TTLJitter(redisx.VideoDynamicHashExpire, 1*time.Hour))
		}
	}

	for _, vid := range emptyIDs {
		emptyKey := r.client.BuildKey(redisx.VideoEmptyHashKey, strconv.FormatUint(uint64(vid), 10))
		pipe.HSet(ctxPipe, emptyKey, "empty", "1")
		pipe.Expire(ctxPipe, emptyKey, redisx.VideoEmptyHashExpire)
	}

	if _, err := pipe.Exec(ctxPipe); err != nil {
		r.logger.Error("[BatchCache] batch write cache failed", zap.Error(err))
	}
}

// DeleteVideoCache 删除指定视频的全部 Redis 缓存：静态 Hash、动态 Hash、空对象标记。
func (r *VideoCacheRepo) DeleteVideoCache(ctx context.Context, videoID uint) error {
	if r.client.Client == nil {
		return redisx.ErrRedisUnavailable
	}
	ctx, cancel := r.client.WithTimeout(ctx)
	defer cancel()
	vid := strconv.FormatUint(uint64(videoID), 10)
	keys := []string{
		r.client.BuildKey(redisx.VideoStaticHashKey, vid),
		r.client.BuildKey(redisx.VideoDynamicHashKey, vid),
		r.client.BuildKey(redisx.VideoEmptyHashKey, vid),
	}
	return r.client.Client.Del(ctx, keys...).Err()
}
