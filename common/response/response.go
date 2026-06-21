package response

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const TraceIDKey = "trace_id"

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func Error(c *gin.Context, status int, code, message string) {
	if code == "" {
		code = codeForStatus(status)
	}
	c.JSON(status, ErrorBody{Code: code, Message: message, RequestID: TraceID(c)})
}

func Abort(c *gin.Context, status int, code, message string) {
	Error(c, status, code, message)
	c.Abort()
}

func TraceID(c *gin.Context) string {
	if traceID, exists := c.Get(TraceIDKey); exists {
		if s, ok := traceID.(string); ok && s != "" {
			return s
		}
	}
	traceID := strings.TrimSpace(c.GetHeader("X-Trace-ID"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	c.Set(TraceIDKey, traceID)
	c.Header("X-Trace-ID", traceID)
	return traceID
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusServiceUnavailable:
		return "SERVICE_UNAVAILABLE"
	default:
		return "INTERNAL_ERROR"
	}
}
