package httpx

// =============================================================================
// 通用分页响应
// =============================================================================

// PaginatedResp 分页响应通用结构
type PaginatedResp[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// =============================================================================
// 关注 / 粉丝
// =============================================================================

// FollowUserItem 关注/粉丝列表项
type FollowUserItem struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Signature string `json:"signature"`
}
