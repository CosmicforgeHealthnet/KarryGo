// Package usecases implements organization-managed provider provisioning:
// resolve-before-provision, consent-based association, capabilities, policies
// and sub-wallet linkage.
//
// This service never creates a competing provider identity and never issues a
// provider credential. Provider authentication stays in the owning driver
// service, which is already passwordless (email + phone + emailed OTP).
package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// EventPublisher appends domain events to the outbox in the caller's tx.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// OrganizationReader supplies the organization facts provider flows need.
type OrganizationReader interface {
	OrganizationStatus(ctx context.Context, organizationID string) (name string, status string, err error)
}

// Options configure the provider service.
type Options struct {
	Pool          *pgxpool.Pool
	Providers     *providerrepos.Repository
	Organizations OrganizationReader
	Contracts     platformclients.ContractSet
	Sender        authclients.Sender
	Events        EventPublisher
	Audit         *audit.Recorder
	Metrics       *observability.Metrics

	InvitationSecret   []byte
	InvitationTTL      time.Duration
	AcceptURLTemplate  string
	DefaultCountryCode string
	Now                func() time.Time
}

// Service implements managed provider use cases.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.InvitationTTL <= 0 {
		opts.InvitationTTL = 14 * 24 * time.Hour
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

// CreateProviderInput asks to link or provision a managed provider.
type CreateProviderInput struct {
	Tenant           tenant.Context
	LegalName        string
	Email            string
	Phone            string
	ServiceCategory  string
	HomeBranchID     string
	RelationshipType string
	EmploymentType   string
	EmployeeNumber   string
	IdempotencyKey   string
}

// CreateProviderResult describes the outcome of a create request.
type CreateProviderResult struct {
	Membership *models.Membership
	Saga       *models.ProvisioningSaga
	// InvitationSent reports whether the consent invitation was emailed.
	InvitationSent bool
}

