package trip

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"cosmicforge/logistics/shared/go/mediaclient"
)

// TestMediaProofStorageUploadsToMediaFileService is the regression guard for
// dispatch's production storage gap: trip proof uploads previously had no
// backend that worked outside local dev (firebase mode was never
// implemented, and unset mode hard-errors in production). Mode "media" now
// routes through media-file-service, split into the "proof_image" /
// "signature" purposes by upload kind.
func TestMediaProofStorageUploadsToMediaFileService(t *testing.T) {
	var gotOwnerID, gotPurpose string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotOwnerID = r.FormValue("owner_id")
		gotPurpose = r.FormValue("purpose")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"id":  "asset-1",
				"url": "https://storage.googleapis.com/bucket/trips/provider-1/trip-1/proof/asset-1_photo.jpg",
			},
		})
	}))
	defer server.Close()

	client := mediaclient.New(mediaclient.Config{BaseURL: server.URL, ServiceName: "dispatch-delivery-service", Token: "development-media-token"})
	storage := NewMediaProofStorage(client)

	content := append(fakeJPEGHeader, make([]byte, 20)...)
	header := &multipart.FileHeader{
		Filename: "photo.jpg",
		Size:     int64(len(content)),
		Header:   textproto.MIMEHeader{"Content-Type": []string{"image/jpeg"}},
	}
	file := &fakeReadSeeker{Reader: bytes.NewReader(content)}

	ref, err := storage.SaveProofFile(context.Background(), "provider-1", "trip-1", file, header, "photo")
	if err != nil {
		t.Fatalf("SaveProofFile: %v", err)
	}
	if ref != "https://storage.googleapis.com/bucket/trips/provider-1/trip-1/proof/asset-1_photo.jpg" {
		t.Fatalf("ref = %q, want the media-file-service asset URL", ref)
	}
	if gotOwnerID != "trip-1" {
		t.Fatalf("owner_id = %q, want trip ID", gotOwnerID)
	}
	if gotPurpose != "proof_image" {
		t.Fatalf("purpose = %q, want proof_image for kind=photo", gotPurpose)
	}
}

func TestMediaProofStorageSignatureKindUsesSignaturePurpose(t *testing.T) {
	var gotPurpose string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		gotPurpose = r.FormValue("purpose")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    map[string]interface{}{"id": "asset-2", "url": "https://example.test/asset-2"},
		})
	}))
	defer server.Close()

	client := mediaclient.New(mediaclient.Config{BaseURL: server.URL, ServiceName: "dispatch-delivery-service", Token: "development-media-token"})
	storage := NewMediaProofStorage(client)

	content := append(fakeJPEGHeader, make([]byte, 20)...)
	header := &multipart.FileHeader{
		Filename: "sig.jpg",
		Size:     int64(len(content)),
		Header:   textproto.MIMEHeader{"Content-Type": []string{"image/jpeg"}},
	}
	file := &fakeReadSeeker{Reader: bytes.NewReader(content)}

	if _, err := storage.SaveProofFile(context.Background(), "provider-1", "trip-1", file, header, "signature"); err != nil {
		t.Fatalf("SaveProofFile: %v", err)
	}
	if gotPurpose != "signature" {
		t.Fatalf("purpose = %q, want signature for kind=signature", gotPurpose)
	}
}

// TestNewProofStorageFromEnvMediaModeRequiresClient is the regression guard
// for a nil-pointer panic at upload time when mode "media" is selected but no
// media-file-service client is configured.
func TestNewProofStorageFromEnvMediaModeRequiresClient(t *testing.T) {
	t.Setenv("TRIP_PROOF_STORAGE_MODE", "media")
	storage, err := NewProofStorageFromEnv("production", nil)
	if storage != nil {
		t.Fatalf("storage = %#v, want nil", storage)
	}
	if err == nil {
		t.Fatal("expected an error when media mode has no client configured")
	}
	if errors.Is(err, ErrProofStorageNotConfigured) {
		t.Fatal("expected the media-specific error, not the generic unconfigured error")
	}
}
