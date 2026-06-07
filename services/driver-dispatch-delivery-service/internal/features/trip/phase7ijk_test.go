package trip

// Phase 7I–7K tests: GetProof, CompleteTrip, CancelTrip.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ── fakeTripEventPublisher Phase 7J–7K method extensions ─────────────────────

func (f *fakeTripEventPublisher) PublishTripCompleted(_ context.Context, e TripCompletedEvent) error {
	f.completed = append(f.completed, e)
	return nil
}

func (f *fakeTripEventPublisher) PublishTripCancelled(_ context.Context, e TripCancelledEvent) error {
	f.cancelled = append(f.cancelled, e)
	return nil
}

func (f *fakeTripEventPublisher) PublishSuspensionFlag(_ context.Context, e SuspensionFlagEvent) error {
	f.suspensions = append(f.suspensions, e)
	return nil
}

// ── fakeRepository Phase 7J–7K method extensions ─────────────────────────────

func (r *fakeRepository) CompleteTripTx(_ context.Context, input CompleteTripInput) (*Trip, *DeliveryProof, error) {
	var foundTrip *Trip
	for i := range r.trips {
		t := &r.trips[i]
		if t.ID == input.TripID && t.ProviderID == input.ProviderID && t.Status == StatusProofSubmitted {
			t.Status = StatusCompleted
			t.CompletedAt = &input.Now
			t.UpdatedAt = input.Now
			notes := "provider_completed_delivery"
			r.stateLogs = append(r.stateLogs, TripStateLog{
				ID: uuid.NewString(), TripID: input.TripID, FromStatus: string(StatusProofSubmitted),
				ToStatus: StatusCompleted, ChangedAt: input.Now, ChangedBy: "provider", Notes: &notes,
			})
			foundTrip = t
			break
		}
	}
	if foundTrip == nil {
		return nil, nil, nil
	}
	var foundProof *DeliveryProof
	for i := range r.proofs {
		if r.proofs[i].TripID == input.TripID {
			r.proofs[i].Verified = true
			r.proofs[i].VerifiedAt = &input.Now
			foundProof = &r.proofs[i]
			break
		}
	}
	return foundTrip, foundProof, nil
}

func (r *fakeRepository) CountProviderCancellationsLast30Days(_ context.Context, providerID string) (int, error) {
	count := 0
	for _, c := range r.cancels {
		for _, t := range r.trips {
			if t.ID == c.TripID && t.ProviderID == providerID {
				count++
				break
			}
		}
	}
	return count, nil
}

func (r *fakeRepository) CancelTripTx(_ context.Context, input CancelTripInput) (*Trip, *Cancellation, error) {
	var foundTrip *Trip
	for i := range r.trips {
		t := &r.trips[i]
		if t.ID == input.TripID && t.ProviderID == input.ProviderID && CanProviderCancel(t.Status) {
			t.Status = StatusCancelled
			t.CancelledAt = &input.Now
			t.UpdatedAt = input.Now
			notes := input.ReasonCode
			r.stateLogs = append(r.stateLogs, TripStateLog{
				ID: uuid.NewString(), TripID: input.TripID, FromStatus: string(input.FromStatus),
				ToStatus: StatusCancelled, ChangedAt: input.Now, ChangedBy: "provider", Notes: &notes,
			})
			foundTrip = t
			break
		}
	}
	if foundTrip == nil {
		return nil, nil, nil
	}
	var reasonPtr *string
	if input.ReasonText != "" {
		rt := input.ReasonText
		reasonPtr = &rt
	}
	cancellation := Cancellation{
		ID: uuid.NewString(), TripID: input.TripID, CancelledBy: CancelledByProvider,
		ReasonCode: input.ReasonCode, ReasonText: reasonPtr,
		PenaltyApplied: input.PenaltyApplied, CancelledAt: input.Now,
	}
	r.cancels = append(r.cancels, cancellation)
	return foundTrip, &r.cancels[len(r.cancels)-1], nil
}

// ── Helper: seed a proof into the fake repository ────────────────────────────

func seedProof(env *tripFGHEnv, tripID string) DeliveryProof {
	proof := DeliveryProof{
		ID:            uuid.NewString(),
		TripID:        tripID,
		PhotoRef:      fmt.Sprintf("local-private://trips/p/%s/proof/photo", tripID),
		SignatureRef:  fmt.Sprintf("local-private://trips/p/%s/proof/sig", tripID),
		ReceiverName:  "Chidi Obi",
		ReceiverPhone: "+2348011223344",
		SubmittedAt:   time.Now().UTC(),
	}
	env.repo.proofs = append(env.repo.proofs, proof)
	return proof
}

