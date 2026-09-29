package bookingusecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

type secureMemoryChallenge struct {
	record handover.CodeRecord
	status string
}

type secureMemoryRepo struct {
	*trackingRepo
	challengeMu sync.Mutex
	challenges  map[handover.Step]secureMemoryChallenge
	binding     bookingrepo.CommerceHandoverBinding
}

func newSecureMemoryRepo(booking bookingmodels.Booking) *secureMemoryRepo {
	return &secureMemoryRepo{trackingRepo: newTrackingRepo(booking), challenges: make(map[handover.Step]secureMemoryChallenge), binding: bookingrepo.CommerceHandoverBinding{MissionRef: "commerce-" + booking.ID, BookingID: booking.ID, CustomerID: booking.CustomerID}}
}

func (r *secureMemoryRepo) CommerceHandoverBinding(_ context.Context, missionRef string) (bookingrepo.CommerceHandoverBinding, error) {
	if missionRef != r.binding.MissionRef {
		return bookingrepo.CommerceHandoverBinding{}, bookingrepo.ErrSecureHandoverChallengeNotFound
	}
	return r.binding, nil
}

func (r *secureMemoryRepo) MarkArrivedAtPickup(ctx context.Context, id string, rec handover.CodeRecord) (bookingmodels.Booking, error) {
	r.challengeMu.Lock()
	r.challenges[handover.StepPickup] = secureMemoryChallenge{record: rec, status: "ACTIVE"}
	r.challengeMu.Unlock()
	b, err := r.trackingRepo.MarkArrivedAtPickup(ctx, id, handover.CodeRecord{})
	return b, err
}

func (r *secureMemoryRepo) MarkEnRouteDelivery(ctx context.Context, id string, rec handover.CodeRecord) (bookingmodels.Booking, error) {
	r.challengeMu.Lock()
	r.challenges[handover.StepDelivery] = secureMemoryChallenge{record: rec, status: "ACTIVE"}
	r.challengeMu.Unlock()
	return r.trackingRepo.MarkEnRouteDelivery(ctx, id, handover.CodeRecord{})
}

func (r *secureMemoryRepo) ResendPickupCode(_ context.Context, _ string, rec handover.CodeRecord) error {
	return r.replaceChallenge(handover.StepPickup, rec)
}

func (r *secureMemoryRepo) ResendDeliveryCode(_ context.Context, _ string, rec handover.CodeRecord) error {
	return r.replaceChallenge(handover.StepDelivery, rec)
}

func (r *secureMemoryRepo) replaceChallenge(stage handover.Step, rec handover.CodeRecord) error {
	r.challengeMu.Lock()
	defer r.challengeMu.Unlock()
	current := r.challenges[stage]
	if current.status != "ACTIVE" || rec.Version != current.record.Version+1 {
		return errors.New("challenge changed")
	}
	r.challenges[stage] = secureMemoryChallenge{record: rec, status: "ACTIVE"}
	return nil
}

func (r *secureMemoryRepo) GetHandoverChallenge(_ context.Context, _ string, stage handover.Step) (handover.CodeRecord, string, error) {
	r.challengeMu.Lock()
	defer r.challengeMu.Unlock()
	challenge, ok := r.challenges[stage]
	if !ok {
		return handover.CodeRecord{}, "", bookingrepo.ErrSecureHandoverChallengeNotFound
	}
	return challenge.record, challenge.status, nil
}

