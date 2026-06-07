package verification

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewUploaderFromEnvUsesGenericLocalPrivateRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("VERIFICATION_STORAGE_MODE", "local_private")
	t.Setenv("VERIFICATION_UPLOAD_ROOT", root)
	t.Setenv("VERIFICATION_STORAGE_BASE_URL", "")

	uploader, err := NewUploaderFromEnv("development")
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

		uploader, err := NewUploaderFromEnv("development")
		if uploader != nil {
			t.Fatalf("uploader = %#v, want nil", uploader)
		}
		if !errors.Is(err, ErrFirebaseStorageNotConfigured) {
			t.Fatalf("err = %v, want ErrFirebaseStorageNotConfigured", err)
		}
	})

	t.Run("production mode omitted", func(t *testing.T) {
		t.Setenv("VERIFICATION_STORAGE_MODE", "")

		uploader, err := NewUploaderFromEnv("production")
		if uploader != nil {
			t.Fatalf("uploader = %#v, want nil", uploader)
		}
		if !errors.Is(err, ErrStorageNotConfigured) {
			t.Fatalf("err = %v, want ErrStorageNotConfigured", err)
		}
	})
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