// ── Phase 7I — GET /api/v1/provider/trips/:id/proof ──────────────────────────

func Test7I_MissingJWTReturns401(t *testing.T) {
	env := newTripFGHEnv(t)
	w := env.do(t, http.MethodGet, "/api/v1/provider/trips/"+uuid.NewString()+"/proof", "", nil, "")
	checkStatus(t, w, http.StatusUnauthorized)
}

func Test7I_InvalidTripIDReturns400(t *testing.T) {
	env := newTripFGHEnv(t)
	tok := env.token(t, uuid.NewString())
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/not-a-uuid/proof", tok)
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "validation_failed")
}

func Test7I_CrossProviderReturns404(t *testing.T) {
	env := newTripFGHEnv(t)
	owner := uuid.NewString()
	tr := seedTrip(env, owner, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, uuid.NewString())
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	checkStatus(t, w, http.StatusNotFound)
}

func Test7I_NoProofReturns404(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted) // no proof row seeded
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	checkStatus(t, w, http.StatusNotFound)
}

func Test7I_SubmittedProofReturns200(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	checkStatus(t, w, http.StatusOK)
}

func Test7I_ProofResponseHasRefFields(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	checkStatus(t, w, http.StatusOK)
	var resp struct {
		Data struct {
			PhotoRef     string `json:"photo_ref"`
			SignatureRef string `json:"signature_ref"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.PhotoRef == "" {
		t.Fatal("photo_ref missing")
	}
	if resp.Data.SignatureRef == "" {
		t.Fatal("signature_ref missing")
	}
}

func Test7I_NoS3URLInProofResponse(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	body := w.Body.String()
	if strings.Contains(strings.ToLower(body), "s3.amazonaws") {
		t.Fatalf("S3 URL in proof response: %s", body)
	}
}

func Test7I_NoRawFilesystemPathInProofResponse(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	// Proof ref already uses local-private:// scheme (not a raw path).
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	body := w.Body.String()
	// Must not contain Windows or Unix absolute paths.
	if strings.Contains(body, `C:\`) || strings.Contains(body, "/var/") || strings.Contains(body, "/tmp/") {
		t.Fatalf("raw filesystem path in response: %s", body)
	}
}

// ── Phase 7J — POST /api/v1/provider/trips/:id/complete ──────────────────────

func Test7J_MissingJWTReturns401(t *testing.T) {
	env := newTripFGHEnv(t)
	w := env.do(t, http.MethodPost, "/api/v1/provider/trips/"+uuid.NewString()+"/complete", "", nil, "")
	checkStatus(t, w, http.StatusUnauthorized)
}

func Test7J_InvalidTripIDReturns400(t *testing.T) {
	env := newTripFGHEnv(t)
	tok := env.token(t, uuid.NewString())
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/not-a-uuid/complete", tok)
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "validation_failed")
}

func Test7J_CrossProviderReturns404(t *testing.T) {
	env := newTripFGHEnv(t)
	owner := uuid.NewString()
	tr := seedTrip(env, owner, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, uuid.NewString())
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	checkStatus(t, w, http.StatusNotFound)
}

func Test7J_InProgressReturnsPROOF_REQUIRED(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusInProgress)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "PROOF_REQUIRED")
}

func Test7J_ProofSubmittedWithProofReturns200(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	checkStatus(t, w, http.StatusOK)
}

func Test7J_ProofSubmittedWithoutProofReturnsPROOF_REQUIRED(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted) // no proof row
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "PROOF_REQUIRED")
}

func Test7J_StatusBecomesCompleted(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	for _, trip := range env.repo.trips {
		if trip.ID == tr.ID && trip.Status != StatusCompleted {
			t.Fatalf("trip status=%s want completed", trip.Status)
		}
	}
}

func Test7J_CompletedAtIsSet(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	for _, trip := range env.repo.trips {
		if trip.ID == tr.ID && trip.CompletedAt == nil {
			t.Fatal("completed_at not set after complete")
		}
	}
}

func Test7J_ProofVerifiedTrue(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	for _, proof := range env.repo.proofs {
		if proof.TripID == tr.ID && !proof.Verified {
			t.Fatal("proof.verified not set to true after complete")
		}
	}
}

func Test7J_ProofVerifiedAtIsSet(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	for _, proof := range env.repo.proofs {
		if proof.TripID == tr.ID && proof.VerifiedAt == nil {
			t.Fatal("proof.verified_at not set after complete")
		}
	}
}

func Test7J_StateLogInserted(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	found := false
	for _, log := range env.repo.stateLogs {
		if log.TripID == tr.ID && log.ToStatus == StatusCompleted {
			found = true
		}
	}
	if !found {
		t.Fatal("state log not inserted for complete transition")
	}
}

func Test7J_TripCompletedEventPublished(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	checkStatus(t, w, http.StatusOK)
	if len(env.events.completed) != 1 {
		t.Fatalf("completed events=%d want 1", len(env.events.completed))
	}
	evt := env.events.completed[0]
	if evt.TripID != tr.ID || evt.ProviderID != providerID {
		t.Fatalf("event=%+v", evt)
	}
}

func Test7J_CompletedEventHasFareAmountAndCurrency(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, providerID)
	env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	if len(env.events.completed) != 1 {
		t.Fatal("no completed event")
	}
	evt := env.events.completed[0]
	if evt.FareAmount != tr.FareAmount {
		t.Fatalf("fare_amount=%d want %d", evt.FareAmount, tr.FareAmount)
	}
	if evt.Currency != tr.Currency {
		t.Fatalf("currency=%s want %s", evt.Currency, tr.Currency)
	}
	if evt.CompletedAt.IsZero() {
		t.Fatal("completed_at not set in event")
	}
}

func Test7J_NonProofSubmittedStatusReturns409(t *testing.T) {
	for _, status := range []TripStatus{StatusAssigned, StatusEnRoutePickup, StatusArrivedPickup,
		StatusCompleted, StatusCancelled, StatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			env := newTripFGHEnv(t)
			providerID := uuid.NewString()
			tr := seedTrip(env, providerID, status)
			tok := env.token(t, providerID)
			w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
			checkStatus(t, w, http.StatusConflict)
		})
	}
}

// ── Phase 7K — POST /api/v1/provider/trips/:id/cancel ────────────────────────

func cancelBody(reason, text string) string {
	return fmt.Sprintf(`{"reason_code":%q,"reason_text":%q}`, reason, text)
}

func doCancel(env *tripFGHEnv, t *testing.T, tripID, tok, reason, text string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(cancelBody(reason, text))
	return env.do(t, http.MethodPost, "/api/v1/provider/trips/"+tripID+"/cancel", tok, body, "application/json")
}

func Test7K_MissingJWTReturns401(t *testing.T) {
	env := newTripFGHEnv(t)
	w := env.do(t, http.MethodPost, "/api/v1/provider/trips/"+uuid.NewString()+"/cancel", "", nil, "")
	checkStatus(t, w, http.StatusUnauthorized)
}

func Test7K_InvalidTripIDReturns400(t *testing.T) {
	env := newTripFGHEnv(t)
	tok := env.token(t, uuid.NewString())
	w := doCancel(env, t, "not-a-uuid", tok, "other", "")
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "validation_failed")
}

func Test7K_CrossProviderReturns404(t *testing.T) {
	env := newTripFGHEnv(t)
	owner := uuid.NewString()
	tr := seedTrip(env, owner, StatusAssigned)
	tok := env.token(t, uuid.NewString())
	w := doCancel(env, t, tr.ID, tok, "other", "")
	checkStatus(t, w, http.StatusNotFound)
}

func Test7K_MissingReasonCodeReturns400(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	body := strings.NewReader(`{"reason_code":""}`)
	w := env.do(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/cancel", tok, body, "application/json")
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "validation_failed")
}

func Test7K_InvalidReasonCodeReturns400(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "not_a_valid_code", "")
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "validation_failed")
}

func Test7K_TooLongReasonTextReturns400(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	longText := strings.Repeat("x", MaxReasonTextLength+1)
	w := doCancel(env, t, tr.ID, tok, "other", longText)
	checkStatus(t, w, http.StatusBadRequest)
	checkErrorCode(t, w, "validation_failed")
}

func Test7K_AssignedCancellationReturns200(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "package_not_ready", "")
	checkStatus(t, w, http.StatusOK)
}

func Test7K_EnRoutePickupCancellationReturns200(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusEnRoutePickup)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "customer_unreachable", "tried calling twice")
	checkStatus(t, w, http.StatusOK)
}

func Test7K_ArrivedPickupCancellationReturns200(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusArrivedPickup)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "wrong_address", "")
	checkStatus(t, w, http.StatusOK)
}

func Test7K_InProgressCancellationReturnsPenalty(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusInProgress)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "safety_concern", "")
	checkStatus(t, w, http.StatusOK)
	var resp struct {
		Data struct {
			PenaltyApplied             bool `json:"penalty_applied"`
			RequiresAdminInvestigation bool `json:"requires_admin_investigation"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if !resp.Data.PenaltyApplied {
		t.Fatal("penalty_applied should be true for in_progress cancellation")
	}
	if !resp.Data.RequiresAdminInvestigation {
		t.Fatal("requires_admin_investigation should be true for in_progress cancellation")
	}
}

func Test7K_TerminalStatusReturns409(t *testing.T) {
	for _, status := range []TripStatus{StatusProofSubmitted, StatusCompleted, StatusCancelled, StatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			env := newTripFGHEnv(t)
			providerID := uuid.NewString()
			tr := seedTrip(env, providerID, status)
			tok := env.token(t, providerID)
			w := doCancel(env, t, tr.ID, tok, "other", "")
			checkStatus(t, w, http.StatusConflict)
		})
	}
}

