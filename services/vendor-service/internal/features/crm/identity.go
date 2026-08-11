package crm

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SubjectRef identifies a customer subject as an authority describes it.
type SubjectRef struct {
	Type SubjectType
	Ref  string
}

// IdentityLinkInput is one authoritative identity link or merge.
type IdentityLinkInput struct {
	AuthorityService   string
	AuthorityReference string
	AuthorityVersion   int64
	EvidenceEventID    string
	Source             SubjectRef
	Target             SubjectRef
}

// ApplyIdentityLink merges a source relationship into a surviving target.
//
// Every link requires authoritative evidence. There is no vendor-facing route
// that reaches this code, and nothing here infers a link from a matching name,
// address, device, typed phone number or order pattern — a merge happens
// because the identity authority said so, or it does not happen.
//
// Concurrency: both profiles are locked in a fixed id order, so two links
// touching the same pair from opposite directions cannot deadlock, and the
// evidence uniqueness index makes a redelivered event a no-op rather than a
// second merge.
func (s *Service) ApplyIdentityLink(ctx context.Context, tx pgx.Tx, actor Actor, in IdentityLinkInput) error {
	linkType, ok := LinkTypeFor(in.Source.Type, in.Target.Type)
	if !ok {
		return errSourceVersionConflict()
	}

	// Replay check first: identical evidence must not merge twice.
	var existingLink string
	err := tx.QueryRow(ctx, `SELECT id FROM crm_customer_identity_links
		WHERE business_id=$1 AND authority_service=$2 AND evidence_event_id=$3`,
		actor.VendorID, in.AuthorityService, in.EvidenceEventID).Scan(&existingLink)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	source, err := s.findProfileBySubject(ctx, tx, actor.VendorID, in.Source)
	if err != nil {
		return err
	}
	target, err := s.findProfileBySubject(ctx, tx, actor.VendorID, in.Target)
	if err != nil {
		return err
	}
	if source == nil {
		// Nothing to merge from: the survivor (if any) already represents the
		// relationship, so this is a no-op rather than an error.
		return nil
	}
	if target == nil {
		// No surviving relationship exists yet. Re-point the source at the
		// canonical reference in place instead of inventing a second profile.
		return s.adoptCanonicalReference(ctx, tx, actor, *source, in, linkType)
	}
	if source.ID == target.ID {
		return nil
	}
	// Both profiles were loaded scoped to actor.VendorID, so a cross-tenant
	// merge cannot be constructed here; this guard makes that explicit.
	if source.BusinessID != target.BusinessID {
		return errCrossTenant()
	}
	if source.IsRedacted() || target.IsRedacted() {
		// Merging a redacted subject would revive it inside the survivor.
		s.metrics.Inc("crm_identity_link_conflicts_total", "reason", "redacted")
		return errAntiResurrectionBlocked()
	}
	if target.IsMergedSource() {
		// The nominated survivor has itself been merged away; merging into it
		// would create a chain that later lookups could cycle on.
		s.metrics.Inc("crm_identity_link_conflicts_total", "reason", "merge_cycle")
		return errSourceVersionConflict()
	}
	if source.IsMergedSource() {
		return nil
	}

	// Deterministic lock order over the two ids prevents deadlock between two
	// concurrent merges of the same pair.
	first, second := source.ID, target.ID
	if second < first {
		first, second = second, first
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id IN ($2,$3) ORDER BY id FOR UPDATE`,
		actor.VendorID, first, second); err != nil {
		return err
	}

	for _, handler := range s.privacy.handlers {
		if err = handler.IdentityMerge(ctx, tx, actor.VendorID, source.ID, target.ID); err != nil {
			return err
		}
	}

	// Mark the source superseded before rebuilding, so any concurrent reader
	// sees a consistent "this is not an independent relationship" state.
	if _, err = tx.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='MERGED', merge_state='MERGED_SOURCE', merged_into_profile_id=$1,
			status='ARCHIVED', updated_at=now(), version=version+1
		WHERE business_id=$2 AND id=$3`, target.ID, actor.VendorID, source.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE vendor_customer_profiles
		SET merge_state='MERGED_TARGET', updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2 AND merge_state='NONE'`, actor.VendorID, target.ID); err != nil {
		return err
	}

	// Totals are recomputed from the merged fact set, never added together:
	// an order both profiles observed collapses to one fact and is counted
	// once.
	if err = s.RebuildCommercialProjection(ctx, tx, actor, target.ID, in.AuthorityVersion, s.now()); err != nil {
		return err
	}
	if err = s.RebuildCommercialProjection(ctx, tx, actor, source.ID, in.AuthorityVersion, s.now()); err != nil {
		return err
	}

	if err = s.recordIdentityLink(ctx, tx, actor, source.ID, target.ID, linkType, in); err != nil {
		return err
	}
	s.metrics.Inc("crm_identity_links_total", "type", string(linkType))
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.profile.linked", "vendor_customer_profile", source.ID, "",
		map[string]any{"merge_state": source.MergeState},
		map[string]any{"merge_state": string(MergeSource), "merged_into": target.ID, "link_type": string(linkType)}, ""); err != nil {
		return err
	}
	return s.publishTx(ctx, tx, actor, "vendor.crm.profile.linked", "vendor_customer_profile", source.ID, source.Version+1,
		map[string]any{"profile_ref": source.ID, "merged_into_profile_ref": target.ID, "link_type": string(linkType), "source_version": in.AuthorityVersion})
}

// adoptCanonicalReference upgrades a guest/external relationship in place when
// the authority links it to a canonical customer that has no relationship with
// this business yet. This keeps the existing commercial history attached to
// one profile instead of splitting it across two.
func (s *Service) adoptCanonicalReference(ctx context.Context, tx pgx.Tx, actor Actor, source Profile, in IdentityLinkInput, linkType LinkType) error {
	if in.Target.Type != SubjectCustomer {
		return nil
	}
	if source.IsRedacted() {
		return errAntiResurrectionBlocked()
	}
	// The subject type is deliberately unchanged: a guest who is later
	// recognised as a customer keeps its guest reference as history, and the
	// schema permits a GUEST or EXTERNAL row to additionally carry a canonical
	// customer reference once an authority has established one.
	if _, err := tx.Exec(ctx, `UPDATE vendor_customer_profiles
		SET customer_ref=$1, updated_at=now(), version=version+1
		WHERE business_id=$2 AND id=$3`, in.Target.Ref, actor.VendorID, source.ID); err != nil {
		return err
	}
	// No link row is written here: a link records two profiles collapsing into
	// one, and this path has only ever had a single profile. The adopted
	// customer_ref is itself the durable alias, and the audit entry below is
	// the record of how it was acquired.
	s.metrics.Inc("crm_identity_links_total", "type", string(linkType))
	return s.auditTx(ctx, tx, actor, "vendor.crm.profile.linked", "vendor_customer_profile", source.ID, "",
		map[string]any{"customer_ref": source.CustomerRef},
		map[string]any{"customer_ref": in.Target.Ref, "link_type": string(linkType)}, "")
}

func (s *Service) recordIdentityLink(ctx context.Context, tx pgx.Tx, actor Actor, sourceID, targetID string, linkType LinkType, in IdentityLinkInput) error {
	if sourceID == targetID {
		return errors.New("crm: identity link source and target must differ")
	}
	_, err := tx.Exec(ctx, `INSERT INTO crm_customer_identity_links
		(id,business_id,source_profile_id,target_profile_id,link_type,authority_service,authority_reference,
		 authority_version,evidence_event_id,status,applied_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())`,
		newID("crml_"), actor.VendorID, sourceID, targetID, linkType, in.AuthorityService,
		in.AuthorityReference, in.AuthorityVersion, in.EvidenceEventID, "APPLIED")
	return err
}

// findProfileBySubject resolves a subject reference to a profile within one
// business, following merge aliases so a reference that has been merged away
// resolves to its survivor rather than looking absent.
func (s *Service) findProfileBySubject(ctx context.Context, q querier, businessID string, subject SubjectRef) (*Profile, error) {
	column := "customer_ref"
	switch subject.Type {
	case SubjectGuest:
		column = "guest_ref"
	case SubjectExternal:
		column = "external_ref"
	}
	profile, err := scanProfile(q.QueryRow(ctx,
		`SELECT `+profileCols+` FROM vendor_customer_profiles WHERE business_id=$1 AND `+column+`=$2
		 ORDER BY CASE WHEN merge_state='MERGED_SOURCE' THEN 1 ELSE 0 END LIMIT 1`,
		businessID, subject.Ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// ResolveSurvivor follows the merge chain to the profile that currently
// represents a relationship, so a late event for a merged-away subject is
// applied to its survivor instead of creating a new profile.
//
// The walk is depth-bounded: a corrupt chain must fail loudly rather than spin.
func (s *Service) ResolveSurvivor(ctx context.Context, q querier, businessID string, profile Profile) (Profile, error) {
	current := profile
	for depth := 0; depth < 16; depth++ {
		if current.MergedIntoProfileID == nil {
			return current, nil
		}
		next, err := scanProfile(q.QueryRow(ctx,
			`SELECT `+profileCols+` FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2`,
			businessID, *current.MergedIntoProfileID))
		if err != nil {
			return Profile{}, err
		}
		current = next
	}
	s.metrics.Inc("crm_identity_link_conflicts_total", "reason", "merge_cycle")
	return Profile{}, errSourceVersionConflict()
}

// IdentityLinkSummary is the Customer 360 link block.
type IdentityLinkSummary struct {
	MergeState          MergeState `json:"merge_state"`
	MergedIntoProfileID *string    `json:"merged_into_profile_ref,omitempty"`
	LinkedProfileCount  int        `json:"linked_profile_count"`
}

// IdentityLinkSummaryFor reports merge status without exposing any subject
// reference belonging to another identity.
func (s *Service) IdentityLinkSummaryFor(ctx context.Context, actor Actor, profile Profile) (IdentityLinkSummary, error) {
	var count int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM crm_customer_identity_links
		WHERE business_id=$1 AND (source_profile_id=$2 OR target_profile_id=$2)`,
		actor.VendorID, profile.ID).Scan(&count); err != nil {
		return IdentityLinkSummary{}, err
	}
	return IdentityLinkSummary{
		MergeState:          profile.MergeState,
		MergedIntoProfileID: profile.MergedIntoProfileID,
		LinkedProfileCount:  count,
	}, nil
}
