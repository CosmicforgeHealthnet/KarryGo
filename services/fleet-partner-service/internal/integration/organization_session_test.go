package integration_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
)

func organizationSource(t *testing.T, h *authHarness) (*authusecases.TokenPair, authusecases.SelectOrganizationInput) {
	t.Helper()
	id, code := h.startAndCode(t, "org-session@example.com", "08012345670")
	verified, err := h.service.Verify(context.Background(), verifyInputFor(id, code))
	if err != nil {
		t.Fatal("verify fixture failed")
	}
	h.memberships.memberships = []authusecases.MembershipSummary{{ID: "ousr_selected", OrganizationID: "org_selected", Status: "ACTIVE", OrganizationStatus: "ACTIVE", RoleVersion: 2}}
	return verified.Tokens, authusecases.SelectOrganizationInput{IdentityID: verified.IdentityID, SessionID: verified.Tokens.SessionID, MembershipID: "ousr_selected"}
}

func assertNoActiveOrganizationSuccessor(t *testing.T, h *authHarness, identityID string) {
	t.Helper()
	var active int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_corporate_sessions WHERE identity_id=$1 AND status='ACTIVE'`, identityID).Scan(&active); err != nil || active != 0 {
		t.Fatal("active successor survived revocation")
	}
}

func TestOrganizationSelectionRejectsRevokedJWTAndExpiredSource(t *testing.T) {
	requireOrganizationSessionDB(t)
	for _, state := range []string{"revoked", "expired", "compromised", "identity_suspended", "missing", "foreign_identity"} {
		t.Run(state, func(t *testing.T) {
			router, h := buildAuthRouter(t)
			tokens, in := organizationSource(t, h)
			ctx := context.Background()
			var err error
			switch state {
			case "revoked":
				err = h.service.LogoutRefresh(ctx, tokens.RefreshToken, "req", "cor")
			case "expired":
				_, err = h.pool.Exec(ctx, `UPDATE fleet_partner_corporate_sessions SET refresh_expires_at=now()-interval '1 second' WHERE id=$1`, in.SessionID)
			case "compromised":
				_, err = h.pool.Exec(ctx, `UPDATE fleet_partner_corporate_sessions SET status='COMPROMISED' WHERE id=$1`, in.SessionID)
			case "identity_suspended":
				_, err = h.pool.Exec(ctx, `UPDATE fleet_partner_corporate_identities SET status='SUSPENDED' WHERE id=$1`, in.IdentityID)
			case "missing":
				in.SessionID = "ses_nonexistent"
			case "foreign_identity":
				in.IdentityID = "cid_other"
			}
			if err != nil {
				t.Fatal("source state setup failed")
			}
			jwt := signTestAccessToken(t, in.IdentityID, in.SessionID) // signature and exp valid
			out := postJSON(router, "/fleet-partner/v1/auth/select-organization", map[string]string{"membershipId": in.MembershipID}, map[string]string{"Authorization": "Bearer " + jwt})
			if out.Code != http.StatusUnauthorized || len(out.Result().Cookies()) != 0 {
				t.Fatalf("invalid persisted source status=%d", out.Code)
			}
			var total int
			if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_sessions`).Scan(&total); err != nil || total != 1 {
				t.Fatal("invalid source minted a session")
			}
		})
	}
}

func TestOrganizationSelectionPreservesFamilyRevocationLineage(t *testing.T) {
	requireOrganizationSessionDB(t)
	for _, revoke := range []string{"cookie_logout", "session_logout", "user_revocation", "refresh_replay"} {
		t.Run(revoke, func(t *testing.T) {
			h := newAuthHarness(t, nil)
			first, in := organizationSource(t, h)
			ctx := context.Background()
			repo := authrepos.NewSessionRepository(h.pool)
			source, err := repo.GetByID(ctx, first.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			next, err := h.service.SelectOrganization(ctx, in)
			if err != nil {
				t.Fatal("active source rejected")
			}
			successor, err := repo.GetByID(ctx, next.SessionID)
			if err != nil || successor.SessionFamilyID != source.SessionFamilyID || successor.ID == source.ID || next.RefreshToken == first.RefreshToken || successor.OrganizationID == nil || *successor.OrganizationID != "org_selected" {
				t.Fatal("invalid organization transition")
			}
			if _, err = h.service.SelectOrganization(ctx, in); err == nil || apperrors.From(err).Status != 401 {
				t.Fatal("spent source JWT revived a session")
			}
			switch revoke {
			case "cookie_logout":
				err = h.service.LogoutRefresh(ctx, first.RefreshToken, "req", "cor")
			case "session_logout":
				err = h.service.Logout(ctx, first.SessionID, in.IdentityID, "req", "cor")
			case "user_revocation":
				err = h.service.RevokeSession(ctx, in.IdentityID, first.SessionID, "req", "cor")
			case "refresh_replay":
				_, err = h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: first.RefreshToken})
				if err == nil || apperrors.From(err).Status != 401 {
					t.Fatal("spent refresh credential accepted")
				}
				err = nil
			}
			if err != nil {
				t.Fatal("family revocation failed")
			}
			assertNoActiveOrganizationSuccessor(t, h, in.IdentityID)
		})
	}
}

