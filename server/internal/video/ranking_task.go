package video

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/redisx"
	"fake_tiktok/pkg/util/lock"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// cronLockKeyRebuildZSet RebuildZSet 分布式锁键名。
const cronLockKeyRebuildZSet = "cron:rebuild_zset"

// cronLockTTL RebuildZSet 锁的过期时间（留足大数据量重建时间，进程崩溃也能自动释放）。
const cronLockTTL = 300 * time.Second

// cronLockRefreshInterval RebuildZSet 锁续期间隔（TTL 的 1/5）。
const cronLockRefreshInterval = 60 * time.Second

// VideoRankingTask 负责视频热度 ZSet 重建、热度回写 MySQL。
//
// 原位于 internal/pkg/task，随视频域迁移；拆服务后由 worker 进程的 cron 调度，
// 只访问 feed_video 库与共享 Redis。
type VideoRankingTask struct {
	videoRepo   *VideoRepo
	redisClient *redisx.RedisClient
	db          *gorm.DB
	client      *redis.Client // 分布式锁使用原生 client
	logger      *zap.Logger
}

func NewVideoRankingTask(videoRepo *VideoRepo, redisClient *redisx.RedisClient, db *gorm.DB, logger *zap.Logger) *VideoRankingTask {
	return &VideoRankingTask{
		videoRepo:   videoRepo,
		redisClient: redisClient,
		db:          db,
		client:      redisClient.Client,
		logger:      logger,
	}
}

// CalculatePopularity 综合多项指标计算视频热度分。
//
//	score = Likes×3 + Comments×2 + Plays×0.1
//	popularity = score / (hours_since_publish + 2)^1.5
func CalculatePopularity(video model.Video) float64 {
	hoursSincePublish := time.Since(video.CreatedAt).Hours()
	if hoursSincePublish < 0 {
		hoursSincePublish = 0
	}
	gravity := 1.5
	score := float64(video.LikesCount)*3 +
		float64(video.CommentsCount)*2 +
		float64(video.PlayCount)*0.1
	popularity := score / math.Pow(hoursSincePublish+2, gravity)
	return popularity
}

