package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv    string
	HTTPAddr  string
	Migration bool

	DatabaseURL string
	Redis       redisx.Config

	ServiceSecrets map[string][]byte

	FirebaseProjectID     string
	GoogleCredentialsFile string

	EmailProvider string
	SMTP          SMTPConfig

	RealtimeTokenSecret []byte
	WorkerConcurrency   int
	MaxAttempts         int
	RequestStream       string
	DeliveryStream      string
	DeadLetterStream    string
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	TLSMode  string
	From     string
}

func Load() Config {
	return Config{
		AppEnv:    getEnv("APP_ENV", "development"),
		HTTPAddr:  getEnv("HTTP_ADDR", ":8106"),
		Migration: getEnvBool("MIGRATION", false),

		DatabaseURL: getEnv("NOTIFICATION_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5438/notification_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("NOTIFICATION_REDIS_ADDR", "localhost:6385"),
			Password: os.Getenv("NOTIFICATION_REDIS_PASSWORD"),
			DB:       getEnvInt("NOTIFICATION_REDIS_DB", 0),
		},

		ServiceSecrets: parseServiceSecrets(os.Getenv("NOTIFICATION_SERVICE_SECRETS")),

		FirebaseProjectID:     os.Getenv("FIREBASE_PROJECT_ID"),
		GoogleCredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),

		EmailProvider: getEnv("NOTIFICATION_EMAIL_PROVIDER", "smtp"),
		SMTP: SMTPConfig{
			Host:     os.Getenv("NOTIFICATION_SMTP_HOST"),
			Port:     getEnvInt("NOTIFICATION_SMTP_PORT", 587),
			Username: os.Getenv("NOTIFICATION_SMTP_USERNAME"),
			Password: os.Getenv("NOTIFICATION_SMTP_PASSWORD"),
			TLSMode:  getEnv("NOTIFICATION_SMTP_TLS_MODE", "starttls"),
			From:     os.Getenv("NOTIFICATION_EMAIL_FROM"),
		},

		RealtimeTokenSecret: []byte(getEnv("NOTIFICATION_REALTIME_TOKEN_SECRET", "development-notification-realtime-token-secret")),
		WorkerConcurrency:   getEnvInt("NOTIFICATION_WORKER_CONCURRENCY", 5),
		MaxAttempts:         getEnvInt("NOTIFICATION_MAX_ATTEMPTS", 5),
		RequestStream:       getEnv("NOTIFICATION_REQUEST_STREAM", "notification:requests"),
		DeliveryStream:      getEnv("NOTIFICATION_DELIVERY_STREAM", "notification:deliveries"),
		DeadLetterStream:    getEnv("NOTIFICATION_DEAD_LETTER_STREAM", "notification:dead_letters"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
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

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func parseServiceSecrets(value string) map[string][]byte {
	secrets := map[string][]byte{}
	for _, part := range splitComma(value) {
		name, secret, ok := cutSecret(part)
		if ok && name != "" && secret != "" {
			secrets[name] = []byte(secret)
		}
	}
	return secrets
}

func splitComma(value string) []string {
	var parts []string
	start := 0
	for i, char := range value {
		if char == ',' {
			parts = append(parts, trimSpace(value[start:i]))
			start = i + 1
		}
	}
	if value != "" {
		parts = append(parts, trimSpace(value[start:]))
	}
	return parts
}

func cutSecret(value string) (string, string, bool) {
	for i, char := range value {
		if char == '=' || char == ':' {
			return trimSpace(value[:i]), trimSpace(value[i+1:]), true
		}
	}
	return "", "", false
}

func trimSpace(value string) string {
	start := 0
	for start < len(value) && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	end := len(value)
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}

func RetryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return 30 * time.Second
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<uint(attempt-1)) * 30 * time.Second
}
