package interaction

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/pkg/redisx"

	"go.uber.org/zap"
)

// InteractionCacheRepo 互动状态缓存仓储（Redis 由三服务共享）。
//
// 数据结构：
//   - 点赞：SET，member=用户ID，用于 O(1) 判重（SISMEMBER/SADD）
//
// 关注 SET 已随关注关系迁往 user-svc，本服务不再维护。
type InteractionCacheRepo struct {
	client *redisx.RedisClient
	logger *zap.Logger
}

func NewInteractionCacheRepo(client *redisx.RedisClient, logger *zap.Logger) *InteractionCacheRepo {
	return &InteractionCacheRepo{client: client, logger: logger}
}

// saddWithExpireScript 将 SADD + EXPIRE 合并为原子操作，避免 SADD 成功但 EXPIRE 失败导致 SET 永不过期。
const saddWithExpireScript = `
local added = redis.call('SADD', KEYS[1], ARGV[1])
if added == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return added
`

// AddUserToVideoLikedSet 将用户加入视频的点赞 SET（标记已点赞）。
// 返回值 added=true 表示本次新增（首次点赞），false 表示用户已在 SET 中。
func (r *InteractionCacheRepo) AddUserToVideoLikedSet(ctx context.Context, userID string, videoID uint) (bool, error) {
	key := r.client.BuildKey(redisx.VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	// 使用 Lua 脚本原子执行 SADD + EXPIRE
	result, err := r.client.Client.Eval(ctx, saddWithExpireScript, []string{key}, userID, int64(redisx.InteractionCacheExpire/time.Second)).Int64()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// RemoveUserFromVideoLikedSet 从视频点赞 SET 中移除用户（回滚 SADD 操作）。
func (r *InteractionCacheRepo) RemoveUserFromVideoLikedSet(ctx context.Context, userID string, videoID uint) error {
	key := r.client.BuildKey(redisx.VideoLikedUsersSetKey, strconv.FormatUint(uint64(videoID), 10))
	return r.client.SRem(ctx, key, userID)
}