func (r *secureMemoryRepo) ConsumeHandoverChallenge(_ context.Context, bookingID string, stage handover.Step, actorID, credential string, secret []byte, _, _ string) (bookingmodels.Booking, bool, error) {
	r.challengeMu.Lock()
	defer r.challengeMu.Unlock()
	challenge := r.challenges[stage]
	if challenge.status != "ACTIVE" {
		return bookingmodels.Booking{}, false, errors.New("consumed")
	}
	rec := challenge.record
	r.trackingRepo.mu.Lock()
	b := r.trackingRepo.bookings[bookingID]
	if b.ProviderID == nil || rec.ProviderID != *b.ProviderID || rec.OwnerID != b.CustomerID ||
		(stage == handover.StepPickup && actorID != b.CustomerID) ||
		(stage == handover.StepDelivery && actorID != *b.ProviderID) ||
		(stage == handover.StepPickup && b.Status != bookingmodels.StatusArrivedAtPickup) ||
		(stage == handover.StepDelivery && b.Status != bookingmodels.StatusPickedUp && b.Status != bookingmodels.StatusEnRouteDelivery) {
		r.trackingRepo.mu.Unlock()
		return bookingmodels.Booking{}, false, errors.New("wrong context")
	}
	if time.Now().After(rec.ExpiresAt) || rec.Attempts >= handover.MaxVerifyAttempts {
		r.trackingRepo.mu.Unlock()
		return bookingmodels.Booking{}, false, errors.New("expired or locked")
	}
	method, expected := "QR", rec.QRHash
	if len(strings.TrimSpace(credential)) == 6 {
		method, expected = "OTP", rec.Hash
	}
	if !handover.VerifySecureCredential(secret, method, rec, credential, expected) {
		challenge.record.Attempts++
		if challenge.record.Attempts >= handover.MaxVerifyAttempts {
			challenge.status = "LOCKED"
		}
		r.challenges[stage] = challenge
		r.trackingRepo.mu.Unlock()
		return bookingmodels.Booking{}, false, errors.New("incorrect")
	}
	challenge.status = "CONSUMED"
	r.challenges[stage] = challenge
	now := time.Now()
	if stage == handover.StepPickup {
		b.PickupCodeVerifiedAt = &now
	} else {
		b.Status = bookingmodels.StatusDelivered
		b.DeliveryCodeVerifiedAt = &now
	}
	r.trackingRepo.bookings[bookingID] = b
	r.trackingRepo.mu.Unlock()
	return b, true, nil
}

func TestSecureChallengeRejectsTerminalAndReassignedBookingContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*bookingmodels.Booking)
	}{
		{name: "terminal", mutate: func(b *bookingmodels.Booking) { b.Status = bookingmodels.StatusCancelled }},
		{name: "reassigned", mutate: func(b *bookingmodels.Booking) { b.ProviderID = ptr("other-provider") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-context", CustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusEnRoutePickup})
			svc := secureMemoryService(repo)
			issued, err := svc.MarkArrivedAtPickup(context.Background(), "b-context", "provider")
			if err != nil {
				t.Fatal(err)
			}
			repo.mu.Lock()
			booking := repo.bookings["b-context"]
			tc.mutate(&booking)
			repo.bookings["b-context"] = booking
			repo.mu.Unlock()
			if _, err = svc.VerifyDriverAtPickup(context.Background(), "customer", "b-context", issued.PickupCredential.QRToken); err == nil {
				t.Fatal("credential verified after authoritative context changed")
			}
		})
	}
}

func secureMemoryService(repo *secureMemoryRepo) *BookingService {
	return NewBookingService(Options{
		Bookings: repo, SecureHandover: repo, HandoverSecret: testHandoverSecret,
		Notifier: bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
	})
}

func TestSecurePickupOTPAndQRShareSingleUseLifecycle(t *testing.T) {
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-secure", CustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusEnRoutePickup})
	svc := secureMemoryService(repo)
	issued, err := svc.MarkArrivedAtPickup(context.Background(), "b-secure", "provider")
	if err != nil {
		t.Fatal(err)
	}
	credential := issued.PickupCredential
	if credential.Code == "" || credential.QRToken == "" || credential.Code == credential.QRToken || strings.Contains(credential.QRToken, credential.Code) {
		t.Fatal("issue boundary did not return independent OTP and opaque QR")
	}
	status, err := svc.GetPickupCredential(context.Background(), "provider", "b-secure")
	if err != nil {
		t.Fatal(err)
	}
	if status.Code != "" || status.QRToken != "" || status.Status != "ACTIVE" {
		t.Fatalf("generic read leaked a credential: %+v", status)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "\"code\"") || strings.Contains(string(encoded), "qr_token") || strings.Contains(string(encoded), credential.Code) || strings.Contains(string(encoded), credential.QRToken) {
		t.Fatalf("generic JSON response contains secret-bearing fields: %s", encoded)
	}

	var successes atomic.Int32
	var wg sync.WaitGroup
	for _, value := range []string{credential.Code, credential.QRToken} {
		value := value
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, verifyErr := svc.VerifyDriverAtPickup(context.Background(), "customer", "b-secure", value); verifyErr == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful OTP/QR race consumers = %d, want 1", successes.Load())
	}
	if _, err = svc.VerifyDriverAtPickup(context.Background(), "other-customer", "b-secure", credential.Code); err == nil {
		t.Fatal("customer IDOR unexpectedly succeeded")
	}
}

