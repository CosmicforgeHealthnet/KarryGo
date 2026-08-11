package crm

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8A internal event types. Each is owned by exactly one authority, and
// the caller allowlist below is what stops one service asserting another's
// facts — payment-wallet-service cannot change consent, and a commerce
// publisher cannot redact a customer.
const (
	EventRelationshipObserved = "CUSTOMER_RELATIONSHIP_OBSERVED"
	EventIdentityLinked       = "CUSTOMER_IDENTITY_LINKED"
	EventIdentityMerged       = "CUSTOMER_IDENTITY_MERGED"
	EventConsentChanged       = "CUSTOMER_CONSENT_CHANGED"
	EventPrivacyChanged       = "CUSTOMER_PRIVACY_CHANGED"
	EventOrderFactChanged     = "CRM_ORDER_FACT_CHANGED"
)

// EventAuthority declares which service owns an event type and which feature
// flag must be enabled for it to be processed.
type EventAuthority struct {
	// AllowedCallers is the exact set of service identities permitted to send
	// this event type. An empty set means the event type is not accepted.
	AllowedCallers []string
	// RequiresFlag names the integration flag gating this event type.
	RequiresFlag func(Flags) bool
}

// EventRegistry maps event types to their authority.
//
// The identity, consent and privacy events are owned by customer-service, the
// platform's customer authority. Order facts are owned by whichever service
// becomes the authoritative vendor commerce order publisher; none exists yet,
// so that entry names the intended owner and stays disabled by its flag rather
// than accepting events from anyone.
var EventRegistry = map[string]EventAuthority{
	EventRelationshipObserved: {
		AllowedCallers: []string{"customer-service"},
		RequiresFlag:   func(f Flags) bool { return f.CustomerProjectionEnabled },
	},
	EventIdentityLinked: {
		AllowedCallers: []string{"customer-service"},
		RequiresFlag:   func(f Flags) bool { return f.CustomerProjectionEnabled },
	},
	EventIdentityMerged: {
		AllowedCallers: []string{"customer-service"},
		RequiresFlag:   func(f Flags) bool { return f.CustomerProjectionEnabled },
	},
	EventConsentChanged: {
		AllowedCallers: []string{"customer-service"},
		RequiresFlag:   func(f Flags) bool { return f.ConsentIntegrationEnabled },
	},
	EventPrivacyChanged: {
		AllowedCallers: []string{"customer-service"},
		RequiresFlag:   func(f Flags) bool { return f.PrivacyIntegrationEnabled },
	},
	EventOrderFactChanged: {
		AllowedCallers: []string{"vendor-order-service"},
		RequiresFlag:   func(f Flags) bool { return f.CustomerProjectionEnabled },
	},
}

// AuthorizeEvent checks that this caller owns this event type and that the
// integration is enabled.
func (s *Service) AuthorizeEvent(caller, eventType string) error {
	authority, known := EventRegistry[eventType]
	if !known {
		s.metrics.Inc("crm_internal_event_rejections_total", "reason", "unknown_event_type")
		return apperrors.BadRequest("Unsupported CRM event type.", nil)
	}
	if !containsString(authority.AllowedCallers, caller) {
		s.metrics.Inc("crm_internal_event_rejections_total", "reason", "caller_not_authority")
		return apperrors.Forbidden("This service is not the authority for this event type.", nil)
	}
	if authority.RequiresFlag != nil && !authority.RequiresFlag(s.flags) {
		s.metrics.Inc("crm_internal_event_rejections_total", "reason", "integration_disabled")
		return apperrors.Unavailable("This CRM integration is not enabled.", nil)
	}
	return nil
}

// EventEnvelope is the wire shape of an inbound CRM event.
type EventEnvelope struct {
	EventID       string       `json:"event_id"`
	EventType     string       `json:"event_type"`
	SourceService string       `json:"source_service"`
	SourceVersion int64        `json:"source_version"`
	OccurredAt    time.Time    `json:"occurred_at"`
	CorrelationID string       `json:"correlation_id"`
	CausationID   string       `json:"causation_id"`
	BusinessRef   string       `json:"business_ref"`
	Subject       EventSubject `json:"subject"`
	Payload       EventPayload `json:"payload"`
}

// EventSubject identifies the customer subject an event concerns.
type EventSubject struct {
	Type              string `json:"type"`
	Ref               string `json:"ref"`
	PrivacySubjectRef string `json:"privacy_subject_ref"`
}

