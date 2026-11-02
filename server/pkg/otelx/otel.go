// Package otelx 统一封装 OpenTelemetry 初始化与埋点辅助。
//
// 为什么集中在这里：OTel 的全局 Provider / Propagator 是一次性配置，
// 分散到各业务包会出现"有的服务采样率不同、有的没注册 propagator"的漂移；
// 统一入口后各 cmd 只需 Init 一次 + 挂中间件，业务代码只拿 Tracer。
//
// 数据流：
//   - Trace：OTLP gRPC → otel-collector → Tempo
//   - Metric：SDK 写入 Prometheus exporter 的默认 registry，由 /metrics 被 Prometheus 抓取
package otelx

import (
	"context"
	"errors"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"

	"fake_tiktok/pkg/config"
)

// 默认值集中在这里，避免每个服务的 yaml 都要写一遍。
const (
	defaultEndpoint    = "otel-collector:4317"
	defaultServiceName = "fake_tiktok"
	defaultEnvironment = "docker"
	defaultVersion     = "dev"
)

// enabled 记录 OTel 是否已成功启用。
// gormx / redisx / kafkax 这类基础设施包通过 Enabled() 判断是否挂载自动埋点，
// 避免把 "是否开启观测" 的判断继续散落到业务代码里。
var enabled atomic.Bool

// Enabled 返回 OTel 是否已启用。
func Enabled() bool {
	return enabled.Load()
}

// Tracer 返回指定 instrumentation scope 的 Tracer。
// OTel 未启用时全局 TracerProvider 是 noop 实现，调用方无需再做分支判断。
func Tracer(scope string) trace.Tracer {
	return otel.Tracer(scope)
}

// Init 初始化 TracerProvider（OTLP gRPC → collector）与 MeterProvider（Prometheus 拉取模式）。
//
// 返回的 shutdown 必须在进程退出前调用：BatchSpanProcessor 有内存缓冲，
// 不 flush 会丢掉最后一批 span（尤其是刚发生错误的那条链路）。
//
// 当 cfg.OTEL.Enabled=false 时直接返回 noop shutdown：不创建任何 exporter、
// 不注册任何 collector，零额外开销，本地没有采集器也能正常启动。
func Init(ctx context.Context, cfg *config.Config) (func(context.Context) error, error) {
	noopShutdown := func(context.Context) error { return nil }

	ocfg := cfg.OTEL
	if !ocfg.Enabled {
		return noopShutdown, nil
	}

	endpoint := ocfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	serviceName := ocfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	environment := ocfg.Environment
	if environment == "" {
		environment = defaultEnvironment
	}
	version := ocfg.Version
	if version == "" {
		version = defaultVersion
	}
	// 采样率语义：<=0 或 >1 视为"未配置"，回退到全量采样，避免误配导致完全采不到数据。
	ratio := ocfg.SampleRatio
	if ratio <= 0 || ratio > 1 {
		ratio = 1.0
	}

	// 用字面量 key 而不是 semconv 包：service.* 是稳定约定，不随 semconv 版本升级而变动。
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", serviceName),
		attribute.String("service.version", version),
		attribute.String("deployment.environment", environment),
	))
	if err != nil {
		return noopShutdown, err
	}

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return noopShutdown, err
	}

	// ParentBased：保留上游采样决策，保证跨服务链路不会被下游重新采样截断。
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)

	// Metric 只注册 Prometheus exporter，不做 OTLP metric push：
	// 推模式与 Prometheus 的拉模式重复，且 collector 侧还要额外维护 metrics pipeline。
	metricExporter, err := otelprom.New()
	if err != nil {
		// TracerProvider 已创建但未注册为全局，关闭底层 exporter 即可
		_ = tp.Shutdown(ctx)
		return noopShutdown, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(metricExporter),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	// 只传播 TraceContext（traceparent）：本项目不使用 Baggage，少一层解析开销。
	otel.SetTextMapPropagator(propagation.TraceContext{})
	enabled.Store(true)

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		enabled.Store(false)
		return errors.Join(errs...)
	}
	return shutdown, nil
}
