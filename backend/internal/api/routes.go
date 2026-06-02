package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"karrygo/backend/internal/customer"
	"karrygo/backend/internal/platform/apperrors"
	"karrygo/backend/internal/platform/cache"
	"karrygo/backend/internal/platform/httpx"
)

type Dependencies struct {
	Redis *redis.Client
	Cache *cache.Store
}

func RegisterRoutes(router *gin.Engine, deps Dependencies) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "status": "ok"})
	})

	router.GET("/ready", func(c *gin.Context) {
		if deps.Redis == nil {
			httpx.Abort(c, apperrors.Unavailable("redis is not configured", nil))
			return
		}

		if err := deps.Redis.Ping(c.Request.Context()).Err(); err != nil {
			httpx.Abort(c, apperrors.Unavailable("redis is not ready", err))
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "status": "ready"})
	})

	router.POST("/debug/cache", func(c *gin.Context) {
		if deps.Cache == nil {
			httpx.Abort(c, apperrors.Unavailable("cache is not configured", nil))
			return
		}

		payload := map[string]string{"service": "karrygo", "status": "cached"}
		if err := deps.Cache.SetJSON(c.Request.Context(), "debug:status", payload, time.Minute); err != nil {
			httpx.Abort(c, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{"success": true})
	})

	v1 := router.Group("/api/v1")
	customer.RegisterRoutes(v1)

	router.NoRoute(func(c *gin.Context) {
		httpx.Abort(c, apperrors.NotFound("Route not found.", nil))
	})
}
