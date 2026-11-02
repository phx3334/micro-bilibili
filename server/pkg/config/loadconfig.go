package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string `mapstructure:"host"`          // 服务监听主机地址
	Port         int    `mapstructure:"port"`          // 服务监听端口（HTTP）
	GRPCPort     int    `mapstructure:"grpc_port"`     // 服务监听端口（gRPC），user/video/interaction 各用一个
	SkipCaptcha  bool   `mapstructure:"skip_captcha"`  // 跳过图形验证码校验（内网/演示环境用，生产保持 false）
	RouterPrefix string `mapstructure:"router_prefix"` // 路由前缀
}

// GRPCConfig 拆分后各服务需要调用的下游 gRPC 地址。
//
// 客户端使用 grpc.NewClient 懒连接：启动不依赖下游存活，
// 真正发起调用时才建连，避免服务启动顺序耦合。
type GRPCConfig struct {
	UserAddr        string `mapstructure:"user_addr"`        // user-svc 地址，默认 localhost:9081
	VideoAddr       string `mapstructure:"video_addr"`       // video-svc 地址，默认 localhost:9082
	InteractionAddr string `mapstructure:"interaction_addr"` // interaction-svc 地址，默认 localhost:9083
}

// UploadConfig 上传配置
type UploadConfig struct {
	MaxFileSize int64 `mapstructure:"max_file_size"` // 最大文件大小（字节）
}

// TranscodeConfig 转码任务配置
//
// 转码任务走 Kafka topic（kafkax.TopicVideoTranscode）；并发度由 worker 副本数
// （K8s 副本 / KEDA）控制，单进程内只保留单次执行的超时兜底。
type TranscodeConfig struct {
	Timeout int `mapstructure:"timeout"` // 单次转码超时时间（秒），超时后杀死 ffmpeg 进程并标记失败
}

// MinioConfig MinIO（S3 兼容）对象存储配置。
// 媒体文件全部写入对象存储，数据库只保存对象 key。
type MinioConfig struct {
	Endpoint   string        `mapstructure:"endpoint"`    // 服务地址（host:port，不带 scheme）
	AccessKey  string        `mapstructure:"access_key"`  // AccessKey
	SecretKey  string        `mapstructure:"secret_key"`  // SecretKey
	Bucket     string        `mapstructure:"bucket"`      // 存储桶名称
	UseSSL     bool          `mapstructure:"use_ssl"`     // 是否使用 TLS
	PublicRead bool          `mapstructure:"public_read"` // 是否公开直链（true 匿名可读；false 预签名）
	URLExpiry  time.Duration `mapstructure:"url_expiry"`  // 预签名 URL 有效期
}

// FFmpegConfig ffmpeg 转码参数配置
type FFmpegConfig struct {
	BinaryPath  string `mapstructure:"binary_path"`  // ffmpeg 可执行文件路径，容器内默认 ffmpeg（在 PATH 中）
	OutputCodec string `mapstructure:"output_codec"` // 输出视频编码器，如 libx264
	CRF         int    `mapstructure:"crf"`          // 恒定码率因子（18~28，越小质量越好、文件越大）
	Preset      string `mapstructure:"preset"`       // 编码预设（ultrafast/fast/medium/slow 等）
}

// JWTConfig JWT 配置
type JWTConfig struct {
	AccessTokenSecret      string `mapstructure:"access_token_secret"`       // AccessToken 签名密钥
	RefreshTokenSecret     string `mapstructure:"refresh_token_secret"`      // RefreshToken 签名密钥
	AccessTokenExpiryTime  string `mapstructure:"access_token_expiry_time"`  // AccessToken 过期时间
	RefreshTokenExpiryTime string `mapstructure:"refresh_token_expiry_time"` // RefreshToken 过期时间
	Issuer                 string `mapstructure:"issuer"`                    // Token 签发者
}

