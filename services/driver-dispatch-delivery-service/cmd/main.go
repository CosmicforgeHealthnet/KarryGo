package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/database"
	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	bookingclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking/clients"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	bookingfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/router"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/walletclient"
)

func main() {
	// Best-effort .env load for local development (same pattern as hauling).
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()
	db, err := connectPostgresWithRetry(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect dispatch delivery postgres: %v", err)
	}
	defer db.Close()

	redisClient, err := connectRedisWithRetry(ctx, cfg)
	if err != nil {
		log.Fatalf("connect dispatch delivery redis: %v", err)
	}
	defer redisClient.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	walletClient := walletclient.Client{
		BaseURL:     cfg.WalletServiceURL,
		ServiceName: cfg.WalletServiceSource,
		Secret:      cfg.WalletServiceSecret,
	}

	// media-file-service client for *_STORAGE_MODE=media (production-ready
	// upload backend for trip proof, verification, vehicle, and avatar
	// uploads). ServiceName must be "dispatch-delivery-service" to match
	// media-file-service's MEDIA_FILE_SERVICE_TOKENS entry for this caller —
	// note this differs from the "driver-dispatch-delivery-service" identity
	// used with notification-service.
	mediaClient := mediaclient.New(mediaclient.Config{
		BaseURL:     cfg.MediaFileBaseURL,
		ServiceName: "dispatch-delivery-service",
		Token:       cfg.MediaFileServiceToken,
	})

	var emailClient authclients.EmailClient
	if cfg.SMTPHost != "" && cfg.SMTPUser != "" && cfg.SMTPPassword != "" && cfg.SMTPFrom != "" {
		emailClient = &authclients.CpanelEmailClient{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			TLSMode:  cfg.SMTPTLSMode,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}
		log.Printf("dispatch rider otp email smtp configured host=%s port=%d tls_mode=%q from=%s", cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPTLSMode, cfg.SMTPFrom)
	} else {
		emailClient = authclients.NoopEmailClient{}
		log.Printf("dispatch rider otp email smtp not configured; email OTP delivery will be skipped when optional and fail when required")
	}

	authService := authusecases.NewAuthUsecase(authusecases.Options{
		Identities:           authrepositories.NewPostgresIdentityRepository(db),
		OTPs:                 authrepositories.NewPostgresOTPRepository(db),
		Sessions:             authrepositories.NewPostgresSessionRepository(db),
		Notifier:             authclients.NewLoggingNotificationClient(cfg.OTPDebug),
		EmailClient:          emailClient,
		Publisher:            authclients.NewRedisEventPublisher(redisClient),
		AccessTokenSecret:    cfg.AccessTokenSecret,
		RefreshTokenSecret:   cfg.RefreshTokenSecret,
		OTPSecret:            cfg.OTPSecret,
		AccessTokenTTL:       cfg.AccessTokenTTL,
		RefreshTokenTTL:      cfg.RefreshTokenTTL,
		OTPTTL:               cfg.OTPTTL,
		OTPRateWindow:        cfg.OTPRateWindow,
		OTPMaxRequests:       cfg.OTPMaxRequests,
		OTPMaxAttempts:       cfg.OTPMaxAttempts,
		OTPLockoutTTL:        cfg.OTPLockoutTTL,
		OTPDebug:             cfg.OTPDebug,
		RequireEmailDelivery: cfg.AppEnv != "development",
		Redis:                redisClient,
	})

	asynqRedis := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	asynqClient := asynq.NewClient(asynqRedis)
	defer asynqClient.Close()

	// Lifecycle notifier: sends customer/rider notifications via
	// notification-service. No-op when DISPATCH_NOTIFICATION_URL/SECRET unset.
	bookingNotifier := bookingclients.NewBookingNotifier(cfg.NotificationURL, cfg.NotificationSecret)
	if cfg.NotificationURL != "" {
		log.Printf("notification-service integration enabled url=%s", cfg.NotificationURL)
	} else {
		log.Printf("notification-service integration disabled (DISPATCH_NOTIFICATION_URL unset); apps fall back to polling")
	}

	proofStorage, err := tripfeature.NewProofStorageFromEnv(cfg.AppEnv, mediaClient)
	if err != nil {
		log.Fatalf("configure trip proof storage: %v", err)
	}
	tripService := tripfeature.NewService(tripfeature.NewPostgresRepository(db), proofStorage).
		WithEventPublisher(tripfeature.NewRedisEventPublisher(redisClient)).
		WithWalletClient(walletClient).
		WithNotifier(bookingNotifier).
		WithHandoverSecret(cfg.HandoverSecret)
	tripfeature.StartSubscribers(ctx, redisClient, tripService)

	// Rider request push: real notification-service sender when configured,
	// logging stub otherwise (asynq send_push task retries on hard errors).
	var requestSender requestfeature.NotificationSender = requestfeature.LoggingNotificationSender{}
	if cfg.NotificationURL != "" && len(cfg.NotificationSecret) > 0 {
		requestSender = requestfeature.NewNotificationServiceSender(cfg.NotificationURL, cfg.NotificationSecret)
	}

	requestService := requestfeature.NewService(
		requestfeature.NewPostgresRepository(db),
		redisClient,
		requestfeature.NewHTTPNearbyClient(cfg.AvailabilityServiceURL, cfg.InternalServiceKey),
		requestSender,
		requestfeature.NewRedisEventPublisher(redisClient),
		asynqClient,
		requestfeature.Config{
			InitialRadiusKM:   cfg.BroadcastInitialRadiusKM,
			RadiusIncrementKM: cfg.BroadcastRadiusIncrementKM,
			MaxAttempts:       cfg.BroadcastMaxAttempts,
			BroadcastWindow:   cfg.BroadcastWindow,
		},
	)
	requestfeature.StartSubscribers(ctx, redisClient, requestService)

	bookingService := bookingfeature.NewService(
		bookingfeature.NewPostgresRepository(db),
		requestService,
		bookingfeature.TripCancelAdapter{Trips: tripService},
		availability.NewRedisLiveStore(redisClient),
		bookingfeature.NewWalletPaymentClient(walletClient),
		bookingfeature.NewRedisEventPublisher(redisClient),
		cfg.AvailabilityRadiusKM,
	).WithNotifier(bookingNotifier)
	bookingfeature.StartSubscribers(ctx, redisClient, bookingService)

	asynqServer := asynq.NewServer(asynqRedis, asynq.Config{
		Concurrency: 10,
		Queues:      map[string]int{"critical": 6, "default": 3, "low": 1},
	})
	asynqMux := asynq.NewServeMux()
	requestfeature.RegisterWorkerHandlers(asynqMux, requestfeature.NewWorker(requestService))
	go func() {
		if err := asynqServer.Run(asynqMux); err != nil {
			log.Printf("request asynq server stopped: %v", err)
		}
	}()

	// Re-adopt broadcasts stranded mid-window by a crash before their expire
	// task was scheduled (idempotent — the worker re-checks state).
	go func() {
		if err := requestService.RecoverStrandedBroadcasts(ctx); err != nil {
			log.Printf("request stranded-broadcast recovery failed: %v", err)
		}
	}()

	// Settle trips left stuck in 'proof_submitted' when the rider's app never
	// called complete (killed, crashed, lost network). Without this the
	// customer's escrowed fare stays held and the rider is never paid.
	go tripService.RunAutoCompleteWorker(ctx)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router.New(cfg, db, redisClient, authService, requestService, tripService, bookingService, mediaClient),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("%s listening on %s", cfg.ServiceName, cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("%s stopped: %v", cfg.ServiceName, err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("%s shutdown: %v", cfg.ServiceName, err)
	}
	asynqServer.Shutdown()
}

func connectPostgresWithRetry(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err := database.NewPool(ctx, databaseURL)
		if err == nil {
			return pool, nil
		}

		lastErr = err
		log.Printf("waiting for dispatch delivery postgres attempt=%d error=%v", attempt, err)
		time.Sleep(2 * time.Second)
	}

	return nil, lastErr
}

func connectRedisWithRetry(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		client := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})

		if err := client.Ping(ctx).Err(); err == nil {
			return client, nil
		} else {
			lastErr = err
			_ = client.Close()
			log.Printf("waiting for dispatch delivery redis attempt=%d error=%v", attempt, err)
			time.Sleep(2 * time.Second)
		}
	}

	return nil, lastErr
}
