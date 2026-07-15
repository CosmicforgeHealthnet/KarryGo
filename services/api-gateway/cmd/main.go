package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/api-gateway/internal/config"
	"cosmicforge/logistics/services/api-gateway/internal/database"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/billing"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/callbacks"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	partnerhttp "cosmicforge/logistics/services/api-gateway/internal/features/partner/http"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/infra"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/ratelimit"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
	"cosmicforge/logistics/services/api-gateway/internal/pkg/pricing"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/redisx"
)

const (
	serviceName         = "api-gateway"
	retryWorkerEvery    = 15 * time.Second
	billingWorkerEvery  = 1 * time.Hour
	depInitTimeout      = 10 * time.Second
	shutdownGracePeriod = 10 * time.Second
)

// gatewayDeps bundles the initialized runtime dependencies.
type gatewayDeps struct {
	routeDeps    partnerhttp.Dependencies
	accountDeps  partneraccount.Dependencies
	pool         *pgxpool.Pool
	redisClient  *redis.Client
	webhookSvc   *webhooks.Service
	billingSvc   *billing.Service
	accountSvc   *partneraccount.Service
	partnerReady bool
}

func main() {
	cfg := config.Load()
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Root context cancelled on SIGINT/SIGTERM. Shared by the HTTP server,
	// dispatch subscribers, webhook retry worker, monthly invoice worker, and
	// invoice charge worker for graceful shutdown.
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps := buildPartnerDeps(cfg)

	router := gin.New()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
	router.Use(httpx.RequestID())
	router.Use(httpx.Logger())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())
	registerServiceEndpoints(router, deps)

	if deps.partnerReady {
		partnerhttp.RegisterPartnerRoutes(router, deps.routeDeps)
		partneraccount.RegisterAccountRoutes(router, deps.accountDeps)
		log.Printf("%s: partner API + dashboard + internal routes registered under /api/v1/partner/v1 and /api/v1/internal/partner", serviceName)

		// Internal HTTP callbacks (dispatch lifecycle + wallet charge-result).
		// Dispatch callbacks reuse the live webhooks pipeline; endpoints answer
		// 503 when the corresponding service is not configured.
		var dispatchProcessor callbacks.DispatchProcessor
		if deps.webhookSvc != nil {
			dispatchProcessor = deps.webhookSvc
		}
		var chargeHandler callbacks.ChargeResultHandler
		if deps.billingSvc != nil {
			chargeHandler = deps.billingSvc
		}
		callbacks.RegisterCallbackRoutes(router, callbacks.Dependencies{
			InternalServiceKey: cfg.PartnerAuth.InternalServiceKey,
			Dispatch:           dispatchProcessor,
			Billing:            chargeHandler,
		})
		log.Printf("%s: internal callback routes registered under /api/v1/internal/callbacks", serviceName)

		if deps.webhookSvc != nil {
			webhooks.StartSubscribers(rootCtx, deps.redisClient, deps.webhookSvc)
			go deps.webhookSvc.RunRetryWorker(rootCtx, retryWorkerEvery)
			log.Printf("%s: webhook subscribers and retry worker started", serviceName)
		}

		if deps.accountDeps.BillingService != nil {
			go deps.accountDeps.BillingService.RunMonthlyInvoiceWorker(rootCtx, billingWorkerEvery)
			log.Printf("%s: monthly billing invoice worker started (rate=%d kobo/delivery)",
				serviceName, cfg.Billing.RatePerDeliveryKobo)
		}

		if deps.billingSvc != nil && cfg.Billing.ChargeInterval > 0 {
			go deps.billingSvc.RunChargeWorker(rootCtx, cfg.Billing.ChargeInterval)
			log.Printf("%s: billing charge worker started (every %s)", serviceName, cfg.Billing.ChargeInterval)
		}
	} else {
		log.Printf("%s: partner API routes NOT registered (DB/Redis unavailable); serving health/ready/meta only", serviceName)
	}

	runServer(rootCtx, cfg, router)

	// Post-shutdown cleanup.
	if deps.redisClient != nil {
		_ = deps.redisClient.Close()
	}
	if deps.pool != nil {
		deps.pool.Close()
	}
}

