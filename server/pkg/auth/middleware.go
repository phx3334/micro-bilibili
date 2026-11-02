package auth

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/httpx"
)

// TokenBlacklist 由 user-svc 提供实现；其它服务传 nil 表示不做黑名单校验
type TokenBlacklist interface {
	IsBlackListed(ctx context.Context, refreshToken string) bool
}

// JWTAuth 构造无状态 JWT 中间件。
//
// 与旧实现的行为差异：
//   - blacklist 为 nil 时跳过黑名单检查（video/interaction 等无账号库的服务直接传 nil）
//   - access token 过期时用 refresh token 换发新 access token，不再回查账号库
//
// 两者共同去掉了对 AccountRepository 的依赖，使中间件可以从 internal 提到 pkg，
// 拆服务后各服务直接复用而不需要共享用户仓储。
func JWTAuth(cfg *config.Config, blacklist TokenBlacklist) gin.HandlerFunc {
	// 在中间件初始化时创建 JWT 实例，避免每次请求都重复创建
	// JWT 实例是无状态的（只包含 secret 字节切片），可以安全复用
	j := NewJWT(&cfg.JWT)

	return func(c *gin.Context) {
		accessToken := GetAccessToken(c)
		refreshToken := GetRefreshToken(c)

		// 黑名单依赖 Redis 记录，只有持有该实现的服务（user-svc）才校验
		if blacklist != nil && blacklist.IsBlackListed(c.Request.Context(), refreshToken) {
			ClearRefreshToken(c)
			httpx.NoAuth("Please login again", c)
			c.Abort()
			return
		}
		claims, err := j.ParseAccessToken(accessToken)
		if err != nil {
			if accessToken == "" || errors.Is(err, TokenExpired) {
				refreshClaims, err := j.ParseRefreshToken(refreshToken)
				if err != nil {
					ClearRefreshToken(c)
					httpx.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				// 无状态刷新：只用 refresh token 中的 UserID，无需 accountRepo.FindByID
				newAccessClaims, err := j.CreateAccessClaims(request.BaseClaims{
					UserID: refreshClaims.UserID,
				}, &cfg.JWT)
				if err != nil {
					ClearRefreshToken(c)
					httpx.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				newaccessToken, err := j.CreateAccessToken(newAccessClaims)
				if err != nil {
					ClearRefreshToken(c)
					httpx.NoAuth("Please login again", c)
					c.Abort()
					return
				}
				c.Header("new-access-token", newaccessToken)
				c.Header("new-access-expiry", strconv.FormatInt(newAccessClaims.ExpiresAt.Unix(), 10))
				c.Set("claims", &newAccessClaims)
				c.Next()
				return
			}
			ClearRefreshToken(c)
			httpx.NoAuth("Please login again", c)
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}
