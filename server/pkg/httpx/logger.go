package httpx

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinLogger 返回 Gin 请求日志中间件，使用依赖注入的 logger
func GinLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()
		cost := time.Since(start)
		logger.Info(path,
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("cost", cost),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.String("error-message", c.Errors.ByType(gin.ErrorTypePrivate).String()),
		)
	}
}

// GinRecovery 返回 Gin panic 恢复中间件，使用依赖注入的 logger
func GinRecovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool

				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") ||
							strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)

				if brokenPipe {
					logger.Error(c.Request.URL.Path,
						zap.Any("error", err),
						zap.String("request", string(httpRequest)),
					)
					c.Error(err.(error))
					c.Abort()
					return
				}

				logger.Error("[Recovery from panic]",
					zap.Any("error", err),
					zap.String("request", string(httpRequest)),
					zap.String("stack", string(debug.Stack())),
				)

				// 返回 JSON 格式的错误信息，使前端能显示具体的错误提示
				// 而非仅收到 500 状态码无响应体
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code": 4,
					"data": nil,
					"msg":  fmt.Sprintf("服务器内部错误: %v", err),
				})
			}
		}()

		c.Next()
	}
}
