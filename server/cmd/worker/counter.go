package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"fake_tiktok/pkg/kafkax"
)

// incrementMsg 三类计数增量消息（play/like/comment）字段结构完全相同：
// {video_id, increment}。用一个本地结构体解析即可，避免为每种 topic 重复写 loop。
type incrementMsg struct {
	VideoID   uint  `json:"video_id"`
	Increment int64 `json:"increment"`
}

// counterAggregator 把一个 videos 计数列的增量先聚合到内存，再定期批量回刷 MySQL。
//
// 为什么仍然延迟批写：
//   - 短时间大量点赞/播放会产生大量消息，聚合后一次 UPDATE col + N 显著减少 SQL 次数
//   - 计数是弱一致数据，几秒延迟用户几乎无感
//
// 数据安全：
//   - 先复制快照再释放锁，UPDATE 不持锁
//   - UPDATE 成功后才从 pending 中减去对应增量；失败保留，下次 flush 自动重试
//   - 停机时做最后一次 flush，避免内存增量丢失
type counterAggregator struct {
	db     *gorm.DB
	logger *zap.Logger
	column string

	mu      sync.Mutex
	pending map[uint]int64
}

func newCounterAggregator(db *gorm.DB, column string, logger *zap.Logger) *counterAggregator {
	return &counterAggregator{
		db:      db,
		logger:  logger,
		column:  column,
		pending: make(map[uint]int64),
	}
}

// add 累加一条增量（消费回调中调用）。
func (a *counterAggregator) add(videoID uint, increment int64) {
	a.mu.Lock()
	a.pending[videoID] += increment
	a.mu.Unlock()
}

// flush 把内存增量批量写入 MySQL 对应列。
func (a *counterAggregator) flush(ctx context.Context) {
	a.mu.Lock()
	if len(a.pending) == 0 {
		a.mu.Unlock()
		return
	}
	batch := make(map[uint]int64, len(a.pending))
	for k, v := range a.pending {
		batch[k] = v
	}
	a.mu.Unlock()

	var succeededIDs []uint
	for videoID, increment := range batch {
		// 单条 UPDATE 5s 超时，防止慢查询阻塞 flush 协程导致无法响应停机信号
		updateCtx, updateCancel := context.WithTimeout(ctx, 5*time.Second)
		expr := gorm.Expr(a.column+" + ?", increment)
		result := a.db.WithContext(updateCtx).Model(&struct{}{}).Table("videos").
			Where("id = ?", videoID).
			UpdateColumn(a.column, expr)
		updateCancel()
		if result.Error != nil {
			a.logger.Error("回刷视频计数失败",
				zap.String("column", a.column),
				zap.Uint("video_id", videoID),
				zap.Int64("increment", increment),
				zap.Error(result.Error))
			// 失败：pending 保留原值，下次 flush 重试
		} else {
			succeededIDs = append(succeededIDs, videoID)
		}
	}

	// 减去已成功写入的增量；flush 期间新增的增量会保留
	a.mu.Lock()
	for _, videoID := range succeededIDs {
		increment := batch[videoID]
		if pendingVal, ok := a.pending[videoID]; ok {
			if pendingVal <= increment {
				delete(a.pending, videoID)
			} else {
				a.pending[videoID] = pendingVal - increment
			}
		}
	}
	a.mu.Unlock()
}

// runFlusher 每 5s flush 一次；ctx 取消时做最后一次 flush 后退出。
func (a *counterAggregator) runFlusher(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// 用独立 context：根 ctx 已取消，直接传入会立刻超时
			a.flush(context.Background())
			return
		case <-ticker.C:
			a.flush(context.Background())
		}
	}
}

// consumeLoop 消费指定 topic，把增量累加到聚合器。
// 分区分配、心跳和断线重连由 kafka-go Reader 内部处理。
func (a *counterAggregator) consumeLoop(ctx context.Context, brokers []string, topic, groupID string) {
	reader := kafkax.NewReader(brokers, topic, groupID)
	defer reader.Close()

	kafkax.Consume(ctx, reader, a.logger, func(_ context.Context, _ []byte, value []byte) error {
		var inc incrementMsg
		if err := json.Unmarshal(value, &inc); err != nil {
			return err
		}
		a.add(inc.VideoID, inc.Increment)
		return nil
	})
}
