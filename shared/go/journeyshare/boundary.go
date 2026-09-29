package journeyshare

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/sha256"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"time"
)

// RedisBoundary retains nonces across replicas/restarts; unavailable Redis fails closed.
func RedisBoundary(verifier *serviceauth.Verifier, client redis.UniversalClient, caller string) []gin.HandlerFunc {
	return []gin.HandlerFunc{serviceauth.Middleware(verifier), func(c *gin.Context) {
		if c.GetString("service_name") != caller {
			httpx.RespondError(c, apperrors.Forbidden("Caller is not allowed.", nil))
			c.Abort()
			return
		}
		nonce := c.GetHeader(serviceauth.HeaderNonce)
		if len(nonce) > 128 || client == nil {
			httpx.RespondError(c, apperrors.Unavailable("Replay protection is unavailable.", nil))
			c.Abort()
			return
		}
		hash := sha256.Sum256([]byte(caller + ":" + nonce))
		ok, err := client.SetNX(c.Request.Context(), "journeyshare:nonce:"+hex.EncodeToString(hash[:]), "1", 11*time.Minute).Result()
		if err != nil {
			httpx.RespondError(c, apperrors.Unavailable("Replay protection is unavailable.", nil))
			c.Abort()
			return
		}
		if !ok {
			httpx.RespondError(c, apperrors.New(http.StatusConflict, "replayed_request", "Request was already processed.", nil))
			c.Abort()
			return
		}
		c.Next()
	}}
}

type Limiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
}
type RedisLimiter struct{ Client redis.UniversalClient }

var limitScript = redis.NewScript(`local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return n`)

func (l RedisLimiter) Allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	if l.Client == nil {
		return false, apperrors.Unavailable("Rate limiting is unavailable.", nil)
	}
	n, err := limitScript.Run(ctx, l.Client, []string{"journeyshare:limit:" + key}, window.Milliseconds()).Int64()
	return n <= int64(max), err
}
