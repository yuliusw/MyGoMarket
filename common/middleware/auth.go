// common/middleware/auth.go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/response"
	"github.com/yuliusw/RPA-market/common/utils"
	"github.com/yuliusw/RPA-market/services/iam/domain"
)

const ContextUserIDKey = "user_id"

var userRepo domain.UserRepository

func InitJWTAuth(repo domain.UserRepository) {
	userRepo = repo
}

// JWTAuth 鉴权中间件：短时 JWT 服务端验签 + 长时 Session 自动登录
//
// Cookie 约定：
//   - auth_token:  短时 JWT（30min），用于无状态验签
//   - session_id:  长时会话凭证（7d），存 Redis 做自动登录与顶号判定
//
// 鉴权流程：
//  1. 取 JWT（Header Bearer 或 auth_token cookie）
//  2. 严格 ParseToken：
//     - 有效 → 校验 Redis Session 与 cookie session_id 一致（顶号），通过则放行
//     - 过期 → 用 ParseTokenIgnoreExpiry 提取 UserID，再校验 Session，命中则续签新 JWT 并刷新 Session TTL
//  3. Session 不存在 / 不一致 → 视为登出或顶号，返回 401
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, fromAuthorizationHeader := extractJWT(c)
		if tokenString == "" {
			abortWithJSON(c, "Authorization token is required")
			return
		}

		sessionCookie, _ := c.Cookie("session_id")

		// 1. 严格校验 JWT（含过期）
		claims, err := utils.ParseToken(tokenString)
		if err == nil && claims.UserID != "" {
			// Cookie 登录保持 Session 顶号校验；纯 Bearer API 客户端允许只携带有效 JWT。
			if !fromAuthorizationHeader && !checkSession(c, claims.UserID, sessionCookie) {
				abortWithJSON(c, "Account logged in from another device")
				return
			}
			c.Set(ContextUserIDKey, claims.UserID)
			c.Next()
			return
		}

		// 2. JWT 过期（或不可严格校验），尝试提取 UserID 做续签
		claims, ignoreErr := utils.ParseTokenIgnoreExpiry(tokenString)
		if ignoreErr != nil || claims.UserID == "" {
			abortWithJSON(c, "Invalid or expired token")
			return
		}

		// 3. 校验长时 Session：Redis 中存在且与 cookie 一致才允许续签
		cachedSession, sessErr := userRepo.GetSession(c.Request.Context(), claims.UserID)
		if sessErr != nil || cachedSession == "" || cachedSession != sessionCookie {
			abortWithJSON(c, "Session expired, please log in again")
			return
		}

		// 4. 续签：签发新 JWT 并写回 cookie，同时滑动刷新 Redis Session 过期
		newToken, genErr := utils.GenerateToken(claims.UserID)
		if genErr != nil {
			abortWithJSON(c, "Failed to refresh token")
			return
		}
		c.SetCookie("auth_token", newToken, int(utils.AccessTokenExpiry.Seconds()), "/", "", false, true)

		if refreshErr := userRepo.SetSession(
			c.Request.Context(), claims.UserID, cachedSession, utils.SessionExpiry,
		); refreshErr != nil {
			// 续签失败不阻塞本次请求（JWT 已发），仅记录
			_ = refreshErr
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Next()
	}
}

// extractJWT 从 Authorization: Bearer 或 auth_token cookie 取 JWT
func extractJWT(c *gin.Context) (string, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1], true
		}
	}
	if cookieToken, err := c.Cookie("auth_token"); err == nil && cookieToken != "" {
		return cookieToken, false
	}
	return "", false
}

// checkSession 校验 Redis 中的 session 与 cookie 携带的 session_id 是否一致（顶号判定）
func checkSession(c *gin.Context, userID, sessionCookie string) bool {
	cached, err := userRepo.GetSession(c.Request.Context(), userID)
	if err != nil || cached == "" {
		return false
	}
	return cached == sessionCookie
}

// abortWithJSON 辅助函数：统一错误响应
func abortWithJSON(c *gin.Context, msg string) {
	response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", msg)
	c.Abort()
}

// GetUserID 供 Controller 调用，安全获取 UserID
func GetUserID(c *gin.Context) string {
	val, _ := c.Get(ContextUserIDKey)
	id, _ := val.(string)
	return id
}
