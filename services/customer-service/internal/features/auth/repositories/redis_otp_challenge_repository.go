package authrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	authmodels "karrygo/services/customer-service/internal/features/auth/models"
	"karrygo/shared/go/apperrors"
)

type OTPChallengeRepository interface {
	Save(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration, rateWindow time.Duration, maxRequests int) error
	Get(ctx context.Context, phone string) (authmodels.OTPChallenge, bool, error)
	RecordFailedAttempt(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration) error
	Delete(ctx context.Context, phone string) error
}

type RedisOTPChallengeRepository struct {
	client *redis.Client
}

func NewRedisOTPChallengeRepository(client *redis.Client) *RedisOTPChallengeRepository {
	return &RedisOTPChallengeRepository{client: client}
}

func (r *RedisOTPChallengeRepository) Save(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration, rateWindow time.Duration, maxRequests int) error {
	rateKey := rateKey(challenge.Phone)
	count, err := r.client.Incr(ctx, rateKey).Result()
	if err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}
	if count == 1 {
		if err := r.client.Expire(ctx, rateKey, rateWindow).Err(); err != nil {
			return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
		}
	}
	if count > int64(maxRequests) {
		return apperrors.RateLimited("Too many attempts. Please try again shortly.", nil)
	}

	payload, err := json.Marshal(challenge)
	if err != nil {
		return apperrors.Internal("OTP challenge could not be prepared.", err)
	}
	if err := r.client.Set(ctx, challengeKey(challenge.Phone), payload, ttl).Err(); err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}

	return nil
}

func (r *RedisOTPChallengeRepository) Get(ctx context.Context, phone string) (authmodels.OTPChallenge, bool, error) {
	value, err := r.client.Get(ctx, challengeKey(phone)).Result()
	if errors.Is(err, redis.Nil) {
		return authmodels.OTPChallenge{}, false, nil
	}
	if err != nil {
		return authmodels.OTPChallenge{}, false, apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}

	var challenge authmodels.OTPChallenge
	if err := json.Unmarshal([]byte(value), &challenge); err != nil {
		return authmodels.OTPChallenge{}, false, apperrors.Internal("OTP challenge could not be read.", err)
	}

	return challenge, true, nil
}

func (r *RedisOTPChallengeRepository) RecordFailedAttempt(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration) error {
	challenge.Attempts++
	payload, err := json.Marshal(challenge)
	if err != nil {
		return apperrors.Internal("OTP challenge could not be prepared.", err)
	}
	if err := r.client.Set(ctx, challengeKey(challenge.Phone), payload, ttl).Err(); err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}

	return nil
}

func (r *RedisOTPChallengeRepository) Delete(ctx context.Context, phone string) error {
	if err := r.client.Del(ctx, challengeKey(phone)).Err(); err != nil {
		return apperrors.Unavailable("OTP service is temporarily unavailable.", err)
	}
	return nil
}

func challengeKey(phone string) string {
	return fmt.Sprintf("customer:auth:otp:%s", phone)
}

func rateKey(phone string) string {
	return fmt.Sprintf("customer:auth:otp-rate:%s", phone)
}
