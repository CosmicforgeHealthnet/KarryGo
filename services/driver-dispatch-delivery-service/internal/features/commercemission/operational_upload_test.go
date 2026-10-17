package commercemission

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestOperationalImageRejectsInvalidAndOversizedContent(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("%PDF-1.7"), []byte("not a photo"), {0x89, 'P', 'N', 'G'}} {
		if _, _, err := operationalImage(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	if _, _, err := operationalImage(bytes.NewReader([]byte{1}), maxOperationalImage+1); err == nil {
		t.Fatal("oversized image accepted")
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	data, mime, err := operationalImage(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil || mime != "image/png" || len(data) != buffer.Len() {
		t.Fatalf("valid PNG rejected: %s %v", mime, err)
	}
	if _, _, err = operationalImage(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()+1)); err == nil {
		t.Fatal("truncated upload accepted")
	}
}
func TestProviderExceptionOptionsFollowAuthorityAndRecoveryPolicy(t *testing.T) {
	if len(providerExceptionOptions("IN_TRANSIT", false)) != 0 {
		t.Fatal("replacement rider got current-provider actions")
	}
	for _, state := range []string{"COMPLETED", "CANCELLED", "RETURNED"} {
		if len(providerExceptionOptions(state, true)) != 0 {
			t.Fatalf("terminal state %s has actions", state)
		}
	}
	for _, option := range providerExceptionOptions("IN_TRANSIT", true) {
		if option.Reason == ReasonVendorNotReady {
			t.Fatal("vendor-not-ready offered after pickup")
		}
		for _, action := range option.RecoveryActions {
			if action == RecoveryAlternativeReceiver || !RecoveryAllowed(option.Reason, action, false) {
				t.Fatalf("unsupported recovery %s %s", option.Reason, action)
			}
		}
		if option.PhotoRequired != (option.Reason == ReasonPackageMismatch || option.Reason == ReasonDamagedPackage) {
			t.Fatal("photo requirement mismatch")
		}
	}
}
