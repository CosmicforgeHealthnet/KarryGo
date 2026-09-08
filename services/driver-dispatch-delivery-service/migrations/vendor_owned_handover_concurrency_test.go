package migrations_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/vendorownedhandover"
	"cosmicforge/logistics/shared/go/notifications"
)

type credentialRecorder struct {
	mu     sync.Mutex
	values map[string]map[string]string
}

func (r *credentialRecorder) Send(_ context.Context, in notifications.Request) (notifications.SendResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = map[string]map[string]string{}
	}
	r.values[in.SourceAggregateID] = map[string]string{"OTP": fmt.Sprint(in.TemplateData["code"]), "QR": fmt.Sprint(in.TemplateData["qr_token"])}
	return notifications.SendResponse{MessageID: uuid.NewString(), Status: "ACCEPTED"}, nil
}
func (r *credentialRecorder) get(ref, method string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.values[ref][method]
}

type vendorAuthorityFixture struct {
	service  *vendorownedhandover.Service
	recorder *credentialRecorder
	binding  vendorownedhandover.MissionBinding
	pool     *pgxpool.Pool
	cleanup  func()
}

func newVendorAuthorityFixture(t *testing.T) vendorAuthorityFixture {
	t.Helper()
	pool, cleanup := operationalTestPool(t)
	rec := &credentialRecorder{}
	repo, err := vendorownedhandover.NewRepository(pool, []byte("vendor-owned-postgres-secret"), vendorownedhandover.Policy{Enabled: true, MaxAttempts: 3, MaxResends: 3})
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	svc := vendorownedhandover.NewService(repo, vendorownedhandover.Policy{Enabled: true, MaxAttempts: 3, MaxResends: 3}).WithNotifier(rec)
	id := uuid.NewString()
	b := vendorownedhandover.MissionBinding{MissionRef: "mission-" + id, FulfilmentRef: "fulfilment-" + id, BusinessID: "business-" + id, BranchID: "branch-" + id, ProviderRef: "staff-" + id, ReceiverRef: "receiver-" + id, ReceiverPhone: "+15550000001", ManifestRef: "manifest-" + id, ManifestVersion: 1, ParcelRefs: []string{"parcel-" + id}, OperationRef: "bind-" + id, CorrelationID: id}
	if _, err = svc.Bind(context.Background(), b); err != nil {
		cleanup()
		t.Fatal(err)
	}
	return vendorAuthorityFixture{svc, rec, b, pool, cleanup}
}
func command(f vendorAuthorityFixture, stage vendorownedhandover.Stage, ref, method, credential, op string) vendorownedhandover.ConfirmCommand {
	return vendorownedhandover.ConfirmCommand{MissionRef: f.binding.MissionRef, Stage: stage, ChallengeRef: ref, ProviderRef: f.binding.ProviderRef, Method: method, Credential: credential, ManifestRef: f.binding.ManifestRef, ManifestVersion: f.binding.ManifestVersion, OperationRef: op}
}

func TestVendorOwnedHandoverPostgresOTPVsQRRace(t *testing.T) {
	f := newVendorAuthorityFixture(t)
	defer f.cleanup()
	o, err := f.service.Read(context.Background(), f.binding.MissionRef, vendorownedhandover.Pickup)
	if err != nil {
		t.Fatal(err)
	}
	methods := []string{"OTP", "QR"}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, m := range methods {
		go func(method string) {
			<-start
			_, e := f.service.Confirm(context.Background(), command(f, vendorownedhandover.Pickup, o.HandoverRef, method, f.recorder.get(o.HandoverRef, method), "verify-"+method), "vendor-service", "race")
			errs <- e
		}(m)
	}
	close(start)
	success := 0
	for range methods {
		if <-errs == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("canonical consumers=%d want 1", success)
	}
}

