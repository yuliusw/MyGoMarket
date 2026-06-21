package audit

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/response"
	"gorm.io/gorm"
)

const exportBatchSize = 500

type ExportFilter struct {
	EventType string
	From      time.Time
	To        time.Time
	Cursor    string
	Limit     int
}

type exportCursor struct {
	CreatedAt time.Time `json:"created_at"`
	EventID   uuid.UUID `json:"event_id"`
}

// ExportCSV streams audit_events with a stable created_at,event_id cursor.
func ExportCSV(ctx context.Context, db *gorm.DB, w io.Writer, filter ExportFilter) error {
	if db == nil {
		return fmt.Errorf("audit db is nil")
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"event_id", "event_type", "trace_id", "actor_id", "resource", "metadata", "error", "created_at", "next_cursor"}); err != nil {
		return err
	}

	cursor, err := decodeCursor(filter.Cursor)
	if err != nil {
		return err
	}
	exported := 0
	for {
		batchLimit := exportBatchSize
		if filter.Limit > 0 && filter.Limit-exported < batchLimit {
			batchLimit = filter.Limit - exported
		}
		if batchLimit <= 0 {
			break
		}

		var rows []auditEventRow
		query := db.WithContext(ctx).Order("created_at ASC, event_id ASC").Limit(batchLimit)
		query = applyExportFilter(query, filter, cursor)
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			cursor = &exportCursor{CreatedAt: row.CreatedAt, EventID: row.EventID}
			nextCursor, err := encodeCursor(*cursor)
			if err != nil {
				return err
			}
			record := []string{
				row.EventID.String(),
				row.EventType,
				row.TraceID,
				row.ActorID,
				row.Resource,
				string(row.Metadata),
				row.Error,
				row.CreatedAt.Format(time.RFC3339Nano),
				nextCursor,
			}
			if err := cw.Write(record); err != nil {
				return err
			}
			exported++
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		if len(rows) < batchLimit || (filter.Limit > 0 && exported >= filter.Limit) {
			break
		}
	}
	cw.Flush()
	return cw.Error()
}

func ExportCSVHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter, err := parseExportFilter(c)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_AUDIT_EXPORT_QUERY", err.Error())
			return
		}

		filename := "audit_events_" + time.Now().UTC().Format("20060102T150405Z") + ".csv"
		httpReader, httpWriter := io.Pipe()
		minioReader, minioWriter := io.Pipe()
		objectName := "audit-exports/" + filename

		writer := io.Writer(httpWriter)
		if database.GlobalMinio != nil {
			writer = io.MultiWriter(httpWriter, minioWriter)
			go func() {
				_, uploadErr := database.GlobalMinio.UploadReader(context.Background(), objectName, minioReader, "text/csv; charset=utf-8")
				if uploadErr != nil {
					log.Printf("failed to upload audit export object=%s: %v", objectName, uploadErr)
				}
			}()
		} else {
			_ = minioReader.Close()
			_ = minioWriter.Close()
		}

		go func() {
			err := ExportCSV(c.Request.Context(), db, writer, filter)
			_ = httpWriter.CloseWithError(err)
			if database.GlobalMinio != nil {
				_ = minioWriter.CloseWithError(err)
			}
		}()

		c.DataFromReader(http.StatusOK, -1, "text/csv; charset=utf-8", httpReader, map[string]string{
			"Content-Disposition": `attachment; filename="` + filename + `"`,
			"X-MinIO-Object":      objectName,
		})
	}
}

func applyExportFilter(query *gorm.DB, filter ExportFilter, cursor *exportCursor) *gorm.DB {
	if filter.EventType != "" {
		query = query.Where("event_type = ?", filter.EventType)
	}
	if !filter.From.IsZero() {
		query = query.Where("created_at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		query = query.Where("created_at <= ?", filter.To)
	}
	if cursor != nil {
		query = query.Where("created_at > ? OR (created_at = ? AND event_id > ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.EventID)
	}
	return query
}

func parseExportFilter(c *gin.Context) (ExportFilter, error) {
	filter := ExportFilter{
		EventType: strings.TrimSpace(c.Query("event_type")),
		Cursor:    strings.TrimSpace(c.Query("cursor")),
	}
	var err error
	filter.From, err = parseOptionalTime(c.Query("from"))
	if err != nil {
		return filter, fmt.Errorf("invalid from")
	}
	filter.To, err = parseOptionalTime(c.Query("to"))
	if err != nil {
		return filter, fmt.Errorf("invalid to")
	}
	if limitText := strings.TrimSpace(c.Query("limit")); limitText != "" {
		filter.Limit, err = strconv.Atoi(limitText)
		if err != nil || filter.Limit <= 0 || filter.Limit > 100000 {
			return filter, fmt.Errorf("invalid limit")
		}
	}
	return filter, nil
}

func parseOptionalTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", value)
}

func decodeCursor(value string) (*exportCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	var cursor exportCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.CreatedAt.IsZero() || cursor.EventID == uuid.Nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &cursor, nil
}

func encodeCursor(cursor exportCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
