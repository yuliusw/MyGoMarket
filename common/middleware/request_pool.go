package middleware

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/panjf2000/ants/v2"
	"github.com/yuliusw/RPA-market/common/config"
	"github.com/yuliusw/RPA-market/common/metrics"
	"github.com/yuliusw/RPA-market/common/response"
	"github.com/yuliusw/RPA-market/common/utils/pool"
)

func ConfiguredRequestPoolFastFail() gin.HandlerFunc {
	if config.AppConfig == nil || !config.AppConfig.Features.RequestPool.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	requestPool := pool.InitGetRequestPool(config.AppConfig.Features.RequestPool.Capacity)
	return RequestPoolFastFail(requestPool)
}

func RequestPoolFastFail(requestPool *ants.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if requestPool == nil {
			c.Next()
			return
		}

		done := make(chan struct{})
		err := requestPool.Submit(func() {
			defer close(done)
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("request pool task panic: %v", recovered)
					if !c.Writer.Written() {
						response.Abort(c, http.StatusInternalServerError, "REQUEST_POOL_PANIC", "internal server error")
					}
				}
			}()
			c.Next()
		})
		if err != nil {
			if errors.Is(err, ants.ErrPoolOverload) {
				metrics.IncRequestPoolRejected()
				response.Abort(c, http.StatusServiceUnavailable, "SERVER_BUSY", "server is busy, please retry later")
				return
			}
			log.Printf("request pool submit failed: %v", err)
			response.Abort(c, http.StatusInternalServerError, "REQUEST_SCHEDULING_FAILED", "request scheduling failed")
			return
		}

		<-done
	}
}
