package uploadclients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	gcs "cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

type StorageObject struct {
	Bucket string
	Path   string
	URL    string
}

type UploadObjectInput struct {
	Path        string
	Body        io.Reader
	ContentType string
	Metadata    map[string]string
}

type ObjectStorage interface {
	Upload(ctx context.Context, input UploadObjectInput) (StorageObject, error)
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Check(ctx context.Context) error
}

type FirebaseStorageClient struct {
	bucketName      string
	bucket          *gcs.BucketHandle
	publicBaseURL   string
	useSignedURLs   bool
	credentialsJSON []byte
}

type FirebaseStorageOptions struct {
	BucketName      string
	CredentialsFile string
	CredentialsJSON string
	PublicBaseURL   string
}

func NewFirebaseStorageClient(ctx context.Context, opts FirebaseStorageOptions) (*FirebaseStorageClient, error) {
	if opts.BucketName == "" {
		return nil, fmt.Errorf("firebase storage bucket is required")
	}

	clientOptions := []option.ClientOption{}
	var credentialsJSON []byte
	if opts.CredentialsJSON != "" {
		credentialsJSON = []byte(opts.CredentialsJSON)
		clientOptions = append(clientOptions, option.WithCredentialsJSON(credentialsJSON))
	} else if opts.CredentialsFile != "" {
		clientOptions = append(clientOptions, option.WithCredentialsFile(opts.CredentialsFile))
	}

	app, err := firebase.NewApp(ctx, &firebase.Config{
		StorageBucket: opts.BucketName,
	}, clientOptions...)
	if err != nil {
		return nil, err
	}

	storageClient, err := app.Storage(ctx)
	if err != nil {
		return nil, err
	}

	bucket, err := storageClient.DefaultBucket()
	if err != nil {
		return nil, err
	}

	publicBaseURL := strings.TrimRight(opts.PublicBaseURL, "/")
	if publicBaseURL == "" {
		publicBaseURL = "https://storage.googleapis.com/" + opts.BucketName
	}

	useSignedURLs := opts.CredentialsJSON != "" || opts.CredentialsFile != ""

	return &FirebaseStorageClient{
		bucketName:      opts.BucketName,
		bucket:          bucket,
		publicBaseURL:   publicBaseURL,
		useSignedURLs:   useSignedURLs,
		credentialsJSON: credentialsJSON,
	}, nil
}

func (c *FirebaseStorageClient) Upload(ctx context.Context, input UploadObjectInput) (StorageObject, error) {
	writer := c.bucket.Object(input.Path).NewWriter(ctx)
	writer.ContentType = input.ContentType
	writer.CacheControl = "public, max-age=31536000, immutable"
	writer.Metadata = input.Metadata

	if _, err := io.Copy(writer, input.Body); err != nil {
		_ = writer.CloseWithError(err)
		return StorageObject{}, err
	}
	if err := writer.Close(); err != nil {
		return StorageObject{}, err
	}

	var signedURL string
	if c.useSignedURLs && len(c.credentialsJSON) > 0 {
		url, err := c.generateSignedURL(ctx, input.Path)
		if err != nil {
			return StorageObject{}, fmt.Errorf("generate signed url: %w", err)
		}
		signedURL = url
	} else {
		signedURL = c.publicURL(input.Path)
	}

	return StorageObject{
		Bucket: c.bucketName,
		Path:   input.Path,
		URL:    signedURL,
	}, nil
}

func (c *FirebaseStorageClient) generateSignedURL(ctx context.Context, path string) (string, error) {
	type serviceAccount struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey  string `json:"private_key"`
		ClientEmail string `json:"client_email"`
	}

	var sa serviceAccount
	if err := json.Unmarshal(c.credentialsJSON, &sa); err != nil {
		return "", fmt.Errorf("parse credentials: %w", err)
	}

	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return "", fmt.Errorf("missing client_email or private_key in credentials")
	}

	opts := &gcs.SignedURLOptions{
		Scheme:         gcs.SigningSchemeV4,
		Method:         "GET",
		Expires:        time.Now().Add(24 * time.Hour * 365),
		GoogleAccessID: sa.ClientEmail,
		PrivateKey:     []byte(sa.PrivateKey),
	}

	signedURL, err := gcs.SignedURL(c.bucketName, path, opts)
	if err != nil {
		return "", fmt.Errorf("sign url (bucket=%s path=%s email=%s): %w", c.bucketName, path, sa.ClientEmail, err)
	}

	return signedURL, nil
}

func (c *FirebaseStorageClient) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	reader, err := c.bucket.Object(path).NewReader(ctx)
	if err != nil {
		if errors.Is(err, gcs.ErrObjectNotExist) {
			return nil, fmt.Errorf("file not found")
		}
		return nil, err
	}
	return reader, nil
}

func (c *FirebaseStorageClient) Delete(ctx context.Context, path string) error {
	err := c.bucket.Object(path).Delete(ctx)
	if errors.Is(err, gcs.ErrObjectNotExist) {
		return nil
	}
	return err
}

func (c *FirebaseStorageClient) Check(ctx context.Context) error {
	_, err := c.bucket.Attrs(ctx)
	return err
}

func (c *FirebaseStorageClient) publicURL(path string) string {
	return c.publicBaseURL + "/" + escapeStoragePath(path)
}

func escapeStoragePath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}

	return strings.Join(parts, "/")
}
