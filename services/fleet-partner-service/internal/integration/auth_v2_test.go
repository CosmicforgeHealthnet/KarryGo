package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/shared/go/apperrors"
)

func TestAuthV2HTTPRegistrationAndBothSignInIdentifiers(t *testing.T) {
	router, h := buildAuthRouter(t)
	ctx := context.Background()
	start := postJSON(router, "/fleet-partner/v1/auth/sign-up/start", map[string]string{"email": "Owner@Example.com", "phone": "+44 7911 123456"}, nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("signup status %d", start.Code)
	}
	data := decodeEnvelope(t, start)["data"].(map[string]any)
	if data["expiresAt"] == nil || data["maskedDestination"] != "o***@example.com" {
		t.Fatal("missing authoritative deadline/destination")
	}
	var n int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_identities`).Scan(&n); err != nil || n != 0 {
		t.Fatal("identity created before OTP")
	}
	msg, _ := h.sender.LastOTP()
	verify := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{"challengeId": data["challengeId"].(string), "code": msg.Code}, nil)
	if verify.Code != 200 {
		t.Fatalf("verify status %d", verify.Code)
	}
	verified := decodeEnvelope(t, verify)["data"].(map[string]any)
	identityID := verified["identityId"].(string)
	if verified["nextStep"] != "ONBOARDING_REQUIRED" {
		t.Fatal("new owner not sent to onboarding")
	}
	for _, pair := range []map[string]string{
		{"email": "OWNER@example.com", "phone": "+447911123456"},
		{"email": "owner@example.com", "phone": "+447911999999"},
		{"email": "new@example.com", "phone": "+447911123456"},
	} {
		before := h.sender.OTPCount()
		response := postJSON(router, "/fleet-partner/v1/auth/sign-up/start", pair, nil)
		if response.Code != 409 || h.sender.OTPCount() != before {
			t.Fatalf("duplicate status %d or delivery occurred", response.Code)
		}
	}
	// Deprecated endpoint is signup-only too, even if a caller supplies purpose.
	legacy := postJSON(router, "/fleet-partner/v1/auth/start", map[string]string{"email": "owner@example.com", "phone": "+447911123456", "purpose": "SIGN_IN"}, nil)
	if legacy.Code != 409 {
		t.Fatal("legacy endpoint bypass")
	}
	for _, identifier := range []string{"OWNER@example.com", "+44 (7911) 123456"} {
		// Elapse the cooldown between successive sign-in starts.
		if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		before := h.sender.OTPCount()
		response := postJSON(router, "/fleet-partner/v1/auth/sign-in/start", map[string]string{"identifier": identifier, "email": "attacker@example.com"}, nil)
		if response.Code != 202 || h.sender.OTPCount() != before+1 {
			t.Fatalf("login status %d or missing OTP", response.Code)
		}
		msg, _ = h.sender.LastOTP()
		if msg.Email != "owner@example.com" || msg.Purpose != authmodels.PurposeSignIn {
			t.Fatal("login destination/purpose changed")
		}
		loginID := decodeEnvelope(t, response)["data"].(map[string]any)["challengeId"].(string)
		result := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{"challengeId": loginID, "code": msg.Code}, nil)
		if result.Code != 200 || decodeEnvelope(t, result)["data"].(map[string]any)["identityId"] != identityID {
			t.Fatal("identifier resolved to different identity")
		}
	}
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_identities`).Scan(&n); err != nil || n != 1 {
		t.Fatal("sign-in created identity")
	}
}

