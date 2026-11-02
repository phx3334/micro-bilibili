package main

import (
	"fmt"
	"runtime"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// zapLogger 为 robfig/cron 适配 zap 日志接口
type zapLogger struct {
	logger *zap.Logger
}

func (l *zapLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, l.toFields(keysAndValues...)...)
}

func (l *zapLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.logger.Error(msg, append(l.toFields(keysAndValues...), zap.Error(err))...)
}

func (l *zapLogger) toFields(keysAndValues ...interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			fields = append(fields, zap.Any(key, keysAndValues[i+1]))
		}
	}
	return fields
}

// InitCron 创建支持秒级精度的 Cron 调度器实例（zap 日志）。
// panic 恢复由 RecoverCronJob 在注册任务时统一包装。
func InitCron(logger *zap.Logger) *cron.Cron {
	zl := &zapLogger{logger: logger}
	return cron.New(cron.WithLogger(zl), cron.WithSeconds())
}

// RecoverCronJob 为单个 cron job 提供 panic 恢复包装器。
func RecoverCronJob(logger *zap.Logger) cron.JobWrapper {
	return func(j cron.Job) cron.Job {
		return cron.FuncJob(func() {
			defer func() {
				if r := recover(); r != nil {
					buf := make([]byte, 64<<10) // 64KB 堆栈缓冲
					buf = buf[:runtime.Stack(buf, false)]
					logger.Error("cron job panic",
						zap.Any("recover", r),
						zap.String("stack", string(buf)),
					)
				}
			}()
			j.Run()
		})
	}
}
