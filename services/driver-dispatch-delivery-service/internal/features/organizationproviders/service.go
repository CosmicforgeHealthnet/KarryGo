package organizationproviders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"cosmicforge/logistics/shared/go/orgprovider"
	"cosmicforge/logistics/shared/go/phonenumber"
)

// Store is the persistence the service needs.
type Store interface {
	FindByEmail(ctx context.Context, email string) (Candidate, error)
	FindByPhone(ctx context.Context, phone string) (Candidate, error)
	GetProvider(ctx context.Context, providerID string) (Candidate, error)
	ProvisionSponsored(ctx context.Context, key, fingerprint string, in Sponsored) (orgprovider.ProviderView, error)
	CreateLink(ctx context.Context, key, fingerprint, caller, correlation string, in orgprovider.LinkRequest) (LinkRecord, bool, error)
	GetLink(ctx context.Context, linkID, organizationID string) (LinkRecord, error)
	GetLiveLinkByProvider(ctx context.Context, providerID, organizationID string) (LinkRecord, error)
	TransitionLink(ctx context.Context, linkID, key, fingerprint, caller, correlation string, in orgprovider.TransitionRequest) (LinkRecord, error)
	ListForProvider(ctx context.Context, providerID string) ([]LinkRecord, error)
	Decide(ctx context.Context, providerID, linkID string, accept bool, expectedVersion int64) (LinkRecord, error)
}

// Flags gate the organization-facing writes. Reads and the rider's own
// decision stay available so an association created before a switch-off can
// still be answered.
type Flags struct{ Enabled, KillSwitch bool }

// Service owns the business rules of the contract.
type Service struct {
	store Store
	flags Flags
}

// NewService builds the service.
func NewService(store Store, flags Flags) *Service { return &Service{store: store, flags: flags} }

func (s *Service) writable() error {
	if !s.flags.Enabled || s.flags.KillSwitch {
		return ErrDisabled
	}
	return nil
}

// Fingerprint is the stable hash of a request body used for replay checks.
func Fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalizePhone(raw string) (string, error) {
	phone, err := phonenumber.NormalizeNigerianPhoneNumber(raw)
	if err != nil {
		return "", ErrInvalid
	}
	return phone, nil
}

func validEmail(email string) bool {
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1 && !strings.ContainsAny(email, " \t")
}

// Resolve answers "is this person already a rider?" by both contacts. Two
// different riders behind the two contacts is a CONFLICT, never a merge.
func (s *Service) Resolve(ctx context.Context, req orgprovider.ResolveRequest) (orgprovider.ResolveResponse, error) {
	email := orgprovider.NormalizeEmail(req.EmailNormalized)
	if !validEmail(email) {
		return orgprovider.ResolveResponse{}, ErrInvalid
	}
	phone, err := normalizePhone(req.PhoneE164)
	if err != nil {
		return orgprovider.ResolveResponse{}, err
	}
	byEmail, emailErr := s.store.FindByEmail(ctx, email)
	if emailErr != nil && !errors.Is(emailErr, ErrNotFound) {
		return orgprovider.ResolveResponse{}, emailErr
	}
	byPhone, phoneErr := s.store.FindByPhone(ctx, phone)
	if phoneErr != nil && !errors.Is(phoneErr, ErrNotFound) {
		return orgprovider.ResolveResponse{}, phoneErr
	}
	var emailMatch, phoneMatch *orgprovider.Match
	if emailErr == nil {
		emailMatch = byEmail.match()
	}
	if phoneErr == nil {
		phoneMatch = byPhone.match()
	}
	return orgprovider.Resolution(ServiceCategory, emailMatch, phoneMatch), nil
}

// ProvisionSponsored creates one organization-sponsored rider.
func (s *Service) ProvisionSponsored(ctx context.Context, key string, req orgprovider.SponsoredRequest) (orgprovider.ProviderView, error) {
	if err := s.writable(); err != nil {
		return orgprovider.ProviderView{}, err
	}
	email := orgprovider.NormalizeEmail(req.EmailNormalized)
	phone, err := normalizePhone(req.PhoneE164)
	if err != nil || strings.TrimSpace(key) == "" || strings.TrimSpace(req.OrganizationID) == "" ||
		!validEmail(email) || !strings.EqualFold(req.ServiceCategory, ServiceCategory) ||
		(req.OnboardingMode != "" && req.OnboardingMode != orgprovider.OnboardingSponsored) {
		return orgprovider.ProviderView{}, ErrInvalid
	}
	first, last := orgprovider.SplitLegalName(req.LegalName)
	if first == "" {
		return orgprovider.ProviderView{}, ErrInvalid
	}
	return s.store.ProvisionSponsored(ctx, key, Fingerprint(req), Sponsored{
		OrganizationID: strings.TrimSpace(req.OrganizationID), FirstName: first, LastName: last,
		Email: email, Phone: phone,
	})
}