func TestCommercePickupRelayConcurrentDuplicateReconcilesCanonicalState(t *testing.T) {
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-commerce", CustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusEnRoutePickup})
	svc := secureMemoryService(repo)
	issued, err := svc.MarkArrivedAtPickup(context.Background(), "b-commerce", "provider")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			o, confirmErr := svc.ConfirmCommercePickup(context.Background(), "commerce-b-commerce", issued.PickupCredential.ChallengeRef, "OTP", issued.PickupCredential.Code, "same-operation")
			if confirmErr == nil && o.Status != "CONSUMED" {
				confirmErr = errors.New("canonical state was not consumed")
			}
			results <- confirmErr
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err = <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, err = svc.ReadCommerceHandover(context.Background(), "commerce-other", "PICKUP"); err == nil {
		t.Fatal("cross-mission read succeeded")
	}
}

func TestSecureChallengeSharesAttemptLockAndExpiryAcrossMethods(t *testing.T) {
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-lock", CustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusEnRoutePickup})
	svc := secureMemoryService(repo)
	issued, err := svc.MarkArrivedAtPickup(context.Background(), "b-lock", "provider")
	if err != nil {
		t.Fatal(err)
	}
	for i, wrong := range []string{"000000", "wrong-opaque-token-1", "111111", "wrong-opaque-token-2", "222222"} {
		if wrong == issued.PickupCredential.Code {
			wrong = "333333"
		}
		if _, err = svc.VerifyDriverAtPickup(context.Background(), "customer", "b-lock", wrong); err == nil {
			t.Fatalf("wrong credential %d unexpectedly verified", i)
		}
	}
	if _, err = svc.VerifyDriverAtPickup(context.Background(), "customer", "b-lock", issued.PickupCredential.QRToken); err == nil {
		t.Fatal("valid QR bypassed the shared five-attempt lock")
	}

	expiredRepo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-expired", CustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusEnRoutePickup})
	expiredSvc := secureMemoryService(expiredRepo)
	expired, err := expiredSvc.MarkArrivedAtPickup(context.Background(), "b-expired", "provider")
	if err != nil {
		t.Fatal(err)
	}
	expiredRepo.challengeMu.Lock()
	challenge := expiredRepo.challenges[handover.StepPickup]
	challenge.record.ExpiresAt = time.Now().Add(-time.Second)
	expiredRepo.challenges[handover.StepPickup] = challenge
	expiredRepo.challengeMu.Unlock()
	if _, err = expiredSvc.VerifyDriverAtPickup(context.Background(), "customer", "b-expired", expired.PickupCredential.Code); err == nil {
		t.Fatal("expired OTP unexpectedly verified")
	}
}

func TestSecurePickupResendInvalidatesOldOTPAndQR(t *testing.T) {
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-resend", CustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusEnRoutePickup})
	svc := secureMemoryService(repo)
	first, err := svc.MarkArrivedAtPickup(context.Background(), "b-resend", "provider")
	if err != nil {
		t.Fatal(err)
	}
	repo.challengeMu.Lock()
	challenge := repo.challenges[handover.StepPickup]
	oldSent := time.Now().Add(-handover.ResendCooldown - time.Second)
	challenge.record.LastSentAt = &oldSent
	repo.challenges[handover.StepPickup] = challenge
	repo.challengeMu.Unlock()
	second, err := svc.ResendPickupCredential(context.Background(), "provider", "b-resend")
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != first.PickupCredential.Version+1 || second.Code == first.PickupCredential.Code || second.QRToken == first.PickupCredential.QRToken {
		t.Fatal("resend did not create a fresh credential version")
	}
	for _, old := range []string{first.PickupCredential.Code, first.PickupCredential.QRToken} {
		if _, err = svc.VerifyDriverAtPickup(context.Background(), "customer", "b-resend", old); err == nil {
			t.Fatal("superseded credential unexpectedly verified")
		}
	}
	if _, err = svc.VerifyDriverAtPickup(context.Background(), "customer", "b-resend", second.QRToken); err != nil {
		t.Fatalf("new QR failed: %v", err)
	}
	if _, err = svc.VerifyDriverAtPickup(context.Background(), "customer", "b-resend", second.Code); err == nil {
		t.Fatal("OTP replay after QR consumption unexpectedly succeeded")
	}
}

