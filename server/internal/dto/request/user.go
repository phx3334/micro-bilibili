package request

type Register struct {
	Username  string `json:"username" binding:"required,max=20"`
	Password  string `json:"password" binding:"required,min=6,max=15"`
	Captcha   string `json:"captcha" binding:"required,len=6"`
	CaptchaID string `json:"captcha_id" binding:"required"`
}

type Login struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required,min=6,max=15"`
	Captcha   string `json:"captcha" binding:"required,len=6"`
	CaptchaID string `json:"captcha_id" binding:"required"`
}

type UserCard struct {
	UserID string `json:"user_id" form:"user_id" binding:"required"`
	PageInfo
}

type UserChangeInfo struct {
	UserID    string `json:"-"`
	Username  string `json:"username" binding:"required,max=20"`
	Signature string `json:"signature" binding:"max=320"`
	Avatar    string `json:"avatar"`
	Birthday  string `json:"birthday"`
	Gender    string `json:"gender"`
}
