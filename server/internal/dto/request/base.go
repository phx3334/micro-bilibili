package request

type PageInfo struct {
	Page     int `json:"page" form:"page"`
	PageSize int `json:"page_size" form:"page_size"`
}

type CursorPage struct {
	Limit  int    `json:"limit" form:"limit"`
	Cursor string `json:"cursor" form:"cursor"`
}
