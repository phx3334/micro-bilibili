package request

// =============================================================================
// 关注 / 粉丝列表
// =============================================================================

// ListFollowersReq 粉丝列表请求
type ListFollowersReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// ListFollowingReq 关注列表请求
type ListFollowingReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// =============================================================================
// 用户主页
// =============================================================================

// ListUserVideosReq 用户主页视频列表请求
type ListUserVideosReq struct {
	UserID   string `form:"user_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}
