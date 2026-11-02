package cache

// TranscodeMsg 视频转码消息体，由 API 发布到 video.transcode topic，
// worker 消费后执行 ffmpeg 转码并将结果回写到 videos 表。
//
// 字段说明：
//   - VideoID：已插入的 Video 记录 ID，worker 完成后据此 UPDATE
//   - SourceKey：源视频在对象存储中的 key（video/src/{id}.{ext}）
//   - SourceCoverKey：用户自带封面的对象 key；为空时由 worker 调用 ffmpeg 截帧生成 JPG
//   - UserID：作者 ID，用于日志追踪与权限校验
//
// 媒体文件不落本地磁盘，worker 下载到本机临时文件后转码，产物再写回对象存储。
type TranscodeMsg struct {
	VideoID        uint   `json:"video_id"`
	SourceKey      string `json:"source_key"`
	SourceCoverKey string `json:"source_cover_key"` // 可能为空，表示需要 ffmpeg 截帧生成封面
	UserID         string `json:"user_id"`
}
