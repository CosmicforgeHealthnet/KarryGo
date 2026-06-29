package vehicle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Service holds the business logic for the vehicle feature.
type Service struct {
	repository Repository
	events     EventPublisher
	uploader   FileUploader
}

type ServiceOption func(*Service)

func WithEventPublisher(ep EventPublisher) ServiceOption {
	return func(s *Service) { s.events = ep }
}

func WithUploader(up FileUploader) ServiceOption {
	return func(s *Service) { s.uploader = up }
}

func NewService(repository Repository, opts ...ServiceOption) *Service {
	svc := &Service{repository: repository, uploader: UnconfiguredVehicleUploader{}}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// ── Phase 4C: RegisterBike ────────────────────────────────────────────────────

// RegisterBike validates and persists a new bike for the given provider.
func (s *Service) RegisterBike(ctx context.Context, providerID, correlationID string, input RegisterBikeInput) (Bike, error) {
	if err := validateRegisterInput(input); err != nil {
		return Bike{}, err
	}

	// Normalize plate number: trim + uppercase.
	input.PlateNumber = strings.ToUpper(strings.TrimSpace(input.PlateNumber))

	// Check plate uniqueness.
	exists, err := s.repository.PlateNumberExists(ctx, input.PlateNumber)
	if err != nil {
		return Bike{}, err
	}
	if exists {
		return Bike{}, apperrors.Conflict("Plate number is already registered.", nil)
	}

	// Determine primary flag.
	hasAny, err := s.repository.HasAnyBike(ctx, providerID)
	if err != nil {
		return Bike{}, err
	}
	isPrimary := !hasAny

	bike, err := s.repository.InsertBike(ctx, providerID, input, isPrimary)
	if err != nil {
		return Bike{}, err
	}

	// Insert audit row.
	notes := "Bike registered."
	if err := s.repository.InsertAudit(ctx, AuditInput{
		BikeID:     bike.ID,
		ProviderID: providerID,
		Action:     AuditRegistered,
		FromStatus: VehicleUnverified,
		ToStatus:   VehicleUnverified,
		Notes:      &notes,
	}); err != nil {
		return Bike{}, err
	}

	// Publish vehicle.registered event (Phase 4I).
	if s.events != nil {
		_ = s.events.PublishVehicleRegistered(ctx, VehicleRegisteredEvent{
			Event:         TopicVehicleRegistered,
			CorrelationID: correlationIDOrFallback(correlationID),
			ProviderID:    providerID,
			BikeID:        bike.ID,
			CreatedAt:     time.Now().UTC(),
		})
	}

	return bike, nil
}

// ── Phase 4D: ListMyBikes / GetBike ──────────────────────────────────────────

// ListMyBikes returns all bikes for a provider (ordered: primary first, then created_at ASC).
func (s *Service) ListMyBikes(ctx context.Context, providerID string) ([]Bike, error) {
	bikes, err := s.repository.ListBikesByProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if bikes == nil {
		return []Bike{}, nil
	}
	return bikes, nil
}

// GetBike returns a single bike owned by the provider including its documents.
func (s *Service) GetBike(ctx context.Context, bikeID, providerID string) (BikeWithDocuments, error) {
	bike, ok, err := s.repository.GetBikeByID(ctx, bikeID, providerID)
	if err != nil {
		return BikeWithDocuments{}, err
	}
	if !ok {
		return BikeWithDocuments{}, apperrors.NotFound("Bike was not found.", nil)
	}

	docs, err := s.repository.ListBikeDocuments(ctx, bikeID, providerID)
	if err != nil {
		return BikeWithDocuments{}, err
	}
	if docs == nil {
		docs = []BikeDocument{}
	}

	return BikeWithDocuments{
		ID:                 bike.ID,
		ProviderID:         bike.ProviderID,
		BikeType:           bike.BikeType,
		Brand:              bike.Brand,
		Model:              bike.Model,
		Year:               bike.Year,
		Color:              bike.Color,
		PlateNumber:        bike.PlateNumber,
		EngineCc:           bike.EngineCc,
		ChassisNumber:      bike.ChassisNumber,
		VerificationStatus: bike.VerificationStatus,
		IsActive:           bike.IsActive,
		IsPrimary:          bike.IsPrimary,
		CreatedAt:          bike.CreatedAt,
		UpdatedAt:          bike.UpdatedAt,
		Documents:          docs,
	}, nil
}

// ── Phase 4E: UpdateBike ──────────────────────────────────────────────────────

// UpdateBike applies a partial update to a provider's bike.
func (s *Service) UpdateBike(ctx context.Context, bikeID, providerID, correlationID string, input UpdateBikeInput) (Bike, error) {
	if err := validateUpdateInput(input); err != nil {
		return Bike{}, err
	}

	current, ok, err := s.repository.GetBikeByID(ctx, bikeID, providerID)
	if err != nil {
		return Bike{}, err
	}
	if !ok {
		return Bike{}, apperrors.NotFound("Bike was not found.", nil)
	}

	// Suspended bikes are admin-controlled — block all updates.
	if current.VerificationStatus == VehicleSuspended {
		return Bike{}, apperrors.Conflict("Bike details cannot be updated while suspended.", nil)
	}

	// Normalize plate if provided.
	if input.PlateNumber != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*input.PlateNumber))
		input.PlateNumber = &normalized
	}

	// Detect identity-changing fields.
	plateChanging := input.PlateNumber != nil && *input.PlateNumber != current.PlateNumber
	typeChanging := input.BikeType != nil && *input.BikeType != current.BikeType

	// Plate uniqueness check only when the plate is actually different.
	if plateChanging {
		exists, err := s.repository.PlateNumberExists(ctx, *input.PlateNumber)
		if err != nil {
			return Bike{}, err
		}
		if exists {
			return Bike{}, apperrors.Conflict("Plate number is already registered.", nil)
		}
	}

	updated, err := s.repository.UpdateBike(ctx, bikeID, providerID, input)
	if err != nil {
		return Bike{}, err
	}

	// When plate or bike_type changes on a verified bike, reset to unverified first.
	if (plateChanging || typeChanging) && current.VerificationStatus == VehicleVerified {
		if _, err := s.repository.UpdateBikeStatus(ctx, bikeID, VehicleUnverified); err != nil {
			return Bike{}, err
		}
		updated.VerificationStatus = VehicleUnverified
	}

	// Publish vehicle.registered whenever the bike is unverified (covers two cases:
	// 1. bike was already unverified before this PATCH — step must reach submitted,
	// 2. verified bike was just reset above — step must re-submit).
	// The verification subscriber is idempotent: if step is already submitted it skips.
	if s.events != nil && updated.VerificationStatus == VehicleUnverified {
		_ = s.events.PublishVehicleRegistered(ctx, VehicleRegisteredEvent{
			Event:         TopicVehicleRegistered,
			CorrelationID: correlationIDOrFallback(correlationID),
			ProviderID:    providerID,
			BikeID:        bikeID,
			CreatedAt:     time.Now().UTC(),
		})
	}

	notes := "Bike details updated."
	_ = s.repository.InsertAudit(ctx, AuditInput{
		BikeID:     bikeID,
		ProviderID: providerID,
		Action:     AuditUpdated,
		FromStatus: current.VerificationStatus,
		ToStatus:   updated.VerificationStatus,
		Notes:      &notes,
	})
	return updated, nil
}

