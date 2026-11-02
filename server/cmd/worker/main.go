// worker 进程入口（无 HTTP/gRPC）。
//
// 职责：
//  1. 消费 video.transcode：调用 ffmpeg 转码并回写 feed_video 库
//  2. 消费 video.playcount / video.likecount / video.commentcount：
//     内存聚合后定期回刷 videos 的 play_count / likes_count / comments_count
//  3. cfg.RunCron=true 时执行热度 ZSet 重建定时任务
//
// 库：feed_video；不调用任何服务间 gRPC。
package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"fake_tiktok/internal/video"
	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/gormx"
	"fake_tiktok/pkg/kafkax"
	"fake_tiktok/pkg/log"
	"fake_tiktok/pkg/otelx"
	"fake_tiktok/pkg/redisx"
	"fake_tiktok/pkg/storagex"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		zap.L().Error("加载配置失败", zap.Error(err))
		os.Exit(1)
	}
	if os.Getenv("APP_DATABASE_DBNAME") == "" {
		cfg.Database.DBName = "feed_video"
	}

	logger := log.InitZap(&cfg.Zap)

	// worker 没有 HTTP 端口（因此不暴露 /metrics），但仍要初始化 OTel：
	// MySQL / Redis / Kafka 的自动埋点与转码 span 都依赖全局 Provider 与 propagator。
	cfg.OTEL.ServiceName = "worker-svc"
	otelShutdown, otelErr := otelx.Init(context.Background(), cfg)
	if otelErr != nil {
		logger.Error("OpenTelemetry 初始化失败，观测能力降级", zap.Error(otelErr))
	}

	// 2. 连接 feed_video 库。迁移由 video-svc 负责，worker 不执行 DDL，避免多进程竞争。
	db := gormx.InitGorm(cfg)

	redisConn, err := redisx.ConnectRedis(&cfg.Redis)
	if err != nil {
		logger.Error("Redis 初始化失败", zap.Error(err))
		os.Exit(1)
	}
	redisClient := redisx.NewRedisClient(redisConn.Client, redisConn.KeyPrefix)

	// 3. 信号处理：SIGINT / SIGTERM 任一到达即触发优雅停机
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 4. 热度 ZSet 重建定时任务（仅 cfg.RunCron=true 时启动）
	var c *cron.Cron
	rankingTask := video.NewVideoRankingTask(video.NewVideoRepo(db), redisClient, db, logger)
	if cfg.RunCron {
		c = InitCron(logger)
		rebuildJob := cron.FuncJob(func() {
			if err := rankingTask.RebuildZSetWithLock(context.Background()); err != nil {
				logger.Error("视频热度 ZSet 重建失败", zap.Error(err))
			}
		})
		if _, err := c.AddJob("@every 1m", RecoverCronJob(logger)(rebuildJob)); err != nil {
			logger.Error("注册 cron 任务失败", zap.Error(err))
		}
		c.Start()
		logger.Info("cron started in worker process")
	} else {
		logger.Info("cron disabled in worker process (set APP_RUN_CRON=true to enable)")
	}

	// 5. 计数聚合器：每个 videos 计数列一个 pending map + flush 协程
	playAgg := newCounterAggregator(db, "play_count", logger)
	likeAgg := newCounterAggregator(db, "likes_count", logger)
	commentAgg := newCounterAggregator(db, "comments_count", logger)

	var wg sync.WaitGroup

	type consumeTarget struct {
		agg     *counterAggregator
		topic   string
		groupID string
	}
	targets := []consumeTarget{
		{playAgg, kafkax.TopicVideoPlaycount, "worker-playcount"},
		{likeAgg, kafkax.TopicVideoLikecount, "worker-likecount"},
		{commentAgg, kafkax.TopicVideoCommentcount, "worker-commentcount"},
	}

	for _, t := range targets {
		wg.Add(2)
		go func(t consumeTarget) {
			defer wg.Done()
			t.agg.runFlusher(ctx)
		}(t)
		go func(t consumeTarget) {
			defer wg.Done()
			t.agg.consumeLoop(ctx, cfg.Kafka.Brokers, t.topic, t.groupID)
		}(t)
	}

	// 6. 转码消费者：Storage 实例用于下载源文件、上传转码产物。
	// MinIO 是 worker 的核心依赖，初始化失败直接退出，避免消费无法完成的任务。
	storageInstance, err := storagex.NewStorage(&cfg.Minio)
	if err != nil {
		logger.Error("failed to init MinIO storage for transcode worker", zap.Error(err))
		os.Exit(1)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		// 单次转码超时来自 APP_TRANSCODE_TIMEOUT（秒），0 时由 transcodeOne 兜底 30 分钟
		transcodeTimeout := time.Duration(cfg.Transcode.Timeout) * time.Second
		transcodeConsumeLoop(ctx, cfg.Kafka.Brokers, db, storageInstance, cfg.FFmpeg, transcodeTimeout, logger)
	}()

	logger.Info("Worker 已启动，正在消费播放量/点赞数/评论数增量与转码任务...")

	// 7. 阻塞等待信号
	<-ctx.Done()
	logger.Info("worker shutdown signal received")

	// 8. 关闭 cron（仅当启用），等待正在运行的 job 结束
	if c != nil {
		logger.Info("stopping cron...")
		cronCtx := c.Stop()
		select {
		case <-cronCtx.Done():
			logger.Info("cron stopped, all jobs finished")
		case <-time.After(5 * time.Second):
			logger.Warn("cron stop timeout (5s), some jobs may still be running")
		}
	}

	// 9. 等待消费/聚合协程退出（带 10 秒兜底）。
	// 聚合器在 ctx.Done 后会做最后一次 flush；必须等它们完成再关闭 DB。
	logger.Info("waiting for goroutines to exit...")
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
		logger.Info("all goroutines exited")
	case <-time.After(10 * time.Second):
		logger.Warn("worker goroutines exit timeout (10s), forcing shutdown")
	}

	// 10. 释放资源
	// OTel 关闭要在连接释放之前：flush BatchSpanProcessor 中尚未发送的 span
	otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer otelCancel()
	if err := otelShutdown(otelCtx); err != nil {
		logger.Error("otel shutdown error", zap.Error(err))
	}

	if err := redisConn.Client.Close(); err != nil {
		logger.Error("关闭 Redis 失败", zap.Error(err))
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		_ = sqlDB.Close()
	}
}
