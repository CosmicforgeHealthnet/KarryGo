package filemetadatamodels

import "time"

const (
	StatusActive  = "active"
	StatusDeleted = "deleted"

	PurposeProfilePhoto = "profile_photo"
	PurposeDocumentFile = "document_file"
	PurposeProofImage   = "proof_image"
	PurposeSignature    = "signature"
)

type MediaAsset struct {
	ID                string                 `json:"id"`
	OwnerService      string                 `json:"owner_service"`
	OwnerID           string                 `json:"owner_id"`
	Purpose           string                 `json:"purpose"`
	OriginalFilename  string                 `json:"original_filename"`
	ContentType       string                 `json:"content_type"`
	SizeBytes         int64                  `json:"size_bytes"`
	ChecksumSHA256    string                 `json:"checksum_sha256"`
	StorageBucket     string                 `json:"bucket"`
	StoragePath       string                 `json:"path"`
	PublicURL         string                 `json:"url"`
	Status            string                 `json:"status"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	UploadedByService string                 `json:"uploaded_by_service,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	DeletedAt         *time.Time             `json:"deleted_at,omitempty"`
}

type CreateMediaAssetInput struct {
	ID                string
	OwnerService      string
	OwnerID           string
	Purpose           string
	OriginalFilename  string
	ContentType       string
	SizeBytes         int64
	ChecksumSHA256    string
	StorageBucket     string
	StoragePath       string
	PublicURL         string
	Metadata          map[string]interface{}
	UploadedByService string
}

type ListMediaAssetsFilter struct {
	OwnerService string
	OwnerID      string
	Purpose      string
	Limit        int
}