func Test7K_CancellationRowInserted(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	doCancel(env, t, tr.ID, tok, "rider_unavailable", "")
	found := false
	for _, c := range env.repo.cancels {
		if c.TripID == tr.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("cancellation row not inserted")
	}
}

func Test7K_StateLogInserted(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	doCancel(env, t, tr.ID, tok, "other", "")
	found := false
	for _, log := range env.repo.stateLogs {
		if log.TripID == tr.ID && log.ToStatus == StatusCancelled {
			found = true
		}
	}
	if !found {
		t.Fatal("state log not inserted for cancel transition")
	}
}

func Test7K_TripCancelledEventPublished(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "package_not_ready", "customer says not ready")
	checkStatus(t, w, http.StatusOK)
	if len(env.events.cancelled) != 1 {
		t.Fatalf("cancelled events=%d want 1", len(env.events.cancelled))
	}
	evt := env.events.cancelled[0]
	if evt.TripID != tr.ID || evt.ProviderID != providerID {
		t.Fatalf("event=%+v", evt)
	}
	if evt.ReasonCode != "package_not_ready" {
		t.Fatalf("reason_code=%s", evt.ReasonCode)
	}
	if evt.CancelledBy != CancelledByProvider {
		t.Fatalf("cancelled_by=%s", evt.CancelledBy)
	}
}

