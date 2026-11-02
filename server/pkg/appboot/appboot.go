// Package appboot 是 user / video / interaction 三个 HTTP+gRPC 服务的统一启动骨架。
//
// 为什么要有这层：
//   - 三个服务的启动/停机流程此前是逐行复制的（配置加载 → 观测 → MySQL → Redis →
//     Kafka → 路由 → 双端口监听 → 优雅停机）。任何基础设施调整都要改三处，容易漂移。
//   - 这里只收拢"生命周期"，不碰业务：repo/logic/handler 的装配与路由注册仍由各服务
//     在 Prepare 回调里完成，保持每个服务入口可读。
//
// 约定：worker 是无 HTTP/gRPC 的消费进程，不使用本包。
package appboot

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"fake_tiktok/pkg/auth"
	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/gormx"
	"fake_tiktok/pkg/httpx"
	"fake_tiktok/pkg/kafkax"
	"fake_tiktok/pkg/log"
	"fake_tiktok/pkg/otelx"
	"fake_tiktok/pkg/redisx"
	"fake_tiktok/pkg/storagex"
)

// Deps 是骨架交给业务回调的运行时依赖。
//
// 只暴露连接与客户端，不暴露任何业务仓储：业务对象的构造属于服务的 Prepare。
type Deps struct {
	Cfg         *config.Config
	Logger      *zap.Logger
	DB          *gorm.DB
	Redis       *redisx.RedisConn
	RedisClient *redisx.RedisClient
	Producer    *kafkax.Producer
	// Storage 仅在 Options.NeedsStorage 为 true 时非 nil
	Storage storagex.Storage
}

// Wiring 是业务回调返回的"接线结果"。
type Wiring struct {
	// Blacklist 为 nil 表示不做 refresh token 黑名单校验（video / interaction 用）
	Blacklist auth.TokenBlacklist
	// Routes 注册 HTTP 路由；public 组无鉴权，private 组已挂 JWTAuth
	Routes func(public, private *gin.RouterGroup)
	// GRPC 注册 gRPC 服务实现；为 nil 时不启动 gRPC 监听
	GRPC func(*grpc.Server)
	// Close 释放服务特有资源（可选）
	Close func()
}

// Options 描述一个服务的启动参数。
type Options struct {
	// ServiceName 写入 OTel 的 service.name，如 user-svc
	ServiceName string
	// 以下三项在对应环境变量为空时用作默认值，保证 `go run ./cmd/xxx` 零配置可并存
	DefaultDBName   string
	DefaultHTTPPort int
	DefaultGRPCPort int
	// NeedsStorage 为 true 时初始化 MinIO 对象存储（user / video 需要读写媒体对象）
	NeedsStorage bool
	// AutoMigrate 本服务域的建表函数；nil 表示不执行 DDL（如 worker 类进程）
	AutoMigrate func(*gorm.DB, *zap.Logger) error
	// Prepare 装配业务对象并返回接线结果；返回 error 时进程退出
	Prepare func(*Deps) (*Wiring, error)
}

