// services/iam/app/user_auth.go
package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yuliusw/RPA-market/common/response"

	// 请根据你的实际路径调整以下 import
	minioPkg "github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/utils"
	"github.com/yuliusw/RPA-market/services/iam/domain"
)

var (
	userRepo    domain.UserRepository
	minioClient *minioPkg.MinioClient // 引入 Minio 客户端依赖
)

var allowedAvatarFileTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// InitUser 初始化依赖注入
func InitUser(userRepoIN domain.UserRepository, minioIN *minioPkg.MinioClient) {
	userRepo = userRepoIN
	minioClient = minioIN
}

// =======================
// 请求结构体定义
// =======================

type RegisterReq struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type UpdateProfileReq struct {
	Username string `json:"username,omitempty" binding:"omitempty,min=3,max=50"`
	Email    string `json:"email,omitempty" binding:"omitempty,email"`
}

type UpdatePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// =======================
// 接口 Handler 实现
// =======================

// Register 注册接口
func Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	// 调用领域工厂：内部处理了 UUID 生成和密码哈希
	user, err := domain.NewUser(req.Username, req.Email, req.Password)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "USER_CREATE_FAILED", "Failed to create user entity")
		return
	}
	// 通过仓储持久化
	if err := userRepo.Save(user); err != nil {
		response.Error(c, http.StatusConflict, "USER_ALREADY_EXISTS", "Username or Email already exists")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully",
		"user_id": user.UserID,
	})
}

// Login 登录接口
func Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := userRepo.FindByEmail(req.Email)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	if !user.CheckPassword(req.Password) {
		response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	// 1. 生成短时 Access Token（30min，服务端验签）
	token, err := utils.GenerateToken(user.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "TOKEN_GENERATION_FAILED", "Failed to generate token")
		return
	}

	// 2. 生成长时 Session ID（7d，Redis + Cookie 承载自动登录与顶号判定）
	sessionID := utils.GenerateSessionID()

	// 3. Redis 存当前 sessionID（白名单 + 顶号依据），跟随 Session 长时过期
	if err := userRepo.SetSession(c.Request.Context(), user.UserID, sessionID, utils.SessionExpiry); err != nil {
		log.Printf("Redis session store failed: %v", err)
		response.Error(c, http.StatusServiceUnavailable, "SESSION_CREATE_FAILED", "Failed to create login session")
		return
	}

	// 4. 双 Cookie：auth_token（短 JWT，HttpOnly）+ session_id（长会话，HttpOnly）
	c.SetCookie("auth_token", token, int(utils.AccessTokenExpiry.Seconds()), "/", "", false, true)
	c.SetCookie("session_id", sessionID, int(utils.SessionExpiry.Seconds()), "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   token, // 移动端可直接用，Web 端亦可仅依赖 cookie
		"user_id": user.UserID,
	})
}

// GetProfile 检查登录状态并动态生成头像链接
func GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "NOT_LOGGED_IN", "Not logged in")
		return
	}

	user, err := userRepo.FindByID(userID.(string))
	if err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return
	}

	// 解析 Minio 头像 URL
	var avatarFullUrl string
	if user.AvatarURL != "" {
		// 每次访问 Profile 时，签发一个有效期 2 小时的临时访问链接
		avatarFullUrl, _ = minioClient.GetFileUrl(c.Request.Context(), user.AvatarURL, time.Hour*2)
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":    user.UserID,
		"username":   user.Username,
		"email":      user.Email,
		"avatar_url": avatarFullUrl, // 带有签名的 Minio 链接，前端可直接渲染
		"created_at": user.CreatedAt,
	})
}

// UpdateProfile 修改用户基础信息
func UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}

	var req UpdateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := userRepo.FindByID(userID.(string))
	if err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return
	}

	isUpdated := false
	if req.Username != "" && req.Username != user.Username {
		user.Username = req.Username
		isUpdated = true
	}
	if req.Email != "" && req.Email != user.Email {
		user.Email = req.Email
		isUpdated = true
	}

	if isUpdated {
		if err := userRepo.Save(user); err != nil {
			response.Error(c, http.StatusInternalServerError, "PROFILE_UPDATE_FAILED", "Failed to update profile or conflict data")
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user": gin.H{
			"username": user.Username,
			"email":    user.Email,
		},
	})
}