// ── Phase 4F: UploadDocument ──────────────────────────────────────────────────

// UploadDocument validates, stores, and records a bike document upload.
func (s *Service) UploadDocument(ctx context.Context, input UploadDocumentInput) (BikeDocument, error) {
	// 1. Validate document type.
	if !IsValidDocumentType(input.DocumentType) {
		msg := "Document type is required."
		if input.DocumentType != "" {
			msg = "Document type must be registration or insurance."
		}
		return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "document_type", Message: msg},
		})
	}

	// 2. Validate file present.
	if input.File == nil || input.Header == nil {
		return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "document_file", Message: "Document file is required."},
		})
	}

	// 3. Validate MIME type from Content-Type header AND magic bytes.
	mimeType := strings.ToLower(strings.TrimSpace(input.Header.GetHeaderValue("Content-Type")))
	// Strip any parameters (e.g. "image/jpeg; charset=…")
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	if !IsAllowedMIMEType(mimeType) {
		return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "document_file", Message: "File type must be JPEG, PNG, or PDF."},
		})
	}

	// Validate magic bytes.
	buf := make([]byte, 512)
	n, readErr := io.ReadFull(input.File, buf)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return BikeDocument{}, apperrors.Internal("Failed to read magic bytes.", readErr)
	}
	buf = buf[:n]

	detectedMime := http.DetectContentType(buf)
	if idx := strings.Index(detectedMime, ";"); idx != -1 {
		detectedMime = strings.TrimSpace(detectedMime[:idx])
	}
	if !IsAllowedMIMEType(detectedMime) {
		return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "document_file", Message: "File type must be JPEG, PNG, or PDF."},
		})
	}

	// Reconstruct the file stream so down-stream uploader reads from the beginning
	input.File = &multiFile{
		Reader: io.MultiReader(bytes.NewReader(buf), input.File),
		closer: input.File,
	}

	// 4. Validate file size.
	if input.Header.GetSize() > MaxDocumentSize {
		return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "document_file", Message: "File must not exceed 5 MB."},
		})
	}

	// 5. Insurance-specific expiry_date validation.
	if input.DocumentType == DocInsurance {
		if input.ExpiryDate == nil || strings.TrimSpace(*input.ExpiryDate) == "" {
			return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
				{Field: "expiry_date", Message: "Expiry date is required for insurance documents."},
			})
		}
		if err := validateFutureDate(*input.ExpiryDate); err != nil {
			return BikeDocument{}, validationError("Check your details.", []apperrors.FieldViolation{
				{Field: "expiry_date", Message: err.Error()},
			})
		}
	}

	// 6. Load and ownership-check the bike.
	bike, ok, err := s.repository.GetBikeByID(ctx, input.BikeID, input.ProviderID)
	if err != nil {
		return BikeDocument{}, err
	}
	if !ok {
		return BikeDocument{}, apperrors.NotFound("Bike was not found.", nil)
	}

	// 7. Block uploads on verified / suspended bikes.
	if bike.VerificationStatus == VehicleVerified || bike.VerificationStatus == VehicleSuspended {
		return BikeDocument{}, apperrors.Conflict("Documents cannot be uploaded in the bike's current status.", nil)
	}

	// 8. Store file.
	objectPath := buildVehicleObjectPath(input.ProviderID, input.BikeID, input.DocumentType, input.Header.GetFilename())
	fileURL, err := s.uploader.Upload(ctx, objectPath, input.File, input.Header)
	if err != nil {
		return BikeDocument{}, apperrors.Internal("File upload failed.", err)
	}

	// 9. Insert bike_documents row.
	fileSize := int(input.Header.GetSize())
	mime := mimeType
	doc, err := s.repository.InsertBikeDocument(ctx, BikeDocument{
		BikeID:       input.BikeID,
		ProviderID:   input.ProviderID,
		DocumentType: input.DocumentType,
		FileURL:      fileURL,
		FileSize:     &fileSize,
		MimeType:     &mime,
		ExpiryDate:   input.ExpiryDate,
	})
	if err != nil {
		return BikeDocument{}, err
	}

	// 10. Transition bike status: unverified/rejected → pending.
	prevStatus := bike.VerificationStatus
	newStatus := prevStatus
	if prevStatus == VehicleUnverified || prevStatus == VehicleRejected {
		newStatus = VehiclePending
		if _, err := s.repository.UpdateBikeStatus(ctx, input.BikeID, VehiclePending); err != nil {
			return BikeDocument{}, err
		}
	}

	// 11. Audit row.
	auditNotes := "Document uploaded."
	_ = s.repository.InsertAudit(ctx, AuditInput{
		BikeID:     input.BikeID,
		ProviderID: input.ProviderID,
		Action:     AuditDocsUploaded,
		FromStatus: prevStatus,
		ToStatus:   newStatus,
		Notes:      &auditNotes,
	})

	// 12. Publish event (non-blocking; failure logged but not returned).
	if s.events != nil {
		_ = s.events.PublishVehicleDocsSubmitted(ctx, VehicleDocsSubmittedEvent{
			Event:         TopicVehicleDocsSubmitted,
			CorrelationID: input.CorrelationID,
			ProviderID:    input.ProviderID,
			BikeID:        input.BikeID,
			DocumentType:  input.DocumentType,
			CreatedAt:     time.Now().UTC(),
		})
	}

	return doc, nil
}