// RebuildZSet 全量重建发布视频的 ZSet 并将热度分回写 MySQL。
func (t *VideoRankingTask) RebuildZSet(ctx context.Context) error {
	t.logger.Info("开始重建视频热度 ZSet")

	if err := t.redisClient.Ping(ctx); err != nil {
		t.logger.Warn("Redis 不可用，跳过 ZSet 重建", zap.Error(err))
		return err
	}

	videos, err := t.videoRepo.FindAllPublishedVideoIDs(ctx)
	if err != nil {
		t.logger.Error("查询已发布视频失败", zap.Error(err))
		return err
	}

	if len(videos) == 0 {
		t.logger.Info("没有已发布视频，清空 ZSet")
		return t.clearZSet(ctx)
	}

	globalKey := t.redisClient.BuildKey(redisx.PublishedVideoZSetKey, "global")

	type zoneMember struct {
		key    string
		member redis.Z
	}

	var globalMembers []redis.Z
	var zoneMembers []zoneMember

	type videoPopularity struct {
		video      model.Video
		popularity int64
	}
	var items []videoPopularity
	for _, video := range videos {
		// 一次计算热度值，Redis 和 MySQL 都复用同一个值
		p := int64(math.Round(CalculatePopularity(video)))
		items = append(items, videoPopularity{video: video, popularity: p})
	}

	for _, item := range items {
		member := redis.Z{
			Score:  float64(item.popularity),
			Member: strconv.FormatUint(uint64(item.video.ID), 10),
		}
		globalMembers = append(globalMembers, member)

		if item.video.Zone != "" {
			zoneKey := t.redisClient.BuildKey(
				redisx.PublishedVideoZSetKey,
				item.video.Zone,
			)
			zoneMembers = append(zoneMembers, zoneMember{key: zoneKey, member: member})
		}
	}

	pipe := t.redisClient.Pipeline()
	if pipe == nil {
		return redisx.ErrRedisUnavailable
	}

	// 使用临时 Key + RENAME 原子切换，避免先删后建期间 ZSet 为空
	tempKey := globalKey + ":temp"
	pipe.Del(ctx, tempKey)
	if len(globalMembers) > 0 {
		pipe.ZAdd(ctx, tempKey, globalMembers...)
		pipe.Expire(ctx, tempKey, redisx.PublishedVideoZSetExpire)
		// 裁剪 ZSet：只保留热度最高的 MaxSize 个成员
		if int64(len(globalMembers)) > redisx.PublishedVideoZSetMaxSize {
			pipe.ZRemRangeByRank(ctx, tempKey, 0, int64(len(globalMembers))-redisx.PublishedVideoZSetMaxSize-1)
		}
	}
	pipe.Rename(ctx, tempKey, globalKey) // 原子切换

	// 分区 ZSet 同样使用临时 Key + RENAME 原子切换
	zoneKeySet := make(map[string]string) // zoneKey -> tempKey
	for _, zm := range zoneMembers {
		if _, ok := zoneKeySet[zm.key]; !ok {
			tempZoneKey := zm.key + ":temp"
			zoneKeySet[zm.key] = tempZoneKey
			pipe.Del(ctx, tempZoneKey)
		}
	}
	for _, zm := range zoneMembers {
		pipe.ZAdd(ctx, zoneKeySet[zm.key], zm.member)
	}
	for k, tempK := range zoneKeySet {
		pipe.Expire(ctx, tempK, redisx.PublishedVideoZSetExpire)
		pipe.ZRemRangeByRank(ctx, tempK, 0, -(redisx.PublishedVideoZSetMaxSize + 1))
		pipe.Rename(ctx, tempK, k)
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err = pipe.Exec(ctxTimeout)
	if err != nil {
		t.logger.Error("Redis Pipeline 执行失败", zap.Error(err))
		return err
	}

	for _, item := range items {
		if item.video.Popularity != item.popularity {
			if updateErr := t.videoRepo.UpdatePopularity(ctx, item.video.ID, item.popularity); updateErr != nil {
				t.logger.Warn("更新视频热度到 MySQL 失败，下次 RebuildZSet 将重新计算",
					zap.Uint("video_id", item.video.ID),
					zap.Int64("popularity", item.popularity),
					zap.Error(updateErr))
			}
		}
	}

	t.logger.Info("视频热度 ZSet 重建完成", zap.Int("video_count", len(videos)))
	return nil
}

func (t *VideoRankingTask) clearZSet(ctx context.Context) error {
	globalKey := t.redisClient.BuildKey(redisx.PublishedVideoZSetKey, "global")
	return t.redisClient.Del(ctx, globalKey)
}

// RebuildZSetWithLock 是带 Redis 分布式锁的 RebuildZSet 包装。
// 多 worker 部署下保证同一时刻只有一个实例执行重建。
func (t *VideoRankingTask) RebuildZSetWithLock(ctx context.Context) error {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	token := fmt.Sprintf("%s_%d", hostname, time.Now().UnixNano())

	l, err := lock.Acquire(ctx, t.client, cronLockKeyRebuildZSet, token, cronLockTTL)
	if err != nil {
		if errors.Is(err, lock.ErrLockHeld) {
			// 锁被别人持有：当前 worker 不执行，让下一轮 cron 再尝试
			t.logger.Info("RebuildZSet skipped, lock held by another worker")
			return nil
		}
		return err
	}

	// 后台续期协程：RebuildZSet 完成后通过 stopCh 退出
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(cronLockRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				// 续期使用 context.Background()，避免外部 ctx 取消导致锁提前回收
				if refreshErr := l.Refresh(context.Background(), cronLockTTL); refreshErr != nil {
					t.logger.Warn("RebuildZSet lock refresh failed", zap.Error(refreshErr))
					return
				}
			}
		}
	}()

	defer func() {
		close(stopCh) // 通知续期协程退出
		_ = l.Release(ctx)
	}()

	return t.RebuildZSet(ctx)
}
