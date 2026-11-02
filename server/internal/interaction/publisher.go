package interaction

import (
	"context"
	"encoding/json"
	"strconv"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/pkg/kafkax"
)

// CounterPublisher 互动计数增量发布器。
//
// 点赞数/评论数都是 videos 表字段（video 库），interaction-svc 只能
// 通过 Kafka 投递增量，由 worker 聚合后回刷 MySQL。这样服务间不共享写权限，
// 也避免同步跨库写造成的事务边界问题（原实现直接 UPDATE videos，已移除）。
type CounterPublisher struct {
	buf *kafkax.PublishBuffer
}

func NewCounterPublisher(buf *kafkax.PublishBuffer) *CounterPublisher {
	return &CounterPublisher{buf: buf}
}

// PublishLikeIncrement 发布点赞数增量到 video.likecount。
func (p *CounterPublisher) PublishLikeIncrement(ctx context.Context, videoID uint, increment int64) error {
	body, err := json.Marshal(cache.VideoLikeIncrementMsg{VideoID: videoID, Increment: increment})
	if err != nil {
		return err
	}
	return p.buf.Publish(ctx, kafkax.TopicVideoLikecount, strconv.FormatUint(uint64(videoID), 10), body, nil)
}

// PublishCommentIncrement 发布评论数增量到 video.commentcount。
func (p *CounterPublisher) PublishCommentIncrement(ctx context.Context, videoID uint, increment int64) error {
	body, err := json.Marshal(cache.VideoCommentIncrementMsg{VideoID: videoID, Increment: increment})
	if err != nil {
		return err
	}
	return p.buf.Publish(ctx, kafkax.TopicVideoCommentcount, strconv.FormatUint(uint64(videoID), 10), body, nil)
}