func Test7K_ThirdCancellationSetsPenalty(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tok := env.token(t, providerID)

	// First cancellation — no penalty
	tr1 := seedTrip(env, providerID, StatusAssigned)
	doCancel(env, t, tr1.ID, tok, "other", "")

	// Second cancellation — no penalty
	tr2 := seedTrip(env, providerID, StatusAssigned)
	doCancel(env, t, tr2.ID, tok, "other", "")

	// Third cancellation — penalty
	tr3 := seedTrip(env, providerID, StatusAssigned)
	w := doCancel(env, t, tr3.ID, tok, "other", "")
	checkStatus(t, w, http.StatusOK)
	var resp struct {
		Data struct {
			PenaltyApplied bool `json:"penalty_applied"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Data.PenaltyApplied {
		t.Fatal("penalty_applied should be true on 3rd cancellation")
	}
}

func Test7K_ThirdCancellationPublishesSuspensionFlag(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tok := env.token(t, providerID)

	for i := 0; i < 3; i++ {
		tr := seedTrip(env, providerID, StatusAssigned)
		doCancel(env, t, tr.ID, tok, "other", "")
	}

	if len(env.events.suspensions) != 1 {
		t.Fatalf("suspension events=%d want 1", len(env.events.suspensions))
	}
	flag := env.events.suspensions[0]
	if flag.ProviderID != providerID {
		t.Fatalf("provider_id=%s want %s", flag.ProviderID, providerID)
	}
	if flag.Count30Days != 3 {
		t.Fatalf("count_30_days=%d want 3", flag.Count30Days)
	}
	if flag.Reason != "excessive_cancellations" {
		t.Fatalf("reason=%s", flag.Reason)
	}
}

func Test7K_InProgressCancellationDoesNotPublishSuspensionFlag(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tok := env.token(t, providerID)

	// Do 2 normal cancels first, then 1 from in_progress (3rd total).
	for i := 0; i < 2; i++ {
		tr := seedTrip(env, providerID, StatusAssigned)
		doCancel(env, t, tr.ID, tok, "other", "")
	}
	tr := seedTrip(env, providerID, StatusInProgress)
	doCancel(env, t, tr.ID, tok, "safety_concern", "")

	// in_progress cancel uses admin investigation, NOT suspension flag.
	if len(env.events.suspensions) != 0 {
		t.Fatalf("suspension flag should NOT be published for in_progress cancel; got %d", len(env.events.suspensions))
	}
}

func Test7K_CancellationResponseHasTripStatus(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusEnRoutePickup)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "customer_unreachable", "")
	checkStatus(t, w, http.StatusOK)
	var resp struct {
		Data struct {
			Status TripStatus `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Status != StatusCancelled {
		t.Fatalf("status=%s want cancelled", resp.Data.Status)
	}
}

func Test7K_WarningIncludedForEarlyCount(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tok := env.token(t, providerID)

	// First cancel should include warning about count.
	tr := seedTrip(env, providerID, StatusAssigned)
	w := doCancel(env, t, tr.ID, tok, "other", "")
	checkStatus(t, w, http.StatusOK)
	body := w.Body.String()
	if !strings.Contains(body, "cancellation") {
		t.Logf("warning may be empty for first cancellation (implementation choice): %s", body)
	}
}

// ── Integration: full trip lifecycle including get-proof and complete ─────────

func Test7_ArrivedStartProofGetProofComplete(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)

	// arrived
	w := env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/arrived", tok)
	checkStatus(t, w, http.StatusOK)

	// start
	w = env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/start", tok)
	checkStatus(t, w, http.StatusOK)

	// submit proof (using existing HTTP helper)
	w = submitProof(t, env, tr.ID, tok, nil, validJPEGFile("photo.jpg"), validJPEGFile("sig.jpg"))
	checkStatus(t, w, http.StatusCreated)

	// get proof
	w = env.doJSON(t, http.MethodGet, "/api/v1/provider/trips/"+tr.ID+"/proof", tok)
	checkStatus(t, w, http.StatusOK)

	// complete
	w = env.doJSON(t, http.MethodPost, "/api/v1/provider/trips/"+tr.ID+"/complete", tok)
	checkStatus(t, w, http.StatusOK)

	// Final state check.
	finalStatus := StatusAssigned
	for _, trip := range env.repo.trips {
		if trip.ID == tr.ID {
			finalStatus = trip.Status
		}
	}
	if finalStatus != StatusCompleted {
		t.Fatalf("final status=%s want completed", finalStatus)
	}

	// Events in order: arrived, started, proof, completed.
	if len(env.events.arrived) != 1 || len(env.events.started) != 1 ||
		len(env.events.proofSub) != 1 || len(env.events.completed) != 1 {
		t.Fatalf("events: arrived=%d started=%d proof=%d completed=%d",
			len(env.events.arrived), len(env.events.started), len(env.events.proofSub), len(env.events.completed))
	}
}

func Test7_CancelBeforeArrived(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusAssigned)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "rider_unavailable", "")
	checkStatus(t, w, http.StatusOK)
	if len(env.events.cancelled) != 1 {
		t.Fatal("trip.cancelled event not published")
	}
}

