package crm

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// PrivacyHandler applies one privacy instruction to one CRM domain.
//
// Section 8A registers handlers for the domains it owns. Later packages (tags,
// segments, interactions, tasks, loyalty, campaigns, corporate) register their
// own, which is why redaction is a registry walk rather than a hard-coded list
// of table names: adding 8B must not mean rewriting the redaction engine.
type PrivacyHandler interface {
	// Domain names the CRM domain this handler is responsible for.
	Domain() string
	// Restrict limits processing without removing the relationship.
	Restrict(ctx context.Context, tx pgx.Tx, businessID, profileID string) error
	// Redact removes this domain's personal-targeting data.
	Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error
	// MarketingSuppress removes targetability without removing the record.
	MarketingSuppress(ctx context.Context, tx pgx.Tx, businessID, profileID string) error
	// IdentityMerge moves this domain's rows from source to target.
	IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error
}

// PrivacyRegistry holds the registered handlers in a deterministic order.
type PrivacyRegistry struct {
	handlers []PrivacyHandler
}

// NewPrivacyRegistry builds the Section 8A registry.
func NewPrivacyRegistry(s *Service) *PrivacyRegistry {
	return &PrivacyRegistry{handlers: []PrivacyHandler{
		profilePrivacyHandler{service: s},
		consentPrivacyHandler{service: s},
		commercialPrivacyHandler{service: s},
		identityLinkPrivacyHandler{service: s},
	}}
}

// Register adds a handler. Later CRM packages call this from the composition
// root; ordering stays deterministic so redaction is reproducible.
func (r *PrivacyRegistry) Register(handler PrivacyHandler) {
	r.handlers = append(r.handlers, handler)
	sort.SliceStable(r.handlers, func(i, j int) bool { return r.handlers[i].Domain() < r.handlers[j].Domain() })
}

// Domains lists the registered domains, which Customer 360 and the docs use to
// state exactly what a redaction covers.
func (r *PrivacyRegistry) Domains() []string {
	out := make([]string, 0, len(r.handlers))
	for _, handler := range r.handlers {
		out = append(out, handler.Domain())
	}
	return out
}

// PrivacyInput is one authoritative privacy instruction.
type PrivacyInput struct {
	SourceService     string
	SourceRef         *string
	SourceVersion     int64
	PrivacySubjectRef string
	Action            PrivacyAction
	ObservedAt        time.Time
	RetentionPolicy   *string
}

