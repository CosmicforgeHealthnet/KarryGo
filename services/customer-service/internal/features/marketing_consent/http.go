package marketingconsent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	// Root-mounted private routes (the /internal/<domain>/v1 convention).
	AudiencePath = "/internal/customer/v1/marketing-audience"
	CheckPath    = "/internal/customer/v1/marketing-consent/check"
	// AudienceCaller is the only service allowed to read the audience.
	AudienceCaller = "notification-service"
)

// Flags gate capture and the internal audience. Both default off. Withdrawal
// and reading one's own consent are always available: a customer can always
// take consent back.
type Flags struct {
	CaptureEnabled  bool
	AudienceEnabled bool
}

func customerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || claims.Type != sharedauth.TokenTypeAccess || claims.Role != "customer" || claims.Service != "customer" || strings.TrimSpace(claims.Subject) == "" {
		fail(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(claims.Subject), true
}

func respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		fail(c, apperrors.Validation("Check your consent details.", nil))
	case errors.Is(err, ErrNotFound):
		fail(c, apperrors.NotFound("Customer could not be found.", nil))
	case errors.Is(err, ErrUnavailable):
		fail(c, apperrors.Unavailable("Marketing consent is unavailable.", nil))
	default:
		fail(c, apperrors.Internal("Marketing consent could not be processed.", err))
	}
}

// RegisterCustomerRoutes mounts GET/PUT /consents/marketing behind customer
// bearer auth.
func RegisterCustomerRoutes(root *gin.RouterGroup, store *Store, signer *sharedauth.TokenSigner, flags Flags) {
	if root == nil || store == nil || signer == nil {
		return
	}
	g := root.Group("/consents/marketing")
	g.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	g.GET("", func(c *gin.Context) {
		id, ok := customerID(c)
		if !ok {
			return
		}
		list, err := store.List(c.Request.Context(), id)
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"consents": list}})
	})
	g.PUT("", func(c *gin.Context) {
		id, ok := customerID(c)
		if !ok {
			return
		}
		var body struct {
			Channel string  `json:"channel"`
			OptedIn *bool   `json:"opted_in"`
			Market  *string `json:"market"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10))
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err != nil || dec.Decode(&body) != nil || body.OptedIn == nil {
			fail(c, apperrors.Validation("Check your consent details.", []apperrors.FieldViolation{{Field: "opted_in", Message: "Channel and opted_in are required."}}))
			return
		}
		if *body.OptedIn && !flags.CaptureEnabled {
			fail(c, apperrors.Unavailable("Marketing consent capture is not enabled.", nil))
			return
		}
		consent, changed, err := store.Set(c.Request.Context(), SetInput{CustomerID: id, Channel: strings.TrimSpace(body.Channel), OptedIn: *body.OptedIn, Market: body.Market, Now: time.Now().UTC()})
		if err != nil {
			respond(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"consent": consent, "changed": changed}})
	})
}

// RegisterInternalRoutes mounts the audience and eligibility reads at the
// router root. boundary must authenticate HMAC v2, pin the caller to
// notification-service and refuse replayed nonces.
func RegisterInternalRoutes(engine *gin.Engine, store *Store, flags Flags, boundary []gin.HandlerFunc) {
	if engine == nil || store == nil || len(boundary) == 0 {
		return
	}
	enabled := func(c *gin.Context) {
		if !flags.AudienceEnabled {
			httpx.RespondError(c, apperrors.Unavailable("Marketing audience is not enabled.", nil))
			c.Abort()
			return
		}
		c.Next()
	}
	chain := func(h gin.HandlerFunc) []gin.HandlerFunc {
		return append(append(append([]gin.HandlerFunc{}, boundary...), enabled), h)
	}
	engine.GET(AudiencePath, chain(func(c *gin.Context) {
		limit := MaxPage
		if v := c.Query("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > MaxPage {
				httpx.RespondError(c, apperrors.Validation("limit must be between 1 and 500.", nil))
				return
			}
			limit = n
		}
		page, err := store.Audience(c.Request.Context(), c.Query("channel"), c.Query("market"), c.Query("cursor"), limit)
		if err != nil {
			respondInternal(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": page})
	})...)
	engine.POST(CheckPath, chain(func(c *gin.Context) {
		var body struct {
			Channel     string   `json:"channel"`
			CustomerIDs []string `json:"customer_ids"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err != nil || dec.Decode(&body) != nil {
			httpx.RespondError(c, apperrors.Validation("Check request is invalid.", nil))
			return
		}
		res, err := store.Check(c.Request.Context(), body.Channel, body.CustomerIDs)
		if err != nil {
			respondInternal(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
	})...)
}

func respondInternal(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.RespondError(c, apperrors.Validation("Request is invalid.", nil))
	case errors.Is(err, ErrUnavailable):
		httpx.RespondError(c, apperrors.Unavailable("Marketing consent is unavailable.", nil))
	default:
		httpx.RespondError(c, apperrors.Internal("Marketing consent could not be read.", err))
	}
}

// fail renders the platform error envelope and stops the chain, independent
// of whether the service mounted the shared error middleware.
func fail(c *gin.Context, err error) {
	httpx.RespondError(c, err)
	c.Abort()
}
