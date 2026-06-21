package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gorm.io/gorm"
)

var (
	HTTPRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rpa_http_requests_total",
		Help: "Total HTTP requests by route, method and status.",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rpa_http_request_duration_seconds",
		Help:    "HTTP request duration by route, method and status.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	RequestPoolRejectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rpa_request_pool_rejected_total",
		Help: "Total requests rejected by the global request pool.",
	})

	DBOpenConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rpa_gorm_db_open_connections",
		Help: "Current number of established database connections.",
	})
	DBInUseConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rpa_gorm_db_in_use_connections",
		Help: "Current number of database connections in use.",
	})
	DBIdleConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rpa_gorm_db_idle_connections",
		Help: "Current number of idle database connections.",
	})
	DBWaitCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rpa_gorm_db_wait_count",
		Help: "Total number of waits for database connections.",
	})
	DBWaitDurationSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rpa_gorm_db_wait_duration_seconds",
		Help: "Total time blocked waiting for database connections.",
	})
)

func HTTPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		HTTPRequestTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(c.Request.Method, path, status).Observe(time.Since(start).Seconds())
	}
}

func ObserveGORMStats(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	stats := sqlDB.Stats()
	DBOpenConnections.Set(float64(stats.OpenConnections))
	DBInUseConnections.Set(float64(stats.InUse))
	DBIdleConnections.Set(float64(stats.Idle))
	DBWaitCount.Set(float64(stats.WaitCount))
	DBWaitDurationSeconds.Set(stats.WaitDuration.Seconds())
}

func StartGORMCollector(db *gorm.DB, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ObserveGORMStats(db)
		}
	}()
}

func IncRequestPoolRejected() {
	RequestPoolRejectedTotal.Inc()
}
