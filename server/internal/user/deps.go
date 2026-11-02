package user

import (
	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/storagex"

	"github.com/mojocn/base64Captcha"
	"go.uber.org/zap"
)

// LogicDeps 用户服务业务层依赖聚合。
//
// 与拆服务前相比：
//   - 视频相关依赖（VideoRepo）已换成 VideoGRPCClient，跨域只走 gRPC
//   - 关注/粉丝关系留在本服务库，使用 FollowRepo
type LogicDeps struct {
	Config        *config.Config
	AccountRepo   *AccountRepo
	FollowRepo    *FollowRepo
	JWTRepo       *JWTRepo
	UserCacheRepo *UserCacheRepo
	BackfillRepo  *BackfillRepo
	CaptchaStore  base64Captcha.Store
	Storage       storagex.Storage
	VideoClient   *VideoGRPCClient
	Logger        *zap.Logger
}

type LogicGroup struct {
	Config        *config.Config
	UserLogic     *UserLogic
	BaseLogic     *BaseLogic
	FollowLogic   *FollowLogic
	UserHomeLogic *UserHomeLogic
}

var LogicGroupApp = &LogicGroup{}

// InitLogicGroup 构造所有用户服务 Logic 业务类
func InitLogicGroup(deps *LogicDeps) *LogicGroup {
	lg := &LogicGroup{
		Config:        deps.Config,
		UserLogic:     NewUserLogic(deps),
		BaseLogic:     NewBaseLogic(deps),
		FollowLogic:   NewFollowLogic(deps),
		UserHomeLogic: NewUserHomeLogic(deps),
	}
	LogicGroupApp = lg
	return lg
}
