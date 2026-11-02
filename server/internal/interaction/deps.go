package interaction

import (
	"fake_tiktok/pkg/config"

	"go.uber.org/zap"
)

// LogicDeps 互动服务业务层依赖聚合。
//
// 跨域依赖：
//   - VideoClient：校验视频存在 / 取作者 / 列表补全视频信息
//   - UserClient：评论者、作者的用户名与头像
//   - CounterPublisher：点赞/评论计数增量（worker 回刷 video 库）
type LogicDeps struct {
	Config *config.Config

	InteractionRepo      *InteractionRepo
	VideoCommentRepo     *VideoCommentRepo
	VideoCommentLikeRepo *VideoCommentLikeRepo

	InteractionCacheRepo  *InteractionCacheRepo
	VideoCacheCounterRepo *VideoCacheCounterRepo
	BackfillRepo          *BackfillRepo
	CounterPublisher      *CounterPublisher

	VideoClient *VideoGRPCClient
	UserClient  *UserGRPCClient

	Logger *zap.Logger
}

type LogicGroup struct {
	Config           *config.Config
	InteractionLogic *InteractionLogic
	CommentLogic     *CommentLogic
}

var LogicGroupApp = &LogicGroup{}

// InitLogicGroup 构造互动服务 Logic 业务类
func InitLogicGroup(deps *LogicDeps) *LogicGroup {
	lg := &LogicGroup{
		Config:           deps.Config,
		InteractionLogic: NewInteractionLogic(deps),
		CommentLogic:     NewCommentLogic(deps),
	}
	LogicGroupApp = lg
	return lg
}
