package serviceapp

import (
	"context"
	"errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/redisx"
)

type Options struct {
	Name         string
	DefaultAddr  string
	APIBase      string
	Capabilities []string
	ReadyChecks  []func(context.Context) error
	Register     func(*gin.RouterGroup)
	RegisterRoot func(*gin.Engine)
	Workers      []func(context.Context) error
	// DisableHTTP supports listener-free worker deployments. Health for that
	// process is expected to be supplied by the deployment runtime/process
	// probe; no public application routes are registered or exposed.
	DisableHTTP bool
}

func Run(opts Options) {
	if opts.Name == "" {
		log.Fatal("service name is required")
	}

	if os.Getenv("APP_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Resolved once: every surface below must report the same build, and a
	// per-request lookup would invite them to drift.
	build := CurrentBuildInfo(opts.Name)
	logging.Structured("info", "service_build", map[string]any{
		"service":     build.Service,
		"version":     build.Version,
		"commit_sha":  build.CommitSHA,
		"build_time":  build.BuildTime,
		"environment": build.Environment,
		"stamped":     build.Stamped(),
	})

	addr := getEnv("HTTP_ADDR", opts.DefaultAddr)
	if addr == "" {
		addr = ":8080"
	}

	var router *gin.Engine
	if !opts.DisableHTTP {
		router = gin.New()
		router.Use(cors.New(cors.Config{
			AllowOrigins:     []string{"http://localhost:3000", "https://karry-go.vercel.app"},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Idempotency-Key", "X-Correlation-ID", "X-Causation-ID"},
			AllowCredentials: true,
		}))
		router.Use(httpx.RequestID())
		router.Use(httpx.Logger())
		router.Use(httpx.Recovery())
		router.Use(httpx.ErrorHandler())

		router.GET("/health", func(c *gin.Context) {
			// The commit is on /health as well as /meta because a health
			// probe is the one endpoint a deployment check is certain to
			// reach, and "is it up" and "is it the build I shipped" are asked
			// at the same moment. It is not a secret: it is a public commit
			// on a public branch.
			c.JSON(http.StatusOK, gin.H{
				"success":    true,
				"service":    opts.Name,
				"status":     "ok",
				"commit_sha": build.CommitSHA,
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
				"version":      build.Version,
				"commit_sha":   build.CommitSHA,
				"build_time":   build.BuildTime,
				"environment":  build.Environment,
			})
		})
		if opts.Register != nil {
			opts.Register(group)
		}
		if opts.RegisterRoot != nil {
			opts.RegisterRoot(router)
		}

	}
	var server *http.Server
	if !opts.DisableHTTP {
		server = &http.Server{
			Addr:              addr,
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		var wg sync.WaitGroup
		for _, worker := range opts.Workers {
			if worker == nil {
				continue
			}
			wg.Add(1)
			go func(run func(context.Context) error) {
				defer wg.Done()
				if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("%s worker stopped: %v", opts.Name, err)
				}
			}(worker)
		}
		wg.Wait()
	}()

	if server != nil {
		go func() {
			log.Printf("%s listening on %s", opts.Name, addr)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("%s stopped: %v", opts.Name, err)
			}
		}()
	}

	<-ctx.Done()
	<-workerDone

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if server != nil {
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("%s shutdown: %v", opts.Name, err)
		}
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
