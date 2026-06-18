package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv   string
	HTTPAddr string

	DatabaseURL string

	FirebaseBucket          string
	FirebaseCredentialsFile string
	FirebaseCredentialsJSON string
	PublicBaseURL           string

	MaxUploadBytes int64
	ServiceTokens  map[string]string
}

func Load() Config {
	firebaseBucket := getEnv("MEDIA_FILE_FIREBASE_BUCKET", "")
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8109"),

		DatabaseURL: getEnv("MEDIA_FILE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5441/media_file_service?sslmode=disable"),

		FirebaseBucket:          firebaseBucket,
		FirebaseCredentialsFile: getEnv("MEDIA_FILE_FIREBASE_CREDENTIALS_FILE", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
		FirebaseCredentialsJSON: getEnv("MEDIA_FILE_FIREBASE_CREDENTIALS_JSON", ""),
		PublicBaseURL:           getEnv("MEDIA_FILE_PUBLIC_BASE_URL", defaultPublicBaseURL(firebaseBucket)),

		MaxUploadBytes: getEnvInt64("MEDIA_FILE_MAX_UPLOAD_BYTES", 25*1024*1024),
		ServiceTokens:  getEnvServiceTokens("MEDIA_FILE_SERVICE_TOKENS"),
	}
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
