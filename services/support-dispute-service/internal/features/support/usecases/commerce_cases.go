package supportusecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/supportcase"
)

var allowedResolutions = map[string]bool{
	"CUSTOMER_UPHELD": true, "VENDOR_UPHELD": true, "PARTIAL": true,
	"REFUND_AUTHORIZED": true, "REPLACEMENT_AUTHORIZED": true,
	"NO_ACTION": true, "ESCALATED": true,
}

func (s *SupportService) commerceCases() (supportrepositories.CommerceCaseRepository, error) {
	repo, ok := s.repo.(supportrepositories.CommerceCaseRepository)
	if !ok {
		return nil, errors.New("commerce support case repository is unavailable")
	}
	return repo, nil
}

func (s *SupportService) CreateCommerceCase(ctx context.Context, in supportcase.CreateRequest) (supportcase.Case, bool, error) {
	in.Description = supportcase.SanitizeDescription(in.Description)
	if err := in.Validate(); err != nil {
		return supportcase.Case{}, false, apperrors.Validation("Commerce support case is invalid.", nil)
	}
	// A caller's opaque string is not proof that Media considers it usable for
	// this order. Resolve every supplied reference before attaching any of
	// them; an unavailable Media authority fails the whole request closed.
	for _, mediaRef := range in.Context.MediaRefs {
		if s.media == nil {
			return supportcase.Case{}, false, apperrors.Unavailable("Media evidence validation is unavailable.", nil)
		}
		verdict, err := s.media.Validate(ctx, mediaRef, "ORDER", in.Context.CommerceOrderRef)
		if err != nil {
			return supportcase.Case{}, false, apperrors.Unavailable("Media evidence could not be checked.", err)
		}
		if !verdict.Usable {
			return supportcase.Case{}, false, apperrors.Validation("MediaRef is not valid for this order.", nil)
		}
	}
	fingerprint, err := supportcase.Fingerprint(in)
	if err != nil {
		return supportcase.Case{}, false, err
	}
	repo, err := s.commerceCases()
	if err != nil {
		return supportcase.Case{}, false, err
	}
	return repo.CreateCommerceCase(ctx, in, fingerprint)
}

func (s *SupportService) GetCommerceCaseByOperation(ctx context.Context, vendorRef, operationRef string) (supportcase.Case, error) {
	if strings.TrimSpace(vendorRef) == "" || strings.TrimSpace(operationRef) == "" {
		return supportcase.Case{}, apperrors.Validation("Vendor and operation references are required.", nil)
	}
	repo, err := s.commerceCases()
	if err != nil {
		return supportcase.Case{}, err
	}
	return repo.GetCommerceCaseByOperation(ctx, strings.TrimSpace(vendorRef), strings.TrimSpace(operationRef))
}

func (s *SupportService) GetCommerceCaseByRef(ctx context.Context, vendorRef, caseRef string) (supportcase.Case, error) {
	if strings.TrimSpace(vendorRef) == "" || strings.TrimSpace(caseRef) == "" {
		return supportcase.Case{}, apperrors.Validation("Vendor and case references are required.", nil)
	}
	repo, err := s.commerceCases()
	if err != nil {
		return supportcase.Case{}, err
	}
	return repo.GetCommerceCaseByRef(ctx, strings.TrimSpace(vendorRef), strings.TrimSpace(caseRef))
}

func (s *SupportService) ResolveCommerceCase(ctx context.Context, caseRef, resolution, financialRef, actor string, now time.Time) (supportcase.ResolutionEvent, error) {
	resolution = strings.ToUpper(strings.TrimSpace(resolution))
	if !allowedResolutions[resolution] {
		return supportcase.ResolutionEvent{}, apperrors.Validation("Commerce case resolution is invalid.", nil)
	}
	if resolution != "REFUND_AUTHORIZED" && strings.TrimSpace(financialRef) != "" {
		return supportcase.ResolutionEvent{}, apperrors.Validation("Financial action is only valid for a refund authorization.", nil)
	}
	repo, err := s.commerceCases()
	if err != nil {
		return supportcase.ResolutionEvent{}, err
	}
	return repo.ResolveCommerceCase(ctx, strings.TrimSpace(caseRef), resolution, strings.TrimSpace(financialRef), strings.TrimSpace(actor), now.UTC())
}