// CreateProvider resolves before provisioning, then invites the provider to
// consent to the organization association.
//
// The order is deliberate and never skipped:
//  1. normalize email and phone;
//  2. ask the OWNING provider service to resolve by BOTH contacts;
//  3. matched  -> link that identity;
//     not found -> request organization-sponsored provisioning;
//     conflict  -> fail atomically with provider_identity_conflict;
//  4. refuse if the provider already has a live membership elsewhere;
//  5. create the association invitation — the provider must accept it
//     themselves. The company can never accept on their behalf.
func (s *Service) CreateProvider(ctx context.Context, input CreateProviderInput) (*CreateProviderResult, error) {
	pair, err := contact.NormalizePair(input.Email, input.Phone, s.opts.DefaultCountryCode)
	if err != nil {
		return nil, apperrors.Validation("A valid email address and phone number are both required.",
			[]apperrors.FieldViolation{{Field: "email", Message: "A valid email and phone are required."}})
	}
	if input.LegalName == "" {
		return nil, apperrors.Validation("A legal name is required.",
			[]apperrors.FieldViolation{{Field: "legalName", Message: "Required."}})
	}
	owningService, ok := platformclients.OwningService(input.ServiceCategory)
	if !ok {
		return nil, apperrors.Validation("Unknown service category.",
			[]apperrors.FieldViolation{{Field: "serviceCategory", Message: "Must be DISPATCH, TAXI or HAULING."}})
	}
	contract, ok := s.opts.Contracts.Providers[input.ServiceCategory]
	if !ok {
		return nil, errorcodes.Unavailable(errorcodes.ProviderServiceUnavailable,
			"This provider service is not available right now.")
	}

	// Create the membership shell first so the saga has a durable anchor even
	// if the downstream call fails.
	membership := &models.Membership{
		ProviderService:  owningService,
		ServiceCategory:  input.ServiceCategory,
		OnboardingSource: models.SourceLinked, // corrected below if sponsored
		RelationshipType: orDefault(input.RelationshipType, models.RelationshipEmployee),
		Status:           models.StatusInvited,
	}
	if input.HomeBranchID != "" {
		membership.HomeBranchID = &input.HomeBranchID
	}
	if input.EmploymentType != "" {
		membership.EmploymentType = &input.EmploymentType
	}
	if input.EmployeeNumber != "" {
		membership.EmployeeNumber = &input.EmployeeNumber
	}

	idempotencyKey := input.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = "provider:" + input.Tenant.OrganizationID + ":" + pair.EmailNormalized + ":" + input.ServiceCategory
	}

	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.CreateMembership(ctx, tx, input.Tenant.OrganizationID, membership); err != nil {
			return err
		}
		return s.opts.Providers.CreateSaga(ctx, tx, input.Tenant.OrganizationID, &models.ProvisioningSaga{
			MembershipID:    membership.ID,
			ProviderService: owningService,
			IdempotencyKey:  idempotencyKey,
		})
	}); err != nil {
		return nil, err
	}

	meta := platformclients.CallMeta{RequestID: input.Tenant.RequestID, CorrelationID: input.Tenant.CorrelationID}

	// Step 1: resolve by BOTH email and phone.
	resolution, err := contract.Resolve(ctx, meta, platformclients.ProviderResolveRequest{
		EmailNormalized: pair.EmailNormalized,
		PhoneE164:       pair.PhoneE164,
	})
	if err != nil {
		s.markSagaStep(ctx, membership.ID, "provider_resolution_status", "UNAVAILABLE",
			models.SagaReconciliationRequired, "provider_service_unavailable")
		s.opts.Metrics.IncCounter("fleet_partner_provider_provisioning_total",
			map[string]string{"outcome": "resolve_unavailable"}, 1)
		return nil, errorcodes.Unavailable(errorcodes.ProviderServiceUnavailable,
			"The provider service is unavailable right now. This request will be retried.")
	}

	switch resolution.Outcome {
	case platformclients.ProviderResolveConflict:
		// Email and phone identify different providers: never auto-merge.
		s.markSagaStep(ctx, membership.ID, "provider_resolution_status", "CONFLICT",
			models.SagaBlocked, string(errorcodes.ProviderIdentityConflict))
		s.opts.Metrics.IncCounter("fleet_partner_provider_identity_conflict_total", nil, 1)
		return nil, errorcodes.Conflict(errorcodes.ProviderIdentityConflict,
			"The email address and phone number belong to different provider accounts.")

	case platformclients.ProviderResolveMatched:
		if resolution.ManagedByOrgID != "" && resolution.ManagedByOrgID != input.Tenant.OrganizationID {
			s.markSagaStep(ctx, membership.ID, "provider_resolution_status", "CONFLICT",
				models.SagaBlocked, string(errorcodes.ProviderAlreadyManaged))
			return nil, errorcodes.Conflict(errorcodes.ProviderAlreadyManaged,
				"This provider is already managed by another organization for this service.")
		}
		if err := s.assertNoLiveMembershipElsewhere(ctx, owningService, resolution.ProviderID, membership.ID); err != nil {
			return nil, err
		}
		if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Providers.SetProviderID(ctx, tx, input.Tenant.OrganizationID, membership.ID,
				resolution.ProviderID, resolution.SourceVersion)
		}); err != nil {
			return nil, err
		}
		membership.ProviderID = &resolution.ProviderID
		membership.OnboardingSource = models.SourceLinked
		s.markSagaStep(ctx, membership.ID, "provider_resolution_status", "RESOLVED_EXISTING",
			models.SagaInProgress, "")

	case platformclients.ProviderResolveNotFound:
		// Step 2: organization-sponsored provisioning by the owning service.
		identity, err := contract.ProvisionSponsored(ctx, meta, platformclients.ProviderProvisionRequest{
			OrganizationID:  input.Tenant.OrganizationID,
			LegalName:       input.LegalName,
			EmailNormalized: pair.EmailNormalized,
			PhoneE164:       pair.PhoneE164,
			ServiceCategory: input.ServiceCategory,
			OnboardingMode:  "ORGANIZATION_SPONSORED",
			ProviderMode:    "ORGANIZATION_MANAGED",
			IdempotencyKey:  idempotencyKey,
		})
		if err != nil {
			// Unknown outcome: a provider may or may not have been created,
			// so the reconciler must read back rather than blindly retry.
			s.markSagaStep(ctx, membership.ID, "provider_identity_status", "UNKNOWN_OUTCOME",
				models.SagaReconciliationRequired, "provisioning_unknown_outcome")
			s.opts.Metrics.IncCounter("fleet_partner_provider_provisioning_total",
				map[string]string{"outcome": "provision_unknown"}, 1)
			return nil, errorcodes.Unavailable(errorcodes.ProviderServiceUnavailable,
				"The provider service is unavailable right now. This request will be retried.")
		}
		if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Providers.SetProviderID(ctx, tx, input.Tenant.OrganizationID, membership.ID,
				identity.ProviderID, identity.SourceVersion)
		}); err != nil {
			return nil, err
		}
		membership.ProviderID = &identity.ProviderID
		membership.OnboardingSource = models.SourceSponsored
		s.markSagaStep(ctx, membership.ID, "provider_identity_status", "PROVISIONED",
			models.SagaInProgress, "")

	default:
		return nil, errorcodes.Unavailable(errorcodes.ProviderServiceUnavailable,
			"The provider service returned an unexpected result.")
	}

	// Step 3: request the association — the provider must consent.
	invitationSent := s.createAssociationInvitation(ctx, input, membership, pair, contract, meta)

	saga, _ := s.opts.Providers.GetSaga(ctx, input.Tenant.OrganizationID, membership.ID)
	reloaded, err := s.opts.Providers.GetMembership(ctx, input.Tenant.OrganizationID, membership.ID)
	if err != nil {
		return nil, err
	}
	return &CreateProviderResult{Membership: reloaded, Saga: saga, InvitationSent: invitationSent}, nil
}