func TestVendorOwnedReturnPostgresOTPVsQRAndCompletionRace(t *testing.T) {
	f := newVendorAuthorityFixture(t)
	defer f.cleanup()
	pickup, err := f.service.Read(context.Background(), f.binding.MissionRef, vendorownedhandover.Pickup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Confirm(context.Background(), command(f, vendorownedhandover.Pickup, pickup.HandoverRef, "OTP", f.recorder.get(pickup.HandoverRef, "OTP"), "pickup-before-return"), "vendor-service", "return-race"); err != nil {
		t.Fatal(err)
	}
	issued, err := f.service.IssueReturn(context.Background(), f.binding.MissionRef, "issue-return", "return-race")
	if err != nil {
		t.Fatal(err)
	}
	methods := []string{"OTP", "QR"}
	start := make(chan struct{})
	errs := make(chan error, len(methods))
	for _, method := range methods {
		go func(method string) {
			<-start
			_, verifyErr := f.service.Confirm(context.Background(), command(f, vendorownedhandover.Return, issued.HandoverRef, method, f.recorder.get(issued.HandoverRef, method), "return-"+method), "vendor-service", "return-race")
			errs <- verifyErr
		}(method)
	}
	close(start)
	success := 0
	for range methods {
		if <-errs == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful return consumes=%d, want 1", success)
	}
	var status string
	if err = f.pool.QueryRow(context.Background(), `SELECT status FROM dispatch_vendor_delivery_missions WHERE mission_ref=$1`, f.binding.MissionRef).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "RETURNED" {
		t.Fatalf("mission status=%s", status)
	}
}

func TestVendorOwnedHandoverPostgresSameCredentialRaces(t *testing.T) {
	for _, method := range []string{"OTP", "QR"} {
		t.Run(method, func(t *testing.T) {
			f := newVendorAuthorityFixture(t)
			defer f.cleanup()
			o, _ := f.service.Read(context.Background(), f.binding.MissionRef, vendorownedhandover.Pickup)
			start := make(chan struct{})
			errs := make(chan error, 2)
			for i := 0; i < 2; i++ {
				go func(n int) {
					<-start
					_, e := f.service.Confirm(context.Background(), command(f, vendorownedhandover.Pickup, o.HandoverRef, method, f.recorder.get(o.HandoverRef, method), fmt.Sprintf("same-%s-%d", method, n)), "vendor-service", "race")
					errs <- e
				}(i)
			}
			close(start)
			success := 0
			for i := 0; i < 2; i++ {
				if <-errs == nil {
					success++
				}
			}
			if success != 1 {
				t.Fatalf("consumers=%d", success)
			}
		})
	}
}

func TestVendorOwnedHandoverPostgresDuplicateRelayIdempotent(t *testing.T) {
	f := newVendorAuthorityFixture(t)
	defer f.cleanup()
	o, _ := f.service.Read(context.Background(), f.binding.MissionRef, vendorownedhandover.Pickup)
	c := command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", f.recorder.get(o.HandoverRef, "OTP"), "same-operation")
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := f.service.Confirm(context.Background(), c, "vendor-service", "duplicate")
			results <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("idempotent relay: %v", err)
		}
	}
}