// CaptchaConfig 验证码配置
type CaptchaConfig struct {
	Height   int     `mapstructure:"height"`    // 验证码图片高度
	Width    int     `mapstructure:"width"`     // 验证码图片宽度
	Length   int     `mapstructure:"length"`    // 验证码字符长度
	MaxSkew  float64 `mapstructure:"max_skew"`  // 最大倾斜度
	DotCount int     `mapstructure:"dot_count"` // 干扰点数量
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string `mapstructure:"host"`              // 数据库主机地址
	Port            int    `mapstructure:"port"`              // 数据库端口
	User            string `mapstructure:"user"`              // 数据库用户名
	Password        string `mapstructure:"password"`          // 数据库密码
	DBName          string `mapstructure:"dbname"`            // 数据库名称
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // 最大空闲连接数
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // 最大打开连接数
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // 连接最大存活时间（秒），超过后连接会被关闭重建
	// 行业标准：MySQL wait_timeout 通常为 8h(28800s)，建议设为 5~30 分钟
	// 防止连接池中使用已被服务端关闭的"过期连接"导致查询报错
	ConnMaxIdleTime int `mapstructure:"conn_max_idle_time"` // 连接最大空闲时间（秒），超过后空闲连接会被关闭
	// 行业标准：建议设为 ConnMaxLifetime 的 1/2 ~ 1/3
	// 避免空闲连接长期占用资源，同时减少新建连接的开销
	AutoMigrate bool `mapstructure:"auto_migrate"` // 是否启动时自动迁移
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host      string `mapstructure:"host"`       // Redis 主机地址
	Port      int    `mapstructure:"port"`       // Redis 端口
	Password  string `mapstructure:"password"`   // Redis 密码
	DB        int    `mapstructure:"db"`         // Redis 数据库编号
	KeyPrefix string `mapstructure:"key_prefix"` // Redis 键前缀
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"` // broker 地址列表，如 ["kafka:9092"]
}

// ZapConfig Zap 日志配置
type ZapConfig struct {
	Level          string `mapstructure:"level"`            // 日志级别（debug/info/warn/error）
	Format         string `mapstructure:"format"`           // 输出格式：json（默认，便于 ELK/Alloy 采集）或 console（本地调试更易读）
	Filename       string `mapstructure:"filename"`         // 日志文件路径，留空则只写 stdout
	MaxSize        int    `mapstructure:"max_size"`         // 单个日志文件最大尺寸（MB）
	MaxBackups     int    `mapstructure:"max_backups"`      // 保留的旧日志文件最大数量
	MaxAge         int    `mapstructure:"max_age"`          // 保留旧日志文件的最大天数
	Compress       bool   `mapstructure:"compress"`         // 是否压缩旧日志文件
	IsConsolePrint bool   `mapstructure:"is_console_print"` // 是否同时输出到控制台
}

// OTELConfig OpenTelemetry 可观测性配置
// 关闭时全链路 noop：不创建 exporter、不产生额外分配，本地无采集器也能正常启动。
type OTELConfig struct {
	Enabled     bool    `mapstructure:"enabled"`      // 总开关（APP_OTEL_ENABLED）
	Endpoint    string  `mapstructure:"endpoint"`     // OTLP gRPC 端点，如 otel-collector:4317
	SampleRatio float64 `mapstructure:"sample_ratio"` // Trace 采样率（0~1），生产可调低以控制成本
	ServiceName string  `mapstructure:"service_name"` // 服务名，由各 main 进程显式覆盖（如 user-svc）
	Environment string  `mapstructure:"environment"`  // 部署环境标识（deployment.environment），如 docker/prod
	Version     string  `mapstructure:"version"`      // 服务版本（service.version），便于按版本对比 Trace
}

