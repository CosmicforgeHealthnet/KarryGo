package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/media-file-service/internal/config"
	"cosmicforge/logistics/services/media-file-service/internal/database"
	filemetadatarepositories "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/repositories"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	uploadhttp "cosmicforge/logistics/services/media-file-service/internal/features/uploads/http"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	logging.Notice("media-file-service config", "migration=%t database=%s firebase_bucket=%s", cfg.Migration, cfg.DatabaseURL, cfg.FirebaseBucket)

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create media file database pool: %v", err)
	}
	defer db.Close()

	logConnectivity(ctx, cfg.DatabaseURL, db)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db); err != nil {
			cancel()
			logging.Fatal("migration", "apply media-file migrations: %v", err)
		}
		cancel()
		logging.Success("migration", "applied successfully")
	}

	storageClient, err := uploadclients.NewFirebaseStorageClient(ctx, uploadclients.FirebaseStorageOptions{
		BucketName:      cfg.FirebaseBucket,
		CredentialsFile: cfg.FirebaseCredentialsFile,
		CredentialsJSON: cfg.FirebaseCredentialsJSON,
		PublicBaseURL:   cfg.PublicBaseURL,
	})
	if err != nil {
		logging.Fatal("firebase", "create storage client: %v", err)
	}

	assetRepo := filemetadatarepositories.NewPostgresMediaAssetRepository(db)
	uploadService := uploadusecases.NewUploadService(uploadusecases.Options{
		Storage:        storageClient,
		Assets:         assetRepo,
		MaxUploadBytes: cfg.MaxUploadBytes,
	})

	serviceapp.Run(serviceapp.Options{
		Name:        "media-file-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/media-files",
		Capabilities: []string{
			"profile photos",
			"document uploads",
			"delivery proof images",
			"recipient signatures",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			storageClient.Check,
		},
		Register: func(group *gin.RouterGroup) {
			uploadhttp.RegisterUploadRoutes(group, uploadService, cfg.ServiceTokens, cfg.MaxUploadBytes)
		},
	})
}

func logConnectivity(ctx context.Context, databaseURL string, db interface{ Ping(context.Context) error }) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "failed url=%s err=%v", databaseURL, err)
	} else {
		logging.Success("database", "connected url=%s", databaseURL)
	}
}
