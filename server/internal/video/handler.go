package video

import (
	"context"
	"strconv"
	"time"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/pkg/auth"
	"fake_tiktok/pkg/httpx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HandlerGroup 视频服务 HTTP 处理器集合
type HandlerGroup struct {
	VideoHandler
	VideoUploadHandler
}

type HandlerDeps struct {
	Logic  *LogicGroup
	Logger *zap.Logger
}

func NewHandlerGroup(deps *HandlerDeps) *HandlerGroup {
	return &HandlerGroup{
		VideoHandler:       VideoHandler{logic: deps.Logic, logger: deps.Logger},
		VideoUploadHandler: VideoUploadHandler{logic: deps.Logic, logger: deps.Logger},
	}
}

// -----------------------------------------------------------------------------
// VideoHandler：已发布视频的列表 / 详情 / 删除
// -----------------------------------------------------------------------------

type VideoHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *VideoHandler) ListPublishedVideos(c *gin.Context) {
	var req request.HomeVideoList
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, "参数错误")
		return
	}

	list, total, nextCursor := h.logic.VideoLogic.ListHotVideos(req)

	httpx.OkWithData(c, gin.H{
		"list":        list,
		"total":       total,
		"next_cursor": nextCursor,
		"has_more":    nextCursor != "",
	})
}

func (h *VideoHandler) GetVideoDetail(c *gin.Context) {
	var req request.VideoDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, "参数错误")
		return
	}

	userID := auth.GetUserID(c, h.logic.Config)
	data, err := h.logic.VideoLogic.GetVideoDetail(c.Request.Context(), userID, req.VideoID)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, data)
}

// DeleteVideo 处理作者删除自己投稿的视频请求。
func (h *VideoHandler) DeleteVideo(c *gin.Context) {
	var req request.VideoDetailReq
	// 优先从 query 字符串读取（前端用 params 传参，与 detail 接口一致）；
	// 兜底再尝试 JSON/表单 body，避免参数放错位置直接报"参数错误"。
	if err := c.ShouldBindQuery(&req); err != nil {
		if err2 := c.ShouldBind(&req); err2 != nil {
			httpx.FailWithMsg(c, "参数错误")
			return
		}
	}

	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "请先登录")
		return
	}

	if err := h.logic.VideoLogic.DeleteVideo(c.Request.Context(), userID, req.VideoID); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithMsg(c, "删除成功")
}

// -----------------------------------------------------------------------------
// VideoUploadHandler：视频投稿上传与转码状态轮询
// -----------------------------------------------------------------------------

type VideoUploadHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *VideoUploadHandler) Upload(c *gin.Context) {
	// 1. 获取调用者 userID；JWTAuth 中间件已校验登录态
	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}

	// 2. 解析必填的视频文件
	file, err := c.FormFile("file")
	if err != nil {
		httpx.FailWithMsg(c, "缺少视频文件")
		return
	}

	// 3. 解析可选的封面文件；不存在时 FormFile 返回错误，忽略即可
	cover, _ := c.FormFile("cover")

	// 4. 绑定表单元数据
	var req request.VideoUploadReq
	if err := c.ShouldBind(&req); err != nil {
		httpx.FailWithMsg(c, "参数错误")
		return
	}

	// 5. 调用 logic 完成原子写入 + DB 插入 + 消息发布
	// 设置 1 小时超时：与前端 axios 超时对齐，支持大文件上传
	uploadCtx, uploadCancel := context.WithTimeout(c.Request.Context(), 60*time.Minute)
	defer uploadCancel()
	videoID, err := h.logic.VideoUploadLogic.Upload(uploadCtx, userID, file, cover, req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, gin.H{"video_id": videoID})
}

func (h *VideoUploadHandler) GetTranscodeStatus(c *gin.Context) {
	userID := auth.GetUserID(c, h.logic.Config)
	if userID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}

	videoIDStr := c.Query("video_id")
	if videoIDStr == "" {
		httpx.FailWithMsg(c, "缺少 video_id 参数")
		return
	}
	videoID, err := strconv.ParseUint(videoIDStr, 10, 64)
	if err != nil {
		httpx.FailWithMsg(c, "video_id 参数错误")
		return
	}

	data, err := h.logic.VideoUploadLogic.GetTranscodeStatus(c.Request.Context(), userID, uint(videoID))
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, data)
}
