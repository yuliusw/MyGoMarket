// services/market/app/market_app.go
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/yuliusw/RPA-market/common/audit"
	"github.com/yuliusw/RPA-market/common/response"

	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/services/market/domain"
	"gorm.io/gorm"
)

var (
	appRepo     domain.AppRepository
	minioClient *database.MinioClient
	redisClient *redis.Client
)

var allowedAppFileTypes = map[string]string{
	".zip": "application/zip",
	".gz":  "application/gzip",
	".tgz": "application/gzip",
}

// InitMarket 初始化依赖注入
func InitMarket(repo domain.AppRepository, minioIN *database.MinioClient, redisIN *redis.Client) {
	appRepo = repo
	minioClient = minioIN
	redisClient = redisIN
}

// =======================
// 请求结构体定义
// =======================

type PublishAppReq struct {
	Name     string   `form:"name" binding:"required"`
	Category string   `form:"category"`
	Tags     []string `form:"tags"`   // 前端表单传多个同名参数，例如 tags=工具&tags=效率
	Status   string   `form:"status"` // 可选，默认为 published
}

type UpdateAppReq struct {
	Name     string          `json:"name"`
	Category string          `json:"category"`
	Tags     []string        `json:"tags"`
	Metadata domain.Metadata `json:"metadata"`
	Status   string          `json:"status"`
}

type ListAppsReq struct {
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
	Keyword  string `form:"keyword"`
	Category string `form:"category"`
	Status   string `form:"status"` // 比如 "published", "off_shelved"
}

type RankingApp struct {
	App   *domain.App `json:"app"`
	Score int64       `json:"score"`
	Rank  int         `json:"rank"`
}

// =======================
// 接口 Handler 实现
// =======================

