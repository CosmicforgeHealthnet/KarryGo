package serviceapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
	"karrygo/shared/go/redisx"
)

type Options struct {
	Name         string
	DefaultAddr  string
	APIBase      string
	Capabilities []string
	ReadyChecks  []func(context.Context) error
	Register     func(*gin.RouterGroup)
}

func Run(opts Options) {
	if opts.Name == "" {
		log.Fatal("service name is required")
	}

	if os.Getenv("APP_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	addr := getEnv("HTTP_ADDR", opts.DefaultAddr)
	if addr == "" {
		addr = ":8080"
	}

	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"service": opts.Name,
			"status":  "ok",
		})
	})

	router.GET("/ready", func(c *gin.Context) {
		redisAddr := os.Getenv("REDIS_ADDR")
		if redisAddr != "" {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()

			client, err := redisx.NewClient(ctx, redisx.Config{
				Addr:     redisAddr,
				Password: os.Getenv("REDIS_PASSWORD"),
				DB:       getEnvInt("REDIS_DB", 0),
			})
			if err != nil {
				httpx.Abort(c, apperrors.Unavailable("redis is not ready", err))
				return
			}
			_ = client.Close()
		}

		for _, check := range opts.ReadyChecks {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			err := check(ctx)
			cancel()
			if err != nil {
				httpx.Abort(c, apperrors.Unavailable("service dependency is not ready", err))
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"service": opts.Name,
			"status":  "ready",
		})
	})

	base := opts.APIBase
	if base == "" {
		base = "/api/v1/" + opts.Name
	}

	group := router.Group(base)
	group.GET("/meta", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"service":      opts.Name,
			"api_base":     base,
			"capabilities": opts.Capabilities,
		})
	})
	if opts.Register != nil {
		opts.Register(group)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("%s listening on %s", opts.Name, addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("%s stopped: %v", opts.Name, err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("%s shutdown: %v", opts.Name, err)
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
