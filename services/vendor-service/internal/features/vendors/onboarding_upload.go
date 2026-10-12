package vendors

import (
	"context"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

const MaxOnboardingUploadBytes int64 = 10 << 20

type OnboardingUploadInput struct {
	Dimension   VerificationDimension
	SubjectType string
	SubjectRef  string
	MIMEType    string
	ByteSize    int64
}

type OnboardingUploadGrant struct {
	SessionID        string    `json:"upload_session_id"`
	MediaRef         string    `json:"media_ref"`
	URL              string    `json:"upload_url"`
	Token            string    `json:"upload_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	MaxBytes         int64     `json:"max_bytes"`
	AllowedMIMETypes []string  `json:"allowed_mime_types"`
}

type OnboardingUploadAuthority interface {
	AuthorizeOnboardingUpload(context.Context, string, OnboardingUploadInput, string) (OnboardingUploadGrant, error)
}

func (s *Service) AuthorizeOnboardingUpload(ctx context.Context, actor Actor, in OnboardingUploadInput, key string) (OnboardingUploadGrant, error) {
	if actor.StaffID == "" || actor.VendorID == "" {
		return OnboardingUploadGrant{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	authorizationBranch := ""
	subjectType := strings.ToUpper(strings.TrimSpace(in.SubjectType))
	if subjectType == "BRANCH" {
		authorizationBranch = strings.TrimSpace(in.SubjectRef)
	}
	if subjectType == "BRANCH_CAPABILITY" {
		scopedBranch, scopeError := s.verificationSubjectBranch(ctx, actor.VendorID, subjectType, strings.TrimSpace(in.SubjectRef))
		if scopeError != nil {
			return OnboardingUploadGrant{}, scopeError
		}
		authorizationBranch = scopedBranch
	}

	if err := s.require(ctx, actor, "vendor.onboarding.manage", branchPtr(authorizationBranch)); err != nil {
		return OnboardingUploadGrant{}, err
	}
	in.SubjectType = strings.ToUpper(strings.TrimSpace(in.SubjectType))
	in.SubjectRef = strings.TrimSpace(in.SubjectRef)
	if !validDimension(in.Dimension) || !validSubject(in.SubjectType) || in.SubjectRef == "" || in.ByteSize <= 0 || in.ByteSize > MaxOnboardingUploadBytes {
		return OnboardingUploadGrant{}, apperrors.BadRequest("Unsupported onboarding document upload scope.", nil)
	}
	switch in.MIMEType {
	case "image/jpeg", "image/png", "image/webp", "application/pdf":
	default:
		return OnboardingUploadGrant{}, apperrors.BadRequest("Choose a JPEG, PNG, WebP image or PDF.", nil)
	}
	if err := s.assertSubjectScope(ctx, actor.VendorID, in.SubjectType, in.SubjectRef); err != nil {
		return OnboardingUploadGrant{}, err
	}
	branch, err := s.verificationSubjectBranch(ctx, actor.VendorID, in.SubjectType, in.SubjectRef)
	if err != nil {
		return OnboardingUploadGrant{}, err
	}
	if err = s.require(ctx, actor, "vendor.onboarding.manage", branchPtr(branch)); err != nil {
		return OnboardingUploadGrant{}, err
	}
	authority, ok := s.media.(OnboardingUploadAuthority)
	if !ok {
		return OnboardingUploadGrant{}, apperrors.Unavailable("Document uploads are unavailable.", nil)
	}
	grant, err := authority.AuthorizeOnboardingUpload(ctx, actor.VendorID, in, key)
	if err != nil {
		return OnboardingUploadGrant{}, apperrors.Unavailable("Document upload authorization is unavailable.", err)
	}
	if grant.SessionID == "" || grant.MediaRef == "" || grant.Token == "" || grant.URL == "" || !grant.ExpiresAt.After(time.Now()) || grant.MaxBytes <= 0 || grant.MaxBytes > in.ByteSize || len(grant.AllowedMIMETypes) != 1 || grant.AllowedMIMETypes[0] != in.MIMEType {
		return OnboardingUploadGrant{}, apperrors.Unavailable("Document upload authorization is incomplete or expired.", nil)
	}
	return grant, nil
}

func (s *Service) OnboardingUploadStatus(ctx context.Context, actor Actor, mediaRef string) (map[string]string, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", nil); err != nil {
		return nil, err
	}
	if !opaqueMediaRefPattern.MatchString(mediaRef) {
		return nil, apperrors.BadRequest("Invalid media reference.", nil)
	}
	if s.media == nil {
		return nil, apperrors.Unavailable("Document upload status is unavailable.", nil)
	}
	object, err := s.media.GetObject(ctx, actor.VendorID, mediaRef)
	if err != nil {
		return nil, apperrors.Unavailable("Document upload status is unavailable.", err)
	}
	if object.ID != mediaRef || object.OwnerReference != actor.VendorID || object.UploadPurpose != "COMPLIANCE_DOCUMENT" || object.DeletionState != "ACTIVE" {
		return nil, apperrors.NotFound("Document upload not found.", nil)
	}
	return map[string]string{"media_ref": object.ID, "upload_status": object.UploadStatus, "safety_status": object.SafetyStatus}, nil
}