func TestSecureDeliverySecretsStayAtCustomerNotificationBoundary(t *testing.T) {
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "b-delivery", CustomerID: "customer", ReceiverCustomerID: "customer", ProviderID: ptr("provider"), Status: bookingmodels.StatusPickedUp})
	capture := &recordingNotifier{}
	svc := NewBookingService(Options{
		Bookings: repo, SecureHandover: repo, HandoverSecret: testHandoverSecret,
		Notifier: bookingclients.NewBookingNotifierWith(capture),
	})
	public, err := svc.MarkEnRouteDelivery(context.Background(), "b-delivery", "provider")
	if err != nil {
		t.Fatal(err)
	}
	encodedPublic, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	challenge, _, err := repo.GetHandoverChallenge(context.Background(), "b-delivery", handover.StepDelivery)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedPublic), challenge.Code) || strings.Contains(string(encodedPublic), challenge.QRToken) {
		t.Fatal("provider-facing booking response leaked the customer's credential")
	}
	view, err := svc.GetVerificationCode(context.Background(), "customer", "b-delivery", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if view.Code != "" || view.QRToken != "" {
		t.Fatal("customer generic read redisplayed a bearer credential")
	}
	secureRead, err := svc.ReadReceiverCredential(context.Background(), "customer", "b-delivery", challenge.ChallengeRef)
	if err != nil || secureRead.Code != challenge.Code || secureRead.QRToken != challenge.QRToken {
		t.Fatalf("receiver-specific read = %+v, %v", secureRead, err)
	}
	if _, err := svc.ReadReceiverCredential(context.Background(), "other", "b-delivery", challenge.ChallengeRef); err == nil {
		t.Fatal("unbound account retrieved delivery credential")
	}
	var delivered bool
	for _, request := range capture.requests {
		if request.EventType == "booking.delivery_code_ready" {
			delivered = request.Recipient.ID == "customer" && request.Data["challenge_ref"] == challenge.ChallengeRef && request.Data["code"] == nil && request.Data["qr_token"] == nil
		}
	}
	if !delivered {
		t.Fatal("customer did not receive a safe retrieval notification")
	}
	if _, err = svc.ConfirmDelivery(context.Background(), "b-delivery", "other-provider", challenge.QRToken, "", ""); err == nil {
		t.Fatal("provider IDOR unexpectedly succeeded")
	}
	if _, err = svc.ConfirmDelivery(context.Background(), "b-delivery", "provider", challenge.QRToken, "", ""); err != nil {
		t.Fatalf("valid delivery QR failed: %v", err)
	}
	if _, err = svc.ConfirmDelivery(context.Background(), "b-delivery", "provider", challenge.Code, "", ""); err == nil {
		t.Fatal("OTP replay after delivery QR consumption unexpectedly succeeded")
	}
}

func TestPhoneOnlyHaulingReceiverFailsBeforeDeliveryChallenge(t *testing.T) {
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "phone-only", CustomerID: "sender", ProviderID: ptr("provider"), Status: bookingmodels.StatusPickedUp, ReceiverPhone: "+2348011111111"})
	svc := NewBookingService(Options{Bookings: repo, SecureHandover: repo, HandoverSecret: testHandoverSecret,
		Notifier: bookingclients.NewBookingNotifierWith(&recordingNotifier{})})
	_, err := svc.MarkEnRouteDelivery(context.Background(), "phone-only", "provider")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Code != "RECEIVER_SECURE_CHANNEL_REQUIRED" || typed.Status != 412 {
		t.Fatalf("phone-only error = %v", err)
	}
	if _, _, err := repo.GetHandoverChallenge(context.Background(), "phone-only", handover.StepDelivery); !errors.Is(err, bookingrepo.ErrSecureHandoverChallengeNotFound) {
		t.Fatalf("phone-only booking created challenge: %v", err)
	}
}

func TestPhoneOnlyHaulingReceiverFailsBeforePickupCustody(t *testing.T) {
	verified := time.Now()
	repo := newSecureMemoryRepo(bookingmodels.Booking{ID: "phone-only-pickup", CustomerID: "sender", ProviderID: ptr("provider"), Status: bookingmodels.StatusArrivedAtPickup,
		ReceiverPhone: "+2348011111111", PickupCodeVerifiedAt: &verified})
	svc := NewBookingService(Options{Bookings: repo, SecureHandover: repo, HandoverSecret: testHandoverSecret,
		Notifier: bookingclients.NewBookingNotifierWith(&recordingNotifier{})})
	_, err := svc.ConfirmPickup(context.Background(), "phone-only-pickup", "provider")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Code != "RECEIVER_SECURE_CHANNEL_REQUIRED" || typed.Status != 412 {
		t.Fatalf("phone-only pickup error = %v", err)
	}
	booking, err := repo.GetByID(context.Background(), "phone-only-pickup")
	if err != nil || booking.Status != bookingmodels.StatusArrivedAtPickup {
		t.Fatalf("phone-only pickup advanced custody: %+v %v", booking, err)
	}
}
