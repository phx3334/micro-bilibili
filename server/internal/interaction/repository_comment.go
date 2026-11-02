package interaction

import (
	"context"

	"fake_tiktok/internal/model"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// VideoCommentRepo —— 视频评论
// ---------------------------------------------------------------------------

// VideoCommentRepo 视频评论表的 MySQL 数据存储
type VideoCommentRepo struct {
	db *gorm.DB
}

func NewVideoCommentRepo(db *gorm.DB) *VideoCommentRepo {
	return &VideoCommentRepo{db: db}
}

// Create 插入一条视频评论
func (r *VideoCommentRepo) Create(ctx context.Context, c *model.Comment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(c).Error
}

// FindByVideoID 分页查询指定视频的顶级评论（Level=1）
func (r *VideoCommentRepo) FindByVideoID(ctx context.Context, videoID uint, page, pageSize int) ([]model.Comment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Comment{}).
		Where("video_id = ? AND level = ?", videoID, 1).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []model.Comment
	err := r.db.WithContext(ctx).
		Where("video_id = ? AND level = ?", videoID, 1).
		Order("like_count DESC, created_at DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

// FindReplies 分页查询指定父评论的回复
func (r *VideoCommentRepo) FindReplies(ctx context.Context, parentID uint64, page, pageSize int) ([]model.Comment, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Comment{}).
		Where("parent_id = ?", parentID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []model.Comment
	err := r.db.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Order("created_at ASC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&comments).Error
	return comments, total, err
}

// FindRepliesByParentIDs 批量查询多个父评论的子回复（用于递归查楼中楼）
func (r *VideoCommentRepo) FindRepliesByParentIDs(ctx context.Context, parentIDs []uint64, limit int) ([]model.Comment, int64, error) {
	if len(parentIDs) == 0 {
		return nil, 0, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.Comment{}).
		Where("parent_id IN ?", parentIDs).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var comments []model.Comment
	err := r.db.WithContext(ctx).
		Where("parent_id IN ?", parentIDs).
		Order("created_at ASC").
		Limit(limit).
		Find(&comments).Error
	return comments, total, err
}

// FindByID 根据 ID 查询单条评论
func (r *VideoCommentRepo) FindByID(ctx context.Context, id uint64) (*model.Comment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var c model.Comment
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Delete 硬删除一条评论
func (r *VideoCommentRepo) Delete(ctx context.Context, id uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Comment{}).Error
}

// IncrementLikeCount 原子更新评论点赞数（delta 可正可负）
func (r *VideoCommentRepo) IncrementLikeCount(ctx context.Context, id uint64, delta int) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&model.Comment{}).
		Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + ?", delta)).Error
}

// CountReplies 统计指定父评论的回复数
func (r *VideoCommentRepo) CountReplies(ctx context.Context, parentID uint64) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&model.Comment{}).
		Where("parent_id = ?", parentID).
		Count(&count).Error
	return count, err
}

// ---------------------------------------------------------------------------
// VideoCommentLikeRepo —— 视频评论点赞
// ---------------------------------------------------------------------------

type VideoCommentLikeRepo struct {
	db *gorm.DB
}

func NewVideoCommentLikeRepo(db *gorm.DB) *VideoCommentLikeRepo {
	return &VideoCommentLikeRepo{db: db}
}

// CreateLike 尝试插入点赞记录。created=true 表示首次新增（用于幂等判断）。
func (r *VideoCommentLikeRepo) CreateLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	like := model.CommentLike{
		UserID:    userID,
		CommentID: commentID,
	}
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *VideoCommentLikeRepo) DeleteLike(ctx context.Context, userID string, commentID uint64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Delete(&model.CommentLike{}).Error
}

func (r *VideoCommentLikeRepo) ExistsLike(ctx context.Context, userID string, commentID uint64) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := r.db.WithContext(ctx).Model(&model.CommentLike{}).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
