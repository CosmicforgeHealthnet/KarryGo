package crm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RelationshipInput is an authoritative observation that a commercial
// relationship exists between a business and a subject.
type RelationshipInput struct {
	Subject               SubjectRef
	AcquisitionSourceType *string
	AcquisitionSourceRef  *string
	ObservedAt            time.Time
}

// resolveProfile finds the profile that currently represents a subject,
// following merge aliases, without creating anything.
func (s *Service) resolveProfile(ctx context.Context, tx pgx.Tx, actor Actor, subject SubjectRef) (*Profile, error) {
	found, err := s.findProfileBySubject(ctx, tx, actor.VendorID, subject)
	if err != nil || found == nil {
		return nil, err
	}
	if found.MergedIntoProfileID == nil {
		return found, nil
	}
	survivor, err := s.ResolveSurvivor(ctx, tx, actor.VendorID, *found)
	if err != nil {
		return nil, err
	}
	return &survivor, nil
}

// ResolveOrCreateProfile is the single entry point by which a relationship
// comes into existence.
//
// It is reachable only from an authenticated authoritative event. Before
// creating or touching anything it checks the subject's privacy history, so a
// late event cannot resurrect a redacted relationship or revive a profile that
// was merged away — the two anti-resurrection guarantees Section 8A rests on.
//
// Creation is idempotent under concurrency: the partial unique indexes on
// (business_id, subject reference) are the real guard, and a losing racer
// re-reads the winner's row rather than failing.
func (s *Service) ResolveOrCreateProfile(ctx context.Context, tx pgx.Tx, actor Actor, in RelationshipInput) (Profile, error) {
	if !IsValidSubjectType(string(in.Subject.Type)) || in.Subject.Ref == "" {
		return Profile{}, errSubjectInvalid()
	}

	existing, err := s.findProfileBySubject(ctx, tx, actor.VendorID, in.Subject)
	if err != nil {
		return Profile{}, err
	}
	if existing != nil {
		// A redacted relationship stays redacted. The event is accepted and
		// deduped by the caller, but nothing is recreated or reactivated.
		if existing.IsRedacted() {
			s.metrics.Inc("crm_anti_resurrection_blocks_total", "reason", "redacted_profile")
			return Profile{}, errAntiResurrectionBlocked()
		}
		if existing.MergedIntoProfileID != nil {
			// Route to the survivor instead of reviving the merged source.
			survivor, resolveErr := s.ResolveSurvivor(ctx, tx, actor.VendorID, *existing)
			if resolveErr != nil {
				return Profile{}, resolveErr
			}
			if survivor.IsRedacted() {
				s.metrics.Inc("crm_anti_resurrection_blocks_total", "reason", "redacted_survivor")
				return Profile{}, errAntiResurrectionBlocked()
			}
			return survivor, nil
		}
		return *existing, nil
	}

	// No relationship exists. Before creating one, honour any privacy
	// instruction already recorded against this subject — an erasure that
	// arrived before the late order event must still win.
	blocked, err := s.subjectRedactionRecorded(ctx, tx, actor.VendorID, in.Subject.Ref)
	if err != nil {
		return Profile{}, err
	}
	if blocked {
		s.metrics.Inc("crm_anti_resurrection_blocks_total", "reason", "privacy_instruction")
		return Profile{}, errAntiResurrectionBlocked()
	}

	created, err := s.insertProfile(ctx, tx, actor, in)
	if err == nil {
		// A new relationship can change every rule that reads intrinsic profile
		// traits. Use the same durable, transaction-local dependency invalidation
		// path as commercial, consent and tag changes: if the surrounding event
		// rolls back, both the profile and its queued evaluations roll back.
		if err = s.invalidateSegmentsForDependencies(ctx, tx, actor.VendorID, []string{
			"profile.subject_type",
			"profile.status",
			"profile.acquisition_source_type",
		}); err != nil {
			return Profile{}, err
		}
		s.metrics.Inc("crm_profiles_created_total", "subject_type", string(in.Subject.Type))
		return created, nil
	}
	// A concurrent event created the same relationship first; adopt it. The
	// INSERT uses ON CONFLICT DO NOTHING so the transaction remains usable for
	// this read instead of being left in PostgreSQL's aborted state.
	if errors.Is(err, pgx.ErrNoRows) {
		winner, lookupErr := s.findProfileBySubject(ctx, tx, actor.VendorID, in.Subject)
		if lookupErr != nil {
			return Profile{}, lookupErr
		}
		if winner == nil {
			return Profile{}, err
		}
		return *winner, nil
	}
	return Profile{}, err
}

