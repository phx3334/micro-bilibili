// Package log 提供跨服务复用的 zap 日志初始化能力。
//
// 拆服务后各进程（user/video/interaction-svc）都从同一份配置构造 logger，
// 保证日志字段、滚动策略和全局 logger 行为一致。
package log

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"fake_tiktok/pkg/config"
)

// InitZap 根据配置初始化 Zap 日志实例
// 支持 JSON/console 两种格式、日志滚动（基于 lumberjack）和 stdout 同步输出
// 同时调用 zap.ReplaceGlobals 替换全局 logger
func InitZap(cfg *config.ZapConfig) *zap.Logger {
	encoder := getEncoder(cfg.Format)

	// 输出目标：
	//   - 容器里日志主要靠 stdout 采集（Alloy/Filebeat），所以 IsConsolePrint=true 时始终写 stdout
	//   - 本地文件轮转保留给需要留存日志文件的场景；filename 留空则只写 stdout
	//   - 两个目标都关闭时兜底写 stdout，避免出现"日志静默丢失"的黑洞
	syncers := make([]zapcore.WriteSyncer, 0, 2)
	if cfg.Filename != "" {
		syncers = append(syncers, getLogWriter(cfg))
	}
	if cfg.IsConsolePrint || len(syncers) == 0 {
		syncers = append(syncers, zapcore.Lock(zapcore.AddSync(os.Stdout)))
	}

	core := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(syncers...), getLogLevel(cfg.Level))
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))

	// 替换全局 logger，使 zap.L() 也能使用
	zap.ReplaceGlobals(logger)
	return logger
}

// getEncoder 根据 format 创建编码器：
//   - json（默认）：结构化日志，便于日志平台按字段检索，也方便 Alloy 采集 stdout
//   - console：本地调试可读性更好
func getEncoder(format string) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "@timestamp"
	encoderConfig.LevelKey = "level"
	encoderConfig.NameKey = "logger"
	encoderConfig.CallerKey = "caller"
	encoderConfig.MessageKey = "msg"
	encoderConfig.StacktraceKey = "stacktrace"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	if strings.ToLower(format) == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		return zapcore.NewConsoleEncoder(encoderConfig)
	}
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	return zapcore.NewJSONEncoder(encoderConfig)
}

// getLogWriter 创建基于 lumberjack 的日志写入器
// 支持日志文件滚动：按大小切割、按数量保留、按天数清理、自动压缩
func getLogWriter(cfg *config.ZapConfig) zapcore.WriteSyncer {
	lumberJackWriter := &lumberjack.Logger{
		Filename:   cfg.Filename,   // 日志文件路径
		MaxSize:    cfg.MaxSize,    // 单个日志文件最大尺寸（MB）
		MaxBackups: cfg.MaxBackups, // 保留的旧日志文件最大数量
		MaxAge:     cfg.MaxAge,     // 保留旧日志文件的最大天数
		Compress:   cfg.Compress,   // 是否压缩旧日志文件
	}
	return zapcore.AddSync(lumberJackWriter)
}

// getLogLevel 将字符串日志级别转换为 zapcore.Level
func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
