package user

import (
	"context"
	"errors"
	"time"

	"fake_tiktok/pkg/redisx"

	"github.com/redis/go-redis/v9"
)

// JWTRepo 提供 JWT Token 的 Redis 存储操作：黑名单管理和 Refresh Token 持久化。
type JWTRepo struct {
	redis *redisx.RedisClient
}

// NewJWTRepo 创建一个 JWT Redis 仓储实例。
func NewJWTRepo(redisClient *redisx.RedisClient) *JWTRepo {
	return &JWTRepo{
		redis: redisClient,
	}
}

// ToBlackList 将 Token 加入黑名单。
// 键格式: {KeyPrefix}:blacklist:{token}
func (r *JWTRepo) ToBlackList(ctx context.Context, token string, ttl time.Duration) error {
	key := r.redis.BuildKey("blacklist", token)
	return r.redis.Set(ctx, key, "1", ttl)
}

// IsBlackListed 检查 Token 是否在黑名单中。
// 采用 fail-closed 策略：Redis 不可用时返回 true（拒绝 Token），
// 避免在 Redis 宕机期间放行已注销的 Token（安全优先）。
func (r *JWTRepo) IsBlackListed(ctx context.Context, token string) bool {
	key := r.redis.BuildKey("blacklist", token)
	_, err := r.redis.Get(ctx, key)
	if err == nil {
		// 键存在：Token 已被拉黑
		return true
	}
	if errors.Is(err, redis.Nil) {
		// 键不存在：Token 未被拉黑（正常路径）
		return false
	}
	// Redis 不可用或其他错误：安全优先，视为已拉黑（fail-closed）
	return true
}

// SetJWT 存储用户的 Refresh Token。
// 键格式: {KeyPrefix}:jwt:{uuid}
func (r *JWTRepo) SetJWT(ctx context.Context, id string, token string, expiry time.Duration) error {
	key := r.redis.BuildKey("jwt", id)
	return r.redis.Set(ctx, key, token, expiry)
}

// GetJWT 获取用户的 Refresh Token。
func (r *JWTRepo) GetJWT(ctx context.Context, id string) (string, error) {
	key := r.redis.BuildKey("jwt", id)
	return r.redis.Get(ctx, key)
}

// DelJWT 删除用户的 Refresh Token（登出时调用）。
func (r *JWTRepo) DelJWT(ctx context.Context, id string) error {
	key := r.redis.BuildKey("jwt", id)
	return r.redis.Del(ctx, key)
}
