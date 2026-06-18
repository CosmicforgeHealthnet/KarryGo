package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/media-file-service/internal/config"
	"cosmicforge/logistics/services/media-file-service/internal/database"
	filemetadatarepositories "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/repositories"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	uploadhttp "cosmicforge/logistics/services/media-file-service/internal/features/uploads/http"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("create media file database pool: %v", err)
	}
	defer db.Close()

	storageClient, err := uploadclients.NewFirebaseStorageClient(ctx, uploadclients.FirebaseStorageOptions{
		BucketName:      cfg.FirebaseBucket,
		CredentialsFile: cfg.FirebaseCredentialsFile,
		CredentialsJSON: cfg.FirebaseCredentialsJSON,
		PublicBaseURL:   cfg.PublicBaseURL,
	})
	if err != nil {
		log.Fatalf("create firebase storage client: %v", err)
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
