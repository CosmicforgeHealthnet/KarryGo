package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/mediaclient"
)

// TestMediaAvatarUploaderUploadsToMediaFileService is the regression guard
// for dispatch's production storage gap: avatar uploads previously had no
// backend that worked outside local dev. Mode "media" now routes through
// media-file-service, inferring content type from the extension the caller
// already appended (avatarExtension only ever produces .jpg or .png; the
// AvatarUploader interface carries no separate content-type parameter).
func TestMediaAvatarUploaderUploadsToMediaFileService(t *testing.T) {
	var gotOwnerID, gotPurpose, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotOwnerID = r.FormValue("owner_id")
		gotPurpose = r.FormValue("purpose")
		if _, header, err := r.FormFile("file"); err == nil {
			gotContentType = header.Header.Get("Content-Type")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"id":  "asset-1",
				"url": "https://storage.googleapis.com/bucket/profiles/provider-1/asset-1_avatar.png",
			},
		})
	}))
	defer server.Close()

	client := mediaclient.New(mediaclient.Config{BaseURL: server.URL, ServiceName: "dispatch-delivery-service", Token: "development-media-token"})
	uploader := newMediaAvatarUploader(client)

	fileURL, err := uploader.Upload(context.Background(), "profiles/provider-1/avatar_uuid.png", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if fileURL != "https://storage.googleapis.com/bucket/profiles/provider-1/asset-1_avatar.png" {
		t.Fatalf("fileURL = %q, want the media-file-service asset URL", fileURL)
	}
	if gotOwnerID != "provider-1" {
		t.Fatalf("owner_id = %q, want provider ID extracted from the object path", gotOwnerID)
	}
	if gotPurpose != "profile_photo" {
		t.Fatalf("purpose = %q, want profile_photo", gotPurpose)
	}
	if gotContentType != "image/png" {
		t.Fatalf("content type = %q, want image/png inferred from the .png extension", gotContentType)
	}
}

func TestAvatarContentTypeFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"profiles/p-1/avatar_x.png", "image/png"},
		{"profiles/p-1/avatar_x.jpg", "image/jpeg"},
		{"profiles/p-1/avatar_x.PNG", "image/png"},
	}
	for _, tc := range cases {
		if got := avatarContentTypeFromPath(tc.path); got != tc.want {
			t.Fatalf("avatarContentTypeFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestNewAvatarUploaderFromEnvMediaModeRequiresClient is the regression guard
// for a nil-pointer panic at upload time when mode "media" is selected but no
// media-file-service client is configured.
func TestNewAvatarUploaderFromEnvMediaModeRequiresClient(t *testing.T) {
	t.Setenv("AVATAR_STORAGE_MODE", "media")
	uploader, err := NewAvatarUploaderFromEnv("production", nil)
	if uploader != nil {
		t.Fatalf("uploader = %#v, want nil", uploader)
	}
	if err == nil {
		t.Fatal("expected an error when media mode has no client configured")
	}
}
