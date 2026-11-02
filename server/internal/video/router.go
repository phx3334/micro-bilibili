package video

import "github.com/gin-gonic/gin"

// RegisterRoutes 注册 video-svc 的 HTTP 路由。
//
// 统一前缀约定：所有路由都挂在 /api/v1/video 下。
func RegisterRoutes(publicGroup, privateGroup *gin.RouterGroup, h *HandlerGroup) {
	videorouter := publicGroup.Group("video")
	{
		videorouter.GET("list", h.VideoHandler.ListPublishedVideos)
		videorouter.GET("detail", h.VideoHandler.GetVideoDetail)
	}

	// 视频投稿上传与转码状态查询（私有组，需登录）
	videouploadrouter := privateGroup.Group("video")
	{
		videouploadrouter.POST("upload", h.VideoUploadHandler.Upload)
		videouploadrouter.GET("status", h.VideoUploadHandler.GetTranscodeStatus)
		// 作者删除自己投稿的视频（权限校验在 Logic 层：操作者须为视频作者）
		videouploadrouter.DELETE("", h.VideoHandler.DeleteVideo)
	}
}
