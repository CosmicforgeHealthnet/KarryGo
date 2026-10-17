package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/mediaclient"
)

func TestNewUploaderFromEnvUsesGenericLocalPrivateRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("VERIFICATION_STORAGE_MODE", "local_private")
	t.Setenv("VERIFICATION_UPLOAD_ROOT", root)
	t.Setenv("VERIFICATION_STORAGE_BASE_URL", "")

	uploader, err := NewUploaderFromEnv("development", nil)
	if err != nil {
		t.Fatalf("NewUploaderFromEnv() error = %v", err)
	}

	header := stubFileHeader{
		filename:    "govt-id.pdf",
		contentType: "application/pdf",
		content:     []byte("%PDF test"),
	}
	file, err := header.Open()
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()

	fileURL, err := uploader.Upload(context.Background(), "verifications/provider-123/identity/govt-id.pdf", file, header)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if fileURL != "local-private://verifications/provider-123/identity/govt-id.pdf" {
		t.Fatalf("fileURL = %q, want local-private URL", fileURL)
	}

	stored, err := os.ReadFile(filepath.Join(root, "verifications", "provider-123", "identity", "govt-id.pdf"))
	if err != nil {
		t.Fatalf("stored upload was not written: %v", err)
	}
	if string(stored) != string(header.content) {
		t.Fatalf("stored content = %q, want %q", stored, header.content)
	}
}

func TestLocalVerificationEvidenceUsesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACL semantics differ from POSIX mode bits")
	}
	root := t.TempDir()
	uploader := NewLocalFileUploader(root, "local-private://")
	header := stubFileHeader{
		filename: "selfie.jpg", contentType: "image/jpeg",
		content: []byte("private-biometric-capture"),
	}
	object := "verifications/provider-123/face/test-private-selfie.jpg"
	first, err := header.Open()
	if err != nil { t.Fatal(err) }
	_, err = uploader.Upload(context.Background(), object, first, header)
	_ = first.Close()
	if err != nil { t.Fatal(err) }
	path := filepath.Join(root, filepath.FromSlash(object))
	info, err := os.Stat(path)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("selfie mode=%#o, want 0600", info.Mode().Perm())
	}
	folder, err := os.Stat(filepath.Dir(path))
	if err != nil { t.Fatal(err) }
	if folder.Mode().Perm() != 0o700 {
		t.Fatalf("verification folder mode=%#o, want 0700", folder.Mode().Perm())
	}
	second, err := header.Open()
	if err != nil { t.Fatal(err) }
	_, err = uploader.Upload(context.Background(), object, second, header)
	_ = second.Close()
	if err == nil { t.Fatal("must refuse overwriting existing biometric evidence") }
}

func TestLocalPrivateReaderRejectsTraversalAndAllowsStoredEvidence(t *testing.T) {
	root := t.TempDir()
	uploader := NewLocalFileUploader(root, "local-private://")
	header := stubFileHeader{
		filename: "proof.pdf", contentType: "application/pdf",
		content: []byte("%PDF-1.4\\nPrivate identity evidence"),
	}
	file, err := header.Open()
	if err != nil { t.Fatal(err) }
	reference, err := uploader.Upload(context.Background(), "verifications/provider-1/identity/proof.pdf", file, header)
	_ = file.Close()
	if err != nil { t.Fatal(err) }
	stored, err := uploader.OpenPrivate(context.Background(), reference)
	if err != nil { t.Fatal(err) }
	buf := make([]byte, 64)
	n, err := stored.Read(buf)
	_ = stored.Close()
	if n == 0 || string(buf[:n]) != string(header.content) {
		t.Fatalf("unexpected private evidence content: n=%d error=%v", n, err)
	}
	for _, malformed := range []string{
		"file:///etc/passwd",
		"local-private://../../etc/passwd",
		"local-private://verifications/../private",
		"local-private://verifications/%2e%2e/secret",
	} {
		asset, err := uploader.OpenPrivate(context.Background(), malformed)
		if asset != nil { _ = asset.Close() }
		if err == nil { t.Fatalf("accepted unsafe file reference: %q", malformed) }
	}
}

func TestBuildVerificationObjectPathIsPrivateVerificationScoped(t *testing.T) {
	objectPath := buildVerificationObjectPath("provider-123", StepIdentity, `..\Govt ID?.pdf`)

	if !strings.HasPrefix(objectPath, "verifications/provider-123/identity/") {
		t.Fatalf("objectPath = %q, want verification identity prefix", objectPath)
	}
	if !strings.HasSuffix(objectPath, "_Govt_ID_.pdf") {
		t.Fatalf("objectPath = %q, want sanitized filename suffix", objectPath)
	}
	if strings.Contains(objectPath, "..") || strings.Contains(objectPath, "\\") {
		t.Fatalf("objectPath = %q, want no traversal or backslashes", objectPath)
	}
	if _, err := cleanStoragePath(objectPath); err != nil {
		t.Fatalf("cleanStoragePath(%q) error = %v", objectPath, err)
	}
}

