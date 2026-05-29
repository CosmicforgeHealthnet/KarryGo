package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"karrygo/backend/internal/config"
	"karrygo/backend/internal/platform/apperrors"
	"karrygo/backend/internal/platform/cache"
	"karrygo/backend/internal/platform/httpx"
	"karrygo/backend/internal/platform/jobs"
	"karrygo/backend/internal/platform/redisx"
)

func main() {
	cfg := config.Load()
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	redisClient, err := redisx.NewClient(ctx, cfg.Redis)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer redisClient.Close()

	cacheStore := cache.NewStore(redisClient, "karrygo")
	queue := jobs.NewQueue(cfg.Redis)
	defer queue.Close()

	worker := jobs.NewWorker(cfg.Redis, jobs.NewHandlerMux())
	go func() {
		if err := worker.Run(); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("job worker stopped: %v", err)
		}
	}()
	defer worker.Shutdown()

	scheduler := jobs.NewScheduler(queue)
	if err := scheduler.Start(ctx); err != nil {
		log.Fatalf("start scheduler: %v", err)
	}
	defer scheduler.Stop()

	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "status": "ok"})
	})

	router.GET("/ready", func(c *gin.Context) {
		if err := redisClient.Ping(c.Request.Context()).Err(); err != nil {
			httpx.Abort(c, apperrors.Unavailable("redis is not ready", err))
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "status": "ready"})
	})

	router.POST("/debug/cache", func(c *gin.Context) {
		payload := map[string]string{"service": "karrygo", "status": "cached"}
		if err := cacheStore.SetJSON(c.Request.Context(), "debug:status", payload, time.Minute); err != nil {
			httpx.Abort(c, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{"success": true})
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("karrygo backend listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server stopped: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}