func TestVendorOwnedHandoverPostgresResendInvalidatesBothCredentials(t *testing.T) {
	f := newVendorAuthorityFixture(t)
	defer f.cleanup()
	ctx := context.Background()
	old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
	oldOTP, oldQR := f.recorder.get(old.HandoverRef, "OTP"), f.recorder.get(old.HandoverRef, "QR")
	// Move only the authoritative cooldown clock; no sleep is used for correctness.
	_, err := f.pool.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET resend_cooldown_until=now()-interval '1 second' WHERE challenge_ref=$1`, old.HandoverRef)
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.service.Resend(ctx, f.binding.MissionRef, vendorownedhandover.Pickup, f.binding.ProviderRef, "resend", "vendor-service", "resend")
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range []struct{ method, credential string }{{"OTP", oldOTP}, {"QR", oldQR}} {
		if _, err = f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, v.method, v.credential, fmt.Sprintf("old-%d", i)), "vendor-service", "old"); err == nil {
			t.Fatalf("old %s survived resend", v.method)
		}
	}
	if current.HandoverRef == old.HandoverRef {
		t.Fatal("resend did not rotate challenge")
	}
}

func consumePickup(t *testing.T, f vendorAuthorityFixture) vendorownedhandover.Observation {
	t.Helper()
	ctx := context.Background()
	pickup, err := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, pickup.HandoverRef, "OTP", f.recorder.get(pickup.HandoverRef, "OTP"), "pickup"), "vendor-service", "pickup"); err != nil {
		t.Fatal(err)
	}
	delivery, err := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	return delivery
}

func TestVendorOwnedHandoverPostgresPickupAndDeliveryOTPQR(t *testing.T) {
	for _, method := range []string{"OTP", "QR"} {
		t.Run(method, func(t *testing.T) {
			f := newVendorAuthorityFixture(t)
			defer f.cleanup()
			delivery := consumePickup(t, f)
			o, err := f.service.Confirm(context.Background(), command(f, vendorownedhandover.Delivery, delivery.HandoverRef, method, f.recorder.get(delivery.HandoverRef, method), "delivery-"+method), "vendor-service", "delivery")
			if err != nil || o.Status != "CONSUMED" {
				t.Fatalf("delivery %s: %#v %v", method, o, err)
			}
			other := "OTP"
			if method == "OTP" {
				other = "QR"
			}
			if _, err = f.service.Confirm(context.Background(), command(f, vendorownedhandover.Delivery, delivery.HandoverRef, other, f.recorder.get(delivery.HandoverRef, other), "delivery-replay"), "vendor-service", "delivery"); err == nil {
				t.Fatal("alternate credential replayed after consumption")
			}
		})
	}
}

func TestVendorOwnedHandoverPostgresProviderAndReceiverRotation(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		oldOTP := f.recorder.get(old.HandoverRef, "OTP")
		_, err := f.service.Reassign(ctx, vendorownedhandover.ChangeCommand{MissionRef: f.binding.MissionRef, ProviderRef: "replacement-staff", OperationRef: "reassign"}, "reassign")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", oldOTP, "old-provider"), "vendor-service", "old"); err == nil {
			t.Fatal("old provider challenge survived reassignment")
		}
	})
	t.Run("receiver", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old := consumePickup(t, f)
		oldOTP := f.recorder.get(old.HandoverRef, "OTP")
		_, err := f.service.ChangeReceiver(ctx, vendorownedhandover.ChangeCommand{MissionRef: f.binding.MissionRef, ReceiverRef: "replacement-receiver", ReceiverPhone: "+15550000002", OperationRef: "receiver-change"}, "receiver-change")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.service.Confirm(ctx, command(f, vendorownedhandover.Delivery, old.HandoverRef, "OTP", oldOTP, "old-receiver"), "vendor-service", "old"); err == nil {
			t.Fatal("old receiver challenge survived receiver change")
		}
	})
}

func TestVendorOwnedHandoverPostgresExpiryAttemptsBindingsAndCancellation(t *testing.T) {
	t.Run("expiry", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		o, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		_, _ = f.pool.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET expires_at=now()-interval '1 second' WHERE challenge_ref=$1`, o.HandoverRef)
		if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", f.recorder.get(o.HandoverRef, "OTP"), "expired"), "vendor-service", "expired"); err == nil {
			t.Fatal("expired credential succeeded")
		}
	})
	t.Run("attempt ceiling", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		o, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		for i := 0; i < 3; i++ {
			_, _ = f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", "000000", fmt.Sprintf("wrong-%d", i)), "vendor-service", "wrong")
		}
		if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", f.recorder.get(o.HandoverRef, "OTP"), "after-lock"), "vendor-service", "locked"); err == nil {
			t.Fatal("correct credential bypassed attempt ceiling")
		}
	})
	t.Run("provider and mission", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		o, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		c := command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", f.recorder.get(o.HandoverRef, "OTP"), "wrong-provider")
		c.ProviderRef = "other"
		if _, err := f.service.Confirm(ctx, c, "vendor-service", "wrong"); err == nil {
			t.Fatal("wrong provider succeeded")
		}
		c = command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", f.recorder.get(o.HandoverRef, "OTP"), "wrong-mission")
		c.MissionRef = "another-mission"
		if _, err := f.service.Confirm(ctx, c, "vendor-service", "wrong"); err == nil {
			t.Fatal("wrong mission succeeded")
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		o, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		if err := f.service.Cancel(ctx, f.binding.MissionRef, "cancel", "cancel"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, o.HandoverRef, "OTP", f.recorder.get(o.HandoverRef, "OTP"), "after-cancel"), "vendor-service", "cancel"); err == nil {
			t.Fatal("cancelled mission revived")
		}
	})
}

