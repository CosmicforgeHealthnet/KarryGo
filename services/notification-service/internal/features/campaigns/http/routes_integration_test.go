package campaignhttp_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/notification-service/internal/features/campaigns/campaigntest"
	campaignhttp "cosmicforge/logistics/services/notification-service/internal/features/campaigns/http"
	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	campaignusecases "cosmicforge/logistics/services/notification-service/internal/features/campaigns/usecases"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

var (
	secret      = []byte("campaign-route-secret-0123456789abcdef")
	snapSecret  = []byte("snapshot-route-secret-0123456789abcdef")
	otherSecret = []byte("other-service-secret-0123456789abcdefgh")
)

type api struct {
	t      *testing.T
	engine *gin.Engine
}

func (a api) call(caller string, key []byte, nonce, method, target string, body []byte) (int, map[string]any) {
	a.t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set(serviceauth.HeaderNonce, nonce)
	if err := serviceauth.SignRequest(req, caller, key, body, time.Now()); err != nil {
		a.t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	a.engine.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func build(t *testing.T, campaignsOn, snapshotOn bool) (api, *campaignrepositories.Repository) {
	gin.SetMode(gin.TestMode)
	db := campaigntest.Pool(t)
	repo := campaignrepositories.New(db)
	service := &campaignusecases.Service{Repo: repo}
	e := gin.New()
	e.Use(httpx.ErrorHandler())
	campaignhttp.RegisterCampaignRoutes(e, service, campaignhttp.Boundary(campaignsOn, secret, repo))
	campaignhttp.RegisterSnapshotRoute(e, service, campaignhttp.Boundary(snapshotOn, snapSecret, repo))
	return api{t, e}, repo
}

func TestCampaignRoutesSecurityIdempotencyAndConflicts(t *testing.T) {
	a, _ := build(t, true, true)
	intent := campaigntest.Intent("marketing.route", 1, strings.Repeat("b", 64), []string{"NG-LAG"}, []string{"push"})
	raw := campaigntest.Raw(t, intent)
	const caller = "admin-backoffice-service"

	if code, _ := a.call("vendor-service", otherSecret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath, raw); code != http.StatusForbidden {
		t.Fatalf("other caller: %d", code)
	}
	if code, _ := a.call(caller, otherSecret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath, raw); code != http.StatusUnauthorized {
		t.Fatalf("wrong secret: %d", code)
	}
	nonce := uuid.NewString()
	code, out := a.call(caller, secret, nonce, http.MethodPost, admincampaign.IntentPath, raw)
	if code != http.StatusAccepted || out["data"].(map[string]any)["state"] != "ACCEPTED_BY_KARRYGO" || out["data"].(map[string]any)["delivered_count"] != nil {
		t.Fatalf("accept: %d %v", code, out)
	}
	if code, _ := a.call(caller, secret, nonce, http.MethodPost, admincampaign.IntentPath, raw); code != http.StatusConflict {
		t.Fatalf("replayed nonce: %d", code)
	}
	if code, _ := a.call(caller, secret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath, raw); code != http.StatusOK {
		t.Fatalf("idempotent replay: %d", code)
	}
	changed := intent
	changed.MaxMessages = 9
	if code, out := a.call(caller, secret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath, campaigntest.Raw(t, changed)); code != http.StatusConflict || out["error"].(map[string]any)["code"] != "campaign_conflict" {
		t.Fatalf("conflicting reuse: %d %v", code, out)
	}
	emergency := intent
	emergency.Category, emergency.IdempotencyKey = "EMERGENCY", "idem-"+uuid.NewString()
	if code, _ := a.call(caller, secret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath, campaigntest.Raw(t, emergency)); code != http.StatusUnprocessableEntity {
		t.Fatalf("emergency category: %d", code)
	}
	status := admincampaign.IntentPath + "/" + intent.CampaignID + "/revisions/1"
	if code, out := a.call(caller, secret, uuid.NewString(), http.MethodGet, status, nil); code != http.StatusOK || out["data"].(map[string]any)["delivery_confirmation"] != "UNAVAILABLE" {
		t.Fatalf("status: %d %v", code, out)
	}
	if code, _ := a.call(caller, secret, uuid.NewString(), http.MethodGet, admincampaign.IntentPath+"/"+intent.CampaignID+"/revisions/9", nil); code != http.StatusNotFound {
		t.Fatalf("missing revision: %d", code)
	}
	cancel, _ := json.Marshal(admincampaign.Cancellation{ContractVersion: admincampaign.Contract, CampaignID: intent.CampaignID, Revision: 1, CancellationKey: "c-" + uuid.NewString(), RequestedBy: "staff-ops", ReasonCode: "STOP"})
	if code, out := a.call(caller, secret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath+"/"+intent.CampaignID+"/cancellations", cancel); code != http.StatusOK || out["data"].(map[string]any)["state"] != "CANCELLED" {
		t.Fatalf("cancel: %d %v", code, out)
	}
	// Snapshot uses its own dedicated secret.
	if code, _ := a.call(caller, secret, uuid.NewString(), http.MethodGet, campaignhttp.SnapshotPath, nil); code != http.StatusUnauthorized {
		t.Fatalf("snapshot with campaign secret: %d", code)
	}
	code, out = a.call(caller, snapSecret, uuid.NewString(), http.MethodGet, campaignhttp.SnapshotPath+"?limit=50", nil)
	if code != http.StatusOK {
		t.Fatalf("snapshot: %d %v", code, out)
	}
	raw, _ = json.Marshal(out["data"])
	var page adminevents.SnapshotPage
	if err := json.Unmarshal(raw, &page); err != nil || page.Validate("notification-service") != nil || len(page.Events) != 1 {
		t.Fatalf("snapshot page invalid: %v %+v", err, page)
	}
}

func TestDisabledRoutesAnswer503(t *testing.T) {
	a, _ := build(t, false, false)
	raw := campaigntest.Raw(t, campaigntest.Intent("k", 1, strings.Repeat("c", 64), []string{"NG-LAG"}, []string{"push"}))
	if code, _ := a.call("admin-backoffice-service", secret, uuid.NewString(), http.MethodPost, admincampaign.IntentPath, raw); code != http.StatusServiceUnavailable {
		t.Fatalf("campaigns disabled: %d", code)
	}
	if code, _ := a.call("admin-backoffice-service", snapSecret, uuid.NewString(), http.MethodGet, campaignhttp.SnapshotPath, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("snapshot disabled: %d", code)
	}
}

func TestLoadSecretRequiresDedicatedStrongSecret(t *testing.T) {
	dir := t.TempDir()
	write := func(name, value string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	if s, err := campaignhttp.LoadSecret(write("good", good+"\n")); err != nil || len(s) != 32 {
		t.Fatalf("good secret: %v", err)
	}
	for name, value := range map[string]string{"short": base64.RawURLEncoding.EncodeToString([]byte("short")), "plain": "not base64 !!"} {
		if _, err := campaignhttp.LoadSecret(write(name, value)); err == nil {
			t.Fatalf("%s secret accepted", name)
		}
	}
	if _, err := campaignhttp.LoadSecret(""); err == nil {
		t.Fatal("missing file accepted")
	}
}
