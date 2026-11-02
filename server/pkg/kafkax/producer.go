// Package kafkax 封装 Kafka 生产/消费能力，替换原来的 RabbitMQ 实现。
//
// 设计要点：
//   - 生产端用 kafka.Writer：RequiredAcks=all 保证消息不因 leader 落盘前崩溃而丢失
//   - Writer 懒初始化：构造时不做任何网络操作，API 启动不依赖 Kafka 可用
//   - 消费端用 kafka.Reader 消费组：分区分配与断线重连由 kafka-go 内部处理
package kafkax

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/otelx"
)

// ErrProducerClosed 表示 Producer 已被 Close；调用方应当回退到缓冲或直接放弃本次发送。
var ErrProducerClosed = errors.New("kafka producer closed")

// Producer 是 *kafka.Writer 的线程安全封装。
//
// Writer 本身支持多 topic 写入（Topic 留空时以 Message.Topic 为准），
// 因此整个进程只用一个 Writer，避免每个 topic 各建一套连接。
type Producer struct {
	brokers []string
	logger  *zap.Logger

	// mu 保护 writer 的懒初始化与 Close，避免并发首次发送重复创建 Writer
	mu     sync.RWMutex
	writer *kafka.Writer
	closed bool
}

// NewProducer 构造 Producer，但不会立刻连接 Kafka——首次 Publish 才创建 Writer。
func NewProducer(cfg *config.KafkaConfig, logger *zap.Logger) *Producer {
	return &Producer{
		brokers: cfg.Brokers,
		logger:  logger,
	}
}

// Publish 发送一条消息。key 同时作为 partition key：
// 相同 key 的消息落到同一分区，从而保证同一实体的增量按序消费。
func (p *Producer) Publish(ctx context.Context, topic, key string, value []byte, headers map[string]string) error {
	w, release, err := p.writerForWrite()
	if err != nil {
		return err
	}
	defer release()

	// 链路透传：把当前 span 的 traceparent 写进消息头，worker 侧据此续接同一条 trace。
	// 只有开启 OTel 时才为 nil headers 建 map，关闭时零额外分配。
	if otelx.Enabled() {
		if headers == nil {
			headers = make(map[string]string, 1)
		}
		otelx.InjectHeaders(ctx, headers)
	}

	msg := kafka.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   value,
		Headers: buildHeaders(headers),
	}
	if err := w.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka publish to %s: %w", topic, err)
	}
	return nil
}

// writerForWrite 返回可用的 Writer，并持有读锁直到调用方写入完成，
// 防止 Close 恰好发生在发送中途导致 writer 被提前关闭。
func (p *Producer) writerForWrite() (*kafka.Writer, func(), error) {
	p.mu.RLock()
	if !p.closed && p.writer != nil {
		w := p.writer
		return w, p.mu.RUnlock, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, nil, ErrProducerClosed
	}
	if p.writer == nil {
		p.writer = &kafka.Writer{
			Addr:     kafka.TCP(p.brokers...),
			Balancer: &kafka.Hash{},
			// 等待所有 ISR 确认，弱一致的计数数据也尽量不丢
			RequiredAcks: kafka.RequireAll,
			Async:        false,
			// 首次写入未知 topic 时由 broker 自动创建，避免必须先手工建 topic
			AllowAutoTopicCreation: true,
		}
		p.logger.Info("kafka producer initialized", zap.Strings("brokers", p.brokers))
	}
	w := p.writer
	p.mu.Unlock()

	p.mu.RLock()
	if p.closed || p.writer != w {
		p.mu.RUnlock()
		return nil, nil, ErrProducerClosed
	}
	return w, p.mu.RUnlock, nil
}

// Close 关闭 Writer。重复调用安全。
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.writer == nil {
		return nil
	}
	err := p.writer.Close()
	p.writer = nil
	return err
}

// buildHeaders 把 map 转成 kafka.Header 切片；空 map 返回 nil 以免产生空切片。
func buildHeaders(headers map[string]string) []kafka.Header {
	if len(headers) == 0 {
		return nil
	}
	hs := make([]kafka.Header, 0, len(headers))
	for k, v := range headers {
		hs = append(hs, kafka.Header{Key: k, Value: []byte(v)})
	}
	return hs
}
