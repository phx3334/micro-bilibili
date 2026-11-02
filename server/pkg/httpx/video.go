package httpx

import (
	"time"
)

type HomeVideoInfo struct {
	ID           uint      `json:"id"`
	UpName       string    `json:"up_name"`
	UpAvatar     string    `json:"up_avatar"`
	Title        string    `json:"title"`
	CoverURL     string    `json:"cover_url"`
	PlayCount    int64     `json:"play_count"`
	CommentCount int64     `json:"comment_count"`
	Duration     float64   `json:"duration"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuthorInfo struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Signature string `json:"signature"`
	FansCount int64  `json:"fans_count"`
}

type VideoDetailResp struct {
	ID          uint                  `json:"id"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	PlayURL     string                `json:"play_url"`
	CoverURL    string                `json:"cover_url"`
	Duration    float64               `json:"duration"`
	Zone        string                `json:"zone"`
	PlayCount   int64                 `json:"play_count"`
	LikesCnt    int64                 `json:"likes_count"`
	CommentCnt  int64                 `json:"comment_count"`
	CreatedAt   time.Time             `json:"created_at"`
	Author      AuthorInfo            `json:"author"`
	Interaction InteractionStatusResp `json:"interaction"`
}

type InteractionStatusResp struct {
	IsLiked    bool `json:"is_liked"`
	IsFollowed bool `json:"is_followed"`
}

// VideoStatusResp 视频转码状态查询响应
//
// status 取值：
//   - transcoding：已落库并投递转码任务，worker 正在执行 ffmpeg
//   - published：转码完成且可对外播放，video_url / cover_url 已就绪
//   - failed：转码失败，fail_reason 携带 stderr 摘要供用户查看
type VideoStatusResp struct {
	Status     string `json:"status"` // transcoding/published/failed
	FailReason string `json:"fail_reason"`
	VideoURL   string `json:"video_url"` // 转码完成后的可播放 URL
	CoverURL   string `json:"cover_url"` // 封面 URL
}
