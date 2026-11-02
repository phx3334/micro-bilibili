package otelx

import (
	"context"

	"go.opentelemetry.io/otel"
)

// mapCarrier 把 map[string]string 适配成 OTel 的 TextMapCarrier，
// 用于在 Kafka 消息头这种非 HTTP 载体上注入/提取 W3C traceparent。
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string { return c[key] }

func (c mapCarrier) Set(key, value string) { c[key] = value }

func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectHeaders 把当前 span 的 traceparent 写入 Kafka 消息头，生产端调用。
// headers 必须由调用方保证非 nil（map 是引用类型，这里无法回填 nil map）。
// OTel 关闭时 propagator 为 noop，不产生任何写入。
func InjectHeaders(ctx context.Context, headers map[string]string) {
	if !enabled.Load() || headers == nil {
		return
	}
	otel.GetTextMapPropagator().Inject(ctx, mapCarrier(headers))
}

// ExtractHeaders 从 Kafka 消息头还原上游链路，消费端在调用 handler 前调用。
// 没有 traceparent 时返回原 ctx（即开启一条新的根链路）。
func ExtractHeaders(ctx context.Context, headers map[string]string) context.Context {
	if !enabled.Load() || len(headers) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, mapCarrier(headers))
}