// ── Phase 4C admin review ─────────────────────────────────────────────────────

// AdminReview allows a platform admin to approve, reject, or suspend a bike.
func (s *Service) AdminReview(ctx context.Context, bikeID, reviewerID, correlationID string, input AdminReviewInput) (Bike, error) {
	if err := validateAdminReviewInput(input); err != nil {
		return Bike{}, err
	}

	// Load current bike (admin has no provider restriction).
	current, ok, err := s.repository.GetBikeByIDAdmin(ctx, bikeID)
	if err != nil {
		return Bike{}, err
	}
	if !ok {
		return Bike{}, apperrors.NotFound("Bike was not found.", nil)
	}

	// Determine target status and guard against idempotent transitions.
	var targetStatus VehicleStatus
	switch input.Action {
	case AuditApproved:
		targetStatus = VehicleVerified
		if current.VerificationStatus == VehicleVerified {
			return Bike{}, apperrors.Conflict("Bike is already verified.", nil)
		}
	case AuditRejected:
		targetStatus = VehicleRejected
		if current.VerificationStatus == VehicleRejected {
			return Bike{}, apperrors.Conflict("Bike is already rejected.", nil)
		}
	case AuditSuspended:
		targetStatus = VehicleSuspended
		if current.VerificationStatus == VehicleSuspended {
			return Bike{}, apperrors.Conflict("Bike is already suspended.", nil)
		}
	}

	updated, err := s.repository.AdminUpdateBikeStatus(ctx, bikeID, targetStatus)
	if err != nil {
		return Bike{}, err
	}

	// Audit.
	reason := strings.TrimSpace(input.Reason)
	var notesPtr *string
	if reason != "" {
		notesPtr = &reason
	}
	_ = s.repository.InsertAudit(ctx, AuditInput{
		BikeID:      bikeID,
		ProviderID:  current.ProviderID,
		Action:      input.Action,
		FromStatus:  current.VerificationStatus,
		ToStatus:    targetStatus,
		PerformedBy: &reviewerID,
		Notes:       notesPtr,
	})

	// Publish vehicle event so Phase 3 verification picks it up.
	if s.events != nil {
		now := time.Now().UTC()
		switch input.Action {
		case AuditApproved:
			_ = s.events.PublishVehicleVerified(ctx, VehicleVerifiedEvent{
				Event:         TopicVehicleVerified,
				CorrelationID: correlationID,
				ProviderID:    current.ProviderID,
				BikeID:        bikeID,
				VerifiedAt:    now,
				CreatedAt:     now,
			})
		case AuditRejected:
			_ = s.events.PublishVehicleRejected(ctx, VehicleRejectedEvent{
				Event:         TopicVehicleRejected,
				CorrelationID: correlationID,
				ProviderID:    current.ProviderID,
				BikeID:        bikeID,
				Reason:        reason,
				CreatedAt:     now,
			})
		case AuditSuspended:
			_ = s.events.PublishVehicleSuspended(ctx, VehicleSuspendedEvent{
				Event:         TopicVehicleSuspended,
				CorrelationID: correlationID,
				ProviderID:    current.ProviderID,
				BikeID:        bikeID,
				Reason:        reason,
				CreatedAt:     now,
			})
		}
	}

	return updated, nil
}

