package video

import (
	"context"

	"fake_tiktok/pkg/kafkax"
	"fake_tiktok/pkg/redisx"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Repos 视频服务仓储集合（只连接 feed_video 库）
type Repos struct {
	VideoRepo          *VideoRepo
	PaginateRepo       *PaginateRepo
	VideoCacheRepo     *VideoCacheRepo
	BackfillRepo       *BackfillRepo
	PlayCountPublisher *PlayCountPublisher
	TranscodePublisher *TranscodePublisher

	// Redis 同时用于热度 ZSet 与缓存键拼接，直接用 pkg 提供的客户端
	RedisClient *redisx.RedisClient
}

// NewRepos 构造视频服务仓储集合。
// producer 允许为 nil（worker 进程只做回刷，不发布消息）。
func NewRepos(db *gorm.DB, redisClient *redisx.RedisClient, producer *kafkax.Producer, userClient *UserGRPCClient, logger *zap.Logger, minioBucket string) *Repos {
	videoCacheRepo := NewVideoCacheRepo(redisClient, logger)

	var buf *kafkax.PublishBuffer
	if producer != nil {
		buf = kafkax.NewPublishBuffer(producer, logger)
		buf.Start(context.Background())
	}

	repos := &Repos{
		VideoRepo:      NewVideoRepo(db),
		PaginateRepo:   NewPaginateRepo(db),
		VideoCacheRepo: videoCacheRepo,
		BackfillRepo:   NewBackfillRepo(videoCacheRepo, userClient, logger, minioBucket),
		RedisClient:    redisClient,
	}
	if buf != nil {
		repos.PlayCountPublisher = NewPlayCountPublisher(buf)
		repos.TranscodePublisher = NewTranscodePublisher(buf)
	}
	return repos
}