// ApplyPrivacy records and processes one authoritative privacy instruction.
//
// Processing is synchronous and transactional: the projection row, the profile
// state change, every registered domain handler, the immutable history row,
// the audit entry and the outbox event all commit together or not at all.
// There is no window in which a subject is recorded as redacted while their
// data is still targetable.
func (s *Service) ApplyPrivacy(ctx context.Context, tx pgx.Tx, actor Actor, profile *Profile, in PrivacyInput) error {
	var (
		existingID      string
		existingVersion int64
		existingAction  PrivacyAction
		existingStatus  PrivacyProcessingStatus
	)
	err := tx.QueryRow(ctx, `SELECT id,source_version,action,processing_status FROM crm_privacy_projections
		WHERE business_id=$1 AND privacy_subject_ref=$2 AND action=$3 FOR UPDATE`,
		actor.VendorID, in.PrivacySubjectRef, in.Action).Scan(&existingID, &existingVersion, &existingAction, &existingStatus)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		var profileID *string
		if profile != nil {
			profileID = &profile.ID
		}
		if err = tx.QueryRow(ctx, `INSERT INTO crm_privacy_projections
			(id,business_id,profile_id,privacy_subject_ref,action,source_service,source_ref,source_version,
			 observed_at,processing_status,retention_policy_ref)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
			newID("crmp_"), actor.VendorID, profileID, in.PrivacySubjectRef, in.Action,
			in.SourceService, in.SourceRef, in.SourceVersion, in.ObservedAt, PrivacyProcessing, in.RetentionPolicy,
		).Scan(&existingID); err != nil {
			return err
		}
	case err != nil:
		return err
	case in.SourceVersion < existingVersion:
		// A stale instruction must never downgrade a newer privacy decision —
		// a late RESTRICT cannot undo an applied REDACT.
		s.metrics.Inc("crm_privacy_stale_updates_total", "action", string(in.Action))
		return s.recordPrivacyHistory(ctx, tx, actor, existingID, profile, in, PrivacyActive, PrivacyActive, "STALE_IGNORED", "SOURCE_VERSION_STALE")
	case in.SourceVersion == existingVersion && existingStatus == PrivacyApplied:
		// Identical redelivery of an already-applied instruction.
		return nil
	default:
		var profileID *string
		if profile != nil {
			profileID = &profile.ID
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_privacy_projections
			SET profile_id=COALESCE($1,profile_id),source_service=$2,source_ref=$3,source_version=$4,
				observed_at=$5,processing_status=$6,retention_policy_ref=$7,updated_at=now(),version=version+1
			WHERE id=$8`,
			profileID, in.SourceService, in.SourceRef, in.SourceVersion, in.ObservedAt,
			PrivacyProcessing, in.RetentionPolicy, existingID); err != nil {
			return err
		}
	}

	// An instruction for a subject with no relationship yet is still recorded,
	// so it can be honoured the moment one is observed and can never be lost.
	if profile == nil {
		if _, err = tx.Exec(ctx, `UPDATE crm_privacy_projections SET processing_status=$1,processed_at=now(),updated_at=now() WHERE id=$2`,
			PrivacyApplied, existingID); err != nil {
			return err
		}
		s.metrics.Inc("crm_privacy_updates_total", "action", string(in.Action), "matched", "false")
		return s.recordPrivacyHistory(ctx, tx, actor, existingID, nil, in, "NONE", "NONE", "APPLIED", "NO_MATCHING_PROFILE")
	}

	previousState := profile.PrivacyState
	if err = s.runPrivacyHandlers(ctx, tx, actor, in.Action, profile); err != nil {
		return err
	}

	newState := PrivacyStateFor(in.Action, previousState)
	if newState != previousState {
		if _, err = tx.Exec(ctx, `UPDATE vendor_customer_profiles
			SET privacy_state=$1, redacted_at=CASE WHEN $1='REDACTED' THEN now() ELSE redacted_at END,
				updated_at=now(), version=version+1
			WHERE business_id=$2 AND id=$3`,
			newState, actor.VendorID, profile.ID); err != nil {
			return err
		}
		profile.PrivacyState = newState
	}
	if _, err = tx.Exec(ctx, `UPDATE crm_privacy_projections SET processing_status=$1,processed_at=now(),updated_at=now() WHERE id=$2`,
		PrivacyApplied, existingID); err != nil {
		return err
	}

	s.metrics.Inc("crm_privacy_updates_total", "action", string(in.Action), "matched", "true")
	if in.Action == PrivacyActionRedact {
		s.metrics.Inc("crm_redactions_total")
	}
	if err = s.recordPrivacyHistory(ctx, tx, actor, existingID, profile, in, previousState, newState, "APPLIED", ""); err != nil {
		return err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.privacy_state.changed", "vendor_customer_profile", profile.ID, "",
		map[string]any{"privacy_state": previousState},
		map[string]any{"privacy_state": newState, "action": in.Action, "source_version": in.SourceVersion}, ""); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, "vendor.crm.privacy_state.changed", "vendor_customer_profile", profile.ID, profile.Version,
		map[string]any{"profile_ref": profile.ID, "privacy_state": string(newState), "action": string(in.Action), "source_version": in.SourceVersion})
}

