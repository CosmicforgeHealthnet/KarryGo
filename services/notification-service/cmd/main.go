package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/notification-service/internal/config"
	"cosmicforge/logistics/services/notification-service/internal/database"
	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagehttp "cosmicforge/logistics/services/notification-service/internal/features/messages/http"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("create notification database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

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

	serviceapp.Run(serviceapp.Options{
		Name:        "notification-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/notifications",
		Capabilities: []string{
			"push notifications",
			"sms notifications",
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
		},
	})
}

func buildPushSender(cfg config.Config) messageclients.PushSender {
	if cfg.FirebaseProjectID == "" || cfg.GoogleCredentialsFile == "" {
		return messageclients.NewLoggingPushSender()
	}
	return messageclients.NewFirebasePushSender(cfg.FirebaseProjectID, cfg.GoogleCredentialsFile)
}

func buildEmailSender(cfg config.Config) messageclients.EmailSender {
	if cfg.EmailProvider != "smtp" || cfg.SMTP.Host == "" || cfg.SMTP.Username == "" || cfg.SMTP.Password == "" || cfg.SMTP.From == "" {
		return messageclients.NewLoggingEmailSender()
	}
	return messageclients.NewSMTPEmailSender(cfg.SMTP)
}
