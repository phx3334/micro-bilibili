package video

import (
	"context"
	"time"

	"fake_tiktok/internal/model"

	"gorm.io/gorm"
)

// defaultDBTimeout 是 MySQL 操作的默认超时时间。
const defaultDBTimeout = 5 * time.Second

// withTimeout 为 context 添加超时控制；已有 deadline 时透传。
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultDBTimeout)
}

// VideoRepo 视频表的 MySQL 数据存储（feed_video 库），覆盖上传落库与转码回写。
type VideoRepo struct {
	db *gorm.DB
}

// NewVideoRepo 创建 VideoRepo 实例。
func NewVideoRepo(db *gorm.DB) *VideoRepo {
	return &VideoRepo{db: db}
}

// FindPublishedVideos 返回已发布视频的基础查询（未执行）。
// 返回 *gorm.DB 供 PaginateRepo 进一步添加分页条件后执行。
func (v *VideoRepo) FindPublishedVideos() *gorm.DB {
	// 每次返回新 Session，避免多 goroutine 共享同一个 *gorm.DB 对象导致竞态
	return v.db.Session(&gorm.Session{}).Model(&model.Video{}).Where("status = ?", "published")
}

// FindPublishedVideosWithZone 返回指定分区已发布视频的基础查询。
func (v *VideoRepo) FindPublishedVideosWithZone(zone string) *gorm.DB {
	return v.FindPublishedVideos().Where("zone = ?", zone)
}

// FindPublishedVideosByIDs 根据视频 ID 列表批量查询已发布的视频。
func (v *VideoRepo) FindPublishedVideosByIDs(ctx context.Context, ids []uint) ([]model.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	if len(ids) == 0 {
		return nil, nil
	}
	var videos []model.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, duration_sec, title, description, zone, play_url, cover_url, play_count, comments_count, likes_count, created_at").
		Where("id IN ? AND status = ?", ids, "published").
		Find(&videos).Error
	return videos, err
}

// FindAllPublishedVideoIDs 查询所有已发布视频的关键字段。
func (v *VideoRepo) FindAllPublishedVideoIDs(ctx context.Context) ([]model.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var videos []model.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, play_count, comments_count, likes_count, popularity, created_at, cover_url, duration_sec, title").
		Where("status = ?", "published").
		Find(&videos).Error
	return videos, err
}

// UpdatePopularity 更新视频的 popularity 热度字段。
func (v *VideoRepo) UpdatePopularity(ctx context.Context, id uint, popularity int64) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	return v.db.WithContext(ctx).Model(&model.Video{}).Where("id = ?", id).Update("popularity", popularity).Error
}

// FindPublishedVideosByAuthorID 查询指定作者的已发布视频，按创建时间倒序
func (v *VideoRepo) FindPublishedVideosByAuthorID(ctx context.Context, authorID string, limit, offset int) ([]model.Video, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var videos []model.Video
	err := v.db.WithContext(ctx).
		Select("id, author_id, duration_sec, title, cover_url, play_count, comments_count, created_at").
		Where("author_id = ? AND status = ?", authorID, "published").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&videos).Error
	return videos, err
}

// CountPublishedByAuthorID 统计作者已发布且未删除的视频数。
func (v *VideoRepo) CountPublishedByAuthorID(ctx context.Context, authorID string) (int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var count int64
	err := v.db.WithContext(ctx).Model(&model.Video{}).
		Where("author_id = ? AND status = ? AND deleted_at IS NULL", authorID, "published").
		Count(&count).Error
	return count, err
}

// SumPublishedStatsByAuthorID 聚合作者已发布视频的统计数据。
// videoCount = COUNT(*)，playCount = SUM(play_count)，likeCount = SUM(likes_count)。
//
// 拆服务后该方法只在本服务内部被 gRPC GetUserStats 使用，供 user-svc 查询主页统计。
func (v *VideoRepo) SumPublishedStatsByAuthorID(ctx context.Context, authorID string) (int64, int64, int64, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var row struct {
		VideoCount int64
		PlayCount  int64
		LikesCount int64
	}
	err := v.db.WithContext(ctx).Model(&model.Video{}).
		Select("COUNT(*) AS video_count, COALESCE(SUM(play_count), 0) AS play_count, COALESCE(SUM(likes_count), 0) AS likes_count").
		Where("author_id = ? AND status = ? AND deleted_at IS NULL", authorID, "published").
		Scan(&row).Error
	return row.VideoCount, row.PlayCount, row.LikesCount, err
}

func (v *VideoRepo) FindVideoByID(ctx context.Context, id uint) (*model.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var video model.Video
	if err := v.db.WithContext(ctx).Where("id = ?", id).First(&video).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

// FindPublishedVideoByID 只查询已发布视频，供跨服务校验使用。
//
// 与 FindVideoByID 的区别：转码中的视频对本服务可见（作者操作需要），
// 但其它服务（interaction 点赞/评论前校验）只能认已发布视频，
// 否则会给用户看不见的视频产生互动记录。
func (v *VideoRepo) FindPublishedVideoByID(ctx context.Context, id uint) (*model.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var video model.Video
	if err := v.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, "published").
		First(&video).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

// DeleteVideo 软删除视频（设置 deleted_at）。
func (v *VideoRepo) DeleteVideo(ctx context.Context, id uint) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Delete(&model.Video{}, id).Error
}

// Create 插入一条 status=transcoding 的 Video 记录。
func (v *VideoRepo) Create(ctx context.Context, video *model.Video) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).Create(video).Error
}

// UpdateTranscodeResult 转码成功后回写 video_url / cover_url / duration_sec / status，
// 并清空源文件 key（源对象已由 worker 删除）。
func (v *VideoRepo) UpdateTranscodeResult(ctx context.Context, videoID uint, videoURL, coverURL string, duration float64, status string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).
		Model(&model.Video{}).
		Where("id = ?", videoID).
		Updates(map[string]interface{}{
			"play_url":         videoURL,
			"cover_url":        coverURL,
			"duration_sec":     duration,
			"status":           status,
			"source_key":       "",
			"source_cover_key": "",
		}).Error
}

// UpdateTranscodeFailure 转码失败时将 status 置为 failed 并写入 fail_reason。
func (v *VideoRepo) UpdateTranscodeFailure(ctx context.Context, videoID uint, failReason string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return v.db.WithContext(ctx).
		Model(&model.Video{}).
		Where("id = ?", videoID).
		Updates(map[string]interface{}{
			"status":      "failed",
			"fail_reason": failReason,
		}).Error
}

// FindByID 按 ID 查询 Video 记录，未命中时返回 gorm.ErrRecordNotFound。
func (v *VideoRepo) FindByID(ctx context.Context, id uint) (*model.Video, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	var video model.Video
	if err := v.db.WithContext(ctx).Where("id = ?", id).First(&video).Error; err != nil {
		return nil, err
	}
	return &video, nil
}
