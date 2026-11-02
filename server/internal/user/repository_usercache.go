package user

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/pkg/redisx"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// UserCacheRepo 用户缓存仓储（Redis 由三个服务共享，key 前缀已带命名空间）
type UserCacheRepo struct {
	client *redisx.RedisClient
	logger *zap.Logger
}

func NewUserCacheRepo(client *redisx.RedisClient, logger *zap.Logger) *UserCacheRepo {
	return &UserCacheRepo{client: client, logger: logger}
}

// GetUserCache 通过 Pipeline 并行查询 user:static 和 user:dynamic 两个 Hash，
// 合并为组合 UserCacheData 返回。
// 两个 Hash 都不存在时返回 (nil, nil)。
// 如果 static 未命中但 dynamic 命中，返回的 UserCacheData 中 StaticHit=false，
// 调用方可据此决定是否对 static 区做降级回源（动态区计数不需要回源）。
func (r *UserCacheRepo) GetUserCache(ctx context.Context, userID string) (*cache.UserCacheData, error) {
	staticKey := r.client.BuildKey(redisx.UserStaticHashKey, userID)
	dynamicKey := r.client.BuildKey(redisx.UserDynamicHashKey, userID)
	emptyKey := r.client.BuildKey(redisx.UserEmptyHashKey, userID)

	pipe := r.client.Pipeline()
	if pipe == nil {
		return nil, redisx.ErrRedisUnavailable
	}
	emptyCmd := pipe.Exists(ctx, emptyKey)
	staticCmd := pipe.HGetAll(ctx, staticKey)
	dynamicCmd := pipe.HGetAll(ctx, dynamicKey)

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	// 先检查空对象标记，命中则视为"用户不存在"的缓存命中，避免穿透到 MySQL
	if exists, _ := emptyCmd.Result(); exists > 0 {
		return nil, nil
	}

	staticMap, _ := staticCmd.Result()
	dynamicMap, _ := dynamicCmd.Result()

	// 两个 Hash 都不存在视为缓存未命中
	if len(staticMap) == 0 && len(dynamicMap) == 0 {
		return nil, nil
	}

	var static *cache.UserStaticData
	var dynamic *cache.UserDynamicData

	if len(staticMap) > 0 {
		static = &cache.UserStaticData{
			ID:        staticMap["id"],
			AvatarURL: staticMap["avatar_url"],
			Username:  staticMap["username"],
			Signature: staticMap["signature"],
			Birthday:  staticMap["birthday"],
			Gender:    staticMap["gender"],
		}
	}

	if len(dynamicMap) > 0 {
		dynamic = &cache.UserDynamicData{}
		if v, err := strconv.ParseInt(dynamicMap["fans_count"], 10, 64); err == nil {
			dynamic.FansCount = v
		}
		if v, err := strconv.ParseInt(dynamicMap["following_count"], 10, 64); err == nil {
			dynamic.FollowingCount = v
		}
	}

	result := cache.MergeUserCacheData(static, dynamic)
	// 标记静态区是否命中，调用方据此决定是否对 static 做降级回源
	result.StaticHit = len(staticMap) > 0
	return result, nil
}

// BatchWriteUserCache 批量写入用户缓存，通过 Pipeline 同时写入 static 和 dynamic 两个 Hash
func (r *UserCacheRepo) BatchWriteUserCache(ctx context.Context, items []cache.UserCacheData) {
	if len(items) == 0 {
		return
	}

	pipe := r.client.Pipeline()
	if pipe == nil {
		r.logger.Error("[UserCache] batch write cache failed", zap.Error(redisx.ErrRedisUnavailable))
		return
	}

	ctxPipe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	for _, item := range items {
		staticKey := r.client.BuildKey(redisx.UserStaticHashKey, item.ID)
		pipe.HSet(ctxPipe, staticKey,
			"id", item.ID,
			"username", item.Username,
			"avatar_url", item.AvatarURL,
			"signature", item.Signature,
			"birthday", item.Birthday,
			"gender", item.Gender,
		)
		pipe.Expire(ctxPipe, staticKey, redisx.TTLJitter(redisx.UserStaticHashExpire, 1*time.Hour))

		dynamicKey := r.client.BuildKey(redisx.UserDynamicHashKey, item.ID)
		// 使用 HSETNX 回填动态计数，避免覆盖 HIncrBy 的并发递增结果
		pipe.HSetNX(ctxPipe, dynamicKey, "fans_count", item.FansCount)
		pipe.HSetNX(ctxPipe, dynamicKey, "following_count", item.FollowingCount)
		// 动态缓存不过期（redisx.UserDynamicHashExpire=0），由业务写操作实时维护
		if redisx.UserDynamicHashExpire > 0 {
			pipe.Expire(ctxPipe, dynamicKey, redisx.TTLJitter(redisx.UserDynamicHashExpire, 1*time.Hour))
		}
	}

	if _, err := pipe.Exec(ctxPipe); err != nil {
		r.logger.Error("[UserCache] batch write cache failed", zap.Error(err))
	}
}

// IncrementFansCount 原子自增用户粉丝数（HIncrBy +1），写入 user:dynamic Hash
func (r *UserCacheRepo) IncrementFansCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(redisx.UserDynamicHashKey, userID)
	_, err := r.client.HIncrBy(ctx, key, "fans_count", 1)
	return err
}

// IncrementFollowingCount 原子自增用户关注数（HIncrBy +1），写入 user:dynamic Hash
func (r *UserCacheRepo) IncrementFollowingCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(redisx.UserDynamicHashKey, userID)
	_, err := r.client.HIncrBy(ctx, key, "following_count", 1)
	return err
}

// DecrementFansCount 原子自减用户粉丝数，取关时调用，保证主页展示的粉丝数实时回落。
func (r *UserCacheRepo) DecrementFansCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(redisx.UserDynamicHashKey, userID)
	_, err := r.client.HIncrBy(ctx, key, "fans_count", -1)
	return err
}

// DecrementFollowingCount 原子自减用户关注数（HIncrBy -1），取关时调用。
func (r *UserCacheRepo) DecrementFollowingCount(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	key := r.client.BuildKey(redisx.UserDynamicHashKey, userID)
	_, err := r.client.HIncrBy(ctx, key, "following_count", -1)
	return err
}

// DeleteUserCache 仅删除静态区缓存（user:static），动态区缓存（计数类）不受影响。
func (r *UserCacheRepo) DeleteUserCache(ctx context.Context, userID string) error {
	staticKey := r.client.BuildKey(redisx.UserStaticHashKey, userID)
	return r.client.Del(ctx, staticKey)
}