func TestAuthV2ConcurrentStartsAndRegistrationRaces(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.service.Start(ctx, authusecases.StartInput{Email: "race@example.com", Phone: "+2348012345678"})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && apperrors.From(err).Status != 429 {
			t.Fatal(err)
		}
	}
	if h.sender.OTPCount() != 1 {
		t.Fatalf("rapid duplicate deliveries %d", h.sender.OTPCount())
	}
	// Different email, same phone: both were unused when challenges started.
	second, err := h.service.Start(ctx, authusecases.StartInput{Email: "other@example.com", Phone: "+2348012345678"})
	if err != nil {
		t.Fatal(err)
	}
	secondMsg, _ := h.sender.LastOTP()
	var firstID string
	if err := h.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_auth_otp_challenges WHERE email_normalized='race@example.com'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	firstMsg := h.sender.SentOTPs[0]
	winners := make(chan bool, 2)
	for _, item := range []struct{ id, code string }{{firstID, firstMsg.Code}, {second.ChallengeID, secondMsg.Code}} {
		wg.Add(1)
		go func(id, code string) {
			defer wg.Done()
			_, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: id, Code: code})
			winners <- err == nil
		}(item.id, item.code)
	}
	wg.Wait()
	close(winners)
	count := 0
	for won := range winners {
		if won {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("registration winners %d", count)
	}
	var n int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_identities`).Scan(&n); err != nil || n != 1 {
		t.Fatal("duplicate identity")
	}
}

func TestAuthV2RestrictionAfterStartAndResendReplacement(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	id, code := h.startAndCode(t, "owner@example.com", "08012345678")
	owner, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: id, Code: code})
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.service.SignIn(ctx, authusecases.StartInput{Identifier: "+2348012345678"})
	if err != nil {
		t.Fatal(err)
	}
	old, _ := h.sender.LastOTP()
	if _, err := h.service.Resend(ctx, authusecases.ResendInput{ChallengeID: login.ChallengeID}); apperrors.From(err).Code != errorcodes.OTPResendTooSoon {
		t.Fatal("cooldown bypass")
	}
	if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second' WHERE id=$1`, login.ChallengeID); err != nil {
		t.Fatal(err)
	}
	replacement, err := h.service.Resend(ctx, authusecases.ResendInput{ChallengeID: login.ChallengeID})
	if err != nil {
		t.Fatal(err)
	}
	latest, _ := h.sender.LastOTP()
	if replacement.ChallengeID == login.ChallengeID || latest.IdempotencyKey == old.IdempotencyKey {
		t.Fatal("resend reused challenge or notification key")
	}
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: login.ChallengeID, Code: old.Code}); apperrors.From(err).Code != errorcodes.OTPChallengeSuperseded {
		t.Fatal("old challenge verified")
	}
	if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_corporate_identities SET status='LOCKED' WHERE id=$1`, owner.IdentityID); err != nil {
		t.Fatal(err)
	}
	if result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: replacement.ChallengeID, Code: latest.Code}); err == nil || result.Tokens != nil {
		t.Fatal("restricted identity obtained session")
	}
	before := h.sender.OTPCount()
	for _, identifier := range []string{"owner@example.com", "+2348012345678", "unknown@example.com", "+2348099999999"} {
		_, err := h.service.SignIn(ctx, authusecases.StartInput{Identifier: identifier})
		if apperrors.From(err).Code != errorcodes.SignInUnavailable {
			t.Fatal("enumeration response differs")
		}
	}
	if h.sender.OTPCount() != before {
		t.Fatal("ineligible sign-in delivered OTP")
	}
}

func TestAuthV2PhoneChangeRequiresBoundEmailProof(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	id, code := h.startAndCode(t, "owner@example.com", "08012345678")
	owner, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: id, Code: code})
	if err != nil {
		t.Fatal(err)
	}
	target := "+447911123456"
	if err := h.service.ChangePhone(ctx, owner.IdentityID, owner.Tokens.SessionID, "", target, ""); err == nil {
		t.Fatal("phone change without email proof")
	}
	challenge, err := h.service.Start(ctx, authusecases.StartInput{Email: "owner@example.com", Phone: "+2348012345678", IdentityID: owner.IdentityID, SessionID: owner.Tokens.SessionID, Purpose: authmodels.PurposeStepUp, ActionClass: "phone_change:" + target})
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := h.sender.LastOTP()
	grant, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challenge.ChallengeID, Code: msg.Code})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.ChangePhone(ctx, owner.IdentityID, owner.Tokens.SessionID, "", "+447911999999", grant.StepUpReference); err == nil {
		t.Fatal("proof accepted different phone")
	}
	if err := h.service.ChangePhone(ctx, owner.IdentityID, owner.Tokens.SessionID, "", target, grant.StepUpReference); err != nil {
		t.Fatal(err)
	}
	if err := h.service.ChangePhone(ctx, owner.IdentityID, owner.Tokens.SessionID, "", target, grant.StepUpReference); err == nil {
		t.Fatal("phone change grant replay")
	}
	identity, err := h.service.IdentityByID(ctx, owner.IdentityID)
	if err != nil || identity.PhoneE164 != target || identity.ID != owner.IdentityID {
		t.Fatal("identity changed incorrectly")
	}
	login, err := h.service.SignIn(ctx, authusecases.StartInput{Identifier: target})
	if err != nil || login.MaskedDestination != "o***@example.com" {
		t.Fatal(fmt.Sprint("new phone login failed: ", err))
	}
}
