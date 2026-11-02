package video

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fake_tiktok/internal/dto/cache"
	"fake_tiktok/internal/dto/request"
	"fake_tiktok/internal/model"
	"fake_tiktok/pkg/httpx"
	"fake_tiktok/pkg/otelx"
	"fake_tiktok/pkg/storagex"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// allowedVideoExt 视频上传扩展名白名单（小写形式）。
var allowedVideoExt = map[string]bool{
	".mp4": true,
	".mov": true,
	".avi": true,
	".mkv": true,
	".flv": true,
}

// allowedCoverExt 封面上传扩展名白名单（小写形式）。
var allowedCoverExt = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// VideoUploadLogic 视频投稿上传业务编排层
type VideoUploadLogic struct {
	deps    *LogicDeps
	storage storagex.Storage
}

// NewVideoUploadLogic 构造一个 VideoUploadLogic 实例。
func NewVideoUploadLogic(deps *LogicDeps, storage storagex.Storage) *VideoUploadLogic {
	return &VideoUploadLogic{deps: deps, storage: storage}
}

// Upload 接收视频投稿：源文件写入 MinIO → 落库（status=transcoding）→ 发布转码任务。
//
// 流程：
//  1. 校验大小/扩展名
//  2. 生成与数据库无关的对象 key，先上传源文件与可选封面
//  3. 插入 status=transcoding 记录（携带源 key，供 worker 下载与失败重试）
//  4. 投递 video.transcode
//
// 上传或落库失败都会清理已上传对象，避免悬挂对象。
func (l *VideoUploadLogic) Upload(ctx context.Context, userID string, file *multipart.FileHeader, cover *multipart.FileHeader, meta request.VideoUploadReq) (uint, error) {
	// 手动埋点：上传是视频异步链路的起点，traceparent 会随转码消息投递给 worker，
	// 从而把"HTTP 上传 → Kafka → worker 转码"串成一条 trace。
	ctx, span := otelx.Tracer("video-svc").Start(ctx, "video.upload")
	defer span.End()
	span.SetAttributes(attribute.String("user_id", userID))
	if file != nil {
		span.SetAttributes(attribute.Int64("file_size", file.Size))
	}

	// 1. 校验视频文件大小与扩展名
	if file.Size > l.deps.Config.Upload.MaxFileSize {
		return 0, errors.New("文件超过大小限制")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedVideoExt[ext] {
		return 0, errors.New("不支持的视频格式")
	}

	// 封面扩展名先校验，避免上传后再失败
	var coverExt string
	if cover != nil {
		coverExt = strings.ToLower(filepath.Ext(cover.Filename))
		if !allowedCoverExt[coverExt] {
			return 0, errors.New("不支持的封面格式")
		}
	}

	// 2. tags → JSON
	var tagsJSON string
	if len(meta.Tags) > 0 {
		body, err := json.Marshal(meta.Tags)
		if err != nil {
			return 0, fmt.Errorf("保存文件失败: %w", err)
		}
		tagsJSON = string(body)
	}

	// 3. 先上传对象（key 由随机串生成，不依赖自增 ID）
	id := randomObjectID()
	sourceKey := fmt.Sprintf("video/src/%s%s", id, ext)
	if err := l.putUpload(ctx, sourceKey, file); err != nil {
		return 0, fmt.Errorf("保存文件失败: %w", err)
	}

	var coverKey string
	if cover != nil {
		coverKey = fmt.Sprintf("cover/src/%s%s", id, coverExt)
		if err := l.putUpload(ctx, coverKey, cover); err != nil {
			_ = l.storage.Delete(ctx, sourceKey)
			return 0, fmt.Errorf("保存文件失败: %w", err)
		}
	}

	// 4. 落库：源 key 持久化，便于 worker 下载与人工重发
	video := &model.Video{
		Status:         "transcoding",
		AuthorID:       userID,
		Title:          meta.Title,
		Description:    meta.Description,
		Zone:           meta.Zone,
		TagsJSON:       tagsJSON,
		SourceKey:      sourceKey,
		SourceCoverKey: coverKey,
		// PlayURL/CoverURL 留空，由 worker 转码完成后回写对象 key
	}
	if err := l.deps.VideoRepo.Create(ctx, video); err != nil {
		_ = l.storage.Delete(ctx, sourceKey)
		if coverKey != "" {
			_ = l.storage.Delete(ctx, coverKey)
		}
		return 0, fmt.Errorf("数据库写入失败: %w", err)
	}

	// 5. 发布转码消息
	// PublishBuffer 内部会入缓冲并自动重放，这里拿到的 err 只是"未能直发"的信号，
	// 不构成致命错误；记录中已保存源 key，可人工重发。
	msg := cache.TranscodeMsg{
		VideoID:        video.ID,
		SourceKey:      sourceKey,
		SourceCoverKey: coverKey,
		UserID:         userID,
	}
	if err := l.deps.TranscodePublisher.Publish(ctx, msg); err != nil {
		if l.deps.Logger != nil {
			// 关键路径日志带 trace_id，便于从 Tempo 直接跳到对应日志
			otelx.Logger(l.deps.Logger, ctx).Warn("transcode publish buffered or failed",
				zap.Uint("video_id", video.ID),
				zap.Error(err))
		}
	}

	return video.ID, nil
}

// GetTranscodeStatus 查询视频转码状态，供前端轮询。
func (l *VideoUploadLogic) GetTranscodeStatus(ctx context.Context, userID string, videoID uint) (httpx.VideoStatusResp, error) {
	video, err := l.deps.VideoRepo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.VideoStatusResp{}, errors.New("视频不存在")
		}
		return httpx.VideoStatusResp{}, fmt.Errorf("查询失败: %w", err)
	}

	if video.AuthorID != userID {
		return httpx.VideoStatusResp{}, errors.New("无权限查看")
	}

	// DB 存的是对象 key，响应时转换成可访问地址
	videoURL, _ := l.storage.URL(ctx, video.PlayURL)
	coverURL, _ := l.storage.URL(ctx, video.CoverURL)

	return httpx.VideoStatusResp{
		Status:     video.Status,
		FailReason: video.FailReason,
		VideoURL:   videoURL,
		CoverURL:   coverURL,
	}, nil
}

// putUpload 打开 multipart 文件并写入对象存储。
func (l *VideoUploadLogic) putUpload(ctx context.Context, key string, file *multipart.FileHeader) error {
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()
	return l.storage.Put(ctx, key, src, file.Size)
}

// randomObjectID 生成 16 字节随机 hex（32 字符），作为对象 key 的唯一后缀。
func randomObjectID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败属于系统级异常，退回纳秒时间戳保证 key 仍可用
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}
