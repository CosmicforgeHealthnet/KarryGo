package verification

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var (
	ErrStorageNotConfigured         = errors.New("verification storage uploader is not configured")
	ErrFirebaseStorageNotConfigured = errors.New("firebase verification storage uploader is not configured")
)

type FileUploader interface {
	Upload(ctx context.Context, path string, file File, header FileHeader) (string, error)
}

type UnconfiguredUploader struct{}

func (u UnconfiguredUploader) Upload(ctx context.Context, path string, file File, header FileHeader) (string, error) {
	return "", ErrStorageNotConfigured
}

type LocalFileUploader struct {
	rootDir string
	baseURL string
}

func NewLocalFileUploader(rootDir string, baseURL string) *LocalFileUploader {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = filepath.Join(os.TempDir(), "karrygo-verification-uploads")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "local-private://"
	}
	return &LocalFileUploader{rootDir: rootDir, baseURL: baseURL}
}

func NewUploaderFromEnv(appEnv string) (FileUploader, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("VERIFICATION_STORAGE_MODE")))
	if mode == "" {
		if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
			return nil, ErrStorageNotConfigured
		}
		mode = "local_private"
	}

	switch mode {
	case "local", "local_private":
		return NewLocalFileUploader(os.Getenv("VERIFICATION_UPLOAD_ROOT"), os.Getenv("VERIFICATION_STORAGE_BASE_URL")), nil
	case "firebase":
		return nil, ErrFirebaseStorageNotConfigured
	default:
		return nil, fmt.Errorf("verification storage mode %q is not supported", mode)
	}
}

func (u *LocalFileUploader) Upload(ctx context.Context, objectPath string, file File, header FileHeader) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	cleanPath, err := cleanStoragePath(objectPath)
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

	return joinStorageURL(u.baseURL, cleanPath), nil
}

func cleanStoragePath(objectPath string) (string, error) {
	cleanPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(objectPath)))
	if cleanPath == "." || cleanPath == "" || strings.HasPrefix(cleanPath, "../") || strings.HasPrefix(cleanPath, "/") {
		return "", fmt.Errorf("invalid storage path")
	}
	return cleanPath, nil
}

func buildVerificationObjectPath(providerID string, step Step, filename string) string {
	return fmt.Sprintf("verifications/%s/%s/%s_%s", providerID, step, uuid.NewString(), sanitizeFilename(filename))
}

func joinStorageURL(baseURL string, objectPath string) string {
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasSuffix(baseURL, "://") {
		return baseURL + objectPath
	}
	return strings.TrimRight(baseURL, "/") + "/" + objectPath
}

func sanitizeFilename(filename string) string {
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
