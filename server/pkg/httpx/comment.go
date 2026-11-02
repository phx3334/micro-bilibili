package httpx

import "time"

// CommentItem 评论列表项（含嵌套回复）
//
// UserCard 复用 httpx/user.go 中的定义，避免重复。
type CommentItem struct {
	ID         uint64        `json:"id"`
	User       UserCard      `json:"user"`
	Content    string        `json:"content"`
	LikeCount  uint64        `json:"like_count"`
	CreatedAt  time.Time     `json:"created_at"`
	ReplyCount int64         `json:"reply_count"`
	Replies    []CommentItem `json:"replies"`
}