// Run 启动服务并阻塞到收到退出信号或监听出错。
func Run(opts Options) {
	cfg, err := config.LoadConfig()
	if err != nil {
		zap.L().Error("加载配置失败", zap.Error(err))
		os.Exit(1)
	}
	applyDefaults(cfg, opts)

	logger := log.InitZap(&cfg.Zap)

	// 观测必须先于 gormx/redisx：这两个包依赖 otelx.Enabled() 判断是否挂自动埋点。
	// 初始化失败只降级，采集器不可用不应阻断业务启动。
	cfg.OTEL.ServiceName = opts.ServiceName
	otelShutdown, otelErr := otelx.Init(context.Background(), cfg)
	if otelErr != nil {
		logger.Error("OpenTelemetry 初始化失败，观测能力降级", zap.Error(otelErr))
	}

	db := gormx.InitGorm(cfg)
	if cfg.Database.AutoMigrate && opts.AutoMigrate != nil {
		if err := opts.AutoMigrate(db, logger); err != nil {
			logger.Error("数据库迁移失败", zap.Error(err))
		}
	}

	redisConn, err := redisx.ConnectRedis(&cfg.Redis)
	if err != nil {
		logger.Error("Redis 初始化失败", zap.Error(err))
		os.Exit(1)
	}
	redisClient := redisx.NewRedisClient(redisConn.Client, redisConn.KeyPrefix)

	// Producer 是懒连接的，构造本身不产生网络请求；不需要发消息的服务也不会真正建连
	producer := kafkax.NewProducer(&cfg.Kafka, logger)

	var storageInstance storagex.Storage
	if opts.NeedsStorage {
		storageInstance, err = storagex.NewStorage(&cfg.Minio)
		if err != nil {
			logger.Error("初始化 MinIO 对象存储失败", zap.Error(err))
			os.Exit(1)
		}
	}

	deps := &Deps{
		Cfg:         cfg,
		Logger:      logger,
		DB:          db,
		Redis:       redisConn,
		RedisClient: redisClient,
		Producer:    producer,
		Storage:     storageInstance,
	}
	wiring, err := opts.Prepare(deps)
	if err != nil {
		logger.Error("服务装配失败", zap.Error(err))
		os.Exit(1)
	}

	engine := newEngine(cfg, logger, db, redisConn)

	publicGroup := engine.Group(cfg.Server.RouterPrefix)
	privateGroup := engine.Group(cfg.Server.RouterPrefix)
	privateGroup.Use(auth.JWTAuth(cfg, wiring.Blacklist))
	if wiring.Routes != nil {
		wiring.Routes(publicGroup, privateGroup)
	}

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: engine,
		// 大文件上传链路：与 handler context、nginx 超时统一为 1 小时
		ReadTimeout:    3600 * time.Second,
		WriteTimeout:   3600 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	var grpcServer *grpc.Server
	var grpcLis net.Listener
	if wiring.GRPC != nil {
		// StatsHandler 为每个 RPC 自动创建 server span
		grpcServer = grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
		wiring.GRPC(grpcServer)
		grpcLis, err = net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.GRPCPort))
		if err != nil {
			logger.Error("gRPC 监听失败", zap.Error(err))
			os.Exit(1)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	if grpcServer != nil {
		go func() { errCh <- grpcServer.Serve(grpcLis) }()
	}
	logger.Info(opts.ServiceName+" started",
		zap.String("http", httpSrv.Addr), zap.String("grpc", grpcAddr(grpcLis)))

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case serveErr := <-errCh:
		if serveErr != nil {
			logger.Error("server exited", zap.Error(serveErr))
		}
	}

	// 停机顺序：先 HTTP（等在途请求）→ 再 gRPC → 再 OTel flush → 最后释放连接
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", zap.Error(err))
	}
	if grpcServer != nil {
		grpcDone := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(grpcDone)
		}()
		select {
		case <-grpcDone:
		case <-time.After(10 * time.Second):
			logger.Warn("grpc graceful stop timeout, forcing stop")
			grpcServer.Stop()
		}
	}

	// OTel 关闭要在连接释放之前：flush BatchSpanProcessor 中尚未发送的 span
	otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer otelCancel()
	if err := otelShutdown(otelCtx); err != nil {
		logger.Error("otel shutdown error", zap.Error(err))
	}

	if wiring.Close != nil {
		wiring.Close()
	}
	if err := producer.Close(); err != nil {
		logger.Error("关闭 Kafka Producer 失败", zap.Error(err))
	}
	if err := redisConn.Client.Close(); err != nil {
		logger.Error("关闭 Redis 失败", zap.Error(err))
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		_ = sqlDB.Close()
	}
	logger.Info(opts.ServiceName + " shutdown complete")
}

// applyDefaults 在对应环境变量未显式设置时套用本服务的默认库名与端口。
// 这样本地 `go run ./cmd/user` 不带任何变量也能和另外两个服务并存。
func applyDefaults(cfg *config.Config, opts Options) {
	if opts.DefaultDBName != "" && os.Getenv("APP_DATABASE_DBNAME") == "" {
		cfg.Database.DBName = opts.DefaultDBName
	}
	if opts.DefaultHTTPPort != 0 && os.Getenv("APP_SERVER_PORT") == "" {
		cfg.Server.Port = opts.DefaultHTTPPort
	}
	if opts.DefaultGRPCPort != 0 && os.Getenv("APP_SERVER_GRPC_PORT") == "" {
		cfg.Server.GRPCPort = opts.DefaultGRPCPort
	}
}

// newEngine 构造 gin 引擎并挂上通用中间件、健康检查与指标端点。
func newEngine(cfg *config.Config, logger *zap.Logger, db *gorm.DB, redisConn *redisx.RedisConn) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// otelx 中间件放在日志中间件之前：日志能拿到请求 ctx 中的 trace_id
	engine.Use(otelx.GinMiddleware(), httpx.GinLogger(logger), httpx.GinRecovery(logger))

	// 健康检查：探 MySQL 与 Redis，任一不可用返回 503，可直接作为 K8s 探针
	engine.GET("/health", func(c *gin.Context) {
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "mysql": dbErr.Error()})
			return
		}
		if pingErr := sqlDB.Ping(); pingErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "mysql": pingErr.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if pingErr := redisConn.Client.Ping(ctx).Err(); pingErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "redis": pingErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	// Prometheus 抓取端点，与 /health 同级（不挂业务路由前缀）
	engine.GET("/metrics", otelx.MetricsHandler())

	return engine
}

func grpcAddr(lis net.Listener) string {
	if lis == nil {
		return ""
	}
	return lis.Addr().String()
}
