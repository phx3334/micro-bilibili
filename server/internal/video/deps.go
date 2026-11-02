package video

import (
	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/redisx"
	"fake_tiktok/pkg/storagex"

	"go.uber.org/zap"
)

// LogicDeps 视频服务业务层依赖聚合。
//
// 跨域依赖：
//   - UserClient：作者用户名/头像（user-svc）
//   - InteractionClient：当前用户的点赞状态（interaction-svc）
//   - RedisClient：热度 ZSet 与缓存键（Redis 由三服务共享）
type LogicDeps struct {
	Config *config.Config

	VideoRepo      *VideoRepo
	PaginateRepo   *PaginateRepo
	VideoCacheRepo *VideoCacheRepo
	BackfillRepo   *BackfillRepo
	RedisClient    *redisx.RedisClient

	PlayCountPublisher *PlayCountPublisher
	TranscodePublisher *TranscodePublisher

	UserClient        *UserGRPCClient
	InteractionClient *InteractionGRPCClient

	Storage storagex.Storage
	Logger  *zap.Logger
}

type LogicGroup struct {
	Config           *config.Config
	VideoLogic       *VideoLogic
	VideoUploadLogic *VideoUploadLogic
}

var LogicGroupApp = &LogicGroup{}

// InitLogicGroup 构造视频服务 Logic 业务类
func InitLogicGroup(deps *LogicDeps) *LogicGroup {
	lg := &LogicGroup{
		Config:           deps.Config,
		VideoLogic:       NewVideoLogic(deps),
		VideoUploadLogic: NewVideoUploadLogic(deps, deps.Storage),
	}
	LogicGroupApp = lg
	return lg
}
