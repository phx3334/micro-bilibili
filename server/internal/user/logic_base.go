package user

import (
	"github.com/mojocn/base64Captcha"
)

type BaseLogic struct {
	deps *LogicDeps
}

func NewBaseLogic(deps *LogicDeps) *BaseLogic {
	return &BaseLogic{deps: deps}
}

func (b *BaseLogic) GenerateCaptcha() (string, string, error) {
	cfg := b.deps.Config
	driver := base64Captcha.NewDriverDigit(
		cfg.Captcha.Height,
		cfg.Captcha.Width,
		cfg.Captcha.Length,
		cfg.Captcha.MaxSkew,
		cfg.Captcha.DotCount,
	)
	captcha := base64Captcha.NewCaptcha(driver, b.deps.CaptchaStore)
	id, b64s, _, err := captcha.Generate()
	return id, b64s, err
}