// assertNoLiveMembershipElsewhere enforces one settlement organization per
// provider per service category.
func (s *Service) assertNoLiveMembershipElsewhere(ctx context.Context, providerService string, providerID string, ownMembershipID string) error {
	existing, err := s.opts.Providers.FindLiveByProvider(ctx, s.opts.Pool, providerService, providerID)
	if err != nil {
		if errors.Is(err, providerrepos.ErrNotFound) {
			return nil
		}
		return err
	}
	if existing.ID == ownMembershipID {
		return nil
	}
	return errorcodes.Conflict(errorcodes.ProviderAlreadyManaged,
		"This provider already has an active organization for this service.")
}

// createAssociationInvitation asks the owning service to create the
// association and emails the provider an invitation.
func (s *Service) createAssociationInvitation(ctx context.Context, input CreateProviderInput, membership *models.Membership, pair contact.Pair, contract platformclients.ProviderContract, meta platformclients.CallMeta) bool {
	organizationName, _, _ := s.opts.Organizations.OrganizationStatus(ctx, input.Tenant.OrganizationID)

	if membership.ProviderID != nil {
		if _, err := contract.CreateAssociation(ctx, meta, platformclients.ProviderAssociationRequest{
			OrganizationID:   input.Tenant.OrganizationID,
			OrganizationName: organizationName,
			ProviderID:       *membership.ProviderID,
			MembershipID:     membership.ID,
			RelationshipType: membership.RelationshipType,
			IdempotencyKey:   "assoc:" + membership.ID,
		}); err != nil {
			s.markSagaStep(ctx, membership.ID, "association_status", "UNAVAILABLE",
				models.SagaReconciliationRequired, "association_unavailable")
		} else {
			s.markSagaStep(ctx, membership.ID, "association_status", "AWAITING_PROVIDER",
				models.SagaInProgress, "")
		}
	}

	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		return false
	}
	invitation := &models.Invitation{
		MembershipID:        &membership.ID,
		ProviderService:     membership.ProviderService,
		LegalName:           input.LegalName,
		EmailNormalized:     pair.EmailNormalized,
		PhoneE164:           pair.PhoneE164,
		ExistingProviderID:  membership.ProviderID,
		InvitationTokenHash: auth.HashRefreshToken(s.opts.InvitationSecret, token),
		ExpiresAt:           s.opts.Now().Add(s.opts.InvitationTTL),
		CreatedBy:           input.Tenant.ActorID,
	}

	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.CreateInvitation(ctx, tx, input.Tenant.OrganizationID, invitation); err != nil {
			return err
		}
		if err := s.opts.Providers.TransitionMembership(ctx, tx, input.Tenant.OrganizationID, membership.ID,
			models.StatusPendingProviderAcceptance, membership.Version, models.HistoryEntry{
				ActorType: string(input.Tenant.ActorType), ActorID: input.Tenant.ActorID,
				ReasonCode: "association_invited",
				RequestID:  input.Tenant.RequestID, CorrelationID: input.Tenant.CorrelationID,
			}); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.provider.invited.v1", "provider_membership",
			membership.ID, membership.Version+1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"membershipId":    membership.ID,
				"organizationId":  input.Tenant.OrganizationID,
				"serviceCategory": membership.ServiceCategory,
				// No token, no OTP, no credential in the event payload.
				"maskedEmail": contact.MaskEmail(pair.EmailNormalized),
			})
	}); err != nil {
		return false
	}

	acceptURL := s.opts.AcceptURLTemplate
	if acceptURL != "" {
		acceptURL = fmt.Sprintf("%s?token=%s", acceptURL, token)
	}
	reference, err := s.opts.Sender.SendInvitation(ctx, authclients.InvitationMessage{
		Email:            pair.EmailNormalized,
		Phone:            pair.PhoneE164,
		OrganizationName: organizationName,
		AcceptURL:        acceptURL,
		ExpiresAt:        invitation.ExpiresAt,
		EventType:        authclients.EventCorporateProviderInvitation,
		IdempotencyKey:   "provider-invite:" + invitation.ID,
		RequestID:        input.Tenant.RequestID,
		CorrelationID:    input.Tenant.CorrelationID,
	})
	if err != nil {
		_ = s.opts.Providers.RecordInvitationDelivery(ctx, invitation.ID, "UNAVAILABLE", "")
		s.markSagaStep(ctx, membership.ID, "notification_status", "UNAVAILABLE", models.SagaInProgress, "invitation_delivery_failed")
		return false
	}
	_ = s.opts.Providers.RecordInvitationDelivery(ctx, invitation.ID, "ACCEPTED", reference)
	s.markSagaStep(ctx, membership.ID, "notification_status", "ACCEPTED", models.SagaInProgress, "")
	s.opts.Metrics.IncCounter("fleet_partner_provider_invitation_total", map[string]string{"outcome": "sent"}, 1)
	return true
}

func (s *Service) markSagaStep(ctx context.Context, membershipID string, column string, value string, overall string, errorCode string) {
	var next *time.Time
	if overall == models.SagaReconciliationRequired {
		retryAt := s.opts.Now().Add(2 * time.Minute)
		next = &retryAt
	}
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Providers.UpdateSagaStep(ctx, tx, membershipID, column, value, overall, errorCode, next)
	})
}

func orDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
