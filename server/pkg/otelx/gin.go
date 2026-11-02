package otelx

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// gin.Context 中保存链路信息的 key，业务/日志可直接读取。
const (
	TraceIDKey = "trace_id"
	SpanIDKey  = "span_id"
)

// GinMiddleware 是自研的 HTTP 埋点中间件（不引入 contrib/otelgin），一次遍历同时产出：
//   - server span（可在 Tempo 中与下游 gRPC / MySQL / Redis 子 span 串起来）
//   - RED 指标：Requests 总数、Errors 数、Duration 直方图
//
// 必须注册在 GinLogger 之前：日志中间件依赖本中间件写回请求 ctx 与 trace_id。
func GinMiddleware() gin.HandlerFunc {
	tracer := otel.Tracer("fake_tiktok/http")
	meter := otel.Meter("fake_tiktok/http")

	requestsTotal, _ := meter.Int64Counter("http_server_requests_total",
		metric.WithDescription("HTTP 请求总数（RED 中的 Rate/Requests）"))
	requestDuration, _ := meter.Float64Histogram("http_server_request_duration_seconds",
		metric.WithDescription("HTTP 请求耗时（秒）"),
		metric.WithUnit("s"))
	errorsTotal, _ := meter.Int64Counter("http_server_errors_total",
		metric.WithDescription("HTTP 5xx 错误总数（RED 中的 Errors）"))

	return func(c *gin.Context) {
		start := time.Now()

		// 1. 续接上游链路：有 traceparent 就复用 trace，没有则新建。
		//    Extract 到 request ctx 后，下游 gRPC/DB/Redis 调用都会自动带上同一 trace_id。
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(),
			propagation.HeaderCarrier(c.Request.Header))

		// 2. 路由模板而非真实路径：/api/v1/video/:id 这类模板能避免指标 label 基数爆炸。
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		ctx, span := tracer.Start(ctx, c.Request.Method+" "+route,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.route", route),
				attribute.String("http.user_agent", c.Request.UserAgent()),
				attribute.String("net.peer.ip", c.ClientIP()),
			),
		)
		defer span.End()

		// 3. 把带 span 的 ctx 放回请求，业务取 c.Request.Context() 即自动继承链路。
		c.Request = c.Request.WithContext(ctx)
		if sc := span.SpanContext(); sc.IsValid() {
			c.Set(TraceIDKey, sc.TraceID().String())
			c.Set(SpanIDKey, sc.SpanID().String())
		}

		// 4. 回写 traceparent 响应头：前端 / CDN / 网关可据此把用户请求与后端链路关联。
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(c.Writer.Header()))

		c.Next()

		status := c.Writer.Status()
		statusStr := strconv.Itoa(status)
		span.SetAttributes(attribute.Int("http.status_code", status))

		// 5. 指标用独立 context.Background：请求被客户端取消时 ctx 已 Done，
		//    继续用请求 ctx 记录可能被 SDK 丢弃，导致统计少算。
		metricCtx := context.Background()
		requestsTotal.Add(metricCtx, 1, metric.WithAttributes(
			attribute.String("method", c.Request.Method),
			attribute.String("route", route),
			attribute.String("status", statusStr),
		))
		requestDuration.Record(metricCtx, time.Since(start).Seconds(), metric.WithAttributes(
			attribute.String("method", c.Request.Method),
			attribute.String("route", route),
		))
		if status >= http.StatusInternalServerError {
			errorsTotal.Add(metricCtx, 1, metric.WithAttributes(
				attribute.String("method", c.Request.Method),
				attribute.String("route", route),
				attribute.String("status", statusStr),
			))
			// 5xx 标记 span 失败，Tempo 里可直接按状态筛选
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	}
}
