package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/notification-service/internal/config"
	"cosmicforge/logistics/services/notification-service/internal/database"
	campaignclients "cosmicforge/logistics/services/notification-service/internal/features/campaigns/clients"
	campaignhttp "cosmicforge/logistics/services/notification-service/internal/features/campaigns/http"
	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	campaignusecases "cosmicforge/logistics/services/notification-service/internal/features/campaigns/usecases"
	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagehttp "cosmicforge/logistics/services/notification-service/internal/features/messages/http"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	logging.Notice("notification-service config", "migration=%t database=%s redis=%s", cfg.Migration, cfg.DatabaseURL, cfg.Redis.Addr)

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create notification database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	logConnectivity(ctx, cfg.DatabaseURL, cfg.Redis.Addr, db, redisClient)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db); err != nil {
			cancel()
			logging.Fatal("migration", "apply notification-service migrations: %v", err)
		}
		cancel()
		logging.Success("migration", "applied successfully")
	}

	repository := messagerepositories.NewPostgresNotificationRepository(db)
	queue := messageclients.NewRedisQueue(redisClient, cfg.RequestStream, cfg.DeliveryStream, cfg.DeadLetterStream)
	hub := messageclients.NewWebSocketHub()

	pushSender := buildPushSender(cfg)
	emailSender := buildEmailSender(cfg)
	notificationService := messageusecases.NewNotificationService(messageusecases.Options{
		Repository:          repository,
		PushSender:          pushSender,
		EmailSender:         emailSender,
		RealtimeSender:      hub,
		Queue:               queue,
		RealtimeTokenSecret: cfg.RealtimeTokenSecret,
		MaxAttempts:         cfg.MaxAttempts,
	})
	notificationService.StartConsumers(ctx, cfg.WorkerConcurrency)

	// Section 17 campaign fulfilment + Section 22 owner events.
	campaignRepo := campaignrepositories.New(db)
	campaignService := &campaignusecases.Service{Repo: campaignRepo}
	var workers []func(context.Context) error
	var campaignSecret, snapshotSecret []byte
	if cfg.CampaignsEnabled {
		if campaignSecret, err = campaignhttp.LoadSecret(cfg.CampaignSecretFile); err != nil {
			logging.Fatal("campaigns", "NOTIFICATION_CAMPAIGN_SECRET_FILE: %v", err)
		}
		audienceSecret, readErr := os.ReadFile(cfg.CustomerAudienceSecretFile)
		trimmed := []byte(strings.TrimSpace(string(audienceSecret)))
		if cfg.CustomerServiceURL == "" || readErr != nil || len(trimmed) < 32 {
			logging.Fatal("campaigns", "campaigns require NOTIFICATION_CUSTOMER_SERVICE_URL and a NOTIFICATION_CUSTOMER_AUDIENCE_SECRET_FILE of at least 32 bytes")
		}
		engine := &campaignusecases.Engine{
			Repo:    campaignRepo,
			Consent: &campaignclients.CustomerAudienceClient{BaseURL: cfg.CustomerServiceURL, Secret: trimmed},
			Sender:  notificationService,
		}
		workers = append(workers, engine.Run)
		logging.Success("campaigns", "campaign fulfilment enabled")
	} else {
		logging.Notice("campaigns", "off (NOTIFICATION_CAMPAIGNS_ENABLED=false): campaign routes answer 503")
	}
	if cfg.AdminEventsSnapshotEnabled {
		if snapshotSecret, err = campaignhttp.LoadSecret(cfg.AdminEventsSnapshotSecretFile); err != nil {
			logging.Fatal("admin-events", "NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE: %v", err)
		}
	}
	relay, err := adminevents.RelayFromEnvironment(db, "NOTIFICATION", "notification-service")
	if err != nil {
		logging.Fatal("admin-events", "%v", err)
	}
	if relay != nil {
		workers = append(workers, relay.Run)
	}

	serviceapp.Run(serviceapp.Options{
		Workers:     workers,
		Name:        "notification-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/notifications",
		Capabilities: []string{
			"push notifications",
			"email notifications",
			"in-app notifications",
			"retry handling",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		},
		Register: func(group *gin.RouterGroup) {
			messagehttp.RegisterRoutes(group, notificationService, hub, serviceauth.Secrets(cfg.ServiceSecrets))
			registerNotificationMetricsRoute(group, db, hub, serviceauth.Secrets(cfg.ServiceSecrets))
		},
		RegisterRoot: func(engine *gin.Engine) {
			campaignhttp.RegisterCampaignRoutes(engine, campaignService, campaignhttp.Boundary(cfg.CampaignsEnabled, campaignSecret, campaignRepo))
			campaignhttp.RegisterSnapshotRoute(engine, campaignService, campaignhttp.Boundary(cfg.AdminEventsSnapshotEnabled, snapshotSecret, campaignRepo))
		},
	})
}

func logConnectivity(ctx context.Context, databaseURL string, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
	Ping(context.Context) *redis.StatusCmd
}) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "failed url=%s err=%v", databaseURL, err)
	} else {
		logging.Success("database", "connected url=%s", databaseURL)
	}

	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logging.Error("redis", "failed addr=%s err=%v", redisAddr, err)
	} else {
		logging.Success("redis", "connected addr=%s", redisAddr)
	}
}

func buildPushSender(cfg config.Config) messageclients.PushSender {
	if cfg.FirebaseProjectID == "" || cfg.GoogleCredentialsFile == "" {
		return messageclients.NewLoggingPushSender()
	}
	return messageclients.NewFirebasePushSender(cfg.FirebaseProjectID, cfg.GoogleCredentialsFile)
}

func buildEmailSender(cfg config.Config) messageclients.EmailSender {
	if cfg.EmailProvider == "smtp" {
		return messageclients.NewSMTPEmailSender(cfg.SMTP)
	}
	if cfg.EmailProvider == "logging" && (cfg.AppEnv == "development" || cfg.AppEnv == "test") {
		return messageclients.NewLoggingEmailSender()
	}
	return messageclients.UnavailableEmailSender{}
}