// buildPartnerDeps wires DB-backed repos, Redis, wallet, delivery, webhook,
// billing services, and the rate limiter. partnerReady is false when DB or
// Redis is unavailable, so partner routes are never backed by nil dependencies.
func buildPartnerDeps(cfg config.Config) gatewayDeps {
	if cfg.DatabaseURL == "" {
		log.Printf("%s: API_GATEWAY_DATABASE_URL not set", serviceName)
		return gatewayDeps{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), depInitTimeout)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Printf("%s: database connect failed: %v", serviceName, err)
		return gatewayDeps{}
	}

	redisClient, err := redisx.NewClient(ctx, cfg.Redis)
	if err != nil {
		log.Printf("%s: redis connect failed: %v", serviceName, err)
		pool.Close()
		return gatewayDeps{}
	}

	if cfg.Wallet.Secret == "" {
		log.Printf("%s: warning: API_GATEWAY_WALLET_SERVICE_SECRET not set; payment calls will fail", serviceName)
	}

	bookingRepo := infra.NewBookingRepo(pool)
	deliverySvc := delivery.NewService(delivery.Deps{
		Bookings:    bookingRepo,
		Idempotency: infra.NewIdempotencyStore(pool),
		Wallet:      infra.NewWalletAdapter(cfg.Wallet.URL, cfg.Wallet.ServiceName, []byte(cfg.Wallet.Secret)),
		Publisher:   infra.NewRedisPublisher(redisClient),
	})

	rateLimiter := ratelimit.NewLimiter(ratelimit.NewRedisStore(redisClient))

	// Partner dashboard auth (password legacy + OTP signup -> session JWT) + key management.
	apiKeyRepo := infra.NewAPIKeyRepo(pool)
	accountRepo := infra.NewAccountRepo(pool)
	if cfg.PartnerAuth.JWTSecret == "" {
		log.Printf("%s: warning: PARTNER_JWT_SECRET not set; dashboard session tokens are insecure", serviceName)
	}
	if cfg.PartnerSMTP.Host == "" || cfg.PartnerSMTP.Username == "" || cfg.PartnerSMTP.Password == "" {
		log.Printf("%s: warning: PARTNER_SMTP_* not fully configured; partner OTP signup email will fail safely", serviceName)
	}
	otpSender := partneraccount.NewSMTPPartnerEmailSender(cfg.PartnerSMTP)
	accountSvc := partneraccount.NewService(partneraccount.Options{
		Accounts:          accountRepo,
		Sessions:          infra.NewSessionRepo(pool),
		Keys:              apiKeyRepo,
		OTPs:              infra.NewOTPChallengeRepo(pool),
		EmailSender:       otpSender,
		JWTSecret:         cfg.PartnerAuth.JWTSecret,
		JWTTTL:            cfg.PartnerAuth.JWTTTL,
		OTPTTL:            cfg.PartnerOTP.TTL,
		OTPMaxAttempts:    cfg.PartnerOTP.MaxAttempts,
		OTPResendCooldown: cfg.PartnerOTP.ResendCooldown,
	})

	// Billing (dashboard + monthly invoice generation + charge loop).
	billingSvc := billing.NewService(billing.Options{
		Invoices:            infra.NewInvoiceRepo(pool),
		PaymentMethods:      infra.NewPaymentMethodRepo(pool),
		Usage:               infra.NewBillingUsageReader(pool),
		Partners:            infra.NewBillingPartnerLister(pool),
		RatePerDeliveryKobo: cfg.Billing.RatePerDeliveryKobo,
		ChargeRepo:          infra.NewBillingRepo(pool),
		Wallet:              infra.NewWalletChargeClient(cfg.Wallet.URL, cfg.Wallet.ServiceName, []byte(cfg.Wallet.Secret)),
	})

	// Webhook service is optional and requires a secure encryption key. Without
	// WEBHOOK_SIGNING_SECRET we do not operate webhooks insecurely: routes stay
	// 501 and no subscribers/worker start.
	var webhookSvc *webhooks.Service
	if cryptor, err := webhooks.NewCryptor(cfg.WebhookSecret); err != nil {
		log.Printf("%s: warning: %v; webhook endpoints disabled", serviceName, err)
	} else {
		webhookSvc = webhooks.NewService(webhooks.Deps{
			Webhooks:   infra.NewWebhookRepo(pool),
			Deliveries: infra.NewWebhookDeliveryRepo(pool),
			Bookings:   bookingRepo,
			Cryptor:    cryptor,
			Sender:     webhooks.NewHTTPSender(),
		})
	}

	return gatewayDeps{
		routeDeps: partnerhttp.Dependencies{
			KeyRepo:        apiKeyRepo,
			Deliveries:     deliverySvc,
			Webhooks:       webhookSvc,
			RateLimiter:    rateLimiter,
			AccountRepo:    accountRepo,
			AccountService: accountSvc,
			Pricing: pricing.Config{
				BaseKobo:          cfg.Pricing.BaseKobo,
				StandardPerKmKobo: cfg.Pricing.StandardPerKmKobo,
				ExpressPerKmKobo:  cfg.Pricing.ExpressPerKmKobo,
				LargeMultiplier:   cfg.Pricing.LargeMultiplier,
			},
		},
		accountDeps: partneraccount.Dependencies{
			Service:            accountSvc,
			BillingService:     billingSvc,
			InternalServiceKey: cfg.PartnerAuth.InternalServiceKey,
		},
		pool:         pool,
		redisClient:  redisClient,
		webhookSvc:   webhookSvc,
		billingSvc:   billingSvc,
		accountSvc:   accountSvc,
		partnerReady: true,
	}
}

func registerServiceEndpoints(router *gin.Engine, deps gatewayDeps) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "service": serviceName, "status": "ok"})
	})

	router.GET("/ready", func(c *gin.Context) {
		if !deps.partnerReady {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "service": serviceName, "status": "degraded: partner dependencies unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if deps.pool != nil {
			if err := deps.pool.Ping(ctx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "service": serviceName, "status": "database not ready"})
				return
			}
		}
		if deps.redisClient != nil {
			if err := deps.redisClient.Ping(ctx).Err(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "service": serviceName, "status": "redis not ready"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "service": serviceName, "status": "ready"})
	})

	router.Group("/api/v1/gateway").GET("/meta", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success":       true,
			"service":       serviceName,
			"api_base":      "/api/v1/gateway",
			"partner_ready": deps.partnerReady,
			"webhooks":      deps.webhookSvc != nil,
			"capabilities": []string{
				"routes app traffic to the correct service",
				"partner delivery API (quotes, deliveries, webhooks)",
				"centralizes request IDs and edge health checks",
			},
		})
	})
}

func runServer(ctx context.Context, cfg config.Config, handler http.Handler) {
	addr := cfg.HTTPAddr
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("%s listening on %s", serviceName, addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("%s stopped: %v", serviceName, err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("%s shutdown: %v", serviceName, err)
	}
}
