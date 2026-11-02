package request

type HomeVideoList struct {
	CursorPage
	Zone string `json:"zone,omitempty" form:"zone"`
}

type VideoDetailReq struct {
	VideoID uint `form:"video_id" binding:"required"`
}

// VideoUploadReq 视频投稿上传请求（multipart/form-data 表单字段）
//
// 字段说明：
//   - Title：标题，必填，最长 255 字符（与 database.Video.Title 列保持一致）
//   - Description：视频简介，可选，最长 512 字符
//   - Zone：分区，可选，最长 64 字符（对应 Video.Zone gorm size:64）
//   - Tags：标签列表，由 logic 层 JSON 序列化后写入 Video.TagsJSON
type VideoUploadReq struct {
	Title       string   `json:"title" form:"title" binding:"required,max=255"`
	Description string   `json:"description" form:"description" binding:"max=512"`
	Zone        string   `json:"zone" form:"zone" binding:"max=64"`
	Tags        []string `json:"tags" form:"tags"`
}
