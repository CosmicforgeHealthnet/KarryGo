package integration_test

import (
	"context"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"net/http"
	"sync"
	"testing"
)

func TestBrowserLogoutSerializesWithFamilyRotation(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	id, code := h.startAndCode(t, "logout-race@example.com", "08012345680")
	issued, err := h.service.Verify(ctx, verifyInputFor(id, code))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	var logoutErr error
	go func() {
		defer wg.Done()
		_, _ = h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: issued.Tokens.RefreshToken})
	}()
	go func() {
		defer wg.Done()
		logoutErr = h.service.LogoutRefresh(ctx, issued.Tokens.RefreshToken, "req", "correlation")
	}()
	wg.Wait()
	if logoutErr != nil {
		t.Fatal("logout failed")
	}
	var active int
	if err = h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_sessions WHERE identity_id=$1 AND status='ACTIVE'`, issued.IdentityID).Scan(&active); err != nil || active != 0 {
		t.Fatal("logout left an active successor")
	}
}

func TestBrowserRefreshCookieRotationAndReplay(t *testing.T) {
	// Never extract refresh secrets from JSON; only the simulated browser cookie jar.
	router, h := buildAuthRouter(t)
	id, code := h.startAndCode(t, "browserrotate@example.com", "08012345679")
	verified := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{"challengeId": id, "code": code}, nil)
	if verified.Code != 200 {
		t.Fatalf("verify status = %d; body=%s", verified.Code, verified.Body.String())
	}
	first := findCookie(t, verified.Result().Cookies(), "karrygo_fleet_refresh")
	out := postJSON(router, "/fleet-partner/v1/auth/refresh", nil, map[string]string{"Cookie": first.Name + "=" + first.Value})
	if out.Code != 200 {
		t.Fatalf("refresh status %d; body=%s", out.Code, out.Body.String())
	}
	second := findCookie(t, out.Result().Cookies(), "karrygo_fleet_refresh")
	if second.Value == first.Value {
		t.Fatal("not rotated")
	}
	replay := postJSON(router, "/fleet-partner/v1/auth/refresh", nil, map[string]string{"Cookie": first.Name + "=" + first.Value})
	if replay.Code != http.StatusUnauthorized {
		t.Fatal("old cookie replay accepted")
	}
	for _, cookie := range []string{"", "karrygo_fleet_refresh=invalid", second.Name + "=" + second.Value} {
		out = postJSON(router, "/fleet-partner/v1/auth/refresh", nil, map[string]string{"Cookie": cookie})
		if out.Code != 401 {
			t.Fatal("missing/invalid/replay-revoked cookie accepted")
		}
	}
}