// Package storagex 提供统一的 MinIO（S3 兼容）对象存储抽象。
//
// 媒体文件（视频/封面/头像）全部写入对象存储，数据库只保存对象 key
// （如 video/42.mp4 / cover/42.jpg / avatar/7.jpg），对外响应时再通过 URL
// 转换成可访问地址：
//   - MinioConfig.PublicRead=true：返回 {scheme}://{endpoint}/{bucket}/{key} 公开直链
//   - MinioConfig.PublicRead=false：返回 PresignedGetObject 预签名地址（TTL=URLExpiry）
//
// 约定：调用方传入的 key 使用正斜杠分隔（如 video/42.mp4），不做任何本地路径处理。
package storagex

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"fake_tiktok/pkg/config"
)

// Storage 对象存储抽象接口。
//
// 调用方约定：
//   - key 使用正斜杠分隔的相对路径，如 "video/42.mp4" / "cover/42.jpg" / "avatar/7.jpg"
//   - Put 的内容类型按 key 扩展名推断，调用方无需关心
//   - URL 返回可直接对外暴露的地址，可能是公开直链或预签名地址
type Storage interface {
	// Put 将 r 中的数据写入 key 对应对象；size 为数据字节数（S3 上传必需）。
	Put(ctx context.Context, key string, r io.Reader, size int64) error

	// Delete 删除 key 对应对象；对象不存在时返回 nil（幂等）。
	Delete(ctx context.Context, key string) error

	// URL 返回 key 对应的展示用地址；空 key 返回空字符串。
	URL(ctx context.Context, key string) (string, error)
}

// Downloader 是可选的读取能力，仅 MinIO 实现提供，供 worker 下载源文件。
type Downloader interface {
	Get(ctx context.Context, key string, w io.Writer) error
}

// minioStorage MinIO 对象存储实现。
type minioStorage struct {
	client     *minio.Client
	bucket     string
	endpoint   string
	useSSL     bool
	publicRead bool
	urlExpiry  time.Duration
}

// NewStorage 连接 MinIO 并确保 bucket 存在。
//
// 存储是本服务的核心依赖：连接/建桶失败直接返回错误，由调用方终止启动，
// 避免上传链路在运行期才暴露问题。
func NewStorage(cfg *config.MinioConfig) (Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("storage: minio config is nil")
	}
	endpoint := normalizeEndpoint(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("storage: minio endpoint is empty")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: minio bucket is empty")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage: minio access_key/secret_key is empty")
	}

	expiry := cfg.URLExpiry
	if expiry <= 0 {
		expiry = 24 * time.Hour
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: create minio client failed: %w", err)
	}
	s := &minioStorage{
		client:     client,
		bucket:     cfg.Bucket,
		endpoint:   endpoint,
		useSSL:     cfg.UseSSL,
		publicRead: cfg.PublicRead,
		urlExpiry:  expiry,
	}

	// 启动期做一次连通性探测并确保 bucket 存在；失败直接阻断启动。
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: check bucket %q failed: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("storage: create bucket %q failed: %w", cfg.Bucket, err)
		}
		log.Printf("storage: bucket %q created", cfg.Bucket)
	}

	// 公开直链模式尽力设置匿名只读策略；失败只记 warn，不阻断启动。
	if cfg.PublicRead {
		if err := client.SetBucketPolicy(ctx, cfg.Bucket, publicReadPolicy(cfg.Bucket)); err != nil {
			log.Printf("storage: set public-read bucket policy failed (bucket=%s): %v", cfg.Bucket, err)
		}
	}
	return s, nil
}

// Put 上传对象，内容类型按扩展名推断。
func (s *minioStorage) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if key == "" {
		return fmt.Errorf("storage: put key is empty")
	}
	contentType := contentTypeForKey(key)
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("storage: put object %q failed: %w", key, err)
	}
	return nil
}

// Delete 删除对象；对象不存在视为成功。
func (s *minioStorage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err == nil {
		return nil
	}
	// MinIO 对不存在对象返回 NoSuchKey；删除语义要求幂等。
	if resp := minio.ToErrorResponse(err); resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket" {
		return nil
	}
	return fmt.Errorf("storage: delete object %q failed: %w", key, err)
}

// URL 返回对象展示地址：公开直链或预签名 GET。
func (s *minioStorage) URL(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}

	if s.publicRead {
		scheme := "http"
		if s.useSSL {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s/%s/%s", scheme, s.endpoint, s.bucket, key), nil
	}

	presigned, err := s.client.PresignedGetObject(ctx, s.bucket, key, s.urlExpiry, nil)
	if err != nil {
		return "", fmt.Errorf("storage: presign object %q failed: %w", key, err)
	}
	return presigned.String(), nil
}

// Get 下载对象到 w（供 worker 读取转码源文件）。
func (s *minioStorage) Get(ctx context.Context, key string, w io.Writer) error {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("storage: get object %q failed: %w", key, err)
	}
	defer obj.Close()

	if _, err := obj.Stat(); err != nil {
		return fmt.Errorf("storage: stat object %q failed: %w", key, err)
	}
	if _, err := io.Copy(w, obj); err != nil {
		return fmt.Errorf("storage: download object %q failed: %w", key, err)
	}
	return nil
}

// Download 通过可选 Downloader 能力下载对象；非 MinIO 实现返回错误。
func Download(ctx context.Context, s Storage, key string, w io.Writer) error {
	if key == "" {
		return fmt.Errorf("storage: download key is empty")
	}
	d, ok := s.(Downloader)
	if !ok {
		return fmt.Errorf("storage: storage implementation does not support download")
	}
	return d.Get(ctx, key, w)
}

// KeyFromURL 从对象地址反推 key，用于把外部传入/缓存的 URL 归一化为 key。
// bucket 为空或 URL 不匹配该 bucket 时返回空字符串；已是 key 的输入原样返回。
func KeyFromURL(rawURL, bucket string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		if strings.HasPrefix(rawURL, "/") {
			return ""
		}
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	if bucket == "" {
		return ""
	}
	if !strings.HasPrefix(path, bucket+"/") {
		return ""
	}
	return strings.TrimPrefix(path, bucket+"/")
}

// normalizeEndpoint 去掉 endpoint 中可能携带的 scheme 与尾部斜杠。
func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	return strings.TrimSuffix(endpoint, "/")
}

// contentTypeForKey 按对象 key 的扩展名推断 Content-Type。
func contentTypeForKey(key string) string {
	if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// publicReadPolicy 生成允许匿名 GetObject 的 bucket policy。
func publicReadPolicy(bucket string) string {
	return fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, bucket)
}
