package audit

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yuliusw/RPA-market/common/database"
	"gorm.io/gorm"
)

type Event struct {
	EventType string
	TraceID   string
	ActorID   string
	Resource  string
	Metadata  map[string]interface{}
	Error     string
}

type auditEventRow struct {
	EventID   uuid.UUID `gorm:"column:event_id"`
	EventType string    `gorm:"column:event_type"`
	TraceID   string    `gorm:"column:trace_id"`
	ActorID   string    `gorm:"column:actor_id"`
	Resource  string    `gorm:"column:resource"`
	Metadata  []byte    `gorm:"column:metadata;type:jsonb"`
	Error     string    `gorm:"column:error"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (auditEventRow) TableName() string { return "audit_events" }

type minioDeleteRetryRow struct {
	RetryID    uuid.UUID `gorm:"column:retry_id"`
	ObjectName string    `gorm:"column:object_name"`
	Reason     string    `gorm:"column:reason"`
	TraceID    string    `gorm:"column:trace_id"`
	Attempts   int       `gorm:"column:attempts"`
	Status     string    `gorm:"column:status"`
	NextRunAt  time.Time `gorm:"column:next_run_at"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (minioDeleteRetryRow) TableName() string { return "minio_delete_retries" }

var writer *Writer
var retryStop context.CancelFunc

type Writer struct {
	db     *gorm.DB
	ch     chan Event
	done   chan struct{}
	closed bool
	mu     sync.Mutex
}

func Start(db *gorm.DB) {
	if db == nil {
		return
	}
	writer = &Writer{db: db, ch: make(chan Event, 1024), done: make(chan struct{})}
	go writer.run()
}

func Emit(event Event) {
	if writer == nil {
		return
	}
	writer.mu.Lock()
	closed := writer.closed
	writer.mu.Unlock()
	if closed {
		return
	}
	select {
	case writer.ch <- event:
	default:
		log.Printf("audit queue full, drop event=%s trace_id=%s", event.EventType, event.TraceID)
	}
}

func Shutdown(ctx context.Context) {
	if writer == nil {
		return
	}
	writer.mu.Lock()
	if !writer.closed {
		writer.closed = true
		close(writer.ch)
	}
	writer.mu.Unlock()
	select {
	case <-writer.done:
	case <-ctx.Done():
		log.Printf("audit shutdown timeout: %v", ctx.Err())
	}
}

func RecordMinioDeleteRetry(ctx context.Context, db *gorm.DB, objectName, reason, traceID string) {
	if db == nil || objectName == "" {
		return
	}
	now := time.Now()
	row := minioDeleteRetryRow{RetryID: uuid.New(), ObjectName: objectName, Reason: reason, TraceID: traceID, Attempts: 0, Status: "pending", NextRunAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		log.Printf("failed to enqueue minio delete retry object=%s: %v", objectName, err)
	}
}

func StartMinioDeleteRetryWorker(db *gorm.DB, minioClient *database.MinioClient) {
	if db == nil || minioClient == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	retryStop = cancel
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runMinioDeleteRetries(ctx, db, minioClient)
			}
		}
	}()
}

func StopMinioDeleteRetryWorker() {
	if retryStop != nil {
		retryStop()
	}
}

func runMinioDeleteRetries(ctx context.Context, db *gorm.DB, minioClient *database.MinioClient) {
	var rows []minioDeleteRetryRow
	if err := db.WithContext(ctx).
		Where("status = ? AND next_run_at <= ? AND attempts < ?", "pending", time.Now(), 5).
		Order("next_run_at ASC").
		Limit(50).
		Find(&rows).Error; err != nil {
		log.Printf("failed to load minio delete retries: %v", err)
		return
	}
	for _, row := range rows {
		deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := minioClient.RemoveFile(deleteCtx, row.ObjectName)
		cancel()
		if err == nil {
			_ = db.WithContext(ctx).Model(&minioDeleteRetryRow{}).Where("retry_id = ?", row.RetryID).Updates(map[string]interface{}{"status": "done", "updated_at": time.Now()}).Error
			continue
		}
		nextAttempts := row.Attempts + 1
		status := "pending"
		if nextAttempts >= 5 {
			status = "failed"
		}
		nextRunAt := time.Now().Add(time.Duration(nextAttempts*nextAttempts) * time.Minute)
		_ = db.WithContext(ctx).Model(&minioDeleteRetryRow{}).Where("retry_id = ?", row.RetryID).Updates(map[string]interface{}{"attempts": nextAttempts, "reason": err.Error(), "status": status, "next_run_at": nextRunAt, "updated_at": time.Now()}).Error
	}
}

func (w *Writer) run() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer close(w.done)

	batch := make([]Event, 0, 100)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		rows := make([]auditEventRow, 0, len(batch))
		for _, event := range batch {
			metadata, _ := json.Marshal(event.Metadata)
			rows = append(rows, auditEventRow{EventID: uuid.New(), EventType: event.EventType, TraceID: event.TraceID, ActorID: event.ActorID, Resource: event.Resource, Metadata: metadata, Error: event.Error, CreatedAt: time.Now()})
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := w.db.WithContext(ctx).Create(&rows).Error; err != nil {
			log.Printf("failed to flush audit batch size=%d: %v", len(rows), err)
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-w.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, event)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
