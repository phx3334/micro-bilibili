package interaction

import (
	"context"
	"strconv"

	"fake_tiktok/pkg/redisx"
)

// VideoCacheCounterRepo 互动操作对"视频动态计数缓存"的即时更新。
//
// 视频计数本身属于 video 库，只能由 worker 通过 Kafka 回刷 MySQL；
// 但 Redis 视频动态缓存是共享的、可重建的缓存，这里直接增减可以让前端
// 立即看到新数字，无需等待 worker 落库。缓存未命中时由下一次回源覆盖。
type VideoCacheCounterRepo struct {
	client *redisx.RedisClient
}

func NewVideoCacheCounterRepo(client *redisx.RedisClient) *VideoCacheCounterRepo {
	return &VideoCacheCounterRepo{client: client}
}

func (r *VideoCacheCounterRepo) dynamicKey(videoID uint) string {
	return r.client.BuildKey(redisx.VideoDynamicHashKey, strconv.FormatUint(uint64(videoID), 10))
}

// IncrementLikeCount 视频动态缓存中 likes_count +1
func (r *VideoCacheCounterRepo) IncrementLikeCount(ctx context.Context, videoID uint) error {
	_, err := r.client.HIncrBy(ctx, r.dynamicKey(videoID), "likes_count", 1)
	return err
}

// DecrementLikeCount 视频动态缓存中 likes_count -1（取消点赞）
func (r *VideoCacheCounterRepo) DecrementLikeCount(ctx context.Context, videoID uint) error {
	_, err := r.client.HIncrBy(ctx, r.dynamicKey(videoID), "likes_count", -1)
	return err
}

// IncrementCommentCount 视频动态缓存中 comment_count +delta（评论时调用）
func (r *VideoCacheCounterRepo) IncrementCommentCount(ctx context.Context, videoID uint, delta int) error {
	_, err := r.client.HIncrBy(ctx, r.dynamicKey(videoID), "comment_count", int64(delta))
	return err
}
