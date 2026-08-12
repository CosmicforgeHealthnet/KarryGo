package crm

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConsentInput is one authoritative consent decision to project.
type ConsentInput struct {
	SourceService string
	SourceRef     *string
	SourceVersion int64
	Purpose       ConsentPurpose
	Channel       ConsentChannel
	Decision      ConsentDecision
	ObservedAt    time.Time
	EffectiveAt   *time.Time
	ExpiresAt     *time.Time
}

// ApplyConsent projects one authoritative consent decision onto a profile.
//
// Vendor Service is not the consent authority: this only ever records what the
// authority said, and no vendor-facing route reaches it. Ordering is settled
// by source version, never by arrival time, so a redelivered or delayed event
// cannot roll a decision backwards.
func (s *Service) ApplyConsent(ctx context.Context, tx pgx.Tx, actor Actor, profileID string, in ConsentInput) error {
	var (
		existingID       string
		existingVersion  int64
		existingDecision ConsentDecision
	)
	err := tx.QueryRow(ctx, `SELECT id,source_version,decision FROM crm_consent_projections
		WHERE business_id=$1 AND profile_id=$2 AND purpose=$3 AND channel=$4 FOR UPDATE`,
		actor.VendorID, profileID, in.Purpose, in.Channel).Scan(&existingID, &existingVersion, &existingDecision)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(ctx, `INSERT INTO crm_consent_projections
			(id,business_id,profile_id,purpose,channel,decision,source_service,source_ref,source_version,observed_at,effective_at,expires_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			newID("crmc_"), actor.VendorID, profileID, in.Purpose, in.Channel, in.Decision,
			in.SourceService, in.SourceRef, in.SourceVersion, in.ObservedAt, in.EffectiveAt, in.ExpiresAt)
		if err != nil {
			return err
		}
		s.metrics.Inc("crm_consent_updates_total", "decision", string(in.Decision))
	case err != nil:
		return err
	case in.SourceVersion < existingVersion:
		// An older authority version can never overwrite a newer one.
		s.metrics.Inc("crm_consent_stale_updates_total")
		return errSourceVersionStale()
	case in.SourceVersion == existingVersion && existingDecision != in.Decision:
		// The same version asserting a different decision means one of the two
		// deliveries is wrong. Overwriting either way would silently pick a
		// winner, so this is refused and surfaced instead.
		s.metrics.Inc("crm_consent_conflicts_total")
		return errSourceVersionConflict()
	case in.SourceVersion == existingVersion:
		// Identical redelivery: a safe no-op.
		return nil
	default:
		_, err = tx.Exec(ctx, `UPDATE crm_consent_projections
			SET decision=$1,source_service=$2,source_ref=$3,source_version=$4,observed_at=$5,
				effective_at=$6,expires_at=$7,updated_at=now(),version=version+1
			WHERE id=$8`,
			in.Decision, in.SourceService, in.SourceRef, in.SourceVersion, in.ObservedAt,
			in.EffectiveAt, in.ExpiresAt, existingID)
		if err != nil {
			return err
		}
		s.metrics.Inc("crm_consent_updates_total", "decision", string(in.Decision))
	}

	// Track the freshest consent observation on the profile so Customer 360
	// can report staleness without joining every channel row.
	if _, err = tx.Exec(ctx, `UPDATE vendor_customer_profiles
		SET consent_projection_version=$1, consent_observed_at=$2, updated_at=now()
		WHERE business_id=$3 AND id=$4
		  AND (consent_projection_version IS NULL OR consent_projection_version <= $1)`,
		in.SourceVersion, in.ObservedAt, actor.VendorID, profileID); err != nil {
		return err
	}
	return s.auditTx(ctx, tx, actor, "vendor.crm.consent_projection.changed", "crm_consent_projection", profileID, "",
		nil, map[string]any{"purpose": in.Purpose, "channel": in.Channel, "decision": in.Decision, "source_version": in.SourceVersion}, "")
}

// ChannelConsent is the projected decision plus the eligibility answer for one
// channel.
type ChannelConsent struct {
	Channel    ConsentChannel  `json:"channel"`
	Decision   ConsentDecision `json:"decision"`
	Eligible   bool            `json:"eligible"`
	Reason     string          `json:"reason"`
	Stale      bool            `json:"stale"`
	ObservedAt *time.Time      `json:"observed_at,omitempty"`
}

// ConsentSummary is the Customer 360 consent block.
type ConsentSummary struct {
	// IntegrationAvailable is false when no consent authority is wired, which
	// is why every channel below reads UNKNOWN/ineligible rather than being
	// omitted: "we cannot know" is the answer, not "there is no answer".
	IntegrationAvailable bool                              `json:"integration_available"`
	Marketing            map[ConsentChannel]ChannelConsent `json:"marketing"`
}

// ConsentSummaryFor builds the eligibility view for one profile.
//
// Absence of a row is UNKNOWN, and UNKNOWN is ineligible. Every channel is
// always reported so a caller cannot mistake a missing key for permission.
func (s *Service) ConsentSummaryFor(ctx context.Context, actor Actor, profile Profile) (ConsentSummary, error) {
	suppressed, err := s.marketingSuppressed(ctx, actor.VendorID, profile.ID)
	if err != nil {
		return ConsentSummary{}, err
	}
	projections, err := s.consentProjections(ctx, actor.VendorID, profile.ID)
	if err != nil {
		return ConsentSummary{}, err
	}

	now := s.now()
	summary := ConsentSummary{
		IntegrationAvailable: s.flags.ConsentIntegrationEnabled,
		Marketing:            make(map[ConsentChannel]ChannelConsent, len(MarketingChannels)),
	}
	for _, channel := range MarketingChannels {
		entry := ChannelConsent{Channel: channel, Decision: ConsentUnknown}
		projection, found := projections[channel]
		if found {
			entry.Decision = projection.EffectiveDecision(now)
			observed := projection.ObservedAt
			entry.ObservedAt = &observed
			entry.Stale = now.Sub(projection.ObservedAt) > ConsentStaleAfter
		}
		entry.Eligible, entry.Reason = MarketingEligibility(MarketingEligibilityInput{
			PrivacyState:                profile.PrivacyState,
			MergeState:                  profile.MergeState,
			Status:                      profile.Status,
			MarketingSuppressed:         suppressed,
			Decision:                    entry.Decision,
			ConsentStale:                entry.Stale,
			ConsentIntegrationAvailable: s.flags.ConsentIntegrationEnabled,
		})
		summary.Marketing[channel] = entry
	}
	return summary, nil
}

func (s *Service) consentProjections(ctx context.Context, businessID, profileID string) (map[ConsentChannel]ConsentProjection, error) {
	rows, err := s.db.Query(ctx, `SELECT id,business_id,profile_id,purpose,channel,decision,source_service,
		source_ref,source_version,observed_at,effective_at,expires_at,version
		FROM crm_consent_projections WHERE business_id=$1 AND profile_id=$2 AND purpose=$3`,
		businessID, profileID, ConsentPurposeMarketing)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ConsentChannel]ConsentProjection{}
	for rows.Next() {
		var c ConsentProjection
		if err = rows.Scan(&c.ID, &c.BusinessID, &c.ProfileID, &c.Purpose, &c.Channel, &c.Decision,
			&c.SourceService, &c.SourceRef, &c.SourceVersion, &c.ObservedAt, &c.EffectiveAt, &c.ExpiresAt, &c.Version); err != nil {
			return nil, err
		}
		out[c.Channel] = c
	}
	return out, rows.Err()
}

// marketingSuppressed reports whether an applied MARKETING_SUPPRESS instruction
// covers this profile. It is derived from the privacy projection rather than
// cached on the profile so the two can never drift apart.
func (s *Service) marketingSuppressed(ctx context.Context, businessID, profileID string) (bool, error) {
	var suppressed bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_privacy_projections
		WHERE business_id=$1 AND profile_id=$2 AND action=$3 AND processing_status=$4)`,
		businessID, profileID, PrivacyActionMarketingSuppress, PrivacyApplied).Scan(&suppressed)
	return suppressed, err
}
