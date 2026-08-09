package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"cosmicforge/logistics/shared/go/mediaclient"
)

var (
	ErrAvatarStorageNotConfigured         = errors.New("avatar storage uploader is not configured")
	ErrFirebaseAvatarStorageNotConfigured = errors.New("firebase avatar storage uploader is not configured")
)

// AvatarUploader uploads a raw reader to the given object path and returns the storage URL.
type AvatarUploader interface {
	Upload(ctx context.Context, objectPath string, file io.Reader) (string, error)
}

type unconfiguredAvatarUploader struct{}

func (u unconfiguredAvatarUploader) Upload(_ context.Context, _ string, _ io.Reader) (string, error) {
	return "", ErrAvatarStorageNotConfigured
}

type localAvatarUploader struct {
	rootDir string
	baseURL string
}

func NewLocalAvatarUploader(rootDir, baseURL string) *localAvatarUploader {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = filepath.Join(os.TempDir(), "cosmicforge-avatar-uploads")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "local-private://"
	}
	return &localAvatarUploader{rootDir: rootDir, baseURL: baseURL}
}

// NewAvatarUploaderFromEnv builds the avatar storage backend from
// AVATAR_STORAGE_MODE. mediaClient is only used for mode "media" — pass nil
// when only the local mode is needed (e.g. tests).
func NewAvatarUploaderFromEnv(appEnv string, mediaClient *mediaclient.Client) (AvatarUploader, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("AVATAR_STORAGE_MODE")))
	if mode == "" {
		if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
			return nil, ErrAvatarStorageNotConfigured
		}
		mode = "local_private"
	}
	switch mode {
	case "local", "local_private":
		return NewLocalAvatarUploader(os.Getenv("AVATAR_UPLOAD_ROOT"), os.Getenv("AVATAR_STORAGE_BASE_URL")), nil
	case "media":
		if mediaClient == nil {
			return nil, fmt.Errorf("avatar storage mode is %q but media-file-service is not configured (set MEDIA_FILE_BASE_URL)", mode)
		}
		return newMediaAvatarUploader(mediaClient), nil
	case "firebase":
		return nil, ErrFirebaseAvatarStorageNotConfigured
	default:
		return nil, fmt.Errorf("avatar storage mode %q is not supported", mode)
	}
}

// mediaAvatarUploader stores profile photos in media-file-service under the
// "profile_photo" purpose — the production-ready backend.
type mediaAvatarUploader struct {
	client *mediaclient.Client
}

func newMediaAvatarUploader(client *mediaclient.Client) *mediaAvatarUploader {
	return &mediaAvatarUploader{client: client}
}

func (u *mediaAvatarUploader) Upload(ctx context.Context, objectPath string, file io.Reader) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	asset, err := u.client.Upload(ctx, mediaclient.UploadRequest{
		OwnerID:     ownerIDFromAvatarObjectPath(objectPath),
		Purpose:     "profile_photo",
		Filename:    sanitizeAvatarFilename(objectPath),
		ContentType: avatarContentTypeFromPath(objectPath),
		Body:        file,
		Metadata:    map[string]interface{}{"object_path": objectPath},
	})
	if err != nil {
		return "", err
	}
	return asset.PublicURL, nil
}

// ownerIDFromAvatarObjectPath extracts the provider ID from paths shaped
// "profiles/{providerID}/...", matching the objectPath built in UploadAvatar.
func ownerIDFromAvatarObjectPath(objectPath string) string {
	parts := strings.Split(objectPath, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// avatarContentTypeFromPath infers content type from the extension UploadAvatar
// already appended (avatarExtension only ever produces .jpg or .png), since the
// AvatarUploader interface carries no separate content-type parameter.
func avatarContentTypeFromPath(objectPath string) string {
	if strings.HasSuffix(strings.ToLower(objectPath), ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

func (u *localAvatarUploader) Upload(ctx context.Context, objectPath string, file io.Reader) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	cleanPath, err := cleanAvatarStoragePath(objectPath)
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
	return joinAvatarStorageURL(u.baseURL, cleanPath), nil
}

func cleanAvatarStoragePath(objectPath string) (string, error) {
	cleanPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(objectPath)))
	if cleanPath == "." || cleanPath == "" || strings.HasPrefix(cleanPath, "../") || strings.HasPrefix(cleanPath, "/") {
		return "", fmt.Errorf("invalid avatar storage path")
	}
	return cleanPath, nil
}

func joinAvatarStorageURL(baseURL, objectPath string) string {
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasSuffix(baseURL, "://") {
		return baseURL + objectPath
	}
	return strings.TrimRight(baseURL, "/") + "/" + objectPath
}

func sanitizeAvatarFilename(filename string) string {
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