// PublishApp 发布应用 (包含文件上传和数据库记录创建)
// 注：权限拦截已在路由层 CasbinRequire("app:create") 完成
func PublishApp(c *gin.Context) {
	// 1. 获取当前登录开发者ID (由 JWT 中间件设置，这是业务逻辑需要的字段，必须保留)
	userIDStr, exists := c.Get("user_id")
	if !exists {
		errorJSON(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}
	developerID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		errorJSON(c, http.StatusUnauthorized, "INVALID_USER_ID", "Invalid User ID")
		return
	}
	traceID := response.TraceID(c)

	// 2. 绑定表单基础数据
	var req PublishAppReq
	if err := c.ShouldBind(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	// 3. 处理文件上传
	file, err := c.FormFile("app_file")
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "APP_FILE_REQUIRED", "App file is required")
		return
	}
	if err := validateUploadedFilename(file.Filename); err != nil {
		errorJSON(c, http.StatusBadRequest, "INVALID_FILENAME", err.Error())
		return
	}

	// 限制文件大小 (例: 100MB)
	if file.Size > 100<<20 {
		errorJSON(c, http.StatusBadRequest, "FILE_TOO_LARGE", "File size exceeds limit (100MB)")
		return
	}

	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(c.PostForm("idempotency_key"))
	}
	if idempotencyKey != "" {
		if len(idempotencyKey) > 128 {
			errorJSON(c, http.StatusBadRequest, "IDEMPOTENCY_KEY_TOO_LONG", "Idempotency-Key is too long")
			return
		}
		existing, err := appRepo.GetByDeveloperIDAndIdempotencyKey(c.Request.Context(), developerID, idempotencyKey)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"message": "App published successfully", "app_id": existing.AppID, "idempotent": true})
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logMarketAudit("publish_idempotency_lookup_failed", traceID, developerID, uuid.Nil, "", err)
			errorJSON(c, http.StatusInternalServerError, "IDEMPOTENCY_LOOKUP_FAILED", "Failed to check idempotency")
			return
		}
		unlock, locked, err := acquirePublishLock(c.Request.Context(), developerID, idempotencyKey)
		if err != nil {
			logMarketAudit("publish_lock_failed", traceID, developerID, uuid.Nil, "", err)
			errorJSON(c, http.StatusInternalServerError, "UPLOAD_LOCK_FAILED", "Failed to acquire upload lock")
			return
		}
		if !locked {
			errorJSON(c, http.StatusConflict, "DUPLICATE_UPLOAD_IN_PROGRESS", "Duplicate upload is in progress")
			return
		}
		defer unlock()
	}

	// 暂存到本地临时目录
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if _, ok := allowedAppFileTypes[ext]; !ok {
		errorJSON(c, http.StatusBadRequest, "UNSUPPORTED_FILE_TYPE", "Unsupported file type")
		return
	}
	tempFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("app_%s%s", uuid.NewString(), ext))
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		logMarketAudit("publish_temp_save_failed", traceID, developerID, uuid.Nil, "", err)
		errorJSON(c, http.StatusInternalServerError, "TEMP_SAVE_FAILED", "Failed to save file temporarily")
		return
	}
	defer os.Remove(tempFilePath) // 用完即删
	contentType, err := detectAllowedContentType(tempFilePath, ext)
	if err != nil {
		logMarketAudit("publish_file_type_rejected", traceID, developerID, uuid.Nil, "", err)
		errorJSON(c, http.StatusBadRequest, "FILE_TYPE_MISMATCH", err.Error())
		return
	}
	checksum, err := fileSHA256(tempFilePath)
	if err != nil {
		logMarketAudit("publish_checksum_failed", traceID, developerID, uuid.Nil, "", err)
		errorJSON(c, http.StatusInternalServerError, "CHECKSUM_FAILED", "Failed to calculate file checksum")
		return
	}

	// 4. 生成业务实体与 UUID
	appID := uuid.New()
	ctx := c.Request.Context()

	// Minio 对象路径只使用服务端生成的安全片段，原始文件名仅进入 metadata。
	objectName := fmt.Sprintf("apps/%s/%s/app%s", developerID.String(), appID.String(), ext)

	// 5. 上传到 Minio
	uploadInfo, err := minioClient.UploadFileInfo(ctx, objectName, tempFilePath, contentType)
	if err != nil {
		logMarketAudit("publish_minio_upload_failed", traceID, developerID, appID, objectName, err)
		errorJSON(c, http.StatusInternalServerError, "MINIO_UPLOAD_FAILED", "Failed to upload file to Minio")
		return
	}

	// 6. 数据装配与持久化
	status := "published"
	if req.Status != "" {
		status = req.Status
	}

	newApp := &domain.App{
		AppID:       appID,
		Name:        req.Name,
		DeveloperID: developerID,
		Category:    req.Category,
		Tags:        req.Tags,
		Status:      status,
		Metadata: domain.Metadata{
			"file_name":       filepath.Base(file.Filename),
			"object_name":     objectName,
			"file_size":       file.Size,
			"content_type":    contentType,
			"sha256":          checksum,
			"etag":            uploadInfo.ETag,
			"version":         "1.0.0",
			"idempotency_key": idempotencyKey,
		},
		CreateAt: time.Now(),
		UpdateAt: time.Now(),
	}

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := appRepo.Create(dbCtx, newApp); err != nil {
		if idempotencyKey != "" {
			lookupCtx, lookupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			existing, lookupErr := appRepo.GetByDeveloperIDAndIdempotencyKey(lookupCtx, developerID, idempotencyKey)
			lookupCancel()
			if lookupErr == nil {
				if removeErr := removeUploadedObject(objectName); removeErr != nil {
					logMarketAudit("publish_duplicate_compensation_failed", traceID, developerID, appID, objectName, removeErr)
				}
				logMarketAudit("publish_duplicate_reused", traceID, developerID, existing.AppID, objectName, err)
				c.JSON(http.StatusOK, gin.H{"message": "App published successfully", "app_id": existing.AppID, "idempotent": true})
				return
			}
		}
		// 补偿机制：写入 DB 失败则清理 Minio 垃圾文件
		if removeErr := removeUploadedObject(objectName); removeErr != nil {
			logMarketAudit("publish_db_failed_compensation_failed", traceID, developerID, appID, objectName, removeErr)
		}
		logMarketAudit("publish_db_create_failed", traceID, developerID, appID, objectName, err)
		errorJSON(c, http.StatusInternalServerError, "APP_CREATE_FAILED", "Failed to create app record in DB")
		return
	}
	logMarketAudit("publish_succeeded", traceID, developerID, appID, objectName, nil)

	c.JSON(http.StatusOK, gin.H{
		"message": "App published successfully",
		"app_id":  newApp.AppID,
	})
}