// EventPayload is the discriminated body. Only the fields belonging to the
// declared event type are read; anything else is ignored by validation rather
// than silently applied.
type EventPayload struct {
	// Consent
	Purpose     string     `json:"purpose"`
	Channel     string     `json:"channel"`
	Decision    string     `json:"decision"`
	EffectiveAt *time.Time `json:"effective_at"`
	ExpiresAt   *time.Time `json:"expires_at"`

	// Privacy
	Action             string `json:"action"`
	RetentionPolicyRef string `json:"retention_policy_ref"`

	// Identity link
	SourceSubject      *EventSubject `json:"source_subject"`
	TargetSubject      *EventSubject `json:"target_subject"`
	AuthorityReference string        `json:"authority_reference"`

	// Order fact
	SourceOrderRef string     `json:"source_order_ref"`
	OrderState     string     `json:"order_state"`
	CompletedAt    *time.Time `json:"completed_at"`
	Currency       string     `json:"currency"`
	GrossMinor     int64      `json:"gross_amount_minor"`
	NetMinor       int64      `json:"net_amount_minor"`
	RefundedMinor  int64      `json:"refunded_amount_minor"`
	BranchRef      string     `json:"branch_ref"`
	CapabilityType string     `json:"capability_type"`

	// Relationship
	AcquisitionSourceType string `json:"acquisition_source_type"`
	AcquisitionSourceRef  string `json:"acquisition_source_ref"`
}

// Validate checks envelope-level invariants shared by every event type.
func (e EventEnvelope) Validate() error {
	var violations []apperrors.FieldViolation
	require := func(ok bool, field, message string) {
		if !ok {
			violations = append(violations, apperrors.FieldViolation{Field: field, Message: message})
		}
	}
	require(strings.TrimSpace(e.EventID) != "", "event_id", "Event id is required.")
	require(strings.TrimSpace(e.EventType) != "", "event_type", "Event type is required.")
	require(strings.TrimSpace(e.SourceService) != "", "source_service", "Source service is required.")
	require(strings.TrimSpace(e.BusinessRef) != "", "business_ref", "Business reference is required.")
	require(e.SourceVersion >= 1, "source_version", "Source version must be positive.")
	require(!e.OccurredAt.IsZero(), "occurred_at", "Occurred-at timestamp is required.")
	if len(violations) > 0 {
		return apperrors.Validation("Check the event envelope.", violations)
	}
	return nil
}

