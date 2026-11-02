package interaction

import (
	"context"
	"errors"

	"fake_tiktok/internal/model"

	"gorm.io/gorm"
)

// InteractionStatus 用户对某视频的互动状态。
// 拆服务后关注关系归 user-svc，这里只保留点赞字段给缓存回填使用。
type InteractionStatus struct {
	IsLiked bool
}

// InteractionRepo 视频点赞数据仓储（feed_interaction 库）。
// 评论点赞在各自文件中实现。
type InteractionRepo struct {
	db *gorm.DB
}

func NewInteractionRepo(db *gorm.DB) *InteractionRepo {
	return &InteractionRepo{db: db}
}

func (r *InteractionRepo) GetUserVideoInteraction(ctx context.Context, userID string, videoID uint) (*InteractionStatus, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var status InteractionStatus
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		First(&model.VideoLike{}).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		status.IsLiked = true
	}
	return &status, nil
}

// CreateVideoLike 同步写入用户点赞记录到 MySQL VideoLike 表
// 使用唯一索引保证幂等：记录已存在时不报错
// created=true 表示真正新建，调用方应发布 MQ 增量。
func (r *InteractionRepo) CreateVideoLike(ctx context.Context, userID string, videoID uint) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	like := model.VideoLike{
		UserID:  userID,
		VideoID: uint64(videoID),
	}
	result := r.db.WithContext(ctx).Where("user_id = ? AND video_id = ?", userID, videoID).
		FirstOrCreate(&like)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeleteVideoLike 删除用户点赞记录（取消点赞）
func (r *InteractionRepo) DeleteVideoLike(ctx context.Context, userID string, videoID uint) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Delete(&model.VideoLike{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListLikedVideoIDs 批量查询用户在给定视频中的点赞记录（gRPC GetVideoUserState 使用）。
func (r *InteractionRepo) ListLikedVideoIDs(ctx context.Context, userID string, videoIDs []uint64) ([]uint64, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var ids []uint64
	err := r.db.WithContext(ctx).Model(&model.VideoLike{}).
		Where("user_id = ? AND video_id IN ?", userID, videoIDs).
		Pluck("video_id", &ids).Error
	return ids, err
}
