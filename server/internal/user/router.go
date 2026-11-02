package user

import "github.com/gin-gonic/gin"

// RegisterRoutes 注册 user-svc 的 HTTP 路由。
//
// 统一前缀约定：所有路由都挂在 /api/v1/user 下（publicGroup/privateGroup 已带 /api/v1）。
// 仅调整路径分组，handler 内部逻辑不变。
func RegisterRoutes(publicGroup, privateGroup *gin.RouterGroup, h *HandlerGroup) {
	userPublic := publicGroup.Group("user")
	userPrivate := privateGroup.Group("user")

	// 验证码（登录/注册依赖）
	userPublic.POST("base/captcha", h.BaseHandler.Captcha)

	// 登录 / 注册
	userPublic.POST("login", h.UserHandler.Login)
	userPublic.POST("register", h.UserHandler.Register)

	// 需登录：登出 / 资料 / 头像
	userPrivate.POST("logout", h.UserHandler.Logout)
	userPrivate.GET("info", h.UserHandler.PersonalInfo)
	userPrivate.PUT("changeInfo", h.UserHandler.UserChangeInfo)
	userPrivate.POST("avatar", h.UserHandler.UploadAvatar)

	// 主页聚合（公开）
	userPublic.GET("home", h.UserHandler.UserHomePage)
	userPublic.GET("videos", h.UserHomeHandler.ListUserVideos)

	// 关注 / 粉丝（拆分后归 user-svc；原 /interaction/follow 一并迁入）
	userPublic.GET("follow/followers", h.FollowHandler.ListFollowers)
	userPublic.GET("follow/following", h.FollowHandler.ListFollowing)
	userPrivate.POST("follow", h.FollowHandler.FollowUser)
	userPrivate.POST("unfollow", h.FollowHandler.UnfollowUser)
}
