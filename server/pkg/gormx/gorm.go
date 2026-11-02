// Package gormx 封装 GORM(MySQL) 连接池初始化。
//
// 连接池超时等参数属于"基础设施约定"，拆服务后各服务都复用同一份实现，
// 避免各自抄一遍导致参数漂移。
package gormx

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"fake_tiktok/pkg/config"
)

func InitGorm(cfg *config.Config) *gorm.DB {
	dbCfg := cfg.Database
	// 修复：使用 url.QueryEscape 转义用户名和密码中的特殊字符（如 @、:、/ 等），
	// 防止密码包含特殊字符时 DSN 解析错误导致连接失败
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		url.QueryEscape(dbCfg.User), url.QueryEscape(dbCfg.Password), dbCfg.Host, dbCfg.Port, dbCfg.DBName,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		zap.L().Error("连接数据库失败", zap.Error(err))
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		zap.L().Error("获取数据库连接失败", zap.Error(err))
		os.Exit(1)
	}
	sqlDB.SetMaxIdleConns(dbCfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(dbCfg.MaxOpenConns)

	// ---- 连接池超时控制（行业标准） ----
	// ConnMaxLifetime：连接最大存活时间，超过后连接会被关闭并从池中移除。
	//   - MySQL 服务端默认 wait_timeout=28800s(8h)，连接池中超过该时间的连接
	//     已被服务端静默关闭，客户端使用时会报 "driver: bad connection"。
	//   - 行业标准建议设为 5~30 分钟，远小于 wait_timeout，确保连接在被服务端
	//     关闭之前就被池主动回收重建。
	//   - 配置值为 0 时使用默认值 10 分钟（兼顾安全性和连接复用效率）。
	connMaxLifetime := 10 * time.Minute
	if dbCfg.ConnMaxLifetime > 0 {
		connMaxLifetime = time.Duration(dbCfg.ConnMaxLifetime) * time.Second
	}
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	// ConnMaxIdleTime：连接最大空闲时间，超过后空闲连接会被关闭。
	//   - 与 ConnMaxLifetime 的区别：ConnMaxLifetime 是连接从创建到销毁的总寿命，
	//     而 ConnMaxIdleTime 只计算连接处于空闲状态的时间。
	//   - 行业标准建议设为 ConnMaxLifetime 的 1/2 ~ 1/3，在保持连接复用效率的同时
	//     避免空闲连接长期占用数据库服务端的连接槽位。
	//   - 配置值为 0 时使用默认值 5 分钟。
	connMaxIdleTime := 5 * time.Minute
	if dbCfg.ConnMaxIdleTime > 0 {
		connMaxIdleTime = time.Duration(dbCfg.ConnMaxIdleTime) * time.Second
	}
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	// MySQL 自动埋点：每条 SQL 生成子 span，在 Tempo 中可直接看到慢查询落在哪一步。
	// 用 WithoutMetrics 只保留 trace：DB 指标交给业务 RED 指标 + Prometheus 观察，
	// 避免每张表/每种语句都生成一套指标，撑爆 registry 与抓取体积。
	// 挂载失败只告警：观测能力缺失不应阻断服务启动。
	if cfg.OTEL.Enabled {
		if err := db.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
			zap.L().Warn("GORM OpenTelemetry 插件初始化失败，SQL 将不产生 span", zap.Error(err))
		}
	}

	return db
}
