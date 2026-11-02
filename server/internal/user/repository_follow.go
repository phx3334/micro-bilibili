package user

import (
	"context"

	"fake_tiktok/internal/model"

	"gorm.io/gorm"
)

// FollowRepo 关注关系仓储。
//
// 拆分后 user_follows 表归 feed_user 库，关注/粉丝读写都留在本服务，
// 不再经过 interaction-svc，因此这里保留了原 InteractionRepo 中的 follow 部分。
type FollowRepo struct {
	db *gorm.DB
}

func NewFollowRepo(db *gorm.DB) *FollowRepo {
	return &FollowRepo{db: db}
}

// GetFansCount 统计用户粉丝数
func (r *FollowRepo) GetFansCount(ctx context.Context, userID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserFollow{}).Where("followee_id = ?", userID).Count(&count).Error
	return count, err
}

// GetFollowingCount 统计用户关注数
func (r *FollowRepo) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserFollow{}).Where("follower_id = ?", userID).Count(&count).Error
	return count, err
}

// IsUserFollowed 判断 followerID 是否关注了 followeeID
func (r *FollowRepo) IsUserFollowed(ctx context.Context, followerID, followeeID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var follow model.UserFollow
	err := r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		First(&follow).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsMutualFollow 判断 a、b 是否互相关注（两个方向都已关注）。
func (r *FollowRepo) IsMutualFollow(ctx context.Context, a, b string) (bool, error) {
	fwd, err := r.IsUserFollowed(ctx, a, b)
	if err != nil {
		return false, err
	}
	if !fwd {
		return false, nil
	}
	return r.IsUserFollowed(ctx, b, a)
}

// CreateFollow 创建关注关系
// 通过唯一索引 idx_user_follow_pair 保证幂等：已存在不报错
func (r *FollowRepo) CreateFollow(ctx context.Context, followerID, followeeID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	// 不允许关注自己
	if followerID == followeeID {
		return false, nil
	}

	follow := model.UserFollow{
		FollowerID: followerID,
		FolloweeID: followeeID,
	}
	result := r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		FirstOrCreate(&follow)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeleteFollow 删除关注关系
func (r *FollowRepo) DeleteFollow(ctx context.Context, followerID, followeeID string) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Delete(&model.UserFollow{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListFollowers 分页查询某用户的粉丝列表（followerID 列表）
func (r *FollowRepo) ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]string, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.UserFollow{}).
		Where("followee_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var follows []model.UserFollow
	if err := r.db.WithContext(ctx).
		Where("followee_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&follows).Error; err != nil {
		return nil, total, err
	}
	ids := make([]string, 0, len(follows))
	for _, f := range follows {
		ids = append(ids, f.FollowerID)
	}
	return ids, total, nil
}

// ListFollowing 分页查询某用户关注的列表（followeeID 列表）
func (r *FollowRepo) ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]string, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.UserFollow{}).
		Where("follower_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || page < 1 || pageSize < 1 {
		return nil, total, nil
	}
	offset := (page - 1) * pageSize
	var follows []model.UserFollow
	if err := r.db.WithContext(ctx).
		Where("follower_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&follows).Error; err != nil {
		return nil, total, err
	}
	ids := make([]string, 0, len(follows))
	for _, f := range follows {
		ids = append(ids, f.FolloweeID)
	}
	return ids, total, nil
}
