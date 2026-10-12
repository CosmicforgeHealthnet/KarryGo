package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ScannerAddress string
	ScannerNetwork string
	ScannerTimeout time.Duration
	AppEnv         string
	HTTPAddr       string
	Migration      bool

	DatabaseURL string

	FirebaseBucket          string
	FirebaseCredentialsFile string
	FirebaseCredentialsJSON string
	PublicBaseURL           string

	MaxUploadBytes   int64
	ServiceTokens    map[string]string
	PartnerJWTSecret string

	// --- WP10 private media object contract ---

	// PrivateAPIEnabled mounts the authoritative private surface. Default
	// false: a deployment that has not configured callers, secrets and a
	// download signing key must not expose the contract at all.
	PrivateAPIEnabled bool
	// ServiceSecrets holds per-caller HMAC secrets ("name=secret,...").
	ServiceSecrets string
	// PrivateAllowedCallers is the caller allowlist for the private surface.
	PrivateAllowedCallers []string
	// PrivateStorageDir roots filesystem-backed private object storage. Used
	// when no bucket is configured.
	PrivateStorageDir string
	// DownloadSecret signs short-lived content grants. Without it, private
	// download is refused rather than made open.
	DownloadSecret []byte

	UploadAuthorizationTTL time.Duration
	DownloadTTL            time.Duration
	PrivateMaxUploadBytes  int64
	DataExportMaxBytes     int64
	PrivateAllowedMIME     []string
	// ProhibitExecutableContent refuses recognised executable signatures.
	ProhibitExecutableContent bool
	// RequireSafeForDownload refuses a content grant for anything not
	// confirmed SAFE. Enabling it without a scanner means no private content
	// can be served, which is the intended fail-closed posture.
	RequireSafeForDownload bool
	DefaultRetention       time.Duration

	// MediaWorkersEnabled runs the in-process maintenance sweep.
	MediaWorkersEnabled  bool
	MediaWorkerInterval  time.Duration
	MediaWorkerLease     time.Duration
	MediaWorkerBatchSize int
	// MediaKillSwitch stops every media worker and write path without
	// requiring a redeploy of a different image.
	MediaKillSwitch bool

	// OutboxEnabled starts the event publisher.
	OutboxEnabled      bool
	OutboxTargetURL    string
	OutboxPath         string
	OutboxSecret       []byte
	OutboxBatchSize    int
	OutboxInterval     time.Duration
	OutboxLease        time.Duration
	OutboxMaxAttempts  int
	HTTPRequestTimeout time.Duration
}

