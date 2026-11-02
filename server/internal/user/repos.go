package user

import (
	"fake_tiktok/pkg/redisx"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Repos 用户服务仓储集合（只连接 feed_user 库）
type Repos struct {
	AccountRepo   *AccountRepo
	FollowRepo    *FollowRepo
	JWTRepo       *JWTRepo
	UserCacheRepo *UserCacheRepo
	BackfillRepo  *BackfillRepo
}

// NewRepos 构造用户服务仓储集合
func NewRepos(db *gorm.DB, redisClient *redisx.RedisClient, logger *zap.Logger) *Repos {
	accountRepo := NewAccountRepo(db)
	followRepo := NewFollowRepo(db)
	userCacheRepo := NewUserCacheRepo(redisClient, logger)

	return &Repos{
		AccountRepo:   accountRepo,
		FollowRepo:    followRepo,
		JWTRepo:       NewJWTRepo(redisClient),
		UserCacheRepo: userCacheRepo,
		BackfillRepo:  NewBackfillRepo(accountRepo, followRepo, userCacheRepo),
	}
}
