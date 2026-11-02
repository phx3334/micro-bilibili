package otelx

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler 暴露 Prometheus 抓取端点。
//
// 使用 promhttp.Handler() 的默认 registry：
// OTel 的 Prometheus exporter 在 Init 时已把 SDK 指标注册到同一个默认 registry，
// 因此这里无需自己拼 gatherer，也不会出现两套指标。
//
// OTel 关闭时默认 registry 只有 Go runtime 指标，端点仍可访问（弱依赖，不报错）。
func MetricsHandler() gin.HandlerFunc {
	handler := promhttp.Handler()
	return func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, c.Request)
	}
}
