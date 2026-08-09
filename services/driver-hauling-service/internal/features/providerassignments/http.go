package providerassignments

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxResponseBytes = 1 << 20

type Client struct {
	baseURL, caller string
	secret          []byte
	http            *http.Client
}

func NewClient(baseURL, caller string, secret []byte, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), caller: caller,
		secret: secret, http: &http.Client{Timeout: timeout}}
}

func (c *Client) configured() bool {
	return c != nil && c.baseURL != "" && c.caller != "" && len(c.secret) > 0
}

type decisionRequest struct {
	ClientResponseID string `json:"clientResponseId"`
	ExpectedVersion  int64  `json:"expectedVersion"`
	ReasonCode       string `json:"reasonCode"`
	Note             string `json:"note"`
}

func (c *Client) relay(g *gin.Context, method, path string, body []byte, key string) {
	if !c.configured() {
		g.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "fleet_private_assignments_unavailable", "message": "Private assignments are unavailable."}})
		return
	}
	req, err := http.NewRequestWithContext(g.Request.Context(), method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		g.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "fleet_private_assignments_unavailable"}})
		return
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if err = serviceauth.SignRequest(req, c.caller, c.secret, body, time.Now()); err != nil {
		g.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "fleet_private_assignments_unavailable"}})
		return
	}
	resp, err := c.http.Do(req)
	if err != nil {
		g.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "fleet_private_assignments_unavailable"}})
		return
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		g.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "fleet_private_assignments_invalid_response"}})
		return
	}
	g.Data(resp.StatusCode, "application/json; charset=utf-8", payload)
}

func (c *Client) get(g *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(g)
	if !ok {
		g.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	path := "/internal/fleet-partner/v1/private-assignments/" + url.PathEscape(g.Param("assignmentId")) +
		"?providerId=" + url.QueryEscape(claims.Subject)
	c.relay(g, http.MethodGet, path, nil, "")
}

func (c *Client) decide(decision string) gin.HandlerFunc {
	return func(g *gin.Context) {
		claims, ok := sharedauth.ClaimsFromContext(g)
		if !ok {
			g.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		var input decisionRequest
		if err := g.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.ClientResponseID) == "" {
			g.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "validation_error", "message": "clientResponseId is required."}})
			return
		}
		body, _ := json.Marshal(map[string]any{"providerId": claims.Subject,
			"expectedVersion": input.ExpectedVersion, "reasonCode": input.ReasonCode, "note": input.Note})
		path := "/internal/fleet-partner/v1/private-assignments/" + url.PathEscape(g.Param("assignmentId")) + "/" + decision
		c.relay(g, http.MethodPost, path, body, input.ClientResponseID)
	}
}

func Register(group *gin.RouterGroup, authService *providerauthusecases.AuthService, client *Client) {
	provider := group.Group("/provider/company-assignments")
	provider.Use(sharedauth.BearerMiddleware(authService.AccessSigner(),
		providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	provider.GET("/:assignmentId", client.get)
	provider.POST("/:assignmentId/accept", client.decide("accept"))
	provider.POST("/:assignmentId/decline", client.decide("decline"))
}