func TestNewUploaderFromEnvFailsClearlyForMissingNonLocalConfig(t *testing.T) {
	t.Run("firebase mode selected", func(t *testing.T) {
		t.Setenv("VERIFICATION_STORAGE_MODE", "firebase")

		uploader, err := NewUploaderFromEnv("development", nil)
		if uploader != nil {
			t.Fatalf("uploader = %#v, want nil", uploader)
		}
		if !errors.Is(err, ErrFirebaseStorageNotConfigured) {
			t.Fatalf("err = %v, want ErrFirebaseStorageNotConfigured", err)
		}
	})

	t.Run("production mode omitted", func(t *testing.T) {
		t.Setenv("VERIFICATION_STORAGE_MODE", "")

		uploader, err := NewUploaderFromEnv("production", nil)
		if uploader != nil {
			t.Fatalf("uploader = %#v, want nil", uploader)
		}
		if !errors.Is(err, ErrStorageNotConfigured) {
			t.Fatalf("err = %v, want ErrStorageNotConfigured", err)
		}
	})

	// Regression guard for the "dispatch cannot boot in production" bug: mode
	// "media" without a configured client must fail with a clear error, not a
	// nil-pointer panic at upload time.
	t.Run("media mode without client configured", func(t *testing.T) {
		t.Setenv("VERIFICATION_STORAGE_MODE", "media")

		uploader, err := NewUploaderFromEnv("production", nil)
		if uploader != nil {
			t.Fatalf("uploader = %#v, want nil", uploader)
		}
		if err == nil {
			t.Fatal("expected an error when media mode has no client configured")
		}
	})
}

// TestMediaFileUploaderUploadsToMediaFileService is the regression guard for
// dispatch's production storage gap: previously every *_STORAGE_MODE=firebase
// (and unset-in-production) uploader returned a hard error and cmd/main.go
// fataled on boot. Mode "media" now routes through media-file-service, which
// is what actually works in production.
func TestMediaFileUploaderUploadsToMediaFileService(t *testing.T) {
	var gotOwnerService, gotOwnerID, gotPurpose string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Karrygo-Service") != "dispatch-delivery-service" {
			t.Fatalf("missing/wrong service header: %s", r.Header.Get("X-Karrygo-Service"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotOwnerService = r.FormValue("owner_service")
		gotOwnerID = r.FormValue("owner_id")
		gotPurpose = r.FormValue("purpose")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"id":  "asset-1",
				"url": "https://storage.googleapis.com/bucket/verifications/provider-123/identity/asset-1_govt-id.pdf",
			},
		})
	}))
	defer server.Close()

	client := mediaclient.New(mediaclient.Config{BaseURL: server.URL, ServiceName: "dispatch-delivery-service", Token: "development-media-token"})
	uploader := NewMediaFileUploader(client)

	header := stubFileHeader{filename: "govt-id.pdf", contentType: "application/pdf", content: []byte("%PDF test")}
	file, err := header.Open()
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()

	fileURL, err := uploader.Upload(context.Background(), "verifications/provider-123/identity/uuid_govt-id.pdf", file, header)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if fileURL != "https://storage.googleapis.com/bucket/verifications/provider-123/identity/asset-1_govt-id.pdf" {
		t.Fatalf("fileURL = %q, want the media-file-service asset URL", fileURL)
	}
	if gotOwnerService != "dispatch-delivery-service" {
		t.Fatalf("owner_service = %q", gotOwnerService)
	}
	if gotOwnerID != "provider-123" {
		t.Fatalf("owner_id = %q, want provider ID extracted from the object path", gotOwnerID)
	}
	if gotPurpose != "document_file" {
		t.Fatalf("purpose = %q, want document_file", gotPurpose)
	}
}

type stubFileHeader struct {
	filename    string
	contentType string
	content     []byte
}

func (h stubFileHeader) Open() (File, error) {
	return memoryFile{Reader: bytes.NewReader(h.content)}, nil
}

func (h stubFileHeader) GetFilename() string {
	return h.filename
}

func (h stubFileHeader) GetSize() int64 {
	return int64(len(h.content))
}

func (h stubFileHeader) GetHeaderValue(key string) string {
	if strings.EqualFold(key, "Content-Type") {
		return h.contentType
	}
	return ""
}

type memoryFile struct {
	*bytes.Reader
}

func (f memoryFile) Close() error {
	return nil
}
