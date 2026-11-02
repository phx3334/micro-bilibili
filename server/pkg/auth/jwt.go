// Package auth 提供 JWT 签发/解析、Gin 读上下文工具与无状态鉴权中间件。
//
// 拆服务后 user-svc 负责签发与黑名单校验，video/interaction-svc 只需用公用的
// JWTAuth 校验 access token，并通过 refresh token 换发新的 access token。
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"fake_tiktok/internal/dto/request"
	"fake_tiktok/pkg/config"
	"fake_tiktok/pkg/util"
)

type JWT struct {
	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
}

var (
	TokenExpired     = errors.New("token is expired")
	TokenNotValidYet = errors.New("token is not valid yet")
	TokenInvalid     = errors.New("token is invalid")
	TokenMalformed   = errors.New("token is malformed")
)

func NewJWT(jwtConfig *config.JWTConfig) *JWT {
	return &JWT{
		AccessTokenSecret:  []byte(jwtConfig.AccessTokenSecret),
		RefreshTokenSecret: []byte(jwtConfig.RefreshTokenSecret),
	}
}

func (j *JWT) CreateAccessClaims(baseclaims request.BaseClaims, jwtConfig *config.JWTConfig) (request.JwtCustomClaims, error) {
	ep, err := util.ParseDuration(jwtConfig.AccessTokenExpiryTime)
	if err != nil {
		return request.JwtCustomClaims{}, err
	}
	return request.JwtCustomClaims{
		BaseClaims: baseclaims,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtConfig.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ep)),
			Audience:  jwt.ClaimStrings{"TAP"},
		},
	}, nil
}

// CreateRefreshClaims 构造 refresh token 的 claims。
//
// 只携带 UserID：刷新 access token 时无需回查账号库。
func (j *JWT) CreateRefreshClaims(baseclaims request.BaseClaims, jwtConfig *config.JWTConfig) (request.JwtCustomRefreshClaims, error) {
	ep, err := util.ParseDuration(jwtConfig.RefreshTokenExpiryTime)
	if err != nil {
		return request.JwtCustomRefreshClaims{}, err
	}
	return request.JwtCustomRefreshClaims{
		UserID: baseclaims.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtConfig.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ep)),
			Audience:  jwt.ClaimStrings{"TAP"},
		},
	}, nil
}

func (j *JWT) CreateAccessToken(claims request.JwtCustomClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.AccessTokenSecret)
}

func (j *JWT) CreateRefreshToken(claims request.JwtCustomRefreshClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.RefreshTokenSecret)
}

func (j *JWT) ParseAccessToken(token string) (*request.JwtCustomClaims, error) {
	claims, err := j.ParseToken(token, &request.JwtCustomClaims{}, j.AccessTokenSecret)
	if err != nil {
		return nil, err
	}
	if customclaims, ok := claims.(*request.JwtCustomClaims); ok {
		return customclaims, nil
	}
	return nil, TokenInvalid
}

func (j *JWT) ParseRefreshToken(token string) (*request.JwtCustomRefreshClaims, error) {
	claims, err := j.ParseToken(token, &request.JwtCustomRefreshClaims{}, j.RefreshTokenSecret)
	if err != nil {
		return nil, err
	}
	if customclaims, ok := claims.(*request.JwtCustomRefreshClaims); ok {
		return customclaims, nil
	}
	return nil, TokenInvalid
}

func (j *JWT) ParseToken(tokenStr string, claims jwt.Claims, secretkey []byte) (jwt.Claims, error) {
	// 使用不同的变量名避免冲突
	parsedToken, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return secretkey, nil
	})
	if err != nil {
		if ve, ok := err.(*jwt.ValidationError); ok {
			switch {
			case ve.Errors&jwt.ValidationErrorExpired != 0:
				return nil, TokenExpired
			case ve.Errors&jwt.ValidationErrorNotValidYet != 0:
				return nil, TokenNotValidYet
			case ve.Errors&jwt.ValidationErrorMalformed != 0:
				return nil, TokenMalformed
			default:
				return nil, TokenInvalid
			}
		}
		return nil, err
	}
	if !parsedToken.Valid {
		return nil, TokenInvalid
	}
	return parsedToken.Claims, nil
}
