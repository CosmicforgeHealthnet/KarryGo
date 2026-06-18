package uploadclients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

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
	Delete(ctx context.Context, path string) error
	Check(ctx context.Context) error
}

type FirebaseStorageClient struct {
	bucketName    string
	bucket        *gcs.BucketHandle
	publicBaseURL string
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
	if opts.CredentialsJSON != "" {
		clientOptions = append(clientOptions, option.WithCredentialsJSON([]byte(opts.CredentialsJSON)))
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

	return &FirebaseStorageClient{
		bucketName:    opts.BucketName,
		bucket:        bucket,
		publicBaseURL: publicBaseURL,
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

	return StorageObject{
		Bucket: c.bucketName,
		Path:   input.Path,
		URL:    c.publicURL(input.Path),
	}, nil
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
