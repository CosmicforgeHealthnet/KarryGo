// Package financereporthttp serves the private Section 26 group finance
// report to admin-backoffice-service only.
//
// The boundary: feature flag (off -> 503), exactly-one HMAC v2 headers signed
// with a dedicated secret by exactly admin-backoffice-service (other callers
// -> 403, bad signature -> 401), a durable single-use nonce (replay -> 409),
// GET only with no body, Cache-Control: no-store. The response carries
// per-currency aggregates only; it never names a customer, provider,
// organization or account.
package financereporthttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	usecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/usecases"
	"cosmicforge/logistics/shared/go/financereport"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	Caller       = "admin-backoffice-service"
	EnvEnabled   = "PAYMENT_WALLET_ADMIN_FINANCE_REPORT_ENABLED"
	EnvSecret    = "PAYMENT_WALLET_ADMIN_FINANCE_REPORT_SECRET_FILE"
	NonceOwner   = "payment-wallet-service:finance-report"
	maxClockSkew = 30 * time.Second
	nonceTTL     = "11 minutes"
)

var ErrConfiguration = errors.New("invalid admin finance report configuration")

type Reporter interface {
	Report(ctx context.Context, req usecases.Request) (financereport.Report, error)
}

type Handler struct {
	Enabled  bool
	Secret   []byte
	DB       *pgxpool.Pool
	Reporter Reporter
}

// FromEnvironment builds the handler. With the flag off it returns a handler
// that answers 503 and reads no secret. With it on, the dedicated secret file
// must hold a base64url (raw) secret of at least 32 bytes that differs from
// every other PAYMENT_WALLET_ADMIN_*_SECRET_FILE the service loads.
func FromEnvironment(db *pgxpool.Pool, reporter Reporter) (*Handler, error) {
	h := &Handler{DB: db, Reporter: reporter, Enabled: os.Getenv(EnvEnabled) == "true"}
	if !h.Enabled {
		return h, nil
	}
	if db == nil || reporter == nil {
		return nil, ErrConfiguration
	}
	secret, err := secretFile(os.Getenv(EnvSecret))
	if err != nil {
		return nil, err
	}
	for _, entry := range os.Environ() {
		name, path, ok := strings.Cut(entry, "=")
		if !ok || name == EnvSecret || path == "" || !strings.HasPrefix(name, "PAYMENT_WALLET_ADMIN_") || !strings.HasSuffix(name, "_SECRET_FILE") {
			continue
		}
		if other, otherErr := secretFile(path); otherErr == nil && bytes.Equal(other, secret) {
			return nil, ErrConfiguration
		}
	}
	h.Secret = secret
	return h, nil
}

func secretFile(path string) ([]byte, error) {
	if path == "" {
		return nil, ErrConfiguration
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, ErrConfiguration
	}
	value := strings.NewReplacer("\n", "", "\r", "").Replace(string(raw))
	secret, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(secret) < 32 {
		return nil, ErrConfiguration
	}
	return secret, nil
}

// Register mounts the route at its contract path on the root router.
func (h *Handler) Register(router *gin.Engine) {
	router.GET(financereport.Path, h.Serve)
}

func (h *Handler) fail(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code}})
}

func (h *Handler) Serve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.Enabled || len(h.Secret) < 32 || h.DB == nil || h.Reporter == nil {
		h.fail(c, http.StatusServiceUnavailable, "finance_report_disabled")
		return
	}
	r := c.Request
	if r.Method != http.MethodGet || r.URL.RawPath != "" || len(r.URL.RequestURI()) > 2048 || r.ContentLength > 0 || len(r.TransferEncoding) > 0 {
		h.fail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			h.fail(c, http.StatusUnauthorized, "unauthorized")
			return
		}
	}
	if r.Header.Get(serviceauth.HeaderServiceName) != Caller {
		h.fail(c, http.StatusForbidden, "caller_not_allowed")
		return
	}
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{Caller: h.Secret}, maxClockSkew)
	if _, err := verifier.VerifyRequest(r); err != nil {
		h.fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if len(nonce) < 16 || len(nonce) > 128 {
		h.fail(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	nonceHash := sha256.Sum256([]byte(NonceOwner + ":" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at) VALUES($1,$2,now()+interval '`+nonceTTL+`') ON CONFLICT DO NOTHING`, NonceOwner, hex.EncodeToString(nonceHash[:]))
	if err != nil {
		h.fail(c, http.StatusServiceUnavailable, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(c, http.StatusConflict, "replay_rejected")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(c, http.StatusBadRequest, "invalid_query")
		return
	}
	req, err := usecases.ParseRequest(values)
	if err != nil {
		h.fail(c, http.StatusBadRequest, "invalid_query")
		return
	}
	report, err := h.Reporter.Report(ctx, req)
	if err != nil {
		slog.Error("admin_finance_report_unavailable", "error_class", errorClass(err))
		h.fail(c, http.StatusServiceUnavailable, "finance_report_unavailable")
		return
	}
	c.JSON(http.StatusOK, report)
}

func errorClass(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "source_read_failed"
}
