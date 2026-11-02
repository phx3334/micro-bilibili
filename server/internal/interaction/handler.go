package interaction

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/pkg/auth"
	"fake_tiktok/pkg/httpx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HandlerGroup 互动服务 HTTP 处理器集合
type HandlerGroup struct {
	InteractionHandler
	CommentHandler
}

type HandlerDeps struct {
	Logic  *LogicGroup
	Logger *zap.Logger
}

func NewHandlerGroup(deps *HandlerDeps) *HandlerGroup {
	return &HandlerGroup{
		InteractionHandler: InteractionHandler{logic: deps.Logic, logger: deps.Logger},
		CommentHandler:     CommentHandler{logic: deps.Logic, logger: deps.Logger},
	}
}

// -----------------------------------------------------------------------------
// InteractionHandler：点赞
// -----------------------------------------------------------------------------

type InteractionHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (i *InteractionHandler) LikeVideo(c *gin.Context) {
	var req request.InteractionVideo
	err := c.ShouldBindJSON(&req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	// 使用 JWT 中提取的 userID，忽略请求体中的 user_id，防止伪造给他人点赞
	req.UserID = auth.GetUserID(c, i.logic.Config)
	if req.UserID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}
	err = i.logic.InteractionLogic.LikeVideo(c.Request.Context(), req)
	if err != nil {
		i.logger.Error("Failed to like video:", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithMsg(c, "Successfully like video")
}

func (i *InteractionHandler) UnlikeVideo(c *gin.Context) {
	var req request.InteractionVideo
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	req.UserID = auth.GetUserID(c, i.logic.Config)
	if req.UserID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}
	if err := i.logic.InteractionLogic.UnlikeVideo(c.Request.Context(), req); err != nil {
		i.logger.Error("Failed to unlike video:", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithMsg(c, "Successfully unlike video")
}

// -----------------------------------------------------------------------------
// CommentHandler：评论
// -----------------------------------------------------------------------------

type CommentHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *CommentHandler) CreateComment(c *gin.Context) {
	var req request.CreateCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}

	commentID, err := h.logic.CommentLogic.CreateComment(c.Request.Context(), userID, req)
	if err != nil {
		h.logger.Error("创建评论失败", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, gin.H{"comment_id": commentID})
}

func (h *CommentHandler) ListComments(c *gin.Context) {
	var req request.ListCommentsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	list, total, err := h.logic.CommentLogic.ListComments(c.Request.Context(), req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, gin.H{
		"list":  list,
		"total": total,
	})
}

func (h *CommentHandler) ListReplies(c *gin.Context) {
	var req request.ListRepliesReq
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	list, total, err := h.logic.CommentLogic.ListReplies(c.Request.Context(), req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, gin.H{
		"list":  list,
		"total": total,
	})
}

func (h *CommentHandler) LikeComment(c *gin.Context) {
	var req request.CommentLikeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.CommentLogic.LikeComment(c.Request.Context(), userID, req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithMsg(c, "点赞成功")
}

func (h *CommentHandler) UnlikeComment(c *gin.Context) {
	var req request.CommentLikeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.CommentLogic.UnlikeComment(c.Request.Context(), userID, req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithMsg(c, "取消点赞成功")
}

func (h *CommentHandler) DeleteComment(c *gin.Context) {
	var req request.DeleteCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}

	if err := h.logic.CommentLogic.DeleteComment(c.Request.Context(), userID, req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithMsg(c, "删除成功")
}