// ListDocuments returns all documents for a provider's bike.
func (s *Service) ListDocuments(ctx context.Context, bikeID, providerID string) ([]BikeDocument, error) {
	_, ok, err := s.repository.GetBikeByID(ctx, bikeID, providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.NotFound("Bike was not found.", nil)
	}
	docs, err := s.repository.ListBikeDocuments(ctx, bikeID, providerID)
	if err != nil {
		return nil, err
	}
	if docs == nil {
		return []BikeDocument{}, nil
	}
	return docs, nil
}

// ── Validation ────────────────────────────────────────────────────────────────

func validateRegisterInput(input RegisterBikeInput) error {
	var fields []apperrors.FieldViolation

	if !IsValidBikeType(input.BikeType) {
		msg := "Bike type is required."
		if input.BikeType != "" {
			msg = "Bike type must be motorcycle, dispatch_bike, scooter, tricycle, bicycle, or electric_bike."
		}
		fields = append(fields, apperrors.FieldViolation{Field: "bike_type", Message: msg})
	}
	if strings.TrimSpace(input.Brand) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "brand", Message: "Brand is required."})
	}
	if strings.TrimSpace(input.Model) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "model", Message: "Model is required."})
	}
	if !isValidYear(input.Year) {
		fields = append(fields, apperrors.FieldViolation{Field: "year", Message: "Year must be a valid 4-digit year."})
	}
	if strings.TrimSpace(input.Color) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "color", Message: "Color is required."})
	}
	if strings.TrimSpace(input.PlateNumber) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "plate_number", Message: "Plate number is required."})
	}
	if input.EngineCc != nil && *input.EngineCc <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "engine_cc", Message: "Engine CC must be a positive number."})
	}

	if len(fields) > 0 {
		return validationError("Check your details.", fields)
	}
	return nil
}