// UpdateApp 更新应用基础信息 (不含文件)
// 注：权限拦截已在路由层 CasbinRequire("app:edit") 完成
func UpdateApp(c *gin.Context) {
	appIDStr := c.Param("app_id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "INVALID_APP_ID", "Invalid App ID format")
		return
	}

	var req UpdateAppReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	ctx := c.Request.Context()
	developerID, ok := currentDeveloperID(c)
	if !ok {
		errorJSON(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}
	if !ensureAppOwner(c, ctx, appID, developerID) {
		return
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if len(req.Tags) > 0 {
		updates["tags"] = pq.StringArray(req.Tags)
	}
	if req.Metadata != nil {
		updates["metadata"] = req.Metadata
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	updates["update_at"] = time.Now()

	app, err := appRepo.UpdateFields(ctx, appID, updates)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "APP_UPDATE_FAILED", "Failed to update app")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "App updated successfully", "data": app})
}

// OffShelfApp 应用下架
// 注：权限拦截已在路由层 CasbinRequire("app:offshelf") 完成
func OffShelfApp(c *gin.Context) {
	appIDStr := c.Param("app_id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "INVALID_APP_ID", "Invalid App ID format")
		return
	}

	ctx := c.Request.Context()
	developerID, ok := currentDeveloperID(c)
	if !ok {
		errorJSON(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}
	if !ensureAppOwner(c, ctx, appID, developerID) {
		return
	}
	_, err = appRepo.UpdateFields(ctx, appID, map[string]interface{}{
		"status":    "off_shelved",
		"update_at": time.Now(),
	})
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "APP_OFFSHELF_FAILED", "Failed to off-shelf app")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "App is now off the shelf"})
}

// ListApps 分页获取应用列表
// 公开接口
func ListApps(c *gin.Context) {
	var req ListAppsReq
	if err := c.ShouldBindQuery(&req); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	// 默认只查询公开上架的应用，除非显式传了状态
	if req.Status == "" {
		req.Status = "published"
	}

	apps, total, err := appRepo.List(c.Request.Context(), req.Page, req.PageSize, req.Keyword, req.Category, req.Status)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "APP_LIST_FAILED", "Failed to query apps")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      apps,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetAppDetail 获取单条应用详情
// 公开接口
func GetAppDetail(c *gin.Context) {
	appIDStr := c.Param("app_id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "INVALID_APP_ID", "Invalid App ID format")
		return
	}

	app, err := appRepo.GetByID(c.Request.Context(), appID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "APP_NOT_FOUND", "App not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": app})
}

// DownloadApp 下载应用文件 (返回预签名直链)
// 公开接口
func DownloadApp(c *gin.Context) {
	appIDStr := c.Param("app_id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "INVALID_APP_ID", "Invalid App ID format")
		return
	}

	ctx := c.Request.Context()
	app, err := appRepo.GetByID(ctx, appID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "APP_NOT_FOUND", "App not found")
		return
	}
	if app.Status != "published" {
		errorJSON(c, http.StatusNotFound, "APP_NOT_FOUND", "App not found")
		return
	}

	// 从 Metadata 解析 Minio 对象路径
	objectNameObj, ok := app.Metadata["object_name"]
	if !ok || objectNameObj == "" {
		errorJSON(c, http.StatusInternalServerError, "APP_FILE_MISSING", "App file path not found")
		return
	}

	// 生成 5 分钟有效的预签名下载链接
	downloadUrl, err := minioClient.GetFileUrl(ctx, objectNameObj.(string), time.Minute*5)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "DOWNLOAD_URL_FAILED", "Failed to generate download link")
		return
	}
	if checksum, ok := app.Metadata["sha256"].(string); ok && checksum != "" {
		c.Header("X-App-SHA256", checksum)
	}
	if etag, ok := app.Metadata["etag"].(string); ok && etag != "" {
		c.Header("X-App-ETag", etag)
	}
	recordDownloadRank(ctx, appID.String())

	// 直接 307 重定向让浏览器开始下载
	c.Redirect(http.StatusTemporaryRedirect, downloadUrl)
}

