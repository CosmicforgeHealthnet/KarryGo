// Package campaignhttp mounts the private campaign contract and the owner
// admin-event snapshot at the router root. Both surfaces are default-off,
// HMAC v2 with a dedicated secret, pinned to exactly one caller, and refuse
// replayed nonces durably.
package campaignhttp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	campaignusecases "cosmicforge/logistics/services/notification-service/internal/features/campaigns/usecases"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	// Caller is the only service that may drive campaigns or read the snapshot.
	Caller       = "admin-backoffice-service"
	SnapshotPath = "/internal/notification/v1/admin-events/snapshot"
)

// LoadSecret reads a dedicated base64url secret of at least 32 bytes.
func LoadSecret(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("secret file is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("secret file is unreadable")
	}
	value := strings.TrimRight(strings.TrimSpace(string(raw)), "=")
	secret, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(secret) < 32 {
		return nil, errors.New("secret must be base64url of at least 32 bytes")
	}
	return secret, nil
}

// Boundary returns the middleware chain: flag, HMAC v2 under the dedicated
// secret (any other caller is forbidden), and durable nonce consumption.
func Boundary(enabled bool, secret []byte, repo *campaignrepositories.Repository) []gin.HandlerFunc {
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{Caller: secret}, 5*time.Minute)
	return []gin.HandlerFunc{
		func(c *gin.Context) {
			if !enabled || len(secret) < 32 || repo == nil {
				fail(c, apperrors.Unavailable("This capability is not enabled.", nil))
				return
			}
			c.Next()
		},
		serviceauth.Middleware(verifier),
		func(c *gin.Context) {
			if c.GetString("service_name") != Caller {
				fail(c, apperrors.Forbidden("Caller is not allowed.", nil))
				return
			}
			nonce := c.GetHeader(serviceauth.HeaderNonce)
			if nonce == "" || len(nonce) > 128 {
				fail(c, apperrors.Unauthorized("Service authentication is invalid.", nil))
				return
			}
			fresh, err := repo.ConsumeNonce(c.Request.Context(), Caller, nonce)
			if err != nil {
				fail(c, apperrors.Unavailable("Replay protection is unavailable.", nil))
				return
			}
			if !fresh {
				fail(c, apperrors.New(http.StatusConflict, "replayed_request", "Request was already processed.", nil))
				return
			}
			c.Next()
		},
	}
}

func fail(c *gin.Context, err error) {
	httpx.RespondError(c, err)
	c.Abort()
}

func respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, campaignusecases.ErrInvalid):
		fail(c, apperrors.Validation("Campaign request is invalid.", nil))
	case errors.Is(err, campaignusecases.ErrConflict):
		fail(c, apperrors.New(http.StatusConflict, "campaign_conflict", "Campaign revision conflicts with an accepted revision.", nil))
	case errors.Is(err, campaignusecases.ErrNotFound):
		fail(c, apperrors.NotFound("Campaign revision not found.", nil))
	default:
		fail(c, apperrors.Internal("Campaign request failed.", err))
	}
}

func body(c *gin.Context) ([]byte, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, admincampaign.MaxBody))
	if err != nil {
		fail(c, apperrors.Validation("Campaign request is invalid.", nil))
		return nil, false
	}
	return raw, true
}

func campaignID(c *gin.Context) (string, bool) {
	id := c.Param("campaign_id")
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		fail(c, apperrors.Validation("Campaign id is invalid.", nil))
		return "", false
	}
	return id, true
}

// RegisterCampaignRoutes mounts the admincampaign contract.
func RegisterCampaignRoutes(engine *gin.Engine, service *campaignusecases.Service, boundary []gin.HandlerFunc) {
	if engine == nil || service == nil {
		return
	}
	chain := func(h gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, boundary...), h)
	}
	engine.POST(admincampaign.IntentPath, chain(func(c *gin.Context) {
		raw, ok := body(c)
		if !ok {
			return
		}
		status, created, err := service.Accept(c.Request.Context(), raw)
		if err != nil {
			respond(c, err)
			return
		}
		code := http.StatusOK
		if created {
			code = http.StatusAccepted
		}
		c.JSON(code, gin.H{"success": true, "data": status})
	})...)
	engine.GET(admincampaign.IntentPath+"/:campaign_id/revisions/:revision", chain(func(c *gin.Context) {
		id, ok := campaignID(c)
		if !ok {
			return
		}
		rev, err := strconv.ParseInt(c.Param("revision"), 10, 64)
		if err != nil || rev < 1 {
			fail(c, apperrors.Validation("Revision is invalid.", nil))
			return
		}
		status, err := service.Status(c.Request.Context(), id, rev)
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
	})...)
	engine.POST(admincampaign.IntentPath+"/:campaign_id/cancellations", chain(func(c *gin.Context) {
		id, ok := campaignID(c)
		if !ok {
			return
		}
		raw, ok := body(c)
		if !ok {
			return
		}
		var cancel admincampaign.Cancellation
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if dec.Decode(&cancel) != nil {
			fail(c, apperrors.Validation("Cancellation is invalid.", nil))
			return
		}
		status, err := service.Cancel(c.Request.Context(), cancel, id)
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
	})...)
}

// RegisterSnapshotRoute mounts the owner admin-event snapshot.
func RegisterSnapshotRoute(engine *gin.Engine, service *campaignusecases.Service, boundary []gin.HandlerFunc) {
	if engine == nil || service == nil {
		return
	}
	engine.GET(SnapshotPath, append(append([]gin.HandlerFunc{}, boundary...), func(c *gin.Context) {
		limit := 200
		if v := c.Query("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 200 {
				fail(c, apperrors.Validation("limit must be between 1 and 200.", nil))
				return
			}
			limit = n
		}
		page, err := service.Snapshot(c.Request.Context(), c.Query("cursor"), limit)
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": page})
	})...)
}
