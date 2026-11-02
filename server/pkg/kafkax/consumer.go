package kafkax

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"fake_tiktok/pkg/otelx"
)

// NewReader 创建消费组 Reader。
//
// kafka-go 的 Reader 内部处理分区分配、心跳和断线重连，因此 worker 不再需要
// 自己写"外层重订阅循环"；ctx 取消后调用 Close 即可优雅退出。
func NewReader(brokers []string, topic, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1,        // 有消息就返回，降低聚合延迟
		MaxBytes: 10 << 20, // 单次拉取上限 10MB，避免一次拉取过大占内存
	})
}

// Consume 是通用消费循环：FetchMessage → handler → CommitMessages。
//
// handler 返回错误时只记录日志、仍然提交 offset：
// 毒消息如果反复重试会卡死整个分区，本项目接受"记录 + 丢弃"的取舍。
func Consume(ctx context.Context, reader *kafka.Reader, logger *zap.Logger, handler func(ctx context.Context, key, value []byte) error) {
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			// ctx 取消或 Reader 被关闭：正常退出
			if ctx.Err() != nil || errors.Is(err, io.EOF) {
				return
			}
			logger.Warn("kafka fetch message failed", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		// 消费端续接生产端链路：handler 内部新建的 span 会成为同一 trace 的子 span，
		// 从而把"HTTP 上传 → 投递转码 → worker 转码"串成一条完整调用链。
		// handler 签名保持不变，提取动作在这里完成，改动面最小。
		if err := handler(otelx.ExtractHeaders(ctx, headersToMap(msg.Headers)), msg.Key, msg.Value); err != nil {
			logger.Error("kafka message handler failed, committing to avoid stuck partition",
				zap.Error(err),
				zap.String("topic", msg.Topic),
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset))
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("kafka commit message failed",
				zap.Error(err),
				zap.String("topic", msg.Topic),
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset))
		}
	}
}

// headersToMap 把 kafka.Header 切片转成 map[string]string 供 OTel propagator 使用。
// 无 header 时返回 nil，ExtractHeaders 会跳过解析。
func headersToMap(headers []kafka.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	m := make(map[string]string, len(headers))
	for _, h := range headers {
		m[string(h.Key)] = string(h.Value)
	}
	return m
}
