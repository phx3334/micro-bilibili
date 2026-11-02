package model

import (
	"time"

	"gorm.io/gorm"
)

type Account struct {
	ID        string    `gorm:"type:char(36);uniqueIndex"`
	Username  string    `gorm:"type:varchar(255);uniqueIndex"`
	Password  string    `gorm:"type:varchar(255);not null" json:"-"`
	AvatarURL string    `gorm:"type:varchar(512)"`
	Signature string    `gorm:"type:varchar(512);default:'签名是空白的，这位用户似乎比较高冷。'"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time
	Gender    string `gorm:"size:16"` // male | female | secret
	Birthday  string `gorm:"size:10"` // YYYY-MM-DD, may be empty
}

type Video struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	AuthorID      string    `gorm:"type:char(36);not null;index:idx_author_time"`
	DurationSec   float64   `gorm:"column:duration_sec"`
	Status        string    `gorm:"size:32;index:idx_video_status"`
	FailReason    string    `gorm:"size:2000"`
	Title         string    `gorm:"type:varchar(255);not null"`
	Description   string    `gorm:"type:varchar(512)"`
	PlayURL       string    `gorm:"type:varchar(512);not null"`
	CoverURL      string    `gorm:"type:varchar(512)"`
	PlayCount     int64     `gorm:"default:0"`
	LikesCount    int64     `gorm:"default:0"`
	CommentsCount int64     `gorm:"default:0"`
	Popularity    int64     `gorm:"column:popularity;not null;default:0;index:idx_videos_popularity_time_id,priority:1,sort:desc"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index:idx_author_time,sort:desc"`
	UpdatedAt     time.Time
	// TagsJSON is a JSON array of strings, e.g. ["录屏","教程"]；空串表示无标签。
	TagsJSON string `gorm:"type:text"`
	// Zone is the publish partition, e.g. "动画" or "生活-日常".
	Zone string `gorm:"size:64"`
	// SourceKey / SourceCoverKey：转码前源视频与用户封面在对象存储中的 key，
	// 由 video-svc 写入、worker 下载使用；转码成功后置空。
	SourceKey      string `gorm:"size:1024"`
	SourceCoverKey string `gorm:"size:1024"`

	// DeletedAt 软删除时间戳；非 nil 表示作者已删除该投稿。
	// GORM 自动在 Where/Find/First 查询中追加 "deleted_at IS NULL" 过滤，
	// 使已删除视频从首页列表、分区榜、作者主页、搜索等所有查询中隐藏。
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type Comment struct {
	ID        uint64 `gorm:"primaryKey"`
	VideoID   uint64 `gorm:"index:idx_comment_video;not null"`
	UserID    string `gorm:"type:varchar(64);index;not null"`
	ParentID  uint64 `gorm:"index;default:0"`
	Level     int    `gorm:"not null"`
	Content   string `gorm:"size:2000;not null"`
	LikeCount uint64 `gorm:"default:0"`
	CreatedAt time.Time
}

// CommentLike records a user's like on a comment.
type CommentLike struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_like_user_comment;not null"`
	CommentID uint64 `gorm:"uniqueIndex:idx_like_user_comment;not null"`
	CreatedAt time.Time
}

// VideoLike records a user's like on a published video (e.g. 动态点赞).
type VideoLike struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    string `gorm:"type:varchar(64);uniqueIndex:idx_video_like_user_video;not null"`
	VideoID   uint64 `gorm:"uniqueIndex:idx_video_like_user_video;not null"`
	CreatedAt time.Time
}

// UserFollow records follower -> followee (关注关系).
type UserFollow struct {
	ID         uint64 `gorm:"primaryKey"`
	FollowerID string `gorm:"type:varchar(64);uniqueIndex:idx_user_follow_pair,priority:1;index:idx_user_follow_follower;not null"`
	FolloweeID string `gorm:"type:varchar(64);uniqueIndex:idx_user_follow_pair,priority:2;index:idx_user_follow_followee;not null"`
	CreatedAt  time.Time
}