// ReadProvider reads one rider's account state.
func (s *Service) ReadProvider(ctx context.Context, providerID string) (orgprovider.ProviderView, error) {
	c, err := s.store.GetProvider(ctx, strings.TrimSpace(providerID))
	if err != nil {
		return orgprovider.ProviderView{}, err
	}
	return orgprovider.ProviderView{ProviderID: c.ProviderID, AccountState: c.AccountState(), SourceVersion: c.SourceVersion}, nil
}

// CreateLink records a REQUESTED association for the calling organization.
// callerOrganization is the X-Organization-ID header; a body naming another
// organization is answered as not found.
func (s *Service) CreateLink(ctx context.Context, key, callerOrganization, caller, correlation string, req orgprovider.LinkRequest) (LinkRecord, bool, error) {
	if err := s.writable(); err != nil {
		return LinkRecord{}, false, err
	}
	req.OrganizationID = strings.TrimSpace(req.OrganizationID)
	req.MembershipID = strings.TrimSpace(req.MembershipID)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if strings.TrimSpace(key) == "" || req.OrganizationID == "" || req.MembershipID == "" || req.ProviderID == "" {
		return LinkRecord{}, false, ErrInvalid
	}
	if req.OrganizationID != strings.TrimSpace(callerOrganization) {
		return LinkRecord{}, false, ErrNotFound
	}
	return s.store.CreateLink(ctx, key, Fingerprint(req), caller, correlation, req)
}

// ReadLink reads an association inside the calling organization.
func (s *Service) ReadLink(ctx context.Context, linkID, organizationID string) (LinkRecord, error) {
	if strings.TrimSpace(organizationID) == "" {
		return LinkRecord{}, ErrNotFound
	}
	return s.store.GetLink(ctx, strings.TrimSpace(linkID), strings.TrimSpace(organizationID))
}

// ReadLinkByProvider reads a rider's live association with the organization.
func (s *Service) ReadLinkByProvider(ctx context.Context, providerID, organizationID string) (LinkRecord, error) {
	if strings.TrimSpace(organizationID) == "" {
		return LinkRecord{}, ErrNotFound
	}
	return s.store.GetLiveLinkByProvider(ctx, strings.TrimSpace(providerID), strings.TrimSpace(organizationID))
}

// TransitionLink applies an organization-initiated change (suspend,
// reactivate, revoke). An organization can never accept on the rider's behalf.
func (s *Service) TransitionLink(ctx context.Context, linkID, key, callerOrganization, caller, correlation string, req orgprovider.TransitionRequest) (LinkRecord, error) {
	if err := s.writable(); err != nil {
		return LinkRecord{}, err
	}
	req.State = strings.ToUpper(strings.TrimSpace(req.State))
	req.OrganizationID = strings.TrimSpace(callerOrganization)
	if strings.TrimSpace(key) == "" || req.OrganizationID == "" || req.ExpectedVersion < 1 || req.State == "" {
		return LinkRecord{}, ErrInvalid
	}
	fp := Fingerprint(map[string]any{"linkId": linkID, "request": req})
	return s.store.TransitionLink(ctx, strings.TrimSpace(linkID), key, fp, caller, correlation, req)
}

// ListForProvider lists the rider's own associations.
func (s *Service) ListForProvider(ctx context.Context, providerID string) ([]orgprovider.ProviderAssociationView, error) {
	if strings.TrimSpace(providerID) == "" {
		return nil, ErrNotFound
	}
	records, err := s.store.ListForProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	out := make([]orgprovider.ProviderAssociationView, 0, len(records))
	for _, r := range records {
		out = append(out, r.ProviderView())
	}
	return out, nil
}

// Decide records the rider's own accept or decline.
func (s *Service) Decide(ctx context.Context, providerID, linkID string, accept bool, expectedVersion int64) (orgprovider.ProviderAssociationView, error) {
	if strings.TrimSpace(providerID) == "" {
		return orgprovider.ProviderAssociationView{}, ErrNotFound
	}
	record, err := s.store.Decide(ctx, providerID, strings.TrimSpace(linkID), accept, expectedVersion)
	if err != nil {
		return orgprovider.ProviderAssociationView{}, err
	}
	return record.ProviderView(), nil
}
