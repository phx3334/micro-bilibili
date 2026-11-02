package request

type InteractionVideo struct {
	UserID  string `json:"user_id"` // 从 JWT 提取，不由客户端传入
	VideoID uint   `json:"video_id" binding:"required"`
}

// FollowUserReq 关注/取关请求
type FollowUserReq struct {
	TargetUserID string `json:"target_user_id" binding:"required"`
}

// ListFollowReq 粉丝/关注列表请求
type ListFollowReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}
