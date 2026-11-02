package otelx

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Logger 返回附带 trace_id / span_id 的 logger，用于把日志与链路关联起来。
//
// 为什么不直接改造全局 logger：trace_id 是请求维度的，只有拿到 ctx 的地方
// 才能正确带上；全局 logger 一旦注入就会串链路的 trace。
// 当 ctx 中没有有效 span（OTel 未启用或非请求路径）时原样返回，避免产生空字段。
func Logger(logger *zap.Logger, ctx context.Context) *zap.Logger {
	if logger == nil {
		logger = zap.L()
	}
	if ctx == nil {
		return logger
	}
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	)
}
