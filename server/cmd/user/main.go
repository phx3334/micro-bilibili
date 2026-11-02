// user-svc 进程入口：注册/登录/JWT/验证码、资料与头像、关注/粉丝、个人主页聚合、用户简配（gRPC）。
//
// 生命周期（配置 → 观测 → MySQL/Redis/Kafka → 路由 → 双端口 → 优雅停机）由
// pkg/appboot 统一负责；本文件只保留"这个服务需要装配什么"。
package main

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"fake_tiktok/api/pb"
	"fake_tiktok/internal/user"
	"fake_tiktok/pkg/appboot"
	"fake_tiktok/pkg/redisx"
)

func main() {
	appboot.Run(appboot.Options{
		ServiceName:     "user-svc",
		DefaultDBName:   "feed_user",
		DefaultHTTPPort: 8081,
		DefaultGRPCPort: 9081,
		// 头像写入 MinIO 对象存储，DB 只保存对象 key
		NeedsStorage: true,
		AutoMigrate:  user.AutoMigrateUser,
		Prepare: func(d *appboot.Deps) (*appboot.Wiring, error) {
			// 验证码存储走 Redis（TTL 5min，GETDEL 原子消费）
			captchaStore := redisx.NewRedisCaptchaStore(d.RedisClient)

			// 个人主页/视频列表需要向 video-svc 拉数据
			videoClient, err := user.NewVideoGRPCClient(d.Cfg.GRPC.VideoAddr)
			if err != nil {
				return nil, err
			}

			repos := user.NewRepos(d.DB, d.RedisClient, d.Logger)
			logicGroup := user.InitLogicGroup(&user.LogicDeps{
				Config:        d.Cfg,
				AccountRepo:   repos.AccountRepo,
				FollowRepo:    repos.FollowRepo,
				JWTRepo:       repos.JWTRepo,
				UserCacheRepo: repos.UserCacheRepo,
				BackfillRepo:  repos.BackfillRepo,
				CaptchaStore:  captchaStore,
				Storage:       d.Storage,
				VideoClient:   videoClient,
				Logger:        d.Logger,
			})
			handlerGroup := user.NewHandlerGroup(&user.HandlerDeps{Logic: logicGroup, Logger: d.Logger})

			return &appboot.Wiring{
				// 只有本服务持有账号库，refresh token 黑名单从这里校验
				Blacklist: repos.JWTRepo,
				Routes: func(public, private *gin.RouterGroup) {
					user.RegisterRoutes(public, private, handlerGroup)
				},
				GRPC: func(s *grpc.Server) {
					pb.RegisterUserServiceServer(s, user.NewUserGRPCServer(repos.AccountRepo, repos.FollowRepo, d.Storage))
				},
			}, nil
		},
	})
}