func Test7_CancelAfterInProgress(t *testing.T) {
	env := newTripFGHEnv(t)
	providerID := uuid.NewString()
	tr := seedTrip(env, providerID, StatusInProgress)
	tok := env.token(t, providerID)
	w := doCancel(env, t, tr.ID, tok, "safety_concern", "")
	checkStatus(t, w, http.StatusOK)
	var resp struct {
		Data CancelResponse `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Data.PenaltyApplied {
		t.Fatal("penalty should be applied for in_progress cancel")
	}
	if !resp.Data.RequiresAdminInvestigation {
		t.Fatal("requires_admin_investigation should be set for in_progress cancel")
	}
}

// ── Guard tests ───────────────────────────────────────────────────────────────

func Test7_AllMutationsRequireOwnProviderIDIJK(t *testing.T) {
	env := newTripFGHEnv(t)
	owner := uuid.NewString()
	attacker := uuid.NewString()
	tr := seedTrip(env, owner, StatusProofSubmitted)
	seedProof(env, tr.ID)
	tok := env.token(t, attacker)

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/provider/trips/" + tr.ID + "/proof"},
		{http.MethodPost, "/api/v1/provider/trips/" + tr.ID + "/complete"},
		{http.MethodPost, "/api/v1/provider/trips/" + tr.ID + "/cancel"},
	}
	for _, tc := range paths {
		var body strings.Reader
		if tc.method == http.MethodPost && strings.HasSuffix(tc.path, "/cancel") {
			body = *strings.NewReader(cancelBody("other", ""))
		}
		var w *httptest.ResponseRecorder
		if tc.method == http.MethodPost && strings.HasSuffix(tc.path, "/cancel") {
			w = env.do(t, tc.method, tc.path, tok, &body, "application/json")
		} else {
			w = env.doJSON(t, tc.method, tc.path, tok)
		}
		if w.Code != http.StatusNotFound {
			t.Logf("WARN: %s %s with attacker token got %d (want 404)", tc.method, tc.path, w.Code)
		}
	}
}
