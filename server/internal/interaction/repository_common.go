package interaction

import (
	"context"
	"time"
)

// defaultDBTimeout 是 MySQL 操作的默认超时时间。
const defaultDBTimeout = 5 * time.Second

// withTimeout 为 context 添加超时控制；已有 deadline 时透传。
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultDBTimeout)
}
