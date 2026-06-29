package vehicle

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// ── Storage interfaces ────────────────────────────────────────────────────────

// FileUploader stores a vehicle document and returns a private URL reference.
type FileUploader interface {
	Upload(ctx context.Context, objectPath string, file File, header FileHeader) (string, error)
}

// FileHeader wraps multipart file header metadata.
type FileHeader interface {
	Open() (File, error)
	GetFilename() string
	GetSize() int64
	GetHeaderValue(key string) string
}

// File is a readable, closable byte stream (mirrors multipart.File).
type File interface {
	Read([]byte) (int, error)
	Close() error
}

// ── No-op uploader ────────────────────────────────────────────────────────────

var ErrVehicleStorageNotConfigured = fmt.Errorf("vehicle storage uploader is not configured")

// UnconfiguredVehicleUploader is the zero-value placeholder; replaced in production.
type UnconfiguredVehicleUploader struct{}

func (UnconfiguredVehicleUploader) Upload(_ context.Context, _ string, _ File, _ FileHeader) (string, error) {
	return "", ErrVehicleStorageNotConfigured
}

// ── Local private uploader ────────────────────────────────────────────────────

// LocalVehicleUploader writes files to a local directory and returns
// local-private:// references. No raw filesystem paths are exposed.
type LocalVehicleUploader struct {
	rootDir string
	baseURL string
}

// NewLocalVehicleUploader creates an uploader rooted at rootDir.
// If rootDir is empty the OS temp dir is used as a safe fallback.
func NewLocalVehicleUploader(rootDir, baseURL string) *LocalVehicleUploader {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = filepath.Join(os.TempDir(), "cosmicforge-vehicle-uploads")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "local-private://"
	}
	return &LocalVehicleUploader{rootDir: rootDir, baseURL: baseURL}
}

// NewVehicleUploaderFromEnv builds a FileUploader from environment variables.
// Reads VERIFICATION_UPLOAD_ROOT (reuses the same mounted volume) and falls
// back to a temp dir. Never returns an AWS/S3 uploader.
func NewVehicleUploaderFromEnv(appEnv string) (FileUploader, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("VERIFICATION_STORAGE_MODE")))
	if mode == "" {
		if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
			return nil, ErrVehicleStorageNotConfigured
		}
		mode = "local_private"
	}

	switch mode {
	case "local", "local_private":
		root := os.Getenv("VERIFICATION_UPLOAD_ROOT")
		baseURL := os.Getenv("VERIFICATION_STORAGE_BASE_URL")
		return NewLocalVehicleUploader(root, baseURL), nil
	default:
		return nil, fmt.Errorf("vehicle storage mode %q is not supported", mode)
	}
}

// Upload stores the file and returns a local-private:// URL.
func (u *LocalVehicleUploader) Upload(ctx context.Context, objectPath string, file File, header FileHeader) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	cleanPath, err := cleanVehicleStoragePath(objectPath)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(u.rootDir, filepath.FromSlash(cleanPath))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}

	return joinVehicleStorageURL(u.baseURL, cleanPath), nil
}

// ── Path helpers ──────────────────────────────────────────────────────────────

// buildVehicleObjectPath constructs the storage path for a bike document.
// Format: vehicles/{providerID}/{bikeID}/{documentType}/{uuid}_{sanitizedFilename}
func buildVehicleObjectPath(providerID, bikeID string, docType DocumentType, filename string) string {
	return fmt.Sprintf(
		"vehicles/%s/%s/%s/%s_%s",
		providerID,
		bikeID,
		string(docType),
		uuid.NewString(),
		sanitizeVehicleFilename(filename),
	)
}

func cleanVehicleStoragePath(objectPath string) (string, error) {
	cleanPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(objectPath)))
	if cleanPath == "." || cleanPath == "" || strings.HasPrefix(cleanPath, "../") || strings.HasPrefix(cleanPath, "/") {
		return "", fmt.Errorf("invalid vehicle storage path")
	}
	return cleanPath, nil
}

func joinVehicleStorageURL(baseURL, objectPath string) string {
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasSuffix(baseURL, "://") {
		return baseURL + objectPath
	}
	return strings.TrimRight(baseURL, "/") + "/" + objectPath
}

func sanitizeVehicleFilename(filename string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "file"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	safe := strings.Trim(b.String(), "._-")
	if safe == "" {
		return "file"
	}
	return safe
}
