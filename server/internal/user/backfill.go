package user

import (
	"context"
	"fmt"

	"fake_tiktok/internal/dto/cache"
)

// BackfillRepo 用户缓存回源：账户信息、粉丝数与关注数都在本服务的 feed_user 库内，
// 因此不需要跨服务调用，直接从本地仓库聚合即可。
type BackfillRepo struct {
	accountRepo   *AccountRepo
	followRepo    *FollowRepo
	userCacheRepo *UserCacheRepo
}

func NewBackfillRepo(accountRepo *AccountRepo, followRepo *FollowRepo, userCacheRepo *UserCacheRepo) *BackfillRepo {
	return &BackfillRepo{
		accountRepo:   accountRepo,
		followRepo:    followRepo,
		userCacheRepo: userCacheRepo,
	}
}

// BackfillUserCache 从 MySQL 回源用户信息（含粉丝数/关注数）并回填 Redis 缓存
//
// 返回值：
//   - data: 用户缓存数据（成功时返回，失败时返回 nil）
//   - err: 错误信息（用于上层记录日志，但不阻塞业务返回）
func (r *BackfillRepo) BackfillUserCache(ctx context.Context, userID string) (*cache.UserCacheData, error) {
	account, err := r.accountRepo.FindByID(ctx, userID)
	if err != nil {
		// 记录错误日志，便于排查 MySQL 故障
		return nil, fmt.Errorf("find account by id %s: %w", userID, err)
	}
	if account == nil {
		// 用户不存在（已注销或 ID 错误），不写缓存
		return nil, nil
	}

	static, dynamic := cache.AccountToUserCacheData(account)

	// 从 follow 表查询粉丝数和关注数
	fansCount, fansErr := r.followRepo.GetFansCount(ctx, userID)
	if fansErr == nil {
		dynamic.FansCount = fansCount
	}
	followingCount, followingErr := r.followRepo.GetFollowingCount(ctx, userID)
	if followingErr == nil {
		dynamic.FollowingCount = followingCount
	}

	data := cache.MergeUserCacheData(static, dynamic)

	// 回填 Redis 缓存
	r.userCacheRepo.BatchWriteUserCache(ctx, []cache.UserCacheData{
		*data,
	})

	return data, nil
}
