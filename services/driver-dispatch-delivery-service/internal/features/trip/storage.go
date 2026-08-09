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

	"cosmicforge/logistics/shared/go/mediaclient"
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
		rootDir = filepath.Join(os.TempDir(), "cosmicforge-trip-proof-uploads")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "local-private://"
	}
	return &LocalProofStorage{rootDir: rootDir, baseURL: baseURL}
}

// NewProofStorageFromEnv builds the trip-proof storage backend from
// TRIP_PROOF_STORAGE_MODE (falling back to VERIFICATION_STORAGE_MODE, then
// "local_private" in non-production). mediaClient is only used for mode
// "media" — pass nil when only local/firebase modes are needed (e.g. tests).
func NewProofStorageFromEnv(appEnv string, mediaClient *mediaclient.Client) (ProofStorage, error) {
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
	case "media":
		if mediaClient == nil {
			return nil, fmt.Errorf("trip proof storage mode is %q but media-file-service is not configured (set MEDIA_FILE_BASE_URL)", mode)
		}
		return NewMediaProofStorage(mediaClient), nil
	case "firebase":
		return nil, ErrFirebaseProofStorageNotConfigured
	default:
		return nil, fmt.Errorf("trip proof storage mode %q is not supported", mode)
	}
}

// MediaProofStorage stores trip proof photos and signatures in
// media-file-service under the "proof_image" / "signature" purposes. This is
// the production-ready backend — local_private only works on a single host
// and firebase mode was never implemented for this service.
type MediaProofStorage struct {
	client *mediaclient.Client
}

func NewMediaProofStorage(client *mediaclient.Client) *MediaProofStorage {
	return &MediaProofStorage{client: client}
}

func (s *MediaProofStorage) SaveProofFile(ctx context.Context, providerID, tripID string, file multipart.File, header *multipart.FileHeader, kind string) (string, error) {
	if header == nil || header.Size <= 0 || header.Size > MaxProofFileSize {
		return "", fmt.Errorf("proof file size is invalid")
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType != "image/jpeg" && contentType != "image/png" && !(kind == "signature" && contentType == "application/pdf") {
		return "", fmt.Errorf("proof file type is not supported")
	}
	purpose := "proof_image"
	if kind == "signature" {
		purpose = "signature"
	} else if kind != "photo" {
		return "", fmt.Errorf("proof kind is invalid")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	asset, err := s.client.Upload(ctx, mediaclient.UploadRequest{
		OwnerID:     tripID,
		Purpose:     purpose,
		Filename:    sanitizeProofFilename(header.Filename),
		ContentType: contentType,
		Body:        file,
		Metadata:    map[string]interface{}{"provider_id": providerID, "kind": kind},
	})
	if err != nil {
		return "", err
	}
	return asset.PublicURL, nil
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
