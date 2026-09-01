package securehandover

import (
	"regexp"
	"strings"
	"testing"
)

func TestCredentialCipherProducesBoundIndependentCredentials(t *testing.T) {
	cipher, err := newCredentialCipher([]byte("test-only-secure-handover-secret"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := randomReference("handover")
	if err != nil {
		t.Fatal(err)
	}
	otp := cipher.deriveOTP(ref, 1)
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(otp) {
		t.Fatalf("OTP format = %q", otp)
	}
	qr := cipher.deriveQR(ref, 1)
	if strings.Contains(qr, otp) {
		t.Fatal("QR token contains the OTP")
	}
	otpHash := cipher.hash(MethodOTP, ref, 1, otp)
	qrHash := cipher.hash(MethodQR, ref, 1, qr)
	if !cipher.verify(MethodOTP, ref, 1, otp, otpHash) || !cipher.verify(MethodQR, ref, 1, qr, qrHash) {
		t.Fatal("valid credential did not verify")
	}
	if cipher.verify(MethodOTP, ref, 2, otp, otpHash) || cipher.verify(MethodQR, ref+"x", 1, qr, qrHash) {
		t.Fatal("credential was not bound to challenge reference and version")
	}
	if otpHash == otp || qrHash == qr {
		t.Fatal("stored credential hash equals plaintext credential")
	}
}

func TestContextKeyBindsStageManifestAndVersion(t *testing.T) {
	context := Context{MissionRef: "mission-a", StopRef: "pickup-a", ManifestRef: "manifest-a", ManifestVersion: 3}
	pickup := context.ContextKey(StagePickup)
	if pickup == context.ContextKey(StageDelivery) {
		t.Fatal("stage is not bound")
	}
	context.ManifestVersion++
	if pickup == context.ContextKey(StagePickup) {
		t.Fatal("manifest version is not bound")
	}
}

func TestActorAuthorizationIsObjectScoped(t *testing.T) {
	context := Context{CustomerID: "customer-a", ProviderID: "provider-a", SourceType: "VENDOR_COMMERCE", SourceRef: "order-a", MissionID: "mission-a", MissionRef: "mission-a"}
	if !actorAuthorized(Actor{Type: "CUSTOMER", Ref: "customer-a"}, context, StagePickup) || actorAuthorized(Actor{Type: "CUSTOMER", Ref: "customer-b"}, context, StagePickup) {
		t.Fatal("customer ownership scope is incorrect")
	}
	if !actorAuthorized(Actor{Type: "PROVIDER", Ref: "provider-a"}, context, StageDelivery) || actorAuthorized(Actor{Type: "PROVIDER", Ref: "provider-b"}, context, StageDelivery) {
		t.Fatal("provider assignment scope is incorrect")
	}
	if !actorAuthorized(Actor{Type: "VENDOR", SourceRef: "mission-a"}, context, StagePickup) || actorAuthorized(Actor{Type: "VENDOR", SourceRef: "mission-b"}, context, StagePickup) {
		t.Fatal("vendor source object scope is incorrect")
	}
}