func (s *Service) runPrivacyHandlers(ctx context.Context, tx pgx.Tx, actor Actor, action PrivacyAction, profile *Profile) error {
	for _, handler := range s.privacy.handlers {
		var err error
		switch action {
		case PrivacyActionRestrict:
			err = handler.Restrict(ctx, tx, actor.VendorID, profile.ID)
		case PrivacyActionRedact:
			err = handler.Redact(ctx, tx, actor.VendorID, profile.ID)
		case PrivacyActionMarketingSuppress:
			err = handler.MarketingSuppress(ctx, tx, actor.VendorID, profile.ID)
		case PrivacyActionIdentityMerge:
			// IDENTITY_MERGE is never applied by rewriting references here; it
			// is routed through the controlled identity-link path, which holds
			// both profiles under lock and rebuilds commercial facts.
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordPrivacyHistory(ctx context.Context, tx pgx.Tx, actor Actor, projectionID string,
	profile *Profile, in PrivacyInput, previous, next PrivacyState, result, reason string) error {
	var profileID *string
	if profile != nil {
		profileID = &profile.ID
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	_, err := tx.Exec(ctx, `INSERT INTO crm_privacy_processing_history
		(id,business_id,privacy_projection_id,profile_id,action,previous_privacy_state,new_privacy_state,
		 processing_result,safe_reason_code,source_version,correlation_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''))`,
		newID("crmph_"), actor.VendorID, projectionID, profileID, in.Action,
		string(previous), string(next), result, reasonPtr, in.SourceVersion, actor.CorrelationID)
	return err
}

// PrivacySummary is the Customer 360 privacy block. It reports state and
// consequences, never the authority's internal reasoning.
type PrivacySummary struct {
	State                PrivacyState `json:"state"`
	MarketingSuppressed  bool         `json:"marketing_suppressed"`
	Redacted             bool         `json:"redacted"`
	RedactedAt           *time.Time   `json:"redacted_at,omitempty"`
	IntegrationAvailable bool         `json:"integration_available"`
	// CoveredDomains lists the CRM domains a redaction would process, which is
	// what makes the guarantee auditable as later packages register handlers.
	CoveredDomains []string `json:"covered_domains"`
}

// PrivacySummaryFor builds the privacy view for one profile.
func (s *Service) PrivacySummaryFor(ctx context.Context, actor Actor, profile Profile) (PrivacySummary, error) {
	suppressed, err := s.marketingSuppressed(ctx, actor.VendorID, profile.ID)
	if err != nil {
		return PrivacySummary{}, err
	}
	return PrivacySummary{
		State:                profile.PrivacyState,
		MarketingSuppressed:  suppressed,
		Redacted:             profile.IsRedacted(),
		RedactedAt:           profile.RedactedAt,
		IntegrationAvailable: s.flags.PrivacyIntegrationEnabled,
		CoveredDomains:       s.privacy.Domains(),
	}, nil
}

// ---------------------------------------------------------------------------
// Section 8A privacy handlers
// ---------------------------------------------------------------------------

// profilePrivacyHandler owns the relationship record itself.
type profilePrivacyHandler struct{ service *Service }

func (profilePrivacyHandler) Domain() string { return "customer_profile" }

func (profilePrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error {
	// Restriction is expressed by privacy_state alone; no field is cleared,
	// because a restriction is not an erasure.
	return nil
}

// Redact clears presentation and acquisition data and detaches the branch
// affinity used for targeting.
//
// The opaque subject reference is deliberately retained. It is the only stable
// way to recognise a late event for this subject and refuse to recreate the
// relationship; hashing it would not make it anonymous, it would only make
// suppression unreliable. The retained reference is never used for marketing,
// presentation, enrichment or matching beyond that suppression check.
func (profilePrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	_, err := tx.Exec(ctx, `UPDATE vendor_customer_profiles
		SET safe_display_label=NULL, acquisition_source_type=NULL, acquisition_source_ref=NULL,
			preferred_branch_ref=NULL, last_branch_ref=NULL, updated_at=now()
		WHERE business_id=$1 AND id=$2`, businessID, profileID)
	return err
}

func (profilePrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	// Suppression removes targetability, which is derived from the applied
	// privacy projection; the relationship record is left intact so lawful
	// operational and commercial history survives.
	return nil
}

func (profilePrivacyHandler) IdentityMerge(context.Context, pgx.Tx, string, string, string) error {
	return nil
}

// consentPrivacyHandler owns the local consent projection.
type consentPrivacyHandler struct{ service *Service }

func (consentPrivacyHandler) Domain() string { return "consent" }

func (consentPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }

// Redact drops the local consent copy: with the relationship redacted there is
// nothing left it could lawfully authorise, and privacy already overrides
// consent for eligibility.
func (consentPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM crm_consent_projections WHERE business_id=$1 AND profile_id=$2`, businessID, profileID)
	return err
}

func (consentPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}

func (consentPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	// Consent belongs to the identity, so a decision does not transfer to the
	// survivor by merge: the authority re-asserts it against the surviving
	// subject. Dropping the source copy avoids a stale decision outliving the
	// subject it described.
	_, err := tx.Exec(ctx, `DELETE FROM crm_consent_projections WHERE business_id=$1 AND profile_id=$2`, businessID, sourceProfileID)
	return err
}

// commercialPrivacyHandler owns order facts and branch relations.
type commercialPrivacyHandler struct{ service *Service }

func (commercialPrivacyHandler) Domain() string { return "commercial_projection" }

func (commercialPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }

// Redact removes the branch relationship rows used for targeting and strips
// the fact rows down to structural commercial evidence.
//
// The fact rows are not deleted: they are CRM's own minimal projection of
// commercial events whose authoritative records live in the order and payment
// services, and which retention policy keeps. What is removed is the ability
// to use them for targeting — the branch relations that drive branch-scoped
// audiences.
func (commercialPrivacyHandler) Redact(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM crm_customer_branch_relations WHERE business_id=$1 AND profile_id=$2`,
		businessID, profileID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE crm_customer_order_facts SET capability_type=NULL, updated_at=now()
		WHERE business_id=$1 AND profile_id=$2`, businessID, profileID)
	return err
}

func (commercialPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}

// IdentityMerge moves fact and branch rows to the survivor. Facts carry a
// unique (business, source_service, source_order_ref) key, so an order both
// profiles observed collapses to one row instead of being counted twice.
func (commercialPrivacyHandler) IdentityMerge(ctx context.Context, tx pgx.Tx, businessID, sourceProfileID, targetProfileID string) error {
	if _, err := tx.Exec(ctx, `UPDATE crm_customer_order_facts f SET profile_id=$1, updated_at=now()
		WHERE business_id=$2 AND profile_id=$3
		  AND NOT EXISTS (SELECT 1 FROM crm_customer_order_facts t
			WHERE t.business_id=f.business_id AND t.source_service=f.source_service
			  AND t.source_order_ref=f.source_order_ref AND t.profile_id=$1)`,
		targetProfileID, businessID, sourceProfileID); err != nil {
		return err
	}
	// Anything left is a duplicate of an order the survivor already holds.
	if _, err := tx.Exec(ctx, `DELETE FROM crm_customer_order_facts WHERE business_id=$1 AND profile_id=$2`,
		businessID, sourceProfileID); err != nil {
		return err
	}
	// Branch relations are rebuilt from the merged fact set by the caller, so
	// the source rows are simply removed here.
	_, err := tx.Exec(ctx, `DELETE FROM crm_customer_branch_relations WHERE business_id=$1 AND profile_id=$2`,
		businessID, sourceProfileID)
	return err
}

// identityLinkPrivacyHandler owns the alias history.
type identityLinkPrivacyHandler struct{ service *Service }

func (identityLinkPrivacyHandler) Domain() string { return "identity_link" }

func (identityLinkPrivacyHandler) Restrict(context.Context, pgx.Tx, string, string) error { return nil }

// Redact keeps the link rows. They contain no personal data — only internal
// profile ids and authority references — and they are what stops a redacted or
// merged subject being resurrected by a late event.
func (identityLinkPrivacyHandler) Redact(context.Context, pgx.Tx, string, string) error { return nil }

func (identityLinkPrivacyHandler) MarketingSuppress(context.Context, pgx.Tx, string, string) error {
	return nil
}

func (identityLinkPrivacyHandler) IdentityMerge(context.Context, pgx.Tx, string, string, string) error {
	return nil
}
