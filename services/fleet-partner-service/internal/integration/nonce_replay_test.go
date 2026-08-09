package integration_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
)

const nonceReplayCallerSecret = "nonce-replay-route-test-service-secret-value"

// nonceReplayRouter builds the real internal router with both
// InternalServiceAuth and RejectReplayedNonce wired in front of a trivial
// echo route — proving the guard as the production middleware chain
// actually assembles it (internal/app/app.go), not a hand-built stand-in.
func nonceReplayRouter(t *testing.T) http.Handler {
	t.Helper()
	pool := testPool(t)
	gin.SetMode(gin.TestMode)
	return app.BuildRouter(app.Options{
		Config:      config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20},
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner([]byte("unused-nonce-replay-user-token-secret")),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			"driver-dispatch-delivery-service": []byte(nonceReplayCallerSecret),
		}, time.Minute),
		NonceStore: noncestore.New(pool),
		InternalRoutes: func(group *gin.RouterGroup) {
			group.POST("/echo", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
			})
		},
	})
}

func signedEcho(t *testing.T, router http.Handler, timestamp time.Time) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/fleet-partner/v1/echo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(req, "driver-dispatch-delivery-service",
		[]byte(nonceReplayCallerSecret), body, timestamp); err != nil {
		t.Fatalf("sign: %v", err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// TestNonceReplay_IdenticalRequestRejectedSecondTime proves the actual gap
// this WP closes: shared/go/serviceauth.Verifier alone accepts a validly
// signed request every time it is presented, including a byte-for-byte
// replay of a request already processed. With RejectReplayedNonce wired in
// (as internal/app/app.go now does for every /internal/fleet-partner/v1
// route), the second delivery of the exact same signed request is refused.
func TestNonceReplay_IdenticalRequestRejectedSecondTime(t *testing.T) {
	router := nonceReplayRouter(t)
	now := time.Now()

	first := signedEcho(t, router, now)
	if first.Code != http.StatusOK {
		t.Fatalf("first delivery status = %d, body = %s", first.Code, first.Body.String())
	}

	// A fresh SignRequest call mints a fresh random nonce internally, so to
	// prove replay rejection this must resend the EXACT same signed request
	// headers a call already sent, not a newly signed one.
	req := httptest.NewRequest(http.MethodPost, "/internal/fleet-partner/v1/echo", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(req, "driver-dispatch-delivery-service",
		[]byte(nonceReplayCallerSecret), []byte(`{}`), now); err != nil {
		t.Fatalf("sign shared request: %v", err)
	}
	// Replay the identical header set the first request actually carried
	// (same nonce, same timestamp, same signature) rather than a freshly
	// signed one.
	replayReq := httptest.NewRequest(http.MethodPost, "/internal/fleet-partner/v1/echo",
		bytes.NewReader([]byte(`{}`)))
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set(serviceauth.HeaderServiceName, req.Header.Get(serviceauth.HeaderServiceName))
	replayReq.Header.Set(serviceauth.HeaderTimestamp, req.Header.Get(serviceauth.HeaderTimestamp))
	replayReq.Header.Set(serviceauth.HeaderVersion, req.Header.Get(serviceauth.HeaderVersion))
	replayReq.Header.Set(serviceauth.HeaderNonce, req.Header.Get(serviceauth.HeaderNonce))
	replayReq.Header.Set(serviceauth.HeaderSignature, req.Header.Get(serviceauth.HeaderSignature))

	firstOfPair := httptest.NewRecorder()
	router.ServeHTTP(firstOfPair, req)
	if firstOfPair.Code != http.StatusOK {
		t.Fatalf("first of exact-pair status = %d, body = %s", firstOfPair.Code, firstOfPair.Body.String())
	}

	secondOfPair := httptest.NewRecorder()
	router.ServeHTTP(secondOfPair, replayReq)
	if secondOfPair.Code != http.StatusConflict {
		t.Fatalf("replayed request status = %d, body = %s, want 409", secondOfPair.Code, secondOfPair.Body.String())
	}
}

// TestNonceReplay_DifferentNoncesAreIndependentlyAccepted proves the guard
// tracks (caller, nonce) pairs, not just the caller: two genuinely distinct
// signed requests from the same caller both succeed.
func TestNonceReplay_DifferentNoncesAreIndependentlyAccepted(t *testing.T) {
	router := nonceReplayRouter(t)
	first := signedEcho(t, router, time.Now())
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, body = %s", first.Code, first.Body.String())
	}
	second := signedEcho(t, router, time.Now())
	if second.Code != http.StatusOK {
		t.Fatalf("second request status = %d, body = %s", second.Code, second.Body.String())
	}
}
