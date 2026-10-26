package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/media-file-service/internal/config"
	"cosmicforge/logistics/services/media-file-service/internal/database"
	"cosmicforge/logistics/services/media-file-service/internal/events"
	filemetadatarepositories "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/repositories"
	mediahttp "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/http"
	mediarepositories "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	mediausecases "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	uploadhttp "cosmicforge/logistics/services/media-file-service/internal/features/uploads/http"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/services/media-file-service/internal/integrations/clamav"
	"cosmicforge/logistics/services/media-file-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	logging.Notice("media-file-service config", "migration=%t database=%s firebase_bucket=%s private_api=%t",
		cfg.Migration, cfg.DatabaseURL, cfg.FirebaseBucket, cfg.PrivateAPIEnabled)

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create media file database pool: %v", err)
	}
	defer db.Close()

	logConnectivity(ctx, cfg.DatabaseURL, db)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db, "migrations"); err != nil {
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
	// Legacy reads of anything but a profile photo are scanned at read time
	// and fail closed without a scanner (Section 19).
	var legacyScanner uploadusecases.LegacyScanner
	if cfg.ScannerAddress != "" {
		scanner, scannerErr := clamav.New(cfg.ScannerNetwork, cfg.ScannerAddress, cfg.ScannerTimeout, cfg.MaxUploadBytes)
		if scannerErr != nil {
			logging.Fatal("scanner", "configure legacy read scanner: %v", scannerErr)
		}
		legacyScanner = legacyScanAdapter{scanner: scanner}
	}
	uploadService := uploadusecases.NewUploadService(uploadusecases.Options{
		Storage:                 storageClient,
		Assets:                  assetRepo,
		MaxUploadBytes:          cfg.MaxUploadBytes,
		Governance:              filemetadatarepositories.NewPostgresLegacyGovernance(db),
		Scanner:                 legacyScanner,
		PublicSensitiveDownload: cfg.LegacyPublicSensitiveDownload,
	})
	if cfg.LegacyPublicSensitiveDownload {
		logging.Notice("media", "MEDIA_LEGACY_PUBLIC_SENSITIVE_DOWNLOAD_ENABLED=true: legacy documents, proof images and signatures are publicly downloadable by id")
	}

	mediaService, privateStorage, err := buildPrivateMediaService(cfg, db, storageClient)
	if err != nil {
		logging.Fatal("media", "configure private media contract: %v", err)
	}

	// No live source-case adapter is enrolled until canonical modern-object
	// membership/remediation contracts exist. Nil keeps issue/redemption blocked.
	caseLegacyCredentials := serviceauth.ParseSecrets(cfg.ServiceSecrets)
	for owner, token := range cfg.ServiceTokens {
		caseLegacyCredentials["legacy-upload:"+owner] = []byte(token)
	}
	caseHandler, err := mediahttp.AdminCaseRuntime(ctx, mediaService, db, caseLegacyCredentials, cfg.DownloadSecret, nil)
	if err != nil {
		logging.Fatal("media", "private case configuration refused")
	}
	// Owner-attested legacy adoption; MEDIA_LEGACY_ADOPTION_ENABLED defaults false.
	adoptionOthers := [][]byte{cfg.DownloadSecret}
	for _, key := range caseLegacyCredentials {
		adoptionOthers = append(adoptionOthers, key)
	}
	adoptionHandler, err := mediahttp.LegacyAdoptionRuntime(mediaService, db, storageClient, cfg.FirebaseBucket, cfg.MaxUploadBytes, adoptionOthers)
	if err != nil {
		logging.Fatal("media", "legacy adoption configuration refused")
	}

	// Private Bridge lookup of adopted legacy mappings;
	// MEDIA_LEGACY_MAPPING_LOOKUP_ENABLED defaults false.
	mappingHandler, err := mediahttp.LegacyMappingRuntime(ctx, mediaService, db, cfg.DownloadSecret, caseLegacyCredentials)
	if err != nil {
		logging.Fatal("media", "legacy mapping configuration refused")
	}

	var workers []func(context.Context) error
	if mediaService != nil && cfg.MediaWorkersEnabled && !cfg.MediaKillSwitch {
		owner := ids.New("mwrk")
		workers = append(workers, func(workerCtx context.Context) error {
			return mediaService.RunMaintenance(workerCtx, owner, cfg.MediaWorkerInterval,
				cfg.MediaWorkerLease, cfg.MediaWorkerBatchSize)
		})
	}
	if mediaService != nil && cfg.OutboxEnabled && !cfg.MediaKillSwitch {
		publisher, publisherErr := events.NewHTTPPublisher(cfg.OutboxTargetURL, cfg.OutboxPath,
			"media-file-service", cfg.OutboxSecret, cfg.HTTPRequestTimeout)
		if publisherErr != nil {
			logging.Fatal("outbox", "configure media outbox publisher: %v", publisherErr)
		}
		outboxWorker := events.NewWorker(events.NewOutboxRepository(db), publisher, events.WorkerConfig{
			Owner:       "media-file-service-outbox",
			BatchSize:   cfg.OutboxBatchSize,
			Interval:    cfg.OutboxInterval,
			Lease:       cfg.OutboxLease,
			MaxAttempts: cfg.OutboxMaxAttempts,
		})
		workers = append(workers, outboxWorker.Run)
	}

	readyChecks := []func(context.Context) error{
		db.Ping,
		storageClient.Check,
		func(checkCtx context.Context) error {
			return database.MigrationsCurrent(checkCtx, db, "migrations")
		},
	}
	if privateStorage != nil {
		readyChecks = append(readyChecks, privateStorage.Check)
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "media-file-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/media-files",
		Capabilities: []string{
			"profile photos",
			"document uploads",
			"delivery proof images",
			"recipient signatures",
			"private document objects",
		},
		ReadyChecks: readyChecks,
		Register: func(group *gin.RouterGroup) {
			uploadhttp.RegisterUploadRoutes(group, uploadService, cfg.ServiceTokens, cfg.MaxUploadBytes, cfg.PartnerJWTSecret)
			registerMediaMetricsRoute(group, db, serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute))
			if mediaService == nil {
				logging.Notice("media", "private media contract not mounted (MEDIA_FILE_PRIVATE_API_ENABLED)")
				return
			}
			mediahttp.RegisterPrivateRoutes(group.Group("/internal/media"),
				mediahttp.NewHandler(mediaService, db),
				serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
				cfg.PrivateAllowedCallers, cfg.PrivateMaxUploadBytes)
			mediahttp.RegisterCustomerUploadRoute(group, mediaService, cfg.PrivateMaxUploadBytes)
			mediahttp.RegisterVendorUploadRoute(group, mediaService, cfg.PrivateMaxUploadBytes)
			logging.Success("media", "private media contract mounted")
		},
		RegisterRoot: func(root *gin.Engine) {
			caseHandler.Register(root)
			adoptionHandler.Register(root)
			mappingHandler.Register(root)
		},
		Workers: workers,
	})
}

