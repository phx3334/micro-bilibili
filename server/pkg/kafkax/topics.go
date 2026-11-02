package kafkax

// Kafka topic 名统一使用点号风格，与 Redis key 的冒号风格区分开。
// 集中定义避免发布方与消费方各写一份字符串导致拼写漂移。
const (
	// TopicVideoTranscode 视频转码任务，worker 消费后调用 ffmpeg 转码
	TopicVideoTranscode = "video.transcode"
	// TopicVideoPlaycount 视频播放量增量
	TopicVideoPlaycount = "video.playcount"
	// TopicVideoLikecount 视频点赞数增量（interaction-svc 发布，worker 回刷 likes_count）
	TopicVideoLikecount = "video.likecount"
	// TopicVideoCommentcount 视频评论数增量（interaction-svc 发布，worker 回刷 comments_count）
	// 评论数存于 video 库，interaction 不能直接写，只能发增量由 worker 回刷
	TopicVideoCommentcount = "video.commentcount"
)