func validateUpdateInput(input UpdateBikeInput) error {
	if !hasAnyUpdateField(input) {
		return validationError("No fields to update.", []apperrors.FieldViolation{
			{Field: "body", Message: "No updatable fields were provided."},
		})
	}

	var fields []apperrors.FieldViolation
	if input.Brand != nil && strings.TrimSpace(*input.Brand) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "brand", Message: "Brand must not be empty."})
	}
	if input.Model != nil && strings.TrimSpace(*input.Model) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "model", Message: "Model must not be empty."})
	}
	if input.Color != nil && strings.TrimSpace(*input.Color) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "color", Message: "Color must not be empty."})
	}
	if input.Year != nil && !isValidYear(*input.Year) {
		fields = append(fields, apperrors.FieldViolation{Field: "year", Message: "Year must be a valid 4-digit year."})
	}
	if input.EngineCc != nil && *input.EngineCc <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "engine_cc", Message: "Engine CC must be a positive number."})
	}
	if input.BikeType != nil && !IsValidBikeType(*input.BikeType) {
		fields = append(fields, apperrors.FieldViolation{Field: "bike_type", Message: "Bike type must be motorcycle, dispatch_bike, scooter, tricycle, bicycle, or electric_bike."})
	}
	if input.PlateNumber != nil && strings.TrimSpace(*input.PlateNumber) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "plate_number", Message: "Plate number must not be empty."})
	}
	if len(fields) > 0 {
		return validationError("Check your details.", fields)
	}
	return nil
}

func validateAdminReviewInput(input AdminReviewInput) error {
	if !IsValidAdminAction(input.Action) {
		msg := "Action is required."
		if input.Action != "" {
			msg = "Action must be approved, rejected, or suspended."
		}
		return validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "action", Message: msg},
		})
	}
	if input.Action == AuditRejected && strings.TrimSpace(input.Reason) == "" {
		return validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "reason", Message: "Reason is required when rejecting."},
		})
	}
	if input.Action == AuditSuspended && strings.TrimSpace(input.Reason) == "" {
		return validationError("Check your details.", []apperrors.FieldViolation{
			{Field: "reason", Message: "Reason is required when suspending."},
		})
	}
	return nil
}

func validateFutureDate(dateStr string) error {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(dateStr))
	if err != nil {
		return fmt.Errorf("expiry_date must be in YYYY-MM-DD format")
	}
	if !t.After(time.Now().UTC().Truncate(24 * time.Hour)) {
		return fmt.Errorf("expiry_date must be a future date")
	}
	return nil
}

func isValidYear(year int) bool {
	y := strconv.Itoa(year)
	if len(y) != 4 {
		return false
	}
	return year >= 1900 && year <= 9999
}

func hasAnyUpdateField(input UpdateBikeInput) bool {
	return input.Brand != nil || input.Model != nil || input.Year != nil || input.Color != nil ||
		input.EngineCc != nil || input.ChassisNumber != nil || input.BikeType != nil || input.PlateNumber != nil
}

// correlationIDOrFallback returns a non-empty correlation ID string.
func correlationIDOrFallback(id string) string {
	if strings.TrimSpace(id) != "" {
		return id
	}
	return fmt.Sprintf("vehicle-%d", time.Now().UnixNano())
}

type multiFile struct {
	io.Reader
	closer io.Closer
}

func (f *multiFile) Close() error {
	if f.closer != nil {
		return f.closer.Close()
	}
	return nil
}