// buildPrivateMediaService assembles the authoritative media contract.
//
// It returns (nil, nil, nil) when the contract is switched off. It returns an
// error — refusing to boot — when the contract is switched ON but cannot be
// operated safely: no download signing secret means no bounded grant can be
// minted, and starting anyway would leave the routes present and every private
// download failing at request time instead of at deploy time.
func buildPrivateMediaService(cfg config.Config, db *pgxpool.Pool,
	storageClient uploadclients.ObjectStorage) (*mediausecases.Service, uploadclients.PrivateStorage, error) {
	if !cfg.PrivateAPIEnabled || cfg.MediaKillSwitch {
		return nil, nil, nil
	}
	if len(cfg.DownloadSecret) == 0 {
		return nil, nil, errMissingDownloadSecret
	}

	var privateStorage uploadclients.PrivateStorage
	bucket := ""
	if cfg.FirebaseBucket != "" {
		privateStorage = uploadclients.NewBucketPrivateStorage(storageClient, cfg.FirebaseBucket)
		bucket = cfg.FirebaseBucket
	} else {
		dir := cfg.PrivateStorageDir
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "karrygo-media-private")
		}
		filesystem, err := uploadclients.NewFilesystemPrivateStorage(dir)
		if err != nil {
			return nil, nil, err
		}
		privateStorage = filesystem
		bucket = "local:" + dir
	}

	var scanner mediausecases.Scanner
	if cfg.ScannerAddress != "" {
		configured, err := clamav.New(cfg.ScannerNetwork, cfg.ScannerAddress, cfg.ScannerTimeout, cfg.PrivateMaxUploadBytes)
		if err != nil {
			return nil, nil, err
		}
		scanner = configured
	}
	service := mediausecases.NewService(mediausecases.Options{
		Pool:           db,
		Objects:        mediarepositories.NewRepository(db),
		Storage:        privateStorage,
		Events:         events.NewDomainPublisher(events.NewOutboxRepository(db)),
		DownloadSecret: cfg.DownloadSecret,
		StorageBucket:  bucket,
		Scanner:        scanner,
		Policy: mediausecases.Policy{
			MaxSizeBytes:              cfg.PrivateMaxUploadBytes,
			AllowedMIMETypes:          cfg.PrivateAllowedMIME,
			UploadAuthorizationTTL:    cfg.UploadAuthorizationTTL,
			DownloadTTL:               cfg.DownloadTTL,
			ProhibitExecutableContent: cfg.ProhibitExecutableContent,
			RequireSafeForDownload:    cfg.RequireSafeForDownload,
			DefaultRetention:          cfg.DefaultRetention,
			DataExportMaxBytes:        cfg.DataExportMaxBytes,
		},
	})
	return service, privateStorage, nil
}

var errMissingDownloadSecret = configError("MEDIA_FILE_DOWNLOAD_SIGNING_SECRET is required when the private media API is enabled")

type configError string

func (e configError) Error() string { return string(e) }

func logConnectivity(ctx context.Context, databaseURL string, db interface{ Ping(context.Context) error }) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "failed url=%s err=%v", databaseURL, err)
	} else {
		logging.Success("database", "connected url=%s", databaseURL)
	}
}

// legacyScanAdapter lets the legacy upload feature use the ClamAV scanner
// without depending on the private media contract's types.
type legacyScanAdapter struct{ scanner mediausecases.Scanner }

func (a legacyScanAdapter) ScanLegacy(ctx context.Context, assetID string, content []byte) (bool, error) {
	result, err := a.scanner.Scan(ctx, "legacy:"+assetID, content)
	if err != nil {
		return false, err
	}
	return result.Safe, nil
}
