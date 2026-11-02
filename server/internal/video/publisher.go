package video

import (
	"context"
	"encoding/json"
	"strconv"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/pkg/kafkax"
)

// PlayCountPublisher 把"播放量 +1"事件发布到 Kafka video.playcount。
//
// 原实现位于 pkg/kafkax（依赖 repository/interfaces）；拆服务后发布器归 video-svc，
// pkg 只保留与业务无关的 Producer/PublishBuffer。
type PlayCountPublisher struct {
	buf *kafkax.PublishBuffer
}

func NewPlayCountPublisher(buf *kafkax.PublishBuffer) *PlayCountPublisher {
	return &PlayCountPublisher{buf: buf}
}

// PublishIncrement 把"videoID 增加 increment"事件发到 video.playcount。
func (p *PlayCountPublisher) PublishIncrement(ctx context.Context, videoID uint, increment int64) error {
	body, err := json.Marshal(cache.PlayCountIncrementMsg{VideoID: videoID, Increment: increment})
	if err != nil {
		// 序列化失败：业务方错误，不应入缓冲
		return err
	}
	// partition key 用 video_id，保证同一视频的增量落到同一分区、按序消费
	return p.buf.Publish(ctx, kafkax.TopicVideoPlaycount, strconv.FormatUint(uint64(videoID), 10), body, nil)
}

// TranscodePublisher 把视频转码任务发布到 Kafka video.transcode。
type TranscodePublisher struct {
	buf *kafkax.PublishBuffer
}

func NewTranscodePublisher(buf *kafkax.PublishBuffer) *TranscodePublisher {
	return &TranscodePublisher{buf: buf}
}

// Publish 将一条 TranscodeMsg 发布到 video.transcode。
func (p *TranscodePublisher) Publish(ctx context.Context, msg cache.TranscodeMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// partition key 用 video_id，同一视频的转码任务固定落到同一分区
	return p.buf.Publish(ctx, kafkax.TopicVideoTranscode, strconv.FormatUint(uint64(msg.VideoID), 10), body, nil)
}
