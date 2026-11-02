package user

import (
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/auth"
	"fake_tiktok/pkg/httpx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// HandlerGroup 用户服务 HTTP 处理器集合
type HandlerGroup struct {
	UserHandler
	BaseHandler
	FollowHandler
	UserHomeHandler
}

type HandlerDeps struct {
	Logic  *LogicGroup
	Logger *zap.Logger
}

func NewHandlerGroup(deps *HandlerDeps) *HandlerGroup {
	return &HandlerGroup{
		UserHandler:     UserHandler{logic: deps.Logic, logger: deps.Logger},
		BaseHandler:     BaseHandler{logic: deps.Logic, logger: deps.Logger},
		FollowHandler:   FollowHandler{logic: deps.Logic, logger: deps.Logger},
		UserHomeHandler: UserHomeHandler{logic: deps.Logic, logger: deps.Logger},
	}
}

// -----------------------------------------------------------------------------
// BaseHandler：图形验证码
// -----------------------------------------------------------------------------

type BaseHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *BaseHandler) Captcha(c *gin.Context) {
	id, b64s, err := h.logic.BaseLogic.GenerateCaptcha()
	if err != nil {
		h.logger.Error("generate captcha failed", zap.Error(err))
		httpx.FailWithMsg(c, "generate captcha failed")
		return
	}
	httpx.OkWithData(c, httpx.Captcha{
		CaptchaID: id,
		PicPath:   b64s,
	})
}

// -----------------------------------------------------------------------------
// UserHandler：注册 / 登录 / 资料 / 头像
// -----------------------------------------------------------------------------

type UserHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *UserHandler) Register(c *gin.Context) {
	var req request.Register
	err := c.ShouldBindJSON(&req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}

	if !h.logic.UserLogic.VerifyCaptcha(req.CaptchaID, req.Captcha) {
		httpx.FailWithMsg(c, "captcha not match or expired")
		return
	}

	u := model.Account{Username: req.Username, Password: req.Password}
	if _, err := h.logic.UserLogic.Register(c.Request.Context(), u); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithMsg(c, "register success")
}

