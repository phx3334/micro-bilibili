package video

import (
	"fake_tiktok/internal/dto/request"
)

// 游标分页选项（列表统一用游标翻页，offset 分页已随旧列表接口一并移除）
type CursorField struct {
	Column    string
	Direction string
}

type CursorOption struct {
	request.CursorPage
	Fields []CursorField
}
