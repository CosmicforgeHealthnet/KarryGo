package trip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

const MaxProofFileSize = 5 << 20

var (
	ErrProofStorageNotConfigured         = errors.New("trip proof storage is not configured")
	ErrFirebaseProofStorageNotConfigured = errors.New("firebase trip proof storage is not configured")
)

type ProofStorage interface {
	SaveProofFile(ctx context.Context, providerID string, tripID string, file multipart.File, header *multipart.FileHeader, kind string) (string, error)
}

type LocalProofStorage struct {
	rootDir string
	baseURL string
}

func NewLocalProofStorage(rootDir, baseURL string) *LocalProofStorage {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = filepath.Join(os.TempDir(), "karrygo-trip-proof-uploads")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "local-private://"
	}
	return &LocalProofStorage{rootDir: rootDir, baseURL: baseURL}
}

func NewProofStorageFromEnv(appEnv string) (ProofStorage, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("TRIP_PROOF_STORAGE_MODE")))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(os.Getenv("VERIFICATION_STORAGE_MODE")))
	}
	if mode == "" {
		if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
			return nil, ErrProofStorageNotConfigured
		}
		mode = "local_private"
	}
	switch mode {
	case "local", "local_private":
		root := firstNonEmpty(os.Getenv("TRIP_PROOF_UPLOAD_ROOT"), os.Getenv("VERIFICATION_UPLOAD_ROOT"))
		baseURL := firstNonEmpty(os.Getenv("TRIP_PROOF_STORAGE_BASE_URL"), os.Getenv("VERIFICATION_STORAGE_BASE_URL"))
		return NewLocalProofStorage(root, baseURL), nil
	case "firebase":
		return nil, ErrFirebaseProofStorageNotConfigured
	default:
		return nil, fmt.Errorf("trip proof storage mode %q is not supported", mode)
	}
}

func (s *LocalProofStorage) SaveProofFile(ctx context.Context, providerID, tripID string, file multipart.File, header *multipart.FileHeader, kind string) (string, error) {
	if header == nil || header.Size <= 0 || header.Size > MaxProofFileSize {
		return "", fmt.Errorf("proof file size is invalid")
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType != "image/jpeg" && contentType != "image/png" && !(kind == "signature" && contentType == "application/pdf") {
		return "", fmt.Errorf("proof file type is not supported")
	}
	if kind != "photo" && kind != "signature" {
		return "", fmt.Errorf("proof kind is invalid")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	objectPath := fmt.Sprintf("trips/%s/%s/proof/%s_%s_%s",
		providerID, tripID, uuid.NewString(), kind, sanitizeProofFilename(header.Filename))
	cleanPath, err := cleanProofStoragePath(objectPath)
	if err != nil {
		return "", err
	}
	target := filepath.Join(s.rootDir, filepath.FromSlash(cleanPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	out, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(file, MaxProofFileSize+1)); err != nil {
		return "", err
	}
	return joinProofStorageReference(s.baseURL, cleanPath), nil
}

func cleanProofStoragePath(value string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid trip proof storage path")
	}
	return clean, nil
}

func joinProofStorageReference(baseURL, objectPath string) string {
	if strings.HasSuffix(strings.TrimSpace(baseURL), "://") {
		return strings.TrimSpace(baseURL) + objectPath
	}
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/" + objectPath
}

func sanitizeProofFilename(filename string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	var result strings.Builder
	for _, char := range name {
		switch {
		case unicode.IsLetter(char), unicode.IsDigit(char), char == '.', char == '-', char == '_':
			result.WriteRune(char)
		default:
			result.WriteByte('_')
		}
	}
	safe := strings.Trim(result.String(), "._-")
	if safe == "" {
		return "file"
	}
	return safe
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
