// interaction-svc 进程入口：点赞/取消、评论与评论点赞。
//
// 视频/用户信息一律通过 gRPC 拉取；点赞/评论计数只发 Kafka 增量，由 worker 回刷 video 库。
// 生命周期由 pkg/appboot 统一负责；本文件只保留"这个服务需要装配什么"。
package main

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/interaction"
	"fake_tiktok/pkg/appboot"
)

func main() {
	appboot.Run(appboot.Options{
		ServiceName:     "interaction-svc",
		DefaultDBName:   "feed_interaction",
		DefaultHTTPPort: 8083,
		DefaultGRPCPort: 9083,
		AutoMigrate:     interaction.AutoMigrateInteraction,
		Prepare: func(d *appboot.Deps) (*appboot.Wiring, error) {
			// 跨域客户端：视频信息来自 video-svc，用户信息来自 user-svc
			videoClient, err := interaction.NewVideoGRPCClient(d.Cfg.GRPC.VideoAddr)
			if err != nil {
				return nil, err
			}
			userClient, err := interaction.NewUserGRPCClient(d.Cfg.GRPC.UserAddr)
			if err != nil {
				return nil, err
			}

			repos := interaction.NewRepos(d.DB, d.RedisClient, d.Producer, d.Logger)
			logicGroup := interaction.InitLogicGroup(&interaction.LogicDeps{
				Config:                d.Cfg,
				InteractionRepo:       repos.InteractionRepo,
				VideoCommentRepo:      repos.VideoCommentRepo,
				VideoCommentLikeRepo:  repos.VideoCommentLikeRepo,
				InteractionCacheRepo:  repos.InteractionCacheRepo,
				VideoCacheCounterRepo: repos.VideoCacheCounterRepo,
				BackfillRepo:          repos.BackfillRepo,
				CounterPublisher:      repos.CounterPublisher,
				VideoClient:           videoClient,
				UserClient:            userClient,
				Logger:                d.Logger,
			})
			handlerGroup := interaction.NewHandlerGroup(&interaction.HandlerDeps{Logic: logicGroup, Logger: d.Logger})

			return &appboot.Wiring{
				// 本服务不持有账号库，黑名单校验交给 user-svc
				Routes: func(public, private *gin.RouterGroup) {
					interaction.RegisterRoutes(public, private, handlerGroup)
				},
				GRPC: func(s *grpc.Server) {
					pb.RegisterInteractionServiceServer(s, interaction.NewInteractionGRPCServer(repos.InteractionRepo))
				},
			}, nil
		},
	})
}