func TestOrganizationSelectionLogoutAndReplayDeterministicRace(t *testing.T) {
	requireOrganizationSessionDB(t)
	for _, action := range []string{"logout", "replay"} {
		t.Run("selection_wins_"+action, func(t *testing.T) {
			h := newAuthHarness(t, nil)
			first, in := organizationSource(t, h)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			h.memberships.onGet = func() {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			selected := make(chan error, 1)
			go func() { _, err := h.service.SelectOrganization(ctx, in); selected <- err }()
			select {
			case <-entered: // persistent validation completed, family and source row locked
			case <-ctx.Done():
				t.Fatal("selection did not reach authority barrier")
			case <-selected:
				t.Fatal("selection failed before authority barrier")
			}
			competing, started := make(chan error, 1), make(chan struct{})
			go func() {
				close(started)
				if action == "logout" {
					competing <- h.service.LogoutRefresh(ctx, first.RefreshToken, "req", "cor")
				} else {
					_, err := h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: first.RefreshToken})
					competing <- err
				}
			}()
			<-started
			close(release)
			if err := <-selected; err != nil {
				t.Fatal("selection winner failed")
			}
			err := <-competing
			if action == "logout" && err != nil {
				t.Fatal("logout failed")
			}
			if action == "replay" && (err == nil || apperrors.From(err).Status != 401) {
				t.Fatal("replay not rejected")
			}
			assertNoActiveOrganizationSuccessor(t, h, in.IdentityID)
		})
	}
	for _, action := range []string{"logout", "compromise", "refresh"} {
		t.Run(action+"_wins", func(t *testing.T) {
			h := newAuthHarness(t, nil)
			first, in := organizationSource(t, h)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			tx, err := h.pool.Begin(ctx)
			if err != nil {
				t.Fatal("begin failed")
			}
			defer tx.Rollback(context.Background())
			repo := authrepos.NewSessionRepository(h.pool)
			source, err := repo.LockByRefreshHash(ctx, tx, auth.HashRefreshToken([]byte("test-session-secret"), first.RefreshToken))
			if err != nil {
				t.Fatal("source lock failed")
			}
			switch action {
			case "logout":
				_, err = repo.LogoutFamily(ctx, tx, source.SessionFamilyID)
			case "compromise":
				_, err = repo.RevokeFamily(ctx, tx, source.SessionFamilyID, "refresh_token_reuse")
			case "refresh":
				_, err = repo.RotateRefresh(ctx, tx, source, auth.HashRefreshToken([]byte("test-session-secret"), "next-test-credential"), time.Now().Add(time.Hour))
			}
			if err != nil {
				t.Fatal("first mutation failed")
			}
			done, started := make(chan error, 1), make(chan struct{})
			go func() { close(started); _, err := h.service.SelectOrganization(ctx, in); done <- err }()
			<-started
			if err := tx.Commit(ctx); err != nil {
				t.Fatal("commit failed")
			}
			if err := <-done; err == nil || apperrors.From(err).Status != 401 {
				t.Fatal("source invalidated during wait still authorized")
			}
			if action != "refresh" {
				assertNoActiveOrganizationSuccessor(t, h, in.IdentityID)
			}
			var scoped int
			if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_sessions WHERE organization_id IS NOT NULL`).Scan(&scoped); err != nil || scoped != 0 {
				t.Fatal("losing selection created a successor")
			}
		})
	}
}

func requireOrganizationSessionDB(t *testing.T) {
	t.Helper()
	if os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL") == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL unset: BLOCKED_BY_TEST_INFRASTRUCTURE")
	}
}

func TestOrganizationSelectionRejectsCompromisedFamilyWithActiveRow(t *testing.T) {
	h := newAuthHarness(t, nil)
	first, in := organizationSource(t, h)
	ctx := context.Background()
	next, err := h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: first.RefreshToken})
	if err != nil {
		t.Fatal("refresh fixture failed")
	}
	// Defensive legacy/inconsistent-family fixture: active child with a
	// compromised ancestor must not escape containment through organization selection.
	if _, err = h.pool.Exec(ctx, `UPDATE fleet_partner_corporate_sessions SET status=$2 WHERE id=$1`, first.SessionID, authmodels.SessionStatusCompromised); err != nil {
		t.Fatal("fixture failed")
	}
	in.SessionID = next.SessionID
	if _, err = h.service.SelectOrganization(ctx, in); err == nil || apperrors.From(err).Status != 401 {
		t.Fatal("compromised family authorized transition")
	}
}
