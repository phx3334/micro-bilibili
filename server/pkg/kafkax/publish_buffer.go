// 本文件提供"MQ 抖动时不阻塞业务请求"的有界缓冲发布能力。
//
// 设计动机与 RabbitMQ 版本保持一致：
//   - 直连发送失败时若让业务请求报错，播放量 / 点赞 / 转码链路的可用性会被 MQ 拖累
//   - 缓冲发布让 Publish 永不阻塞（最坏情况丢最旧），由后台 drainLoop 在恢复后重放
//
// 丢最旧策略：播放量、点赞数是可累加的弱一致数据，偶尔丢一条旧消息影响很小，
// 而阻塞请求或无限增长缓冲（OOM）的代价更大。
package kafkax

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// defaultBufferCapacity 环形缓冲容量上限：10000 条约占 10MB 内存，单机完全可接受。
const defaultBufferCapacity = 10000

// drainLoop 的指数退避区间：1s 起步，最长 30s，避免 Kafka 长时间不可用时空转。
const (
	initialDrainBackoff = 1 * time.Second
	maxDrainBackoff     = 30 * time.Second
)

// bufferedItem 缓冲内的一条待发消息。
type bufferedItem struct {
	topic      string
	key        string
	value      []byte
	headers    map[string]string
	enqueuedAt time.Time
}

// ringBuffer 是有界 FIFO 缓冲；满时丢最旧（drop-oldest）。
type ringBuffer struct {
	mu       sync.Mutex
	items    []bufferedItem
	capacity int
	dropped  atomic.Int64
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{capacity: capacity}
}

// push 推入一条新消息。返回 false 表示缓冲已满，丢弃了最旧一条。
func (b *ringBuffer) push(it bufferedItem) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) >= b.capacity {
		b.items = append(b.items[1:], it)
		b.dropped.Add(1)
		return false
	}
	b.items = append(b.items, it)
	return true
}

// drainAll 取出所有消息并清空缓冲；返回的 slice 所有权转移给调用方。
func (b *ringBuffer) drainAll() []bufferedItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) == 0 {
		return nil
	}
	items := b.items
	b.items = nil
	return items
}

// requeue 把发送失败的消息放回缓冲头部，保持 FIFO 顺序。
// 仍然受容量上限约束，防止 Kafka 长时间不可用时缓冲无限增长。
func (b *ringBuffer) requeue(items []bufferedItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = append(items, b.items...)
	if len(b.items) > b.capacity {
		dropped := len(b.items) - b.capacity
		b.dropped.Add(int64(dropped))
		b.items = b.items[dropped:]
	}
}

func (b *ringBuffer) depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// PublishBuffer 把 Producer 包装为"自动缓冲 + 自动重放"的发布器。
//
//   - Publish() 永不阻塞（最坏情况丢最旧）
//   - 后台 drainLoop 周期性重放缓冲，失败时指数退避
type PublishBuffer struct {
	producer *Producer
	buf      *ringBuffer
	logger   *zap.Logger

	drainDone chan struct{}
}

// NewPublishBuffer 构造缓冲发布器（未启动 drainLoop），调用方需调用 Start(ctx)。
func NewPublishBuffer(producer *Producer, logger *zap.Logger) *PublishBuffer {
	return &PublishBuffer{
		producer:  producer,
		buf:       newRingBuffer(defaultBufferCapacity),
		logger:    logger,
		drainDone: make(chan struct{}),
	}
}

// Start 启动后台 drainLoop，ctx 取消时退出。
func (p *PublishBuffer) Start(ctx context.Context) {
	go p.drainLoop(ctx)
}

// Publish 先尝试直发，失败则入缓冲并返回原始错误（供上层感知"刚才是因为 MQ 不可用"）。
func (p *PublishBuffer) Publish(ctx context.Context, topic, key string, value []byte, headers map[string]string) error {
	err := p.producer.Publish(ctx, topic, key, value, headers)
	if err == nil {
		return nil
	}

	it := bufferedItem{
		topic:      topic,
		key:        key,
		value:      value,
		headers:    headers,
		enqueuedAt: time.Now(),
	}
	if !p.buf.push(it) {
		p.logger.Warn("publish buffer full, dropped oldest",
			zap.Int("depth", p.buf.depth()),
			zap.Int64("total_dropped", p.buf.dropped.Load()))
	}
	return err
}

// drainLoop 是后台重放协程。
//
// 为什么用 time.After 的指数退避而不是固定 ticker：
//   - Kafka 短暂抖动时固定 1s 重试可能连续失败，指数退避能降低无谓请求
//   - 成功后立即回到 1s，保证恢复后的重放延迟可控
func (p *PublishBuffer) drainLoop(ctx context.Context) {
	defer close(p.drainDone)
	backoff := initialDrainBackoff
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		items := p.buf.drainAll()
		if len(items) == 0 {
			backoff = initialDrainBackoff
			continue
		}

		sentAll := true
		for i, it := range items {
			if err := p.producer.Publish(ctx, it.topic, it.key, it.value, it.headers); err != nil {
				p.logger.Warn("buffer drain publish failed, requeue",
					zap.Error(err),
					zap.Int("remaining", len(items)-i))
				p.buf.requeue(items[i:])
				sentAll = false
				break
			}
		}
		if sentAll {
			backoff = initialDrainBackoff
			continue
		}
		backoff *= 2
		if backoff > maxDrainBackoff {
			backoff = maxDrainBackoff
		}
	}
}
