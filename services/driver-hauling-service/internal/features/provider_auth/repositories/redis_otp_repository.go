package providerauthrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type OTPChallengeRepository interface {
	Save(ctx context.Context, challenge providerauthmodels.OTPChallenge, ttl, rateWindow time.Duration, maxRequests int) error
	Get(ctx context.Context, identifierKey string) (providerauthmodels.OTPChallenge, bool, error)
	RecordFailedAttempt(ctx context.Context, challenge providerauthmodels.OTPChallenge, ttl time.Duration) error
	Delete(ctx context.Context, identifierKey string) error
}

type RedisOTPChallengeRepository struct {
	client *redis.Client
}

func NewRedisOTPChallengeRepository(client *redis.Client) *RedisOTPChallengeRepository {
	return &RedisOTPChallengeRepository{client: client}
}

func (r *RedisOTPChallengeRepository) Save(ctx context.Context, challenge providerauthmodels.OTPChallenge, ttl, rateWindow time.Duration, maxRequests int) error {
	rk := rateKey(challenge.IdentifierKey())
	count, err := r.client.Incr(ctx, rk).Result()
	if err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}
	if count == 1 {
		_ = r.client.Expire(ctx, rk, rateWindow)
	}
	if count > int64(maxRequests) {
		return apperrors.RateLimited("Too many OTP requests. Please wait before trying again.", nil)
	}

	payload, err := json.Marshal(challenge)
	if err != nil {
		return apperrors.Internal("OTP challenge could not be prepared.", err)
	}
	if err := r.client.Set(ctx, challengeKey(challenge.IdentifierKey()), payload, ttl).Err(); err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}
	return nil
}

func (r *RedisOTPChallengeRepository) Get(ctx context.Context, identifierKey string) (providerauthmodels.OTPChallenge, bool, error) {
	value, err := r.client.Get(ctx, challengeKey(identifierKey)).Result()
	if errors.Is(err, redis.Nil) {
		return providerauthmodels.OTPChallenge{}, false, nil
	}
	if err != nil {
		return providerauthmodels.OTPChallenge{}, false, apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}

	var challenge providerauthmodels.OTPChallenge
	if err := json.Unmarshal([]byte(value), &challenge); err != nil {
		return providerauthmodels.OTPChallenge{}, false, apperrors.Internal("OTP challenge could not be read.", err)
	}
	return challenge, true, nil
}

func (r *RedisOTPChallengeRepository) RecordFailedAttempt(ctx context.Context, challenge providerauthmodels.OTPChallenge, ttl time.Duration) error {
	challenge.Attempts++
	payload, err := json.Marshal(challenge)
	if err != nil {
		return apperrors.Internal("OTP challenge could not be updated.", err)
	}
	if err := r.client.Set(ctx, challengeKey(challenge.IdentifierKey()), payload, ttl).Err(); err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}
	return nil
}

func (r *RedisOTPChallengeRepository) Delete(ctx context.Context, identifierKey string) error {
	return r.client.Del(ctx, challengeKey(identifierKey)).Err()
}

func challengeKey(identifierKey string) string {
	return fmt.Sprintf("hauling:provider:auth:otp:%s", identifierKey)
}

func rateKey(identifierKey string) string {
	return fmt.Sprintf("hauling:provider:auth:otp-rate:%s", identifierKey)
}