func TestVendorOwnedHandoverPostgresSplitMissionIsolation(t *testing.T) {
	f1 := newVendorAuthorityFixture(t)
	defer f1.cleanup()
	f2 := newVendorAuthorityFixture(t)
	defer f2.cleanup()
	ctx := context.Background()
	o1, _ := f1.service.Read(ctx, f1.binding.MissionRef, vendorownedhandover.Pickup)
	o2, _ := f2.service.Read(ctx, f2.binding.MissionRef, vendorownedhandover.Pickup)
	c := command(f1, vendorownedhandover.Pickup, o1.HandoverRef, "OTP", f1.recorder.get(o1.HandoverRef, "OTP"), "cross")
	c.MissionRef = f2.binding.MissionRef
	c.ChallengeRef = o2.HandoverRef
	if _, err := f2.service.Confirm(ctx, c, "vendor-service", "cross"); err == nil {
		t.Fatal("credential crossed physical missions")
	}
}

func TestVendorOwnedHandoverPostgresMutationVerificationRaces(t *testing.T) {
	t.Run("resend-vs-verify", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		credential := f.recorder.get(old.HandoverRef, "OTP")
		_, _ = f.pool.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET resend_cooldown_until=now()-interval '1 second' WHERE challenge_ref=$1`, old.HandoverRef)
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.Resend(ctx, f.binding.MissionRef, vendorownedhandover.Pickup, f.binding.ProviderRef, "race-resend", "vendor-service", "race")
			done <- err
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", credential, "race-verify"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		latest, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		if latest.HandoverRef != old.HandoverRef {
			if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", credential, "post-resend-old"), "vendor-service", "race"); err == nil {
				t.Fatal("old credential worked after resend committed")
			}
		}
	})
	t.Run("reassignment-vs-verify", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		credential := f.recorder.get(old.HandoverRef, "OTP")
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.Reassign(ctx, vendorownedhandover.ChangeCommand{MissionRef: f.binding.MissionRef, ProviderRef: "race-replacement", OperationRef: "race-reassign"}, "race")
			done <- err
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", credential, "race-provider-verify"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		latest, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		if latest.HandoverRef != old.HandoverRef {
			if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", credential, "post-reassign-old"), "vendor-service", "race"); err == nil {
				t.Fatal("old provider credential worked after reassignment")
			}
		}
	})
	t.Run("receiver-change-vs-verify", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old := consumePickup(t, f)
		credential := f.recorder.get(old.HandoverRef, "QR")
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.ChangeReceiver(ctx, vendorownedhandover.ChangeCommand{MissionRef: f.binding.MissionRef, ReceiverRef: "race-receiver", ReceiverPhone: "+15550000999", OperationRef: "race-receiver-change"}, "race")
			done <- err
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Delivery, old.HandoverRef, "QR", credential, "race-receiver-verify"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		latest, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Delivery)
		if latest.HandoverRef != old.HandoverRef {
			if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Delivery, old.HandoverRef, "QR", credential, "post-receiver-old"), "vendor-service", "race"); err == nil {
				t.Fatal("old receiver credential worked after receiver change")
			}
		}
	})
}

func TestVendorOwnedHandoverPostgresAlternateCredentialAndCancellationRaces(t *testing.T) {
	t.Run("resend-vs-qr-verify", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		credential := f.recorder.get(old.HandoverRef, "QR")
		_, _ = f.pool.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET resend_cooldown_until=now()-interval '1 second' WHERE challenge_ref=$1`, old.HandoverRef)
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.Resend(ctx, f.binding.MissionRef, vendorownedhandover.Pickup, f.binding.ProviderRef, "race-resend-qr", "vendor-service", "race")
			done <- err
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "QR", credential, "race-verify-qr"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		latest, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		if latest.HandoverRef != old.HandoverRef {
			if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "QR", credential, "post-resend-old-qr"), "vendor-service", "race"); err == nil {
				t.Fatal("old QR worked after resend committed")
			}
		}
	})

	t.Run("reassignment-vs-qr-verify", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		credential := f.recorder.get(old.HandoverRef, "QR")
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.Reassign(ctx, vendorownedhandover.ChangeCommand{MissionRef: f.binding.MissionRef, ProviderRef: "race-qr-replacement", OperationRef: "race-reassign-qr"}, "race")
			done <- err
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "QR", credential, "race-provider-verify-qr"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		latest, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		if latest.HandoverRef != old.HandoverRef {
			if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "QR", credential, "post-reassign-old-qr"), "vendor-service", "race"); err == nil {
				t.Fatal("old provider QR worked after reassignment")
			}
		}
	})

	t.Run("receiver-change-vs-otp-verify", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old := consumePickup(t, f)
		credential := f.recorder.get(old.HandoverRef, "OTP")
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.ChangeReceiver(ctx, vendorownedhandover.ChangeCommand{MissionRef: f.binding.MissionRef, ReceiverRef: "race-otp-receiver", ReceiverPhone: "+15550000888", OperationRef: "race-receiver-change-otp"}, "race")
			done <- err
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Delivery, old.HandoverRef, "OTP", credential, "race-receiver-verify-otp"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		latest, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Delivery)
		if latest.HandoverRef != old.HandoverRef {
			if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Delivery, old.HandoverRef, "OTP", credential, "post-receiver-old-otp"), "vendor-service", "race"); err == nil {
				t.Fatal("old receiver OTP worked after receiver change")
			}
		}
	})

	t.Run("cancellation-vs-consumption", func(t *testing.T) {
		f := newVendorAuthorityFixture(t)
		defer f.cleanup()
		ctx := context.Background()
		old, _ := f.service.Read(ctx, f.binding.MissionRef, vendorownedhandover.Pickup)
		credential := f.recorder.get(old.HandoverRef, "OTP")
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			done <- f.service.Cancel(ctx, f.binding.MissionRef, "race-cancel", "race")
		}()
		go func() {
			<-start
			_, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", credential, "race-cancel-verify"), "vendor-service", "race")
			done <- err
		}()
		close(start)
		<-done
		<-done
		var status string
		if err := f.pool.QueryRow(ctx, `SELECT status FROM dispatch_vendor_delivery_missions WHERE mission_ref=$1`, f.binding.MissionRef).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "CANCELLED" {
			t.Fatalf("mission revived after cancellation race: %s", status)
		}
		if _, err := f.service.Confirm(ctx, command(f, vendorownedhandover.Pickup, old.HandoverRef, "OTP", credential, "post-cancel-race"), "vendor-service", "race"); err == nil {
			t.Fatal("credential revived a cancelled mission")
		}
	})
}

func TestVendorOwnedHandoverMigrationSecurityShape(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	for _, forbidden := range []string{"otp", "qr_token", "credential", "secret"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name IN ('dispatch_vendor_delivery_missions','dispatch_vendor_handover_challenges') AND column_name=$1`, forbidden).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("plaintext-capable column %s exists", forbidden)
		}
	}
	for _, required := range []string{"otp_keyed_hash", "qr_keyed_hash"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='dispatch_vendor_handover_challenges' AND column_name=$1`, required).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", required, count, err)
		}
	}
}
