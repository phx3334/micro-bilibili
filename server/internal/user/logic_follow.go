package user

import (
	"context"
	"errors"
	"fmt"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/pkg/httpx"

	"go.uber.org/zap"
)

// FollowLogic 关注/粉丝业务逻辑（关注关系表在本服务 feed_user 库）
type FollowLogic struct {
	deps *LogicDeps
}

func NewFollowLogic(deps *LogicDeps) *FollowLogic {
	return &FollowLogic{deps: deps}
}

// ListFollowers 分页查询某用户的粉丝列表
func (l *FollowLogic) ListFollowers(ctx context.Context, req request.ListFollowersReq) (*httpx.PaginatedResp[httpx.FollowUserItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	followerIDs, total, err := l.deps.FollowRepo.ListFollowers(ctx, req.UserID, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询粉丝列表失败")
	}

	return l.buildFollowListResp(ctx, followerIDs, total, req.Page, req.PageSize)
}

// ListFollowing 分页查询某用户的关注列表
func (l *FollowLogic) ListFollowing(ctx context.Context, req request.ListFollowingReq) (*httpx.PaginatedResp[httpx.FollowUserItem], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	followeeIDs, total, err := l.deps.FollowRepo.ListFollowing(ctx, req.UserID, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("查询关注列表失败")
	}

	return l.buildFollowListResp(ctx, followeeIDs, total, req.Page, req.PageSize)
}

// buildFollowListResp 批量查询用户信息构造响应
func (l *FollowLogic) buildFollowListResp(ctx context.Context, userIDs []string, total int64, page, pageSize int) (*httpx.PaginatedResp[httpx.FollowUserItem], error) {
	if len(userIDs) == 0 {
		return &httpx.PaginatedResp[httpx.FollowUserItem]{
			List:     []httpx.FollowUserItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	users, err := l.deps.AccountRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		// 降级：仅返回 ID 列表
		items := make([]httpx.FollowUserItem, 0, len(userIDs))
		for _, id := range userIDs {
			items = append(items, httpx.FollowUserItem{ID: id})
		}
		return &httpx.PaginatedResp[httpx.FollowUserItem]{
			List:     items,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	// 构建 ID → Account 映射，保证顺序与 userIDs 一致
	userMapIdx := make(map[string]int, len(users))
	for i, u := range users {
		userMapIdx[u.ID] = i
	}

	items := make([]httpx.FollowUserItem, 0, len(userIDs))
	for _, id := range userIDs {
		if idx, ok := userMapIdx[id]; ok {
			u := users[idx]
			items = append(items, httpx.FollowUserItem{
				ID:        u.ID,
				Username:  u.Username,
				AvatarURL: mediaURL(ctx, l.deps, u.AvatarURL),
				Signature: u.Signature,
			})
		} else {
			items = append(items, httpx.FollowUserItem{ID: id})
		}
	}

	return &httpx.PaginatedResp[httpx.FollowUserItem]{
		List:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// FollowUser 关注用户
// 不允许关注自己；幂等：已关注不报错
//
// 原实现位于 interaction-svc 的 InteractionLogic；拆服务后关注关系归 user-svc，
// 因此方法随仓储一起迁到这里，路由也从 /interaction/follow 挪到 /user/follow。
func (l *FollowLogic) FollowUser(ctx context.Context, followerID, followeeID string) error {
	if followerID == followeeID {
		return errors.New("不能关注自己")
	}

	created, err := l.deps.FollowRepo.CreateFollow(ctx, followerID, followeeID)
	if err != nil {
		return fmt.Errorf("关注失败，请稍后重试")
	}

	// 仅在真正新建关注时维护计数。
	// fans_count / following_count 存于 Redis 的 user:dynamic 缓存，
	// 必须由业务写操作实时维护（HIncrBy），否则主页展示的数字不会变化。
	if created {
		if err := l.deps.UserCacheRepo.IncrementFansCount(ctx, followeeID); err != nil {
			l.deps.Logger.Warn("follow: increment fans cache failed",
				zap.String("followee", followeeID), zap.Error(err))
		}
		if err := l.deps.UserCacheRepo.IncrementFollowingCount(ctx, followerID); err != nil {
			l.deps.Logger.Warn("follow: increment following cache failed",
				zap.String("follower", followerID), zap.Error(err))
		}
	}
	return nil
}

// UnfollowUser 取关用户
func (l *FollowLogic) UnfollowUser(ctx context.Context, followerID, followeeID string) error {
	if followerID == followeeID {
		return errors.New("不能取关自己")
	}

	if _, err := l.deps.FollowRepo.DeleteFollow(ctx, followerID, followeeID); err != nil {
		return fmt.Errorf("取关失败，请稍后重试")
	}

	// 取关后同步回退缓存计数（被关注者粉丝 -1，关注者关注数 -1，best-effort）
	if err := l.deps.UserCacheRepo.DecrementFansCount(ctx, followeeID); err != nil {
		l.deps.Logger.Warn("unfollow: decrement fans cache failed",
			zap.String("followee", followeeID), zap.Error(err))
	}
	if err := l.deps.UserCacheRepo.DecrementFollowingCount(ctx, followerID); err != nil {
		l.deps.Logger.Warn("unfollow: decrement following cache failed",
			zap.String("follower", followerID), zap.Error(err))
	}
	return nil
}
