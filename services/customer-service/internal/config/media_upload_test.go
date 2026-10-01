package config

import (
	"strings"
	"testing"
)

func TestMediaUploadSessionFlagDefaultsOffAndFailsClosedWhenIncomplete(t *testing.T) {
	t.Setenv("CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED", "")
	if Load().MediaUploadSessionEnabled {
		t.Fatal("CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED must default to false")
	}
	if err := (Config{}).ValidateMediaUploadSessions(); err != nil {
		t.Fatalf("a disabled flag must not require configuration: %v", err)
	}
	err := (Config{MediaUploadSessionEnabled: true, MediaBaseURL: "http://media:8109"}).ValidateMediaUploadSessions()
	if err == nil {
		t.Fatal("an enabled flag with missing dependencies started")
	}
	for _, name := range []string{"CUSTOMER_MEDIA_PUBLIC_BASE_URL", "CUSTOMER_SUPPORT_BASE_URL", "CUSTOMER_MEDIA_PRIVATE_SECRET"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}
	complete := Config{MediaUploadSessionEnabled: true, MediaBaseURL: "http://media:8109", MediaPublicBaseURL: "https://api.example",
		SupportBaseURL: "http://support:8107", MediaPrivateSecret: []byte("s")}
	if err := complete.ValidateMediaUploadSessions(); err != nil {
		t.Fatalf("complete configuration refused: %v", err)
	}
	if strings.Contains(err.Error(), "media:8109") {
		t.Fatal("validation error echoes configuration values")
	}
}
