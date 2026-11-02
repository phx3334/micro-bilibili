package interaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/httpx"

	"go.uber.org/zap"
)

// CommentTargetVideo 评论目标类型（仅视频）
const CommentTargetVideo = "video"

// CommentLogic 视频多级评论业务逻辑层
type CommentLogic struct {
	deps *LogicDeps
}

func NewCommentLogic(deps *LogicDeps) *CommentLogic {
	return &CommentLogic{deps: deps}
}

// CreateComment 创建评论或回复
//
// 流程：
//  1. 校验 target_type（仅支持 video）和 content 长度
//  2. 通过 video-svc gRPC 查询目标视频，校验存在性
//     （原实现读本地 videos 表；拆库后 videos 在 feed_video）
//  3. 若 parent_id != 0：查询父评论，新 Level = parent.Level + 1（支持无限层级）
//  4. 插入评论记录并发布评论数 +1 增量（worker 回刷 videos.comments_count）
func (l *CommentLogic) CreateComment(ctx context.Context, userID string, req request.CreateCommentReq) (uint64, error) {
	if req.TargetType != "" && req.TargetType != CommentTargetVideo {
		return 0, fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	// ---- 跨服务查询目标元信息，校验评论区是否关闭 ----
	targetInfo, err := l.deps.VideoClient.GetVideo(ctx, req.TargetID)
	if err != nil || targetInfo == nil {
		return 0, fmt.Errorf("目标不存在")
	}

	// ---- 计算评论层级 ----
	level := 1
	var parentID uint64
	if req.ParentID != 0 {
		c, pErr := l.deps.VideoCommentRepo.FindByID(ctx, req.ParentID)
		if pErr != nil {
			return 0, fmt.Errorf("父评论不存在")
		}
		// 评论层级不再限制为最大 3 层，支持无限嵌套回复
		level = c.Level + 1
		parentID = req.ParentID
	}

	// ---- 写入评论 + 更新计数 ----
	c := &model.Comment{
		VideoID:   req.TargetID,
		UserID:    userID,
		ParentID:  parentID,
		Level:     level,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}
	if err := l.deps.VideoCommentRepo.Create(ctx, c); err != nil {
		return 0, err
	}

	// 评论数属于 video 库：发 Kafka 增量，由 worker 回刷 comments_count
	if cerr := l.deps.CounterPublisher.PublishCommentIncrement(ctx, uint(req.TargetID), 1); cerr != nil {
		l.deps.Logger.Warn("发布评论数增量失败",
			zap.Uint64("target_id", req.TargetID), zap.Error(cerr))
	}
	// 同步更新 Redis 视频动态缓存中的 comment_count（缓存共享、可重建）
	if rerr := l.deps.VideoCacheCounterRepo.IncrementCommentCount(ctx, uint(req.TargetID), 1); rerr != nil {
		l.deps.Logger.Warn("Redis 更新评论数缓存失败",
			zap.Uint64("target_id", req.TargetID), zap.Error(rerr))
	}

	return c.ID, nil
}

// ListComments 分页查询顶级评论 + 每条前 3 条回复
func (l *CommentLogic) ListComments(ctx context.Context, req request.ListCommentsReq) ([]httpx.CommentItem, int64, error) {
	if req.TargetType != "" && req.TargetType != CommentTargetVideo {
		return nil, 0, fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	// 分页默认值
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}

	comments, total, err := l.deps.VideoCommentRepo.FindByVideoID(ctx, uint(req.TargetID), page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return l.assembleVideoCommentItems(ctx, comments), total, nil
}

// assembleVideoCommentItems 组装视频评论列表（含每条前 3 条回复 + reply_count）
func (l *CommentLogic) assembleVideoCommentItems(ctx context.Context, comments []model.Comment) []httpx.CommentItem {
	// 收集所有用户 ID 用于批量查询
	authorIDs := make([]string, 0, len(comments))
	seen := make(map[string]bool)
	for _, c := range comments {
		if c.UserID != "" && !seen[c.UserID] {
			authorIDs = append(authorIDs, c.UserID)
			seen[c.UserID] = true
		}
	}
	// 也收集回复的作者 ID
	for _, c := range comments {
		replies, _, _ := l.deps.VideoCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		for _, r := range replies {
			if r.UserID != "" && !seen[r.UserID] {
				authorIDs = append(authorIDs, r.UserID)
				seen[r.UserID] = true
			}
		}
	}
	// 一次性批量取用户名+头像（user-svc gRPC），避免 N+1 查询
	cardMap := l.lookupUserCards(ctx, authorIDs)

	items := make([]httpx.CommentItem, 0, len(comments))
	for _, c := range comments {
		card := cardMap[c.UserID]
		item := httpx.CommentItem{
			ID:        c.ID,
			User:      commentUserCard(c.UserID, card),
			Content:   c.Content,
			LikeCount: c.LikeCount,
			CreatedAt: c.CreatedAt,
		}

		// 获取回复数和前 3 条回复
		replyCount, _ := l.deps.VideoCommentRepo.CountReplies(ctx, c.ID)
		item.ReplyCount = replyCount

		replies, _, _ := l.deps.VideoCommentRepo.FindReplies(ctx, c.ID, 1, 3)
		item.Replies = make([]httpx.CommentItem, 0, len(replies))
		for _, r := range replies {
			rcard := cardMap[r.UserID]
			item.Replies = append(item.Replies, httpx.CommentItem{
				ID:        r.ID,
				User:      commentUserCard(r.UserID, rcard),
				Content:   r.Content,
				LikeCount: r.LikeCount,
				CreatedAt: r.CreatedAt,
			})
		}
		items = append(items, item)
	}
	return items
}

// ListReplies 分页查询指定评论的回复
func (l *CommentLogic) ListReplies(ctx context.Context, req request.ListRepliesReq) ([]httpx.CommentItem, int64, error) {
	if req.TargetType != "" && req.TargetType != CommentTargetVideo {
		return nil, 0, fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}

	replies, total, err := l.deps.VideoCommentRepo.FindReplies(ctx, req.CommentID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	// 递归查询所有层级的回复（支持无限层级嵌套）
	replies = l.fetchAllCommentDescendants(ctx, replies)
	return l.assembleVideoReplyItems(ctx, replies), total, nil
}

// fetchAllCommentDescendants BFS 递归查询视频评论的所有后代回复
func (l *CommentLogic) fetchAllCommentDescendants(ctx context.Context, replies []model.Comment) []model.Comment {
	if len(replies) == 0 {
		return replies
	}
	parentIDs := make([]uint64, len(replies))
	for i, r := range replies {
		parentIDs[i] = r.ID
	}
	for len(parentIDs) > 0 {
		children, _, err := l.deps.VideoCommentRepo.FindRepliesByParentIDs(ctx, parentIDs, 200)
		if err != nil || len(children) == 0 {
			break
		}
		replies = append(replies, children...)
		nextIDs := make([]uint64, 0, len(children))
		for _, c := range children {
			nextIDs = append(nextIDs, c.ID)
		}
		parentIDs = nextIDs
	}
	return replies
}

func (l *CommentLogic) assembleVideoReplyItems(ctx context.Context, replies []model.Comment) []httpx.CommentItem {
	authorIDs := make([]string, 0, len(replies))
	seen := make(map[string]bool)
	for _, r := range replies {
		if r.UserID != "" && !seen[r.UserID] {
			authorIDs = append(authorIDs, r.UserID)
			seen[r.UserID] = true
		}
	}
	// 一次性批量取用户名+头像（user-svc gRPC）
	cardMap := l.lookupUserCards(ctx, authorIDs)

	items := make([]httpx.CommentItem, 0, len(replies))
	for _, r := range replies {
		card := cardMap[r.UserID]
		items = append(items, httpx.CommentItem{
			ID:        r.ID,
			User:      commentUserCard(r.UserID, card),
			Content:   r.Content,
			LikeCount: r.LikeCount,
			CreatedAt: r.CreatedAt,
		})
	}
	return items
}

// lookupUserCards 通过 user-svc 批量查询用户卡片；失败时返回空 map（不阻塞列表返回）。
//
// 原实现读本地 accounts 表；拆库后 accounts 在 feed_user，只能 gRPC 拉取。
func (l *CommentLogic) lookupUserCards(ctx context.Context, userIDs []string) map[string]*pb.UserInfo {
	if len(userIDs) == 0 {
		return make(map[string]*pb.UserInfo)
	}
	users, err := l.deps.UserClient.GetUsers(ctx, userIDs)
	if err != nil {
		l.deps.Logger.Warn("GetUsers via gRPC failed", zap.Error(err))
		return make(map[string]*pb.UserInfo)
	}
	return users
}

// commentUserCard 构造用户卡片。
func commentUserCard(userID string, info *pb.UserInfo) httpx.UserCard {
	card := httpx.UserCard{ID: userID}
	if info != nil {
		card.Username = info.Username
		card.AvatarURL = info.AvatarUrl
	}
	return card
}

// LikeComment 点赞评论（幂等：重复点赞不报错、不重复计数）
func (l *CommentLogic) LikeComment(ctx context.Context, userID string, req request.CommentLikeReq) error {
	if req.TargetType != "" && req.TargetType != CommentTargetVideo {
		return fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	created, err := l.deps.VideoCommentLikeRepo.CreateLike(ctx, userID, req.CommentID)
	if err != nil {
		return fmt.Errorf("点赞失败，请稍后重试")
	}

	// 仅首次点赞时更新计数（评论点赞数在本服务库内，可直接写）
	if created {
		if cerr := l.deps.VideoCommentRepo.IncrementLikeCount(ctx, req.CommentID, 1); cerr != nil {
			l.deps.Logger.Warn("IncrementLikeCount 失败",
				zap.Uint64("comment_id", req.CommentID), zap.Error(cerr))
		}
	}
	return nil
}

// UnlikeComment 取消点赞评论（幂等：未点赞时不报错）
func (l *CommentLogic) UnlikeComment(ctx context.Context, userID string, req request.CommentLikeReq) error {
	if req.TargetType != "" && req.TargetType != CommentTargetVideo {
		return fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	exists, err := l.deps.VideoCommentLikeRepo.ExistsLike(ctx, userID, req.CommentID)
	if err != nil {
		return fmt.Errorf("操作失败，请稍后重试")
	}

	// 未点赞，幂等返回
	if !exists {
		return nil
	}

	if err := l.deps.VideoCommentLikeRepo.DeleteLike(ctx, userID, req.CommentID); err != nil {
		return fmt.Errorf("取消点赞失败，请稍后重试")
	}

	if cerr := l.deps.VideoCommentRepo.IncrementLikeCount(ctx, req.CommentID, -1); cerr != nil {
		l.deps.Logger.Warn("IncrementLikeCount -1 失败",
			zap.Uint64("comment_id", req.CommentID), zap.Error(cerr))
	}
	return nil
}

// DeleteComment 删除评论（仅评论作者或目标 UP 主可删除）
func (l *CommentLogic) DeleteComment(ctx context.Context, userID string, req request.DeleteCommentReq) error {
	if req.TargetType != "" && req.TargetType != CommentTargetVideo {
		return fmt.Errorf("不支持的目标类型: %s", req.TargetType)
	}

	// ---- 查询评论，获取作者 ID 和目标 ID ----
	c, err := l.deps.VideoCommentRepo.FindByID(ctx, req.CommentID)
	if err != nil {
		return fmt.Errorf("评论不存在")
	}

	// ---- 跨服务查询目标 UP 主，校验权限 ----
	targetInfo, terr := l.deps.VideoClient.GetVideo(ctx, c.VideoID)
	if terr != nil || targetInfo == nil {
		return errors.New("目标不存在")
	}

	// 仅评论作者或目标 UP 主可删除
	if c.UserID != userID && targetInfo.AuthorId != userID {
		return fmt.Errorf("无权限删除")
	}

	// ---- 删除评论 + 发布评论数 -1 增量 ----
	if err := l.deps.VideoCommentRepo.Delete(ctx, req.CommentID); err != nil {
		return fmt.Errorf("删除失败，请稍后重试")
	}

	if cerr := l.deps.CounterPublisher.PublishCommentIncrement(ctx, uint(c.VideoID), -1); cerr != nil {
		l.deps.Logger.Warn("发布评论数递减失败",
			zap.Uint64("target_id", c.VideoID), zap.Error(cerr))
	}
	if rerr := l.deps.VideoCacheCounterRepo.IncrementCommentCount(ctx, uint(c.VideoID), -1); rerr != nil {
		l.deps.Logger.Warn("Redis 更新评论数缓存失败",
			zap.Uint64("target_id", c.VideoID), zap.Error(rerr))
	}
	return nil
}
