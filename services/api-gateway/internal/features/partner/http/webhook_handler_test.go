package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
)

// minimal in-package fake WebhookRepository for HTTP-level webhook tests.
type memWebhookRepo struct {
	hooks map[string]*webhooks.Webhook
}

func newMemWebhookRepo() *memWebhookRepo {
	return &memWebhookRepo{hooks: map[string]*webhooks.Webhook{}}
}

func (r *memWebhookRepo) CreateWebhook(_ context.Context, w *webhooks.Webhook) (*webhooks.Webhook, error) {
	cp := *w
	r.hooks[w.ID] = &cp
	return w, nil
}
func (r *memWebhookRepo) ListWebhooks(_ context.Context, partnerID string) ([]webhooks.Webhook, error) {
	var out []webhooks.Webhook
	for _, w := range r.hooks {
		if w.PartnerID == partnerID && w.Status == webhooks.StatusActive {
			out = append(out, *w)
		}
	}
	return out, nil
}
func (r *memWebhookRepo) DeleteWebhook(_ context.Context, partnerID, id string) error {
	if w, ok := r.hooks[id]; ok && w.PartnerID == partnerID {
		w.Status = webhooks.StatusInactive
	}
	return nil
}
func (r *memWebhookRepo) FindWebhookByPartner(_ context.Context, partnerID, id string) (*webhooks.Webhook, error) {
	if w, ok := r.hooks[id]; ok && w.PartnerID == partnerID {
		cp := *w
		return &cp, nil
	}
	return nil, nil
}
func (r *memWebhookRepo) FindWebhookByID(_ context.Context, id string) (*webhooks.Webhook, error) {
	if w, ok := r.hooks[id]; ok {
		cp := *w
		return &cp, nil
	}
	return nil, nil
}
func (r *memWebhookRepo) FindActiveWebhooksForEvent(_ context.Context, partnerID, event string) ([]webhooks.Webhook, error) {
	return nil, nil
}

func newRouterWithWebhooks(t *testing.T) *gin.Engine {
	t.Helper()
	cryptor, err := webhooks.NewCryptor("http-test-key")
	if err != nil {
		t.Fatalf("NewCryptor: %v", err)
	}
	svc := webhooks.NewService(webhooks.Deps{Webhooks: newMemWebhookRepo(), Cryptor: cryptor})
	deliverySvc, _ := newTestService()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPartnerRoutes(r, Dependencies{KeyRepo: newTestRepo(t), Deliveries: deliverySvc, Webhooks: svc})
	return r
}

func TestWebhookCreateReturnsSecretOnceAndListHidesIt(t *testing.T) {
	r := newRouterWithWebhooks(t)
	body := `{"url":"https://pharmacy.com/karrygo-webhook","events":["delivery.accepted","delivery.completed"]}`
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/webhooks", body, true)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /webhooks status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	var created struct {
		Data struct {
			SigningSecret string `json:"signing_secret"`
		} `json:"data"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(created.Data.SigningSecret, "whsec_") {
		t.Fatalf("signing_secret = %q, want whsec_ prefix", created.Data.SigningSecret)
	}
	if created.Note == "" {
		t.Fatal("expected a one-time note about saving the secret")
	}

	// GET /webhooks must never contain the secret.
	wl := requestBody(r, http.MethodGet, "/api/v1/partner/v1/webhooks", "", true)
	if wl.Code != http.StatusOK {
		t.Fatalf("GET /webhooks status = %d, want 200", wl.Code)
	}
	if strings.Contains(wl.Body.String(), "signing_secret") || strings.Contains(wl.Body.String(), created.Data.SigningSecret) {
		t.Fatalf("GET /webhooks leaked secret: %s", wl.Body.String())
	}
}

func TestWebhookCreateRejectsInsecureURL(t *testing.T) {
	r := newRouterWithWebhooks(t)
	for _, body := range []string{
		`{"url":"http://pharmacy.com/hook","events":["delivery.accepted"]}`,
		`{"url":"https://localhost/hook","events":["delivery.accepted"]}`,
		`{"url":"https://10.0.0.5/hook","events":["delivery.accepted"]}`,
		`{"url":"https://pharmacy.com/hook","events":["delivery.bogus"]}`,
	} {
		w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/webhooks", body, true)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("POST %s status = %d, want 400", body, w.Code)
		}
		var env struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &env)
		if env.Error.Code != "validation_error" {
			t.Fatalf("code = %q, want validation_error", env.Error.Code)
		}
	}
}

func TestWebhookRoutes501WhenDisabled(t *testing.T) {
	// nil Webhooks service -> routes fall back to 501 not_implemented.
	svc, _ := newTestService()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPartnerRoutes(r, Dependencies{KeyRepo: newTestRepo(t), Deliveries: svc})
	w := requestBody(r, http.MethodGet, "/api/v1/partner/v1/webhooks", "", true)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 when webhooks disabled", w.Code)
	}
}