func GetRankings(c *gin.Context) {
	rankingType := c.DefaultQuery("type", "daily")
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 || limit > 100 {
		errorJSON(c, http.StatusBadRequest, "INVALID_LIMIT", "Invalid limit")
		return
	}

	key, ok := rankingKey(rankingType, time.Now())
	if !ok {
		errorJSON(c, http.StatusBadRequest, "INVALID_RANKING_TYPE", "Invalid ranking type")
		return
	}
	if redisClient == nil {
		errorJSON(c, http.StatusServiceUnavailable, "RANKING_UNAVAILABLE", "Ranking service unavailable")
		return
	}

	items, err := redisClient.ZRevRangeWithScores(c.Request.Context(), key, 0, int64(limit-1)).Result()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "RANKING_QUERY_FAILED", "Failed to query rankings")
		return
	}

	ids := make([]uuid.UUID, 0, len(items))
	scores := make(map[uuid.UUID]int64, len(items))
	for _, item := range items {
		appID, err := uuid.Parse(fmt.Sprint(item.Member))
		if err != nil {
			continue
		}
		ids = append(ids, appID)
		scores[appID] = int64(item.Score)
	}

	apps, err := appRepo.GetByIDs(c.Request.Context(), ids)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "RANKING_APPS_LOAD_FAILED", "Failed to load ranking apps")
		return
	}
	appsByID := make(map[uuid.UUID]*domain.App, len(apps))
	for _, app := range apps {
		if app.Status == "published" {
			appsByID[app.AppID] = app
		}
	}

	rankings := make([]RankingApp, 0, len(ids))
	for _, id := range ids {
		app, exists := appsByID[id]
		if !exists {
			continue
		}
		rankings = append(rankings, RankingApp{
			App:   app,
			Score: scores[id],
			Rank:  len(rankings) + 1,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"type":  rankingType,
		"limit": limit,
		"data":  rankings,
	})
}

func recordDownloadRank(ctx context.Context, appID string) {
	now := time.Now()
	parsedAppID, parseErr := uuid.Parse(appID)
	if parseErr == nil {
		if err := appRepo.IncrementDownloadMetric(ctx, parsedAppID, now); err != nil {
			log.Printf("failed to persist download metric for app %s: %v", appID, err)
		}
	}
	if redisClient == nil {
		return
	}
	pipe := redisClient.Pipeline()
	if key, ok := rankingKey("daily", now); ok {
		pipe.ZIncrBy(ctx, key, 1, appID)
		pipe.Expire(ctx, key, 48*time.Hour)
	}
	if key, ok := rankingKey("weekly", now); ok {
		pipe.ZIncrBy(ctx, key, 1, appID)
		pipe.Expire(ctx, key, 15*24*time.Hour)
	}
	if key, ok := rankingKey("total", now); ok {
		pipe.ZIncrBy(ctx, key, 1, appID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("failed to record download ranking for app %s: %v", appID, err)
	}
}

func rankingKey(rankingType string, now time.Time) (string, bool) {
	switch rankingType {
	case "daily":
		return fmt.Sprintf("market:rank:downloads:daily:%s", now.Format("20060102")), true
	case "weekly":
		year, week := now.ISOWeek()
		return fmt.Sprintf("market:rank:downloads:weekly:%04d%02d", year, week), true
	case "total":
		return "market:rank:downloads:total", true
	default:
		return "", false
	}
}

// DeleteApp 彻底删除应用及其文件
// 注：权限拦截已在路由层 CasbinRequire("app:delete") 完成
func DeleteApp(c *gin.Context) {
	appIDStr := c.Param("app_id")
	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		errorJSON(c, http.StatusBadRequest, "INVALID_APP_ID", "Invalid App ID format")
		return
	}

	ctx := c.Request.Context()
	developerID, ok := currentDeveloperID(c)
	if !ok {
		errorJSON(c, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}
	app, err := appRepo.GetByID(ctx, appID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "APP_NOT_FOUND", "App not found")
		return
	}
	if app.DeveloperID != developerID {
		errorJSON(c, http.StatusForbidden, "FORBIDDEN", "Forbidden")
		return
	}

	var objectName string
	if objName, ok := app.Metadata["object_name"]; ok && objName != "" {
		objectName, _ = objName.(string)
	}

	if err := appRepo.Delete(ctx, appID); err != nil {
		errorJSON(c, http.StatusInternalServerError, "APP_DELETE_FAILED", "Failed to delete app record")
		return
	}
	if objectName != "" {
		if err := minioClient.RemoveFile(ctx, objectName); err != nil {
			logMarketAudit("delete_minio_remove_failed", getTraceID(c), developerID, appID, objectName, err)
			audit.RecordMinioDeleteRetry(context.Background(), database.DB, objectName, err.Error(), getTraceID(c))
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "App and files deleted successfully"})
}

