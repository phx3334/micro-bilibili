// video-svc 进程入口：视频上传、转码状态、详情、公开列表、分区排行、删除、用户统计（gRPC）。
//
// 生命周期（配置 → 观测 → MySQL/Redis/Kafka → 路由 → 双端口 → 优雅停机）由
// pkg/appboot 统一负责；本文件只保留"这个服务需要装配什么"。
package main

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/video"
	"fake_tiktok/pkg/appboot"
)

func main() {
	appboot.Run(appboot.Options{
		ServiceName:     "video-svc",
		DefaultDBName:   "feed_video",
		DefaultHTTPPort: 8082,
		DefaultGRPCPort: 9082,
		// 上传源文件与转码产物均写入 MinIO 对象存储，DB 只保存对象 key
		NeedsStorage: true,
		AutoMigrate:  video.AutoMigrateVideo,
		Prepare: func(d *appboot.Deps) (*appboot.Wiring, error) {
			// 跨域客户端：作者信息来自 user-svc，点赞状态来自 interaction-svc
			userClient, err := video.NewUserGRPCClient(d.Cfg.GRPC.UserAddr)
			if err != nil {
				return nil, err
			}
			interactionClient, err := video.NewInteractionGRPCClient(d.Cfg.GRPC.InteractionAddr)
			if err != nil {
				return nil, err
			}

			repos := video.NewRepos(d.DB, d.RedisClient, d.Producer, userClient, d.Logger, d.Cfg.Minio.Bucket)
			logicGroup := video.InitLogicGroup(&video.LogicDeps{
				Config:             d.Cfg,
				VideoRepo:          repos.VideoRepo,
				PaginateRepo:       repos.PaginateRepo,
				VideoCacheRepo:     repos.VideoCacheRepo,
				BackfillRepo:       repos.BackfillRepo,
				RedisClient:        repos.RedisClient,
				PlayCountPublisher: repos.PlayCountPublisher,
				TranscodePublisher: repos.TranscodePublisher,
				UserClient:         userClient,
				InteractionClient:  interactionClient,
				Storage:            d.Storage,
				Logger:             d.Logger,
			})
			handlerGroup := video.NewHandlerGroup(&video.HandlerDeps{Logic: logicGroup, Logger: d.Logger})

			return &appboot.Wiring{
				// 本服务不持有账号库，黑名单校验交给 user-svc
				Routes: func(public, private *gin.RouterGroup) {
					video.RegisterRoutes(public, private, handlerGroup)
				},
				GRPC: func(s *grpc.Server) {
					pb.RegisterVideoServiceServer(s, video.NewVideoGRPCServer(repos.VideoRepo, d.Storage))
				},
			}, nil
		},
	})
}
