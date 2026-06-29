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
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/walletclient"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/database"
	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/router"
)

func main() {
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

	// Build email client. NoopEmailClient is used when SMTP is not configured
	// so email delivery is gracefully disabled without changing any other logic.
	var emailClient authclients.EmailClient
	if cfg.SMTPHost != "" && cfg.SMTPUser != "" && cfg.SMTPPassword != "" && cfg.SMTPFrom != "" {
		emailClient = &authclients.CpanelEmailClient{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}
	} else {
		emailClient = authclients.NoopEmailClient{}
	}

	authService := authusecases.NewAuthUsecase(authusecases.Options{
		Identities:         authrepositories.NewPostgresIdentityRepository(db),
		OTPs:               authrepositories.NewPostgresOTPRepository(db),
		Sessions:           authrepositories.NewPostgresSessionRepository(db),
		Notifier:           authclients.NewLoggingNotificationClient(cfg.OTPDebug),
		EmailClient:        emailClient,
		Publisher:          authclients.NewRedisEventPublisher(redisClient),
		AccessTokenSecret:  cfg.AccessTokenSecret,
		RefreshTokenSecret: cfg.RefreshTokenSecret,
		OTPSecret:          cfg.OTPSecret,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		OTPTTL:             cfg.OTPTTL,
		OTPRateWindow:      cfg.OTPRateWindow,
		OTPMaxRequests:     cfg.OTPMaxRequests,
		OTPMaxAttempts:     cfg.OTPMaxAttempts,
		OTPLockoutTTL:      cfg.OTPLockoutTTL,
		OTPDebug:           cfg.OTPDebug,
		Redis:              redisClient,
	})

	asynqRedis := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	asynqClient := asynq.NewClient(asynqRedis)
	defer asynqClient.Close()

	proofStorage, err := tripfeature.NewProofStorageFromEnv(cfg.AppEnv)
	if err != nil {
		log.Fatalf("configure trip proof storage: %v", err)
	}
	tripService := tripfeature.NewService(tripfeature.NewPostgresRepository(db), proofStorage).
		WithEventPublisher(tripfeature.NewRedisEventPublisher(redisClient)).
		WithWalletClient(walletClient)
	tripfeature.StartSubscribers(ctx, redisClient, tripService)

	requestService := requestfeature.NewService(
		requestfeature.NewPostgresRepository(db),
		redisClient,
		requestfeature.NewHTTPNearbyClient(cfg.AvailabilityServiceURL, cfg.InternalServiceKey),
		requestfeature.LoggingNotificationSender{},
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

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router.New(cfg, db, redisClient, authService, requestService, tripService),
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