func (h *UserHandler) handleTokenResult(c *gin.Context, user model.Account) {
	result, err := h.logic.UserLogic.GenerateToken(user)
	if err != nil {
		h.logger.Error("Failed to generate token:", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}
	auth.SetRefreshToken(c, result.RefreshToken, result.RefreshExpiry)
	c.Set("user_id", result.Account.ID)
	httpx.OkWithDetail(c, httpx.Login{
		Account: httpx.LoginAccount{
			ID:        result.Account.ID,
			Username:  result.Account.Username,
			AvatarURL: result.Account.AvatarURL,
			Signature: result.Account.Signature,
			Gender:    result.Account.Gender,
			Birthday:  result.Account.Birthday,
		},
		AccessToken:       result.AccessToken,
		AccessTokenExpire: int(result.AccessTokenExpire),
	}, "login success")
}

func (h *UserHandler) Login(c *gin.Context) {
	var req request.Login
	err := c.ShouldBindJSON(&req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	if !h.logic.UserLogic.VerifyCaptcha(req.CaptchaID, req.Captcha) {
		httpx.FailWithMsg(c, "captcha not match or expired")
		return
	}
	u := model.Account{Username: req.Username, Password: req.Password}
	user, err := h.logic.UserLogic.Login(c.Request.Context(), u)
	if err != nil {
		h.logger.Error("Failed to login:", zap.Error(err))
		httpx.FailWithMsg(c, "login failed")
		return
	}
	h.handleTokenResult(c, user)
}

func (h *UserHandler) UserHomePage(c *gin.Context) {
	var req request.UserCard
	err := c.ShouldBindQuery(&req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	userHome, err := h.logic.UserLogic.UserHome(c.Request.Context(), auth.GetUserID(c, h.logic.Config), req.UserID, req.Page, req.PageSize)
	if err != nil {
		h.logger.Error("Failed to get card:", zap.Error(err))
		httpx.FailWithMsg(c, "Failed to get card")
		return
	}
	httpx.OkWithData(c, userHome)
}

func (h *UserHandler) Logout(c *gin.Context) {
	userID := auth.GetUserID(c, h.logic.Config)
	refreshToken := auth.GetRefreshToken(c)
	auth.ClearRefreshToken(c)
	// Logout 返回 error：Redis 不可用时 JWT 无法拉黑，需告知客户端
	if err := h.logic.UserLogic.Logout(userID, refreshToken); err != nil {
		h.logger.Warn("Logout may not have taken effect",
			zap.String("user_id", userID), zap.Error(err))
		httpx.FailWithMsg(c, "logout may not have taken effect due to server error")
		return
	}
	httpx.OkWithMsg(c, "Successful logout")
}

func (h *UserHandler) PersonalInfo(c *gin.Context) {
	userID := auth.GetUserID(c, h.logic.Config)
	user, err := h.logic.UserLogic.UserInfo(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user info:", zap.Error(err))
		httpx.FailWithMsg(c, "Failed to get user info")
		return
	}
	httpx.OkWithData(c, user)
}

func (h *UserHandler) UserChangeInfo(c *gin.Context) {
	var req request.UserChangeInfo
	err := c.ShouldBindJSON(&req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	req.UserID = auth.GetUserID(c, h.logic.Config)
	err = h.logic.UserLogic.UserChangeInfo(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("Failed to change user info:", zap.Error(err))
		httpx.FailWithMsg(c, "Failed to change user info")
		return
	}
	httpx.OkWithMsg(c, "Successful change user info")
}

func (h *UserHandler) UploadAvatar(c *gin.Context) {
	file, err := c.FormFile("avatar")
	if err != nil {
		httpx.FailWithMsg(c, "please upload an avatar file")
		return
	}

	userID := auth.GetUserID(c, h.logic.Config)
	avatarURL, err := h.logic.UserLogic.UploadAvatar(c.Request.Context(), userID, file)
	if err != nil {
		h.logger.Error("Failed to upload avatar:", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}

	httpx.OkWithData(c, map[string]string{"avatar_url": avatarURL})
}

// -----------------------------------------------------------------------------
// FollowHandler：关注/粉丝列表 + 关注/取关
// -----------------------------------------------------------------------------

type FollowHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *FollowHandler) ListFollowers(c *gin.Context) {
	var req request.ListFollowersReq
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	data, err := h.logic.FollowLogic.ListFollowers(c.Request.Context(), req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithData(c, data)
}

func (h *FollowHandler) ListFollowing(c *gin.Context) {
	var req request.ListFollowingReq
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	data, err := h.logic.FollowLogic.ListFollowing(c.Request.Context(), req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithData(c, data)
}

// FollowUser 关注用户（原路由 /interaction/follow，拆分后归入 user-svc）
func (h *FollowHandler) FollowUser(c *gin.Context) {
	var req request.FollowUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	followerID := auth.GetUserID(c, h.logic.Config)
	if followerID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}
	if err := h.logic.FollowLogic.FollowUser(c.Request.Context(), followerID, req.TargetUserID); err != nil {
		h.logger.Error("Failed to follow user:", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithMsg(c, "关注成功")
}

// UnfollowUser 取关用户
func (h *FollowHandler) UnfollowUser(c *gin.Context) {
	var req request.FollowUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	followerID := auth.GetUserID(c, h.logic.Config)
	if followerID == "" {
		httpx.FailWithMsg(c, "未登录")
		return
	}
	if err := h.logic.FollowLogic.UnfollowUser(c.Request.Context(), followerID, req.TargetUserID); err != nil {
		h.logger.Error("Failed to unfollow user:", zap.Error(err))
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithMsg(c, "取关成功")
}

// -----------------------------------------------------------------------------
// UserHomeHandler：用户主页视频列表
// -----------------------------------------------------------------------------

type UserHomeHandler struct {
	logic  *LogicGroup
	logger *zap.Logger
}

func (h *UserHomeHandler) ListUserVideos(c *gin.Context) {
	var req request.ListUserVideosReq
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.FailWithMsg(c, "参数错误：user_id 必填")
		return
	}

	data, err := h.logic.UserHomeLogic.ListUserVideos(c.Request.Context(), req)
	if err != nil {
		httpx.FailWithMsg(c, err.Error())
		return
	}
	httpx.OkWithData(c, data)
}