func validateUploadedFilename(filename string) error {
	base := filepath.Base(filename)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" || !utf8.ValidString(base) {
		return errors.New("invalid filename")
	}
	if base != filename || strings.ContainsAny(filename, `/\\`) || strings.Contains(filename, "\x00") {
		return errors.New("filename must not contain path separators")
	}
	return nil
}

func detectAllowedContentType(filePath, ext string) (string, error) {
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
	allowed := allowedAppFileTypes[ext]
	if ext == ".tgz" {
		allowed = allowedAppFileTypes[".gz"]
	}
	if detected == allowed || (allowed == "application/gzip" && detected == "application/x-gzip") {
		return allowed, nil
	}
	if ext == ".zip" && detected == "application/octet-stream" {
		return allowed, nil
	}
	return "", fmt.Errorf("file content type %s does not match extension %s", detected, ext)
}

func acquirePublishLock(ctx context.Context, developerID uuid.UUID, idempotencyKey string) (func(), bool, error) {
	if redisClient == nil {
		return func() {}, true, nil
	}
	key := fmt.Sprintf("market:publish:lock:%s:%s", developerID.String(), idempotencyKey)
	locked, err := redisClient.SetNX(ctx, key, "1", 2*time.Minute).Result()
	if err != nil {
		return nil, false, err
	}
	return func() { _ = redisClient.Del(context.Background(), key).Err() }, locked, nil
}

func currentDeveloperID(c *gin.Context) (uuid.UUID, bool) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}
	developerID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return uuid.Nil, false
	}
	return developerID, true
}

func ensureAppOwner(c *gin.Context, ctx context.Context, appID, developerID uuid.UUID) bool {
	app, err := appRepo.GetByID(ctx, appID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "APP_NOT_FOUND", "App not found")
		return false
	}
	if app.DeveloperID != developerID {
		errorJSON(c, http.StatusForbidden, "FORBIDDEN", "Forbidden")
		return false
	}
	return true
}

func removeUploadedObject(objectName string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return minioClient.RemoveFile(cleanupCtx, objectName)
}

func getTraceID(c *gin.Context) string {
	return response.TraceID(c)
}

func logMarketAudit(event, traceID string, developerID, appID uuid.UUID, objectName string, err error) {
	auditEvent := audit.Event{EventType: event, TraceID: traceID, ActorID: developerID.String(), Resource: appID.String(), Metadata: map[string]interface{}{"object_name": objectName}}
	if err != nil {
		auditEvent.Error = err.Error()
		audit.Emit(auditEvent)
		log.Printf("market_audit event=%s trace_id=%s developer_id=%s app_id=%s object_name=%s err=%v", event, traceID, developerID, appID, objectName, err)
		return
	}
	audit.Emit(auditEvent)
	log.Printf("market_audit event=%s trace_id=%s developer_id=%s app_id=%s object_name=%s", event, traceID, developerID, appID, objectName)
}

func errorJSON(c *gin.Context, status int, code, message string) {
	response.Error(c, status, code, message)
}

func fileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
