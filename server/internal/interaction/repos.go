package interaction

import (
	"context"

	"fake_tiktok/pkg/kafkax"
	"fake_tiktok/pkg/redisx"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Repos 互动服务仓储集合（只连接 feed_interaction 库）
type Repos struct {
	InteractionRepo      *InteractionRepo
	VideoCommentRepo     *VideoCommentRepo
	VideoCommentLikeRepo *VideoCommentLikeRepo

	InteractionCacheRepo  *InteractionCacheRepo
	VideoCacheCounterRepo *VideoCacheCounterRepo
	BackfillRepo          *BackfillRepo
	CounterPublisher      *CounterPublisher
}

func NewRepos(db *gorm.DB, redisClient *redisx.RedisClient, producer *kafkax.Producer, logger *zap.Logger) *Repos {
	interactionRepo := NewInteractionRepo(db)

	repos := &Repos{
		InteractionRepo:       interactionRepo,
		VideoCommentRepo:      NewVideoCommentRepo(db),
		VideoCommentLikeRepo:  NewVideoCommentLikeRepo(db),
		InteractionCacheRepo:  NewInteractionCacheRepo(redisClient, logger),
		VideoCacheCounterRepo: NewVideoCacheCounterRepo(redisClient),
		BackfillRepo:          NewBackfillRepo(interactionRepo),
	}

	if producer != nil {
		buf := kafkax.NewPublishBuffer(producer, logger)
		buf.Start(context.Background())
		repos.CounterPublisher = NewCounterPublisher(buf)
	}
	return repos
}
