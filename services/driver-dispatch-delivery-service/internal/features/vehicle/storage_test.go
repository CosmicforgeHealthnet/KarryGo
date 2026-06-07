package vehicle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewVehicleUploaderFromEnvUsesLocalPrivateRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("VERIFICATION_STORAGE_MODE", "local_private")
	t.Setenv("VERIFICATION_UPLOAD_ROOT", root)
	t.Setenv("VERIFICATION_STORAGE_BASE_URL", "")

	uploader, err := NewVehicleUploaderFromEnv("development")
	if err != nil {
		t.Fatalf("NewVehicleUploaderFromEnv() error = %v", err)
	}

	header := &stubFileHeader{
		filename:    "reg.pdf",
		contentType: "application/pdf",
		content:     []byte("%PDF vehicle test"),
	}
	file, err := header.Open()
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()

	fileURL, err := uploader.Upload(context.Background(), "vehicles/p-1/b-1/registration/uuid_reg.pdf", file, header)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if fileURL != "local-private://vehicles/p-1/b-1/registration/uuid_reg.pdf" {
		t.Fatalf("fileURL = %q, want local-private URL", fileURL)
	}

	stored, err := os.ReadFile(filepath.Join(root, "vehicles", "p-1", "b-1", "registration", "uuid_reg.pdf"))
	if err != nil {
		t.Fatalf("stored file not found: %v", err)
	}
	if !bytes.Equal(stored, header.content) {
		t.Fatalf("stored content mismatch")
	}
}

func TestBuildVehicleObjectPathFormat_Detailed(t *testing.T) {
	providerID := "provider-abc"
	bikeID := "bike-xyz"
	objectPath := buildVehicleObjectPath(providerID, bikeID, DocRegistration, `doc with spaces?.pdf`)

	if !strings.HasPrefix(objectPath, "vehicles/provider-abc/bike-xyz/registration/") {
		t.Fatalf("objectPath = %q, want vehicles/provider-abc/bike-xyz/registration/ prefix", objectPath)
	}
	// UUID separates from sanitized filename.
	if !strings.HasSuffix(objectPath, "_doc_with_spaces_.pdf") {
		t.Fatalf("objectPath = %q, want sanitized filename suffix", objectPath)
	}
	if strings.Contains(objectPath, "..") || strings.Contains(objectPath, "\\") {
		t.Fatalf("objectPath = %q, contains unsafe characters", objectPath)
	}
	// Must be valid storage path.
	if _, err := cleanVehicleStoragePath(objectPath); err != nil {
		t.Fatalf("cleanVehicleStoragePath(%q) error = %v", objectPath, err)
	}
}

func TestSanitizeVehicleFilenameHandlesTraversal(t *testing.T) {
	cases := []struct {
		input string
	}{
		{`../../../etc/passwd`},
		{`..\\..\\windows\\system32\\evil.exe`},
		{`/absolute/path/to/file.pdf`},
		{`file with spaces!@#.pdf`},
	}
	for _, tc := range cases {
		safe := sanitizeVehicleFilename(tc.input)
		if strings.Contains(safe, "..") {
			t.Fatalf("safe = %q (from %q), still contains ..", safe, tc.input)
		}
		if strings.Contains(safe, "/") {
			t.Fatalf("safe = %q (from %q), still contains /", safe, tc.input)
		}
	}
}

func TestCleanVehicleStoragePathRejectsInvalidPaths(t *testing.T) {
	invalidPaths := []string{
		"../evil",
		"./../../secrets",
		"/absolute/path",
		"",
		".",
	}
	for _, p := range invalidPaths {
		_, err := cleanVehicleStoragePath(p)
		if err == nil {
			t.Fatalf("cleanVehicleStoragePath(%q) expected error, got nil", p)
		}
	}
}

func TestVehicleObjectPathNoAWSNoS3(t *testing.T) {
	path := buildVehicleObjectPath("p", "b", DocInsurance, "ins.pdf")
	lower := strings.ToLower(path)
	if strings.Contains(lower, "aws") {
		t.Fatalf("path %q contains 'aws'", path)
	}
	if strings.Contains(lower, "s3") {
		t.Fatalf("path %q contains 's3'", path)
	}
}

func TestVehicleUploaderFromEnvDefaultsToLocalPrivate(t *testing.T) {
	t.Setenv("VERIFICATION_STORAGE_MODE", "")
	uploader, err := NewVehicleUploaderFromEnv("development")
	if err != nil {
		t.Fatalf("NewVehicleUploaderFromEnv() error = %v", err)
	}
	if uploader == nil {
		t.Fatal("expected non-nil uploader for development mode")
	}
}
