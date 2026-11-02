package request

import (
	jwt "github.com/golang-jwt/jwt/v4"
)

type JwtCustomClaims struct {
	BaseClaims
	jwt.RegisteredClaims
}

type JwtCustomRefreshClaims struct {
	UserID string
	// 仅携带 UserID，刷新 access token 时无需回查账号库。
	jwt.RegisteredClaims
}

type BaseClaims struct {
	UserID string
}