func (s *Service) insertProfile(ctx context.Context, tx pgx.Tx, actor Actor, in RelationshipInput) (Profile, error) {
	var customerRef, guestRef, externalRef *string
	switch in.Subject.Type {
	case SubjectCustomer:
		customerRef = &in.Subject.Ref
	case SubjectGuest:
		guestRef = &in.Subject.Ref
	case SubjectExternal:
		externalRef = &in.Subject.Ref
	}
	if !ValidateSubjectReferences(in.Subject.Type, customerRef, guestRef, externalRef) {
		return Profile{}, errSubjectInvalid()
	}
	// safe_display_label is deliberately not set. No approved PII-free
	// presentation contract exists, and falling back to an email address or
	// phone number would turn CRM into a second customer master.
	return scanProfile(tx.QueryRow(ctx, `INSERT INTO vendor_customer_profiles
		(id,business_id,subject_type,customer_ref,guest_ref,external_ref,acquisition_source_type,acquisition_source_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING RETURNING `+profileCols,
		newID("vcp_"), actor.VendorID, in.Subject.Type, customerRef, guestRef, externalRef,
		in.AcquisitionSourceType, in.AcquisitionSourceRef))
}

// subjectRedactionRecorded reports whether an erasure instruction already
// exists for this subject reference.
//
// The subject reference is matched directly. That is the minimum retained
// identifier needed to make suppression work, and it is used for nothing else:
// not marketing, not presentation, not enrichment, not re-identification.
// Hashing it here would not make the subject anonymous — it would only make
// this check unreliable, which is precisely the failure this guards against.
func (s *Service) subjectRedactionRecorded(ctx context.Context, q querier, businessID, subjectRef string) (bool, error) {
	var blocked bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_privacy_projections
		WHERE business_id=$1 AND privacy_subject_ref=$2 AND action=$3)`,
		businessID, subjectRef, PrivacyActionRedact).Scan(&blocked)
	return blocked, err
}

func errSubjectInvalid() error { return errInvalid("Customer subject reference is not valid.") }

// ---------------------------------------------------------------------------
// Inbox claim with payload fingerprinting
// ---------------------------------------------------------------------------

// ClaimEvent records an inbound event exactly once.
//
// Redelivery of the same event id with the same body is a safe no-op. The same
// event id carrying a *different* body is an integrity conflict: two different
// facts cannot share one identity, and silently accepting either would let a
// caller rewrite history by reusing an id.
func (s *Service) ClaimEvent(ctx context.Context, tx pgx.Tx, sourceService, eventID, eventType string, payload any) (claimed bool, err error) {
	fingerprint := fingerprintOf(payload)
	tag, err := tx.Exec(ctx, `INSERT INTO vendor_inbox_processed_events
		(source_service,event_id,event_type,processed_at,payload_fingerprint)
		VALUES($1,$2,$3,now(),$4) ON CONFLICT (source_service,event_id) DO NOTHING`,
		sourceService, eventID, eventType, fingerprint)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}

	var existing *string
	if err = tx.QueryRow(ctx, `SELECT payload_fingerprint FROM vendor_inbox_processed_events
		WHERE source_service=$1 AND event_id=$2`, sourceService, eventID).Scan(&existing); err != nil {
		return false, err
	}
	// A pre-existing row without a fingerprint predates this column; treat it
	// as an ordinary duplicate rather than inventing a conflict.
	if existing != nil && *existing != fingerprint {
		s.metrics.Inc("crm_internal_event_rejections_total", "reason", "event_id_payload_conflict")
		return false, errSourceVersionConflict()
	}
	return false, nil
}

func fingerprintOf(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