func Load() Config {
	firebaseBucket := getEnv("MEDIA_FILE_FIREBASE_BUCKET", "")
	return Config{
		AppEnv:    getEnv("APP_ENV", "development"),
		HTTPAddr:  getEnv("HTTP_ADDR", ":8109"),
		Migration: getEnvBool("MIGRATION", false),

		DatabaseURL: getEnv("MEDIA_FILE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5441/media_file_service?sslmode=disable"),

		FirebaseBucket:          firebaseBucket,
		FirebaseCredentialsFile: getEnv("MEDIA_FILE_FIREBASE_CREDENTIALS_FILE", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
		FirebaseCredentialsJSON: getEnv("MEDIA_FILE_FIREBASE_CREDENTIALS_JSON", ""),
		PublicBaseURL:           getEnv("MEDIA_FILE_PUBLIC_BASE_URL", defaultPublicBaseURL(firebaseBucket)),

		MaxUploadBytes:   getEnvInt64("MEDIA_FILE_MAX_UPLOAD_BYTES", 25*1024*1024),
		ServiceTokens:    getEnvServiceTokens("MEDIA_FILE_SERVICE_TOKENS"),
		PartnerJWTSecret: getEnv("PARTNER_JWT_SECRET", ""),

		PrivateAPIEnabled:     getEnvBool("MEDIA_FILE_PRIVATE_API_ENABLED", false),
		ServiceSecrets:        getEnv("MEDIA_FILE_SERVICE_SECRETS", ""),
		PrivateAllowedCallers: getEnvList("MEDIA_FILE_PRIVATE_ALLOWED_CALLERS", "fleet-partner-service"),
		PrivateStorageDir:     getEnv("MEDIA_FILE_PRIVATE_STORAGE_DIR", ""),
		DownloadSecret:        []byte(getEnv("MEDIA_FILE_DOWNLOAD_SIGNING_SECRET", "")),

		UploadAuthorizationTTL: getEnvDuration("MEDIA_FILE_UPLOAD_AUTHORIZATION_TTL_SECONDS", 15*time.Minute),
		DownloadTTL:            getEnvDuration("MEDIA_FILE_PRIVATE_DOWNLOAD_TTL_SECONDS", 5*time.Minute),
		PrivateMaxUploadBytes:  getEnvInt64("MEDIA_FILE_PRIVATE_MAX_UPLOAD_BYTES", 25*1024*1024),
		ScannerAddress:         getEnv("MEDIA_FILE_SCANNER_ADDRESS", ""),
		ScannerNetwork:         getEnv("MEDIA_FILE_SCANNER_NETWORK", "tcp"),
		ScannerTimeout:         getEnvDuration("MEDIA_FILE_SCANNER_TIMEOUT_SECONDS", 5*time.Second),
		PrivateAllowedMIME: getEnvList("MEDIA_FILE_PRIVATE_ALLOWED_MIME_TYPES",
			"application/pdf,image/jpeg,image/png,image/webp,text/csv,application/x-ndjson"),
		// Purpose-specific ceiling for DATA_EXPORT, aligned with the Vendor
		// Section 8H export byte cap. It is separate from the global media
		// limit so raising one never silently raises the other.
		DataExportMaxBytes:        getEnvInt64("MEDIA_FILE_DATA_EXPORT_MAX_BYTES", 20*1024*1024),
		ProhibitExecutableContent: getEnvBool("MEDIA_FILE_PROHIBIT_EXECUTABLE_CONTENT", true),
		RequireSafeForDownload:    getEnvBool("MEDIA_FILE_REQUIRE_SAFE_FOR_DOWNLOAD", false),
		DefaultRetention:          getEnvDuration("MEDIA_FILE_DEFAULT_RETENTION_SECONDS", 0),

		MediaWorkersEnabled:  getEnvBool("MEDIA_FILE_WORKERS_ENABLED", false),
		MediaWorkerInterval:  getEnvDuration("MEDIA_FILE_WORKER_INTERVAL_SECONDS", time.Minute),
		MediaWorkerLease:     getEnvDuration("MEDIA_FILE_WORKER_LEASE_SECONDS", 2*time.Minute),
		MediaWorkerBatchSize: int(getEnvInt64("MEDIA_FILE_WORKER_BATCH_SIZE", 100)),
		MediaKillSwitch:      getEnvBool("MEDIA_FILE_KILL_SWITCH", false),

		OutboxEnabled:      getEnvBool("MEDIA_FILE_OUTBOX_ENABLED", false),
		OutboxTargetURL:    getEnv("MEDIA_FILE_OUTBOX_TARGET_URL", ""),
		OutboxPath:         getEnv("MEDIA_FILE_OUTBOX_PATH", ""),
		OutboxSecret:       []byte(getEnv("MEDIA_FILE_OUTBOX_SECRET", "")),
		OutboxBatchSize:    int(getEnvInt64("MEDIA_FILE_OUTBOX_BATCH_SIZE", 50)),
		OutboxInterval:     getEnvDuration("MEDIA_FILE_OUTBOX_INTERVAL_SECONDS", 5*time.Second),
		OutboxLease:        getEnvDuration("MEDIA_FILE_OUTBOX_LEASE_SECONDS", 30*time.Second),
		OutboxMaxAttempts:  int(getEnvInt64("MEDIA_FILE_OUTBOX_MAX_ATTEMPTS", 10)),
		HTTPRequestTimeout: getEnvDuration("MEDIA_FILE_HTTP_REQUEST_TIMEOUT_SECONDS", 10*time.Second),
	}
}

// getEnvList splits a comma-separated setting, trimming blanks.
func getEnvList(key string, fallback string) []string {
	raw := getEnv(key, fallback)
	values := []string{}
	for _, part := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// getEnvDuration reads a whole-second setting. Seconds rather than a duration
// string keeps every timing knob in this service spelled the same way.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvServiceTokens(key string) map[string]string {
	raw := os.Getenv(key)
	tokens := map[string]string{}
	if raw == "" {
		return tokens
	}

	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		name, token, ok := strings.Cut(pair, "=")
		if !ok {
			name, token, ok = strings.Cut(pair, ":")
		}
		if !ok {
			continue
		}

		name = strings.TrimSpace(name)
		token = strings.TrimSpace(token)
		if name != "" && token != "" {
			tokens[name] = token
		}
	}

	return tokens
}

func defaultPublicBaseURL(bucket string) string {
	if bucket == "" {
		return ""
	}

	return "https://storage.googleapis.com/" + bucket
}