// ProcessEvent applies one authenticated authoritative event.
//
// Everything a single event implies — dedupe claim, profile resolution,
// projection change, audit entry and outbox event — commits in one
// transaction, so a partially-applied event is not a state the database can be
// left in.
func (s *Service) ProcessEvent(ctx context.Context, caller string, envelope EventEnvelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if err := s.AuthorizeEvent(caller, envelope.EventType); err != nil {
		return err
	}
	if envelope.SourceService != caller {
		// The signed caller identity is authoritative; a body claiming a
		// different producer is rejected rather than trusted.
		s.metrics.Inc("crm_internal_event_rejections_total", "reason", "source_service_mismatch")
		return apperrors.Forbidden("Event source service does not match the authenticated caller.", nil)
	}

	actor := Actor{
		VendorID:      envelope.BusinessRef,
		CorrelationID: envelope.CorrelationID,
		CausationID:   envelope.CausationID,
	}
	if err := s.businessExists(ctx, actor.VendorID); err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	claimed, err := s.ClaimEvent(ctx, tx, caller, envelope.EventID, envelope.EventType, envelope.Payload)
	if err != nil {
		return err
	}
	if !claimed {
		// Already processed; committing the (no-op) transaction keeps the
		// response identical to the first delivery.
		return tx.Commit(ctx)
	}

	switch envelope.EventType {
	case EventRelationshipObserved:
		err = s.handleRelationshipObserved(ctx, tx, actor, envelope)
	case EventConsentChanged:
		err = s.handleConsentChanged(ctx, tx, actor, envelope)
	case EventPrivacyChanged:
		err = s.handlePrivacyChanged(ctx, tx, actor, envelope)
	case EventIdentityLinked, EventIdentityMerged:
		err = s.handleIdentityLinked(ctx, tx, actor, envelope)
	case EventOrderFactChanged:
		err = s.handleOrderFactChanged(ctx, tx, actor, envelope)
	default:
		err = apperrors.BadRequest("Unsupported CRM event type.", nil)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) businessExists(ctx context.Context, businessID string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_businesses WHERE id=$1)`, businessID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		s.metrics.Inc("crm_internal_event_rejections_total", "reason", "unknown_business")
		return apperrors.NotFound("Business not found.", nil)
	}
	return nil
}

func (s *Service) handleRelationshipObserved(ctx context.Context, tx pgx.Tx, actor Actor, envelope EventEnvelope) error {
	subject, err := subjectOf(envelope.Subject)
	if err != nil {
		return err
	}
	profile, err := s.ResolveOrCreateProfile(ctx, tx, actor, RelationshipInput{
		Subject:               subject,
		AcquisitionSourceType: optional(envelope.Payload.AcquisitionSourceType),
		AcquisitionSourceRef:  optional(envelope.Payload.AcquisitionSourceRef),
		ObservedAt:            envelope.OccurredAt,
	})
	if err != nil {
		return err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.profile.observed", "vendor_customer_profile", profile.ID, "",
		nil, map[string]any{"subject_type": string(profile.SubjectType), "source_version": envelope.SourceVersion}, ""); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, "vendor.crm.profile.created", "vendor_customer_profile", profile.ID, profile.Version,
		map[string]any{"profile_ref": profile.ID, "subject_type": string(profile.SubjectType)})
}

func (s *Service) handleConsentChanged(ctx context.Context, tx pgx.Tx, actor Actor, envelope EventEnvelope) error {
	if envelope.Payload.Purpose != string(ConsentPurposeMarketing) {
		return apperrors.BadRequest("Unsupported consent purpose.", nil)
	}
	if !IsValidConsentChannel(envelope.Payload.Channel) {
		return apperrors.BadRequest("Unsupported consent channel.", nil)
	}
	decision := ConsentDecision(envelope.Payload.Decision)
	switch decision {
	case ConsentAllow, ConsentDeny, ConsentUnknown:
	default:
		return apperrors.BadRequest("Unsupported consent decision.", nil)
	}

	subject, err := subjectOf(envelope.Subject)
	if err != nil {
		return err
	}
	profile, err := s.resolveProfile(ctx, tx, actor, subject)
	if err != nil {
		return err
	}
	if profile == nil {
		// No relationship exists, so there is nothing to project onto. The
		// event is claimed (and therefore not redelivered forever) and the
		// decision will be re-asserted by the authority if a relationship
		// later appears. Nothing is invented in the meantime.
		return nil
	}
	if profile.IsRedacted() {
		s.metrics.Inc("crm_anti_resurrection_blocks_total", "reason", "consent_after_redaction")
		return nil
	}
	return s.ApplyConsent(ctx, tx, actor, profile.ID, ConsentInput{
		SourceService: envelope.SourceService,
		SourceRef:     optional(envelope.Payload.AuthorityReference),
		SourceVersion: envelope.SourceVersion,
		Purpose:       ConsentPurposeMarketing,
		Channel:       ConsentChannel(envelope.Payload.Channel),
		Decision:      decision,
		ObservedAt:    envelope.OccurredAt,
		EffectiveAt:   envelope.Payload.EffectiveAt,
		ExpiresAt:     envelope.Payload.ExpiresAt,
	})
}

func (s *Service) handlePrivacyChanged(ctx context.Context, tx pgx.Tx, actor Actor, envelope EventEnvelope) error {
	if !IsValidPrivacyAction(envelope.Payload.Action) {
		return apperrors.BadRequest("Unsupported privacy action.", nil)
	}
	action := PrivacyAction(envelope.Payload.Action)

	// The authority's own privacy subject reference is preferred when supplied;
	// it is the identifier designed for suppression. The subject reference is
	// the documented fallback.
	privacySubjectRef := strings.TrimSpace(envelope.Subject.PrivacySubjectRef)
	if privacySubjectRef == "" {
		privacySubjectRef = strings.TrimSpace(envelope.Subject.Ref)
	}
	if privacySubjectRef == "" {
		return apperrors.BadRequest("Privacy subject reference is required.", nil)
	}

	var profile *Profile
	if envelope.Subject.Ref != "" {
		subject, err := subjectOf(envelope.Subject)
		if err != nil {
			return err
		}
		if profile, err = s.resolveProfile(ctx, tx, actor, subject); err != nil {
			return err
		}
	}

	if action == PrivacyActionIdentityMerge {
		// Routed through the controlled link path rather than rewriting
		// references in place.
		if envelope.Payload.SourceSubject == nil || envelope.Payload.TargetSubject == nil {
			return apperrors.BadRequest("Identity merge requires source and target subjects.", nil)
		}
		return s.handleIdentityLinked(ctx, tx, actor, envelope)
	}

	return s.ApplyPrivacy(ctx, tx, actor, profile, PrivacyInput{
		SourceService:     envelope.SourceService,
		SourceRef:         optional(envelope.Payload.AuthorityReference),
		SourceVersion:     envelope.SourceVersion,
		PrivacySubjectRef: privacySubjectRef,
		Action:            action,
		ObservedAt:        envelope.OccurredAt,
		RetentionPolicy:   optional(envelope.Payload.RetentionPolicyRef),
	})
}

func (s *Service) handleIdentityLinked(ctx context.Context, tx pgx.Tx, actor Actor, envelope EventEnvelope) error {
	if envelope.Payload.SourceSubject == nil || envelope.Payload.TargetSubject == nil {
		return apperrors.BadRequest("Identity link requires source and target subjects.", nil)
	}
	source, err := subjectOf(*envelope.Payload.SourceSubject)
	if err != nil {
		return err
	}
	target, err := subjectOf(*envelope.Payload.TargetSubject)
	if err != nil {
		return err
	}
	if strings.TrimSpace(envelope.Payload.AuthorityReference) == "" {
		// A link without an authority reference is not evidence.
		return apperrors.BadRequest("Identity link requires an authority reference.", nil)
	}
	return s.ApplyIdentityLink(ctx, tx, actor, IdentityLinkInput{
		AuthorityService:   envelope.SourceService,
		AuthorityReference: envelope.Payload.AuthorityReference,
		AuthorityVersion:   envelope.SourceVersion,
		EvidenceEventID:    envelope.EventID,
		Source:             source,
		Target:             target,
	})
}

func (s *Service) handleOrderFactChanged(ctx context.Context, tx pgx.Tx, actor Actor, envelope EventEnvelope) error {
	if !IsValidOrderFactState(envelope.Payload.OrderState) {
		return apperrors.BadRequest("Unsupported order state.", nil)
	}
	if strings.TrimSpace(envelope.Payload.SourceOrderRef) == "" {
		return apperrors.BadRequest("Source order reference is required.", nil)
	}
	if len(envelope.Payload.Currency) != 3 {
		return apperrors.BadRequest("Order currency is required.", nil)
	}
	if envelope.Payload.GrossMinor < 0 || envelope.Payload.NetMinor < 0 || envelope.Payload.RefundedMinor < 0 {
		return apperrors.BadRequest("Order amounts cannot be negative.", nil)
	}
	if envelope.Payload.NetMinor > envelope.Payload.GrossMinor {
		return apperrors.BadRequest("Net amount cannot exceed gross amount.", nil)
	}
	state := OrderFactState(envelope.Payload.OrderState)
	if (state == OrderCompleted) != (envelope.Payload.CompletedAt != nil) {
		return apperrors.BadRequest("Completed orders must carry a completion time.", nil)
	}

	subject, err := subjectOf(envelope.Subject)
	if err != nil {
		return err
	}
	profile, err := s.ResolveOrCreateProfile(ctx, tx, actor, RelationshipInput{
		Subject:    subject,
		ObservedAt: envelope.OccurredAt,
	})
	if err != nil {
		return err
	}
	return s.ApplyOrderFact(ctx, tx, actor, profile, OrderFactInput{
		SourceService:  envelope.SourceService,
		SourceOrderRef: envelope.Payload.SourceOrderRef,
		SourceEventID:  envelope.EventID,
		SourceVersion:  envelope.SourceVersion,
		OrderState:     state,
		CompletedAt:    envelope.Payload.CompletedAt,
		Currency:       strings.ToUpper(envelope.Payload.Currency),
		GrossMinor:     envelope.Payload.GrossMinor,
		NetMinor:       envelope.Payload.NetMinor,
		RefundedMinor:  envelope.Payload.RefundedMinor,
		BranchID:       optional(envelope.Payload.BranchRef),
		CapabilityType: optional(envelope.Payload.CapabilityType),
		ObservedAt:     envelope.OccurredAt,
	})
}

func subjectOf(subject EventSubject) (SubjectRef, error) {
	if !IsValidSubjectType(subject.Type) {
		return SubjectRef{}, apperrors.BadRequest("Unsupported customer subject type.", nil)
	}
	ref := strings.TrimSpace(subject.Ref)
	if ref == "" {
		return SubjectRef{}, apperrors.BadRequest("Customer subject reference is required.", nil)
	}
	return SubjectRef{Type: SubjectType(subject.Type), Ref: ref}, nil
}

func optional(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
