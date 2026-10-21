package partneraccount

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Pause at the repository mutation boundary, not at an arbitrary wall-clock
// delay. This forces both orders of the exact digest-change/revocation race.
type revocationBarrierRepo struct {
	SessionRepository
	rotateEntered, rotateRelease chan struct{}
	revokeEntered, revokeRelease chan struct{}
}

func (r *revocationBarrierRepo) Rotate(ctx context.Context, old string, next CreateSessionParams, now time.Time) (bool, error) {
	if r.rotateEntered != nil {
		close(r.rotateEntered)
		<-r.rotateRelease
	}
	return r.SessionRepository.Rotate(ctx, old, next, now)
}

func (r *revocationBarrierRepo) RevokeSession(ctx context.Context, id string) (bool, error) {
	if r.revokeEntered != nil {
		close(r.revokeEntered)
		<-r.revokeRelease
	}
	return r.SessionRepository.RevokeSession(ctx, id)
}

func TestRefreshLogoutDeterministicRace(t *testing.T) {
	for _, refreshFirst := range []bool{true, false} {
		name := "logout_wins"
		if refreshFirst {
			name = "refresh_wins"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness()
			issued := h.mustRegister(t)
			ctx := context.Background()
			barrier := &revocationBarrierRepo{SessionRepository: h.sessions}
			h.svc.sessions = barrier
			var replacement *SessionResult
			if refreshFirst {
				barrier.revokeEntered, barrier.revokeRelease = make(chan struct{}), make(chan struct{})
				done := make(chan error, 1)
				go func() { done <- h.svc.LogoutRefresh(ctx, issued.RefreshToken) }()
				<-barrier.revokeEntered // Logout has authenticated H1 but not revoked yet.
				var err error
				replacement, err = h.svc.Refresh(ctx, issued.RefreshToken, RequestMeta{})
				close(barrier.revokeRelease) // H1 is now gone; stable session must still be revoked.
				logoutErr := <-done
				if err != nil || logoutErr != nil {
					t.Fatal("refresh-first interleaving failed")
				}
			} else {
				barrier.rotateEntered, barrier.rotateRelease = make(chan struct{}), make(chan struct{})
				done := make(chan error, 1)
				go func() { _, err := h.svc.Refresh(ctx, issued.RefreshToken, RequestMeta{}); done <- err }()
				<-barrier.rotateEntered // Refresh checked active H1, but CAS has not run.
				logoutErr := h.svc.LogoutRefresh(ctx, issued.RefreshToken)
				close(barrier.rotateRelease)
				refreshErr := <-done
				if logoutErr != nil {
					t.Fatal("logout failed")
				}
				wantErr(t, refreshErr, "invalid_session", 401)
			}
			h.svc.sessions = h.sessions
			if err := h.svc.LogoutRefresh(ctx, issued.RefreshToken); err != nil {
				t.Fatal("repeated logout failed")
			}
			for _, session := range h.sessions.byHash {
				if session.RevokedAt == nil {
					t.Fatal("active successor survived logout")
				}
			}
			if _, err := h.svc.Refresh(ctx, issued.RefreshToken, RequestMeta{}); err == nil {
				t.Fatal("old credential authenticates")
			}
			if replacement != nil {
				if _, err := h.svc.Refresh(ctx, replacement.RefreshToken, RequestMeta{}); err == nil {
					t.Fatal("successor authenticates after logout")
				}
			}
		})
	}
}

func TestRefreshStableCorrelationRequiresAuthenticCredential(t *testing.T) {
	h := newHarness()
	issued := h.mustRegister(t)
	ctx := context.Background()
	id, secret, ok := h.svc.parseRefreshCredential(issued.RefreshToken)
	if !ok {
		t.Fatal("invalid issued credential")
	}
	stored, _ := h.sessions.FindByRefreshHash(ctx, hashToken(secret))
	if stored == nil || stored.ID != id || stored.RefreshHash == secret {
		t.Fatal("digest or stable identity missing")
	}
	for _, raw := range []string{id + ".forged", strings.Replace(issued.RefreshToken, id, uuid.NewString(), 1), strings.Replace(issued.RefreshToken, secret, strings.Repeat("a", 43), 1)} {
		wantErr(t, h.svc.LogoutRefresh(ctx, raw), "invalid_session", 401)
	}
	// An authentic, but never-persisted credential cannot falsely certify logout.
	unknown, _, err := h.svc.newCredentials(issued.Account.ID, "", RequestMeta{})
	if err != nil {
		t.Fatal("credential generation failed")
	}
	wantErr(t, h.svc.LogoutRefresh(ctx, unknown.RefreshToken), "invalid_session", 401)
	next, err := h.svc.Refresh(ctx, issued.RefreshToken, RequestMeta{})
	if err != nil {
		t.Fatal("tampered credential revoked legitimate session")
	}
	nextID, nextSecret, valid := h.svc.parseRefreshCredential(next.RefreshToken)
	if !valid || nextID != id || nextSecret == secret {
		t.Fatal("rotation did not preserve identity and replace secret")
	}
	_, err = h.svc.Refresh(ctx, issued.RefreshToken, RequestMeta{})
	wantErr(t, err, "invalid_session", 401)
	for _, value := range []any{issued, next} {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{secret, nextSecret, "refreshToken", "refresh_token"} {
			if strings.Contains(string(body), private) {
				t.Fatal("refresh secret exposed")
			}
		}
	}
	if err = h.svc.LogoutRefresh(ctx, issued.RefreshToken); err != nil {
		t.Fatal("retired credential cannot revoke successor")
	}
}

func TestBrowserLogoutRetiredCookieRevokesRotatedSession(t *testing.T) {
	router, h := newTestRouter()
	h.mustRegister(t)
	login := do(router, "POST", "/api/v1/partner/v1/auth/login", `{"email":"dev@pharmacare.com","password":"supersecret"}`, nil)
	first, _ := assertBrowserSession(t, login)
	headers := map[string]string{"Cookie": first.Name + "=" + first.Value}
	rotated := do(router, "POST", "/api/v1/partner/v1/auth/refresh", "", headers)
	next, _ := assertBrowserSession(t, rotated)
	out := do(router, "POST", "/api/v1/partner/v1/auth/logout", "", headers)
	if out.Code != 200 || len(out.Result().Cookies()) != 1 || out.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("retired-cookie logout failed to revoke/delete")
	}
	denied := do(router, "POST", "/api/v1/partner/v1/auth/refresh", "", map[string]string{"Cookie": next.Name + "=" + next.Value})
	if denied.Code != 401 {
		t.Fatal("successor cookie accepted after logout")
	}
	for _, body := range []string{out.Body.String(), denied.Body.String()} {
		if strings.Contains(body, first.Value) || strings.Contains(body, next.Value) || strings.Contains(body, "refreshToken") || strings.Contains(body, "refresh_token") {
			t.Fatal("credential leaked through logout/error response")
		}
	}
}
