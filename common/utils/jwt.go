// common/utils/jwt.go
package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var JWTSecret = []byte("your_super_secret_key_rpa_market") // 实际项目中应从配置文件读取

// 令牌时效：短时 JWT 由服务无状态验签，长时 Session 由 Redis+Cookie 承载自动登录
const (
	AccessTokenExpiry = 30 * time.Minute   // Access Token：服务端验证用
	SessionExpiry     = 7 * 24 * time.Hour // Session：Redis+Cookie 自动登录有效期
)

type CustomClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken 生成短时 Access Token
func GenerateToken(userID string) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "rpa-market-iam",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JWTSecret)
}

// ParseToken 严格解析并校验 Token（含过期校验）
func ParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return JWTSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// ParseTokenIgnoreExpiry 解析 Token 但跳过过期校验（仍校验签名），
// 用于中间件在 JWT 过期后提取 UserID，配合 Redis Session 判断是否续签。
func ParseTokenIgnoreExpiry(tokenString string) (*CustomClaims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, err := parser.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return JWTSecret, nil
	})
	if err != nil {
		// 签名错误等其他错误仍然拒绝
		return nil, err
	}
	if claims, ok := token.Claims.(*CustomClaims); ok {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// GenerateSessionID 生成不透明的会话标识，作为长时自动登录凭证，
// 存入 Redis（白名单/顶号判定）与 HttpOnly Cookie（浏览器自动携带）。
func GenerateSessionID() string {
	return uuid.NewString()
}