// UploadAvatar 上传并修改用户头像
func UploadAvatar(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}

	// 1. 获取前端传来的文件 (表单 key 为 "avatar")
	file, err := c.FormFile("avatar")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "AVATAR_REQUIRED", "Failed to get file from request")
		return
	}

	// 可选：限制文件大小，例如 5MB (5 * 1024 * 1024)
	if file.Size > 5<<20 {
		response.Error(c, http.StatusBadRequest, "FILE_TOO_LARGE", "File size exceeds limit (5MB)")
		return
	}
	if err := validateAvatarFilename(file.Filename); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_FILENAME", err.Error())
		return
	}

	// 2. 将文件暂存到本地临时目录
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if _, ok := allowedAvatarFileTypes[ext]; !ok {
		response.Error(c, http.StatusBadRequest, "UNSUPPORTED_AVATAR_TYPE", "Unsupported avatar file type")
		return
	}
	uploadID := uuid.NewString()
	tempFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_%s%s", userID.(string), uploadID, ext))
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		response.Error(c, http.StatusInternalServerError, "TEMP_SAVE_FAILED", "Failed to save temp file")
		return
	}
	defer os.Remove(tempFilePath) // 函数结束时清理本地临时文件
	contentType, err := detectAvatarContentType(tempFilePath, ext)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "AVATAR_TYPE_MISMATCH", err.Error())
		return
	}

	// 3. 查询用户
	user, err := userRepo.FindByID(userID.(string))
	if err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return
	}

	// 4. 定义 Minio 中的对象路径
	objectName := fmt.Sprintf("avatars/%s/%s%s", userID.(string), uploadID, ext)
	ctx := c.Request.Context()

	// 5. 上传到 Minio
	if err := minioClient.UploadFile(ctx, objectName, tempFilePath, contentType); err != nil {
		response.Error(c, http.StatusInternalServerError, "AVATAR_UPLOAD_FAILED", "Failed to upload file to Minio")
		return
	}

	// 6. 清理旧头像以节省空间
	if user.AvatarURL != "" {
		_ = minioClient.RemoveFile(ctx, user.AvatarURL)
	}

	// 7. 更新数据库字段 (只存 ObjectName)
	user.AvatarURL = objectName
	if err := userRepo.Save(user); err != nil {
		response.Error(c, http.StatusInternalServerError, "AVATAR_UPDATE_FAILED", "Failed to update database")
		return
	}

	// 8. 返回立即可用的预览链接
	previewUrl, _ := minioClient.GetFileUrl(ctx, objectName, time.Hour*24)

	c.JSON(http.StatusOK, gin.H{
		"message":    "Avatar uploaded successfully",
		"avatar_url": previewUrl,
	})
}

func validateAvatarFilename(filename string) error {
	base := filepath.Base(filename)
	if base == "." || strings.TrimSpace(base) == "" || base != filename || strings.ContainsAny(filename, `/\\`) || strings.Contains(filename, "\x00") {
		return errors.New("invalid filename")
	}
	return nil
}

func detectAvatarContentType(filePath, ext string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	detected := http.DetectContentType(buf[:n])
	allowed := allowedAvatarFileTypes[ext]
	if detected == allowed {
		return allowed, nil
	}
	return "", fmt.Errorf("file content type %s does not match extension %s", detected, ext)
}

// UpdatePassword 修改密码
func UpdatePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}

	var req UpdatePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := userRepo.FindByID(userID.(string))
	if err != nil {
		response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return
	}

	// 校验旧密码
	if !user.CheckPassword(req.OldPassword) {
		response.Error(c, http.StatusForbidden, "INCORRECT_OLD_PASSWORD", "Incorrect old password")
		return
	}

	// 哈希新密码并保存
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "Failed to process new password")
		return
	}
	user.PasswordHash = hashedPassword

	if err := userRepo.Save(user); err != nil {
		response.Error(c, http.StatusInternalServerError, "PASSWORD_UPDATE_FAILED", "Failed to update password")
		return
	}

	// 修改密码后强制重新登录：清理 Redis Session 和 Cookie
	_ = userRepo.DeleteSession(c.Request.Context(), user.UserID)
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	c.SetCookie("session_id", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully. Please log in again."})
}

// Logout 退出登录
func Logout(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists || userID == "" {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized or already logged out")
		return
	}

	currentSession, err := c.Cookie("session_id")
	if err == nil {
		cachedSession, _ := userRepo.GetSession(c.Request.Context(), userID.(string))
		if cachedSession == currentSession {
			_ = userRepo.DeleteSession(c.Request.Context(), userID.(string))
		}
	} else {
		_ = userRepo.DeleteSession(c.Request.Context(), userID.(string))
	}

	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	c.SetCookie("session_id", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