// Config 应用总配置
// 聚合所有子配置模块
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`    // 服务器配置
	Upload    UploadConfig    `mapstructure:"upload"`    // 上传配置
	JWT       JWTConfig       `mapstructure:"jwt"`       // JWT 配置
	Captcha   CaptchaConfig   `mapstructure:"captcha"`   // 验证码配置
	Database  DatabaseConfig  `mapstructure:"database"`  // 数据库配置
	Redis     RedisConfig     `mapstructure:"redis"`     // Redis 配置
	Kafka     KafkaConfig     `mapstructure:"kafka"`     // Kafka 配置
	GRPC      GRPCConfig      `mapstructure:"grpc"`      // 下游 gRPC 地址配置（拆服务后跨域调用）
	Zap       ZapConfig       `mapstructure:"zap"`       // Zap 日志配置
	OTEL      OTELConfig      `mapstructure:"otel"`      // OpenTelemetry 可观测性配置
	Transcode TranscodeConfig `mapstructure:"transcode"` // 转码任务配置（队列名/并发数/超时）
	Minio     MinioConfig     `mapstructure:"minio"`     // MinIO 对象存储配置
	FFmpeg    FFmpegConfig    `mapstructure:"ffmpeg"`    // ffmpeg 转码参数配置

	// RunCron 控制本进程是否注册并启动 cron 任务（Task 5）。
	//   - 平铺在 Config 根，避免 "APP_APP_RUN_CRON" 这种重复前缀
	//   - YAML key：`run_cron`，环境变量：`APP_RUN_CRON`
	//   - API 进程保持 false（http 服务 + 业务写路径职责）
	//   - worker 进程通过 `APP_RUN_CRON=true` 覆盖，独自承接定时任务
	RunCron bool `mapstructure:"run_cron"`
}

// LoadConfig 加载应用配置
// 通过 Viper 从 YAML 配置文件、.env 文件和环境变量中读取配置
// 支持通过 CONFIG_NAME 环境变量选择不同的配置文件
func LoadConfig() (*Config, error) {
	// 1. 优先加载 .env 文件（不覆盖已存在的环境变量）
	//    这样 .env 中的值会被 viper.AutomaticEnv 读取到，但系统环境变量优先级更高
	if err := godotenv.Load(".env"); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("加载 .env 文件失败: %v", err)
		}
	} else {
		log.Println("成功加载 .env 文件")
	}

	cfg := &Config{}
	v := viper.New()

	// 通过环境变量选择配置文件
	configName := os.Getenv("CONFIG_NAME")
	if configName == "" {
		configName = v.GetString("CONFIG_NAME")
	}
	if configName == "" {
		configName = "config" // 默认统一配置（容器与本地共用）
	}

	log.Printf("正在加载配置文件: %s.yaml", configName)

	// 设置 Viper 配置
	v.SetConfigType("yaml")

	// 设置环境变量支持
	v.SetEnvPrefix("APP")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 读取配置文件内容并展开其中的 ${VAR} / ${VAR:-default} 环境变量占位符，
	// 使配置项统一由环境变量驱动（缺省时回退到 :- 后的默认值），
	// 然后再交由 viper 解析 YAML。
	configPath, found := findConfigFile(configName)
	if !found {
		log.Printf("配置文件未找到: %s.yaml，使用环境变量配置", configName)
	} else {
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		expanded := expandEnvPlaceholders(string(raw))
		if err := v.ReadConfig(bytes.NewReader([]byte(expanded))); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
		log.Printf("成功加载配置文件: %s", configPath)
	}

	// 将配置绑定到结构体
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// APP_KAFKA_BROKERS 约定为逗号分隔的地址列表，但 viper 的弱类型转换
	// 不会按逗号拆分字符串，这里显式归一化，避免多 broker 被当成一个地址
	if raw := strings.TrimSpace(os.Getenv("APP_KAFKA_BROKERS")); raw != "" {
		brokers := make([]string, 0, strings.Count(raw, ",")+1)
		for _, b := range strings.Split(raw, ",") {
			if b = strings.TrimSpace(b); b != "" {
				brokers = append(brokers, b)
			}
		}
		cfg.Kafka.Brokers = brokers
	}

	// 验证必要配置
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return cfg, nil
}

// findConfigFile 在候选目录中查找 {configName}.yaml，返回首个存在的绝对路径。
func findConfigFile(configName string) (string, bool) {
	candidates := []string{
		"./configs/" + configName + ".yaml",
		"/app/configs/" + configName + ".yaml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// expandEnvPlaceholders 展开字符串中的 ${VAR} 与 ${VAR:-default} 占位符。
//   - ${VAR}            → 环境变量 VAR 的值（未设置则为空）
//   - ${VAR:-default}   → 环境变量 VAR 的值，未设置或为空时回退到 default
//
// 使用 os.Expand 逐个匹配 ${...}，仅支持大括号形式，避免误伤 YAML 中的普通 $ 字符。
func expandEnvPlaceholders(s string) string {
	return os.Expand(s, func(key string) string {
		if idx := strings.Index(key, ":-"); idx >= 0 {
			name := key[:idx]
			def := key[idx+2:]
			if val := os.Getenv(name); val != "" {
				return val
			}
			return def
		}
		return os.Getenv(key)
	})
}

// validateConfig 验证配置的必要字段
func validateConfig(cfg *Config) error {
	if cfg.Server.Port <= 0 {
		return fmt.Errorf("服务器端口必须大于0")
	}
	// 拆成三个微服务后每个进程都对外提供 gRPC，端口缺失会让监听失败，启动期即报错更易定位
	if cfg.Server.GRPCPort <= 0 {
		return fmt.Errorf("gRPC 端口必须大于0")
	}
	if cfg.Database.Host == "" {
		return fmt.Errorf("数据库主机不能为空")
	}
	if cfg.Redis.Host == "" {
		return fmt.Errorf("Redis主机不能为空")
	}
	if len(cfg.Kafka.Brokers) == 0 {
		return fmt.Errorf("Kafka broker 地址不能为空")
	}
	// MinIO 配置不在全局校验里强制：interaction 不初始化存储，
	// 不能被无关配置卡住启动。凭证/桶名校验由 storagex.NewStorage 在初始化时执行。
	return nil
}
