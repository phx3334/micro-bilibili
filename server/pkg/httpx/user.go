package httpx

// LoginAccount 登录/注册响应中的账户信息（只暴露前端需要的公开字段）。
type LoginAccount struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Signature string `json:"signature"`
	Gender    string `json:"gender"`
	Birthday  string `json:"birthday"`
}

type Login struct {
	Account           LoginAccount `json:"account"`
	AccessToken       string       `json:"access_token"`
	AccessTokenExpire int          `json:"access_token_expire"`
}

// UserHomeResp 用户主页响应
type UserHomeResp struct {
	ID                 string `json:"id"`
	AvatarURL          string `json:"avatar_url"`
	Signature          string `json:"signature"`
	Username           string `json:"username"`
	VideoCount         int64  `json:"video_count"`
	Birthday           string `json:"birthday"`
	Gender             string `json:"gender"`
	TotalLikesReceived int64  `json:"total_likes_received"`
	TotalPlayCount     int64  `json:"total_play_count"`
	FansCount          int64  `json:"fans_count"`
	FollowingCount     int64  `json:"following_count"`
	// IsFollowed 当前登录用户是否已关注该主页用户（用于前端"关注/已关注"按钮正确回显）。
	// 未登录 / 查看自己主页时为 false。
	IsFollowed bool            `json:"is_followed"`
	Videos     []HomeVideoInfo `json:"videos"`
}

// UserCard 用户卡片信息（共享结构）。
type UserCard struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}
