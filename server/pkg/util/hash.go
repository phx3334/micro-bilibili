package util

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

// BcryptHash 使用 bcrypt 对密码进行哈希。
// 返回 (hash, error)，调用方必须检查 error。
// 若忽略 error，bcrypt 内部失败时会返回空字符串，导致密码哈希为空，
// 用户将无法登录且存在安全隐患。
func BcryptHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash failed: %w", err)
	}
	return string(hash), nil
}

func BcryptCheck(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
