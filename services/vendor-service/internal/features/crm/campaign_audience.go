package crm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// The audience engine turns a campaign definition into a frozen set of
// relationships, and does so through exactly one door: the Section 8B segment
// engine. There is no second rule language here, no raw SQL parameter and no
// customer-list upload. A campaign can only ask for an audience that a segment
// already describes, which means every branch, privacy and consent wall Section
// 8B built applies here without being rebuilt — and, more importantly, without
// being subtly rebuilt differently.

// CampaignEvaluationConfig bounds the preparation worker's appetite for work.
type CampaignEvaluationConfig struct {
	// BatchSize is how many candidate relationships one pass reads and writes.
	BatchSize int
	// PreviewSampleDefault and PreviewSampleMax bound the preview sample.
	PreviewSampleDefault int
	PreviewSampleMax     int
	// LeaseTTL is how long a worker owns a claimed preparation.
	LeaseTTL time.Duration
	// MaxAttempts before a preparation is dead-lettered.
	MaxAttempts int
	// RetryBackoff is the base delay between attempts.
	RetryBackoff time.Duration
	// HandoffReservation is how long a delivery is reserved while its
	// notification handoff is in flight. The remote call happens outside any
	// database transaction, so this window is what stops a second worker
	// adopting the same delivery meanwhile; it must comfortably exceed the
	// Notification client timeout. A process that dies mid-call simply lets
	// the reservation lapse, and the retry reuses the same idempotency key.
	HandoffReservation time.Duration
}

// DefaultCampaignEvaluationConfig returns the initial tuning.
func DefaultCampaignEvaluationConfig() CampaignEvaluationConfig {
	return CampaignEvaluationConfig{
		BatchSize:            500,
		PreviewSampleDefault: 25,
		PreviewSampleMax:     100,
		LeaseTTL:             2 * time.Minute,
		HandoffReservation:   2 * time.Minute,
		MaxAttempts:          5,
		RetryBackoff:         30 * time.Second,
	}
}

// ---------------------------------------------------------------------------
// Candidate classification
// ---------------------------------------------------------------------------

// candidateOutcome is what the funnel decided about one relationship. Exactly
// one of these is true for every candidate, which is what makes the snapshot
// counts reconcile rather than merely add up by coincidence.
const (
	outcomeTarget          = "TARGET"
	outcomeExcluded        = "EXCLUDED"
	outcomeSuppressed      = "SUPPRESSED"
	outcomeServiceRecovery = "SERVICE_RECOVERY"
)

// campaignCandidate is one relationship read out of the source audience.
type campaignCandidate struct {
	ProfileID    string
	DisplayLabel *string
	PrivacyState PrivacyState
	MergeState   MergeState
	Status       ProfileStatus

	ExcludedBySegment   bool
	MarketingSuppressed bool
	ServiceRecovery     bool
}

// classify decides this candidate's place in the funnel.
//
// Structural suppression is decided by asking MarketingEligibility what it
// would say about this relationship if consent were a perfect ALLOW. Anything
// it still refuses — redaction, merge, marketing suppression, restriction,
// archival — is a reason that has nothing to do with consent, and reusing the
// one eligibility rule for the question means this funnel cannot drift away
// from the rule Customer 360 and Section 8B already answer with.
func (c campaignCandidate) classify(serviceRecoveryEnabled bool) (outcome, reason string) {
	if c.ExcludedBySegment {
		return outcomeExcluded, ReasonExcludedBySegment
	}
	eligible, structuralReason := MarketingEligibility(MarketingEligibilityInput{
		PrivacyState:                c.PrivacyState,
		MergeState:                  c.MergeState,
		Status:                      c.Status,
		MarketingSuppressed:         c.MarketingSuppressed,
		Decision:                    ConsentAllow,
		ConsentStale:                false,
		ConsentIntegrationAvailable: true,
	})
	if !eligible {
		return outcomeSuppressed, structuralReason
	}
	if serviceRecoveryEnabled && c.ServiceRecovery {
		return outcomeServiceRecovery, ReasonServiceRecovery
	}
	return outcomeTarget, ReasonSourceMember
}

// serviceRecoverySQL is the Vendor-local service-recovery signal.
//
// A customer is in service recovery when an OPEN or IN_PROGRESS CRM task is
// linked to a SERVICE_FOLLOW_UP interaction. Both halves matter: an ordinary
// task ("call about the catering quote") is not a complaint, and a
// service-follow-up interaction that was already resolved is not an open
// problem. Anything broader would suppress most of a busy vendor's audience for
// no reason, and anything narrower would market to somebody mid-complaint.
//
// Formal disputes remain owned by support-dispute-service. This service does
// not read them and does not pretend to: see EXTERNAL_SERVICE_RECOVERY_SUPPORT
// in the Section 8E documentation.
const serviceRecoverySQL = `EXISTS (SELECT 1 FROM crm_tasks rt
	JOIN crm_interactions ri ON ri.id = rt.source_interaction_id AND ri.business_id = rt.business_id
	WHERE rt.business_id = p.business_id AND rt.customer_profile_id = p.id
	  AND rt.status IN ('OPEN','IN_PROGRESS') AND rt.display_suppressed = false
	  AND ri.interaction_type = 'SERVICE_FOLLOW_UP' AND ri.display_suppressed = false)`

// audienceQuery is the compiled candidate scan for one campaign definition.
type audienceQuery struct {
	sql  string
	args []any
	// cursorIndex is the placeholder position of the keyset cursor, rebound on
	// each batch.
	cursorIndex int
}

// buildAudienceQuery renders the candidate scan.
//
// The source is a materialized segment membership generation, never a rule:
// binding to the exact generation is what stops two segment rebuilds being
// mixed into one audience. The branch filter is applied inside the query rather
// than in Go, because filtering afterwards would still have counted the rows it
// discarded, and a count is itself the disclosure.
func (s *Service) buildAudienceQuery(businessID string, version CampaignVersion, generation int64, scope Scope, limit int) audienceQuery {
	args := []any{businessID, version.SourceSegmentID, generation}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}

	exclusion := "false"
	if len(version.ExclusionSegmentIDs) > 0 {
		// Static exclusions read their explicit generation-0 membership. Dynamic
		// exclusions read only the exact CURRENT definition/evaluation generation.
		// A historical generation is evidence of an old classification, not a
		// permanent exclusion decision.
		exclusion = `EXISTS (SELECT 1 FROM crm_segment_memberships x
			JOIN crm_segments xs ON xs.id=x.segment_id AND xs.business_id=x.business_id
			LEFT JOIN crm_segment_evaluations xe ON xe.id=xs.current_evaluation_id
				AND xe.business_id=xs.business_id AND xe.segment_id=xs.id
			WHERE x.business_id = p.business_id AND x.profile_id = p.id
			  AND x.segment_id = ANY(` + bind(version.ExclusionSegmentIDs) + `)
			  AND ((xs.segment_type='STATIC' AND x.membership_source='STATIC' AND x.generation=0)
			    OR (xs.segment_type='DYNAMIC' AND xs.evaluation_status='CURRENT'
			      AND xe.status='SUCCESS' AND xe.segment_definition_version=xs.current_definition_version
			      AND x.membership_source='DYNAMIC' AND x.generation=xe.generation
			      AND x.segment_definition_version=xs.current_definition_version)))`
	}

	branchFilter := ""
	if !scope.Global {
		branchFilter = ` AND EXISTS (SELECT 1 FROM crm_customer_branch_relations br
			WHERE br.business_id = p.business_id AND br.profile_id = p.id AND br.branch_id = ANY(` + bind(scope.BranchIDs) + `))`
	}

	cursorIndex := len(args) + 1
	query := `SELECT p.id, p.safe_display_label, p.privacy_state, p.merge_state, p.status,
			` + exclusion + ` AS excluded,
			EXISTS (SELECT 1 FROM crm_privacy_projections pp
				WHERE pp.business_id = p.business_id AND pp.profile_id = p.id
				  AND pp.action = 'MARKETING_SUPPRESS' AND pp.processing_status = 'APPLIED') AS marketing_suppressed,
			` + serviceRecoverySQL + ` AS service_recovery
		FROM crm_segment_memberships m
		JOIN vendor_customer_profiles p ON p.id = m.profile_id AND p.business_id = m.business_id
		WHERE m.business_id = $1 AND m.segment_id = $2 AND m.generation = $3` + branchFilter + `
		  AND p.id > $` + itoa(cursorIndex) + `::text
		ORDER BY p.id ASC LIMIT ` + itoa(limit)

	return audienceQuery{sql: query, args: args, cursorIndex: cursorIndex}
}

func (q audienceQuery) batchArgs(cursor string) []any {
	return append(append([]any{}, q.args...), cursor)
}

func (s *Service) scanCandidates(ctx context.Context, q querier, query audienceQuery, cursor string) ([]campaignCandidate, error) {
	rows, err := q.Query(ctx, query.sql, query.batchArgs(cursor)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []campaignCandidate
	for rows.Next() {
		var candidate campaignCandidate
		if err = rows.Scan(&candidate.ProfileID, &candidate.DisplayLabel, &candidate.PrivacyState,
			&candidate.MergeState, &candidate.Status, &candidate.ExcludedBySegment,
			&candidate.MarketingSuppressed, &candidate.ServiceRecovery); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Snapshot-time consent
// ---------------------------------------------------------------------------

// channelDecision is one relationship's consent evidence for one channel.
type channelDecision struct {
	Channel       string
	Decision      ConsentDecision
	Eligible      bool
	Reason        string
	ObservedAt    *time.Time
	SourceVersion *int64
	State         string
}

// evaluateChannels records what the consent authority's projection said about
// this relationship, for each channel the campaign intends to use.
//
// Every branch that is not an explicit, fresh, unexpired ALLOW produces an
// ineligible channel. A missing projection is UNKNOWN and UNKNOWN is not
// permission; a lapsed ALLOW is UNKNOWN; a projection older than policy is
// stale and stale is not permission; and with no consent authority wired at all
// every channel is ineligible rather than optimistically open. The rule itself
// is MarketingEligibility, so consent means the same thing here as it does
// everywhere else in the CRM.
func (s *Service) evaluateChannels(candidate campaignCandidate, channels []string,
	projections map[ConsentChannel]ConsentProjection, now time.Time) []channelDecision {
	out := make([]channelDecision, 0, len(channels))
	for _, channel := range channels {
		entry := channelDecision{Channel: channel, Decision: ConsentUnknown}
		projection, found := projections[ConsentChannel(channel)]
		stale := false
		if found {
			entry.Decision = projection.EffectiveDecision(now)
			observed := projection.ObservedAt
			entry.ObservedAt = &observed
			sourceVersion := projection.SourceVersion
			entry.SourceVersion = &sourceVersion
			stale = now.Sub(projection.ObservedAt) > ConsentStaleAfter
		}
		entry.Eligible, entry.Reason = MarketingEligibility(MarketingEligibilityInput{
			PrivacyState:                candidate.PrivacyState,
			MergeState:                  candidate.MergeState,
			Status:                      candidate.Status,
			MarketingSuppressed:         candidate.MarketingSuppressed,
			Decision:                    entry.Decision,
			ConsentStale:                stale,
			ConsentIntegrationAvailable: s.flags.ConsentIntegrationEnabled,
		})
		entry.State = ChannelIneligibleConsent
		if entry.Eligible {
			entry.State = ChannelEligibleAtSnapshot
		}
		// The stored decision and the stored eligibility must agree, and a
		// database check enforces that they do. An eligible channel therefore
		// always carries ALLOW; anything else is normalised to the decision the
		// evidence actually supports.
		if !entry.Eligible && entry.Decision == ConsentAllow {
			entry.Decision = ConsentUnknown
		}
		out = append(out, entry)
	}
	return out
}

func (s *Service) consentForProfiles(ctx context.Context, q querier, businessID string, profileIDs []string) (map[string]map[ConsentChannel]ConsentProjection, error) {
	out := map[string]map[ConsentChannel]ConsentProjection{}
	if len(profileIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT id,business_id,profile_id,purpose,channel,decision,source_service,
		source_ref,source_version,observed_at,effective_at,expires_at,version
		FROM crm_consent_projections WHERE business_id=$1 AND purpose=$2 AND profile_id = ANY($3)`,
		businessID, ConsentPurposeMarketing, profileIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c ConsentProjection
		if err = rows.Scan(&c.ID, &c.BusinessID, &c.ProfileID, &c.Purpose, &c.Channel, &c.Decision,
			&c.SourceService, &c.SourceRef, &c.SourceVersion, &c.ObservedAt, &c.EffectiveAt,
			&c.ExpiresAt, &c.Version); err != nil {
			return nil, err
		}
		if out[c.ProfileID] == nil {
			out[c.ProfileID] = map[ConsentChannel]ConsentProjection{}
		}
		out[c.ProfileID][c.Channel] = c
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Source resolution
// ---------------------------------------------------------------------------

// resolvedSource is the exact audience input a preparation is bound to.
type resolvedSource struct {
	Segment           Segment
	DefinitionVersion int64
	// Generation is 0 for a STATIC segment, whose membership has no evaluation
	// run, and the exact materialized generation for a DYNAMIC one.
	Generation   int64
	EvaluationID *string
	Stale        bool
	StaleReason  string
}

// resolveAudienceSource loads the source segment and pins the exact membership
// it will be read from.
//
// A dynamic segment must be CURRENT. A STALE or FAILED audience is refused
// outright rather than quietly used: its member list is, by the segment
// engine's own admission, no longer what the definition describes, and a
// campaign prepared from it would carry that lie forward into a frozen snapshot
// that looks authoritative.
func (s *Service) resolveAudienceSource(ctx context.Context, q querier, actor Actor, scope Scope,
	version CampaignVersion, requireCurrent bool) (resolvedSource, error) {
	var out resolvedSource
	segment, err := s.loadSegment(ctx, q, actor.VendorID, version.SourceSegmentID)
	if err != nil {
		if isCRMCode(err, CodeSegmentNotFound) {
			return out, errCampaignAudienceMissing()
		}
		return out, err
	}
	if err = s.assertSegmentVisible(scope, segment); err != nil {
		return out, errCampaignAudienceMissing()
	}
	if segment.Status == SegmentArchived {
		return out, crmError(apperrors.Conflict("That audience segment is archived.", nil), CodeCampaignAudienceScope)
	}
	out.Segment = segment
	out.DefinitionVersion = segment.CurrentDefinitionVersion

	if !segment.Dynamic() {
		// Static membership lives at generation 0 and is maintained
		// transactionally, so it is always exactly what the vendor last said.
		return out, nil
	}
	if segment.CurrentEvaluationID == nil || segment.EvaluationStatus != EvalCurrent {
		out.Stale = true
		out.StaleReason = segment.EvaluationStatus
		if requireCurrent {
			return out, errCampaignAudienceNotCurrent(segment.EvaluationStatus)
		}
		return out, nil
	}
	var generation int64
	if err = q.QueryRow(ctx, `SELECT generation FROM crm_segment_evaluations WHERE business_id=$1 AND id=$2`,
		actor.VendorID, *segment.CurrentEvaluationID).Scan(&generation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, errCampaignAudienceNotCurrent(EvalNotEvaluated)
		}
		return out, err
	}
	out.Generation = generation
	out.EvaluationID = segment.CurrentEvaluationID
	return out, nil
}

func errCampaignAudienceNotCurrent(state string) error {
	err := apperrors.Conflict("This campaign's audience segment has no current membership. Rebuild the segment and try again.", nil)
	err.Details = map[string]any{"code": CodeCampaignAudienceStale, "segment_evaluation_status": state}
	return err
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

// CampaignPreviewInput is a request to evaluate a campaign without freezing it.
type CampaignPreviewInput struct {
	SampleLimit int
	// HypotheticalSendAt lets a caller ask "would quiet hours block a send at
	// this instant". It schedules nothing and is never persisted.
	HypotheticalSendAt *time.Time
}

// CampaignPreviewSample is one privacy-safe example row.
type CampaignPreviewSample struct {
	ProfileRef        string  `json:"profile_ref"`
	DisplayLabel      *string `json:"display_label"`
	ReasonCode        string  `json:"reason_code"`
	ExperimentVariant string  `json:"experiment_variant_key,omitempty"`
	IsControl         bool    `json:"is_control"`
}

// CampaignChannelProjection is the per-channel eligibility answer.
type CampaignChannelProjection struct {
	Channel                 string         `json:"channel"`
	EligibleCount           int            `json:"consent_eligible_count"`
	IneligibleCount         int            `json:"consent_ineligible_count"`
	IneligibleReasons       map[string]int `json:"consent_ineligible_reasons"`
	SendTimeRecheckRequired bool           `json:"send_time_recheck_required"`
}

// CampaignFrequencyProjection reports what can honestly be said about the
// frequency policy today.
type CampaignFrequencyProjection struct {
	Enabled         bool   `json:"enabled"`
	Cap             int    `json:"cap,omitempty"`
	WindowMinutes   int    `json:"window_minutes,omitempty"`
	EvaluationState string `json:"evaluation_state"`
	RecheckRequired bool   `json:"send_time_recheck_required"`
}

// CampaignQuietHoursProjection reports the stored window and, only when a
// caller supplied an explicit instant, whether it would fall inside it.
type CampaignQuietHoursProjection struct {
	Enabled            bool       `json:"enabled"`
	Start              string     `json:"start,omitempty"`
	End                string     `json:"end,omitempty"`
	Timezone           string     `json:"timezone,omitempty"`
	CrossesMidnight    bool       `json:"crosses_midnight"`
	HypotheticalSendAt *time.Time `json:"hypothetical_send_at,omitempty"`
	WouldBlock         *bool      `json:"would_block,omitempty"`
	RecheckRequired    bool       `json:"send_time_recheck_required"`
}

// CampaignPreviewResult is the answer to "who would this campaign reach".
type CampaignPreviewResult struct {
	CampaignRef       string    `json:"campaign_ref"`
	DefinitionVersion int64     `json:"definition_version"`
	DefinitionHash    string    `json:"definition_hash"`
	EvaluationAsOf    time.Time `json:"evaluation_as_of"`

	ScopeType string  `json:"scope_type"`
	BranchRef *string `json:"branch_ref,omitempty"`

	SourceSegmentRef               string   `json:"source_segment_ref"`
	SourceSegmentState             string   `json:"source_segment_evaluation_status"`
	SourceSegmentDefinitionVersion int64    `json:"source_segment_definition_version"`
	SourceSegmentGeneration        *int64   `json:"source_segment_generation,omitempty"`
	ExclusionSegmentRefs           []string `json:"exclusion_segment_refs"`

	CountState                     string `json:"count_state"`
	SourceCount                    int    `json:"source_count"`
	SegmentExclusionCount          int    `json:"segment_exclusion_count"`
	PrivacySuppressedCount         int    `json:"privacy_suppressed_count"`
	ServiceRecoverySuppressedCount int    `json:"service_recovery_suppressed_count"`
	ConsentSuppressedCount         int    `json:"consent_suppressed_count"`
	TargetCount                    int    `json:"target_count"`
	ControlCount                   int    `json:"control_count"`
	TreatmentCount                 int    `json:"treatment_count"`

	Channels        []CampaignChannelProjection  `json:"channels"`
	VariantCounts   map[string]int               `json:"experiment_variant_counts"`
	Frequency       CampaignFrequencyProjection  `json:"frequency_policy"`
	QuietHours      CampaignQuietHoursProjection `json:"quiet_hours_policy"`
	ServiceRecovery bool                         `json:"service_recovery_suppression_enabled"`

	Sample   []CampaignPreviewSample `json:"sample"`
	Warnings []string                `json:"warnings"`
}

// PreviewCampaign evaluates a campaign's current audience without persisting
// anything.
//
// Preview writes no target row and creates no snapshot. It is an answer about
// right now, and it says so: the source segment's freshness travels with the
// counts, and frequency and quiet hours are reported as policies that will be
// re-evaluated at send time rather than as questions already settled.
func (s *Service) PreviewCampaign(ctx context.Context, actor Actor, scope Scope, campaignID string, in CampaignPreviewInput) (CampaignPreviewResult, error) {
	if err := s.assertCampaignsAvailable(); err != nil {
		return CampaignPreviewResult{}, err
	}
	campaign, err := s.GetCampaign(ctx, actor, scope, campaignID)
	if err != nil {
		return CampaignPreviewResult{}, err
	}
	if err = authorizeObjectScope(scope, campaign.ScopeType, campaign.BranchID, CodeCampaignScopeDenied); err != nil {
		return CampaignPreviewResult{}, err
	}
	version, err := s.CampaignVersionAt(ctx, s.db, actor.VendorID, campaign.ID, campaign.CurrentDefinitionVersion)
	if err != nil {
		return CampaignPreviewResult{}, err
	}
	source, err := s.resolveAudienceSource(ctx, s.db, actor, scope, version, false)
	if err != nil {
		return CampaignPreviewResult{}, err
	}

	sampleLimit := in.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = s.campaignEval.PreviewSampleDefault
	}
	if sampleLimit > s.campaignEval.PreviewSampleMax {
		sampleLimit = s.campaignEval.PreviewSampleMax
	}

	now := s.now()
	started := time.Now()
	result := CampaignPreviewResult{
		CampaignRef: campaign.ID, DefinitionVersion: version.DefinitionVersion,
		DefinitionHash: version.DefinitionHash, EvaluationAsOf: now,
		ScopeType: campaign.ScopeType, BranchRef: campaign.BranchID,
		SourceSegmentRef: version.SourceSegmentID, SourceSegmentState: source.Segment.EvaluationStatus,
		SourceSegmentDefinitionVersion: source.DefinitionVersion,
		ExclusionSegmentRefs:           version.ExclusionSegmentIDs,
		CountState:                     CountExact,
		ServiceRecovery:                version.ServiceRecoverySuppression,
		VariantCounts:                  map[string]int{},
		Sample:                         []CampaignPreviewSample{},
		Warnings:                       []string{},
	}
	if result.ExclusionSegmentRefs == nil {
		result.ExclusionSegmentRefs = []string{}
	}
	if source.Segment.Dynamic() && source.EvaluationID != nil {
		generation := source.Generation
		result.SourceSegmentGeneration = &generation
	}

	allocator := newExperimentAllocator(version)
	channelStats := map[string]*CampaignChannelProjection{}
	for _, channel := range version.Channels {
		channelStats[channel] = &CampaignChannelProjection{
			Channel: channel, IneligibleReasons: map[string]int{}, SendTimeRecheckRequired: true,
		}
	}

	// A stale or unevaluated dynamic segment has no membership to read, so the
	// honest preview is zero with a warning rather than an invented figure.
	if !source.Stale {
		query := s.buildAudienceQuery(actor.VendorID, version, source.Generation, scope, s.campaignEval.BatchSize)
		cursor := ""
		for {
			candidates, scanErr := s.scanCandidates(ctx, s.db, query, cursor)
			if scanErr != nil {
				return CampaignPreviewResult{}, scanErr
			}
			if len(candidates) == 0 {
				break
			}
			targets := make([]campaignCandidate, 0, len(candidates))
			for _, candidate := range candidates {
				cursor = candidate.ProfileID
				result.SourceCount++
				outcome, reason := candidate.classify(version.ServiceRecoverySuppression)
				switch outcome {
				case outcomeExcluded:
					result.SegmentExclusionCount++
				case outcomeSuppressed:
					result.PrivacySuppressedCount++
				case outcomeServiceRecovery:
					result.ServiceRecoverySuppressedCount++
				default:
					targets = append(targets, candidate)
				}
				if len(result.Sample) < sampleLimit && outcome != outcomeTarget {
					result.Sample = append(result.Sample, CampaignPreviewSample{
						ProfileRef: candidate.ProfileID, DisplayLabel: candidate.DisplayLabel, ReasonCode: reason,
					})
				}
			}

			profileIDs := make([]string, 0, len(targets))
			for _, target := range targets {
				profileIDs = append(profileIDs, target.ProfileID)
			}
			consent, consentErr := s.consentForProfiles(ctx, s.db, actor.VendorID, profileIDs)
			if consentErr != nil {
				return CampaignPreviewResult{}, consentErr
			}
			for _, target := range targets {
				result.TargetCount++
				assignment := allocator.Assign(target.ProfileID)
				if assignment.VariantKey != "" {
					result.VariantCounts[assignment.VariantKey]++
				}
				if assignment.IsControl {
					result.ControlCount++
				} else {
					result.TreatmentCount++
				}
				anyEligible := false
				for _, decision := range s.evaluateChannels(target, version.Channels, consent[target.ProfileID], now) {
					stats := channelStats[decision.Channel]
					if decision.Eligible {
						stats.EligibleCount++
						anyEligible = true
						continue
					}
					stats.IneligibleCount++
					stats.IneligibleReasons[decision.Reason]++
				}
				if !anyEligible {
					result.ConsentSuppressedCount++
				}
				if len(result.Sample) < sampleLimit {
					reason := ReasonTreatmentVariant
					if assignment.IsControl {
						reason = ReasonControlGroup
					}
					if !allocator.enabled {
						reason = ReasonSourceMember
					}
					result.Sample = append(result.Sample, CampaignPreviewSample{
						ProfileRef: target.ProfileID, DisplayLabel: target.DisplayLabel, ReasonCode: reason,
						ExperimentVariant: assignment.VariantKey, IsControl: assignment.IsControl,
					})
				}
			}
			if len(candidates) < s.campaignEval.BatchSize {
				break
			}
		}
	}

	for _, channel := range version.Channels {
		result.Channels = append(result.Channels, *channelStats[channel])
	}
	result.Frequency = frequencyProjection(version.Frequency)
	result.QuietHours, err = quietHoursProjection(version.QuietHours, in.HypotheticalSendAt)
	if err != nil {
		return CampaignPreviewResult{}, err
	}

	if source.Stale {
		result.Warnings = append(result.Warnings,
			"The source segment has no current membership, so this preview counts nobody. Rebuild the segment first.")
	}
	if !s.flags.ConsentIntegrationEnabled {
		result.Warnings = append(result.Warnings,
			"No consent authority is integrated, so no channel is eligible. Nobody can be contacted until consent is available.")
	}
	if version.Frequency.Enabled {
		result.Warnings = append(result.Warnings,
			"Frequency capping cannot be evaluated here: no campaign has been delivered yet, so there is no contact history. It will be enforced at send time.")
	}
	if version.QuietHours.Enabled {
		result.Warnings = append(result.Warnings,
			"Quiet hours are stored but not applied here. They will be enforced at send time.")
	}
	s.metrics.Inc("crm_campaign_preview_total", "scope", campaign.ScopeType)
	s.metrics.Add("crm_campaign_preview_duration_seconds", time.Since(started).Seconds())
	return result, nil
}

func frequencyProjection(policy FrequencyPolicy) CampaignFrequencyProjection {
	projection := CampaignFrequencyProjection{
		Enabled: policy.Enabled, Cap: policy.Cap, WindowMinutes: policy.WindowMinutes,
		// NOT_AVAILABLE, never "eligible". Vendor Service holds no delivery
		// history because nothing has ever been delivered, and a cap evaluated
		// against an empty history would always pass — the most misleading
		// possible answer.
		EvaluationState: string(ProjectionNotAvailable), RecheckRequired: true,
	}
	return projection
}

func quietHoursProjection(policy QuietHoursPolicy, at *time.Time) (CampaignQuietHoursProjection, error) {
	projection := CampaignQuietHoursProjection{Enabled: policy.Enabled, RecheckRequired: true}
	if !policy.Enabled {
		return projection, nil
	}
	projection.Start = formatMinuteOfDay(policy.StartMinute)
	projection.End = formatMinuteOfDay(policy.EndMinute)
	projection.Timezone = policy.Timezone
	projection.CrossesMidnight = policy.StartMinute > policy.EndMinute
	if at == nil {
		return projection, nil
	}
	blocked, err := policy.Blocks(*at)
	if err != nil {
		return projection, err
	}
	instant := at.UTC()
	projection.HypotheticalSendAt = &instant
	projection.WouldBlock = &blocked
	return projection, nil
}

// ---------------------------------------------------------------------------
// Preparation
// ---------------------------------------------------------------------------

// CampaignSnapshot is one immutable frozen audience and its own durable job.
type CampaignSnapshot struct {
	ID                        string
	BusinessID                string
	CampaignID                string
	CampaignVersionID         string
	CampaignDefinitionVersion int64

	SourceSegmentID                string
	SourceSegmentDefinitionVersion int64
	SourceSegmentGeneration        *int64
	SourceSegmentEvaluationID      *string

	DefinitionHash string
	AudienceHash   *string
	EvaluationAsOf time.Time
	Status         string

	SourceCount                    int
	SegmentExclusionCount          int
	PrivacySuppressedCount         int
	ServiceRecoverySuppressedCount int
	ConsentSuppressedCount         int
	TargetCount                    int
	ControlCount                   int
	TreatmentCount                 int

	AttemptCount   int
	DeadLettered   bool
	ErrorCode      *string
	RequestedByRef *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
	Version        int64
}

const snapshotCols = `id,business_id,campaign_id,campaign_version_id,campaign_definition_version,
source_segment_id,source_segment_definition_version,source_segment_generation,source_segment_evaluation_id,
definition_hash,audience_hash,evaluation_as_of,status,source_count,segment_exclusion_count,
privacy_suppressed_count,service_recovery_suppressed_count,consent_suppressed_count,target_count,
control_count,treatment_count,attempt_count,dead_lettered,error_code,requested_by_staff_ref,
created_at,updated_at,completed_at,version`

func scanSnapshot(row rowScanner) (CampaignSnapshot, error) {
	var s CampaignSnapshot
	err := row.Scan(&s.ID, &s.BusinessID, &s.CampaignID, &s.CampaignVersionID, &s.CampaignDefinitionVersion,
		&s.SourceSegmentID, &s.SourceSegmentDefinitionVersion, &s.SourceSegmentGeneration, &s.SourceSegmentEvaluationID,
		&s.DefinitionHash, &s.AudienceHash, &s.EvaluationAsOf, &s.Status, &s.SourceCount, &s.SegmentExclusionCount,
		&s.PrivacySuppressedCount, &s.ServiceRecoverySuppressedCount, &s.ConsentSuppressedCount, &s.TargetCount,
		&s.ControlCount, &s.TreatmentCount, &s.AttemptCount, &s.DeadLettered, &s.ErrorCode, &s.RequestedByRef,
		&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.Version)
	return s, err
}

// PrepareCampaign enqueues a durable audience preparation.
//
// Preparation is asynchronous and the snapshot row is its own job record, the
// same shape Section 8B's materializer uses. Requesting a preparation that is
// already in flight returns the run in flight rather than stacking a second
// one — the partial unique index makes that true even when two requests race
// through different processes, so the loser reads back the winner's run instead
// of preparing the same audience twice.
func (s *Service) PrepareCampaign(ctx context.Context, actor Actor, scope Scope, campaignID string) (CampaignSnapshot, error) {
	if err := s.assertCampaignWrites(); err != nil {
		return CampaignSnapshot{}, err
	}
	if !s.flags.CampaignPreparationEnabled {
		return CampaignSnapshot{}, crmError(
			apperrors.Unavailable("Campaign audience preparation is temporarily disabled.", nil), CodeCampaignsDisabled)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CampaignSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	campaign, err := s.loadCampaignForUpdate(ctx, tx, actor.VendorID, campaignID)
	if err != nil {
		return CampaignSnapshot{}, err
	}
	if err = s.assertCampaignVisible(scope, campaign); err != nil {
		return CampaignSnapshot{}, err
	}
	if err = authorizeObjectScope(scope, campaign.ScopeType, campaign.BranchID, CodeCampaignScopeDenied); err != nil {
		return CampaignSnapshot{}, err
	}
	if campaign.Status == CampaignArchived {
		return CampaignSnapshot{}, errCampaignState("An archived campaign cannot be prepared.")
	}

	// A run already in flight is the answer, not an error: a user pressing the
	// button twice should see the same preparation both times.
	existing, err := scanSnapshot(tx.QueryRow(ctx, `SELECT `+snapshotCols+` FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2 AND status='BUILDING'`, actor.VendorID, campaignID))
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return CampaignSnapshot{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CampaignSnapshot{}, err
	}

	version, err := s.CampaignVersionAt(ctx, tx, actor.VendorID, campaign.ID, campaign.CurrentDefinitionVersion)
	if err != nil {
		return CampaignSnapshot{}, err
	}
	source, err := s.resolveAudienceSource(ctx, tx, actor, scope, version, true)
	if err != nil {
		return CampaignSnapshot{}, err
	}
	if source.Segment.Dynamic() {
		// Lock the evaluation row before pinning it. Section 8B's generation
		// cleanup takes the same lock before releasing a generation's
		// membership, so the two serialize: either this snapshot exists and
		// cleanup's retention check sees it, or cleanup ran first and the check
		// below finds the generation already superseded.
		var status string
		if err = tx.QueryRow(ctx, `SELECT status FROM crm_segment_evaluations
			WHERE business_id=$1 AND segment_id=$2 AND generation=$3 FOR UPDATE`,
			actor.VendorID, source.Segment.ID, source.Generation).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return CampaignSnapshot{}, errCampaignAudienceNotCurrent(EvalNotEvaluated)
			}
			return CampaignSnapshot{}, err
		}
		if status != RunSuccess {
			return CampaignSnapshot{}, errCampaignAudienceNotCurrent(status)
		}
	}

	var generation *int64
	if source.Segment.Dynamic() {
		value := source.Generation
		generation = &value
	}
	snapshot, err := scanSnapshot(tx.QueryRow(ctx, `INSERT INTO crm_campaign_audience_snapshots
		(id,business_id,campaign_id,campaign_version_id,campaign_definition_version,
		 source_segment_id,source_segment_definition_version,source_segment_generation,source_segment_evaluation_id,
		 definition_hash,evaluation_as_of,status,requested_by_staff_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'BUILDING',NULLIF($12,''))
		RETURNING `+snapshotCols,
		newID("crmcs_"), actor.VendorID, campaign.ID, version.ID, version.DefinitionVersion,
		version.SourceSegmentID, source.DefinitionVersion, generation, source.EvaluationID,
		version.DefinitionHash, s.now(), actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			// Another request won the race for the single in-flight slot.
			return CampaignSnapshot{}, crmError(
				apperrors.Conflict("A preparation is already running for this campaign.", nil), CodeCampaignPreparing)
		}
		return CampaignSnapshot{}, err
	}

	if _, err = tx.Exec(ctx, `UPDATE crm_campaigns SET status='PREPARING', updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2`, actor.VendorID, campaign.ID); err != nil {
		return CampaignSnapshot{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.campaign.preparation.started", "crm_campaign", campaign.ID,
		derefOr(campaign.BranchID), map[string]any{"status": campaign.Status},
		map[string]any{"status": CampaignPreparing, "snapshot_ref": snapshot.ID,
			"campaign_version": version.DefinitionVersion}, ""); err != nil {
		return CampaignSnapshot{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CampaignSnapshot{}, err
	}
	s.metrics.Inc("crm_campaign_prepare_total", "scope", campaign.ScopeType)
	return snapshot, nil
}

// RunCampaignPreparationPass claims and runs up to limit due preparations.
//
// Claiming is a lease with SKIP LOCKED, so several instances are safe: two
// workers never build the same snapshot, and a worker that dies mid-run simply
// lets its lease lapse for another to adopt.
func (s *Service) RunCampaignPreparationPass(ctx context.Context, workerID string, limit int) (int, error) {
	if !s.flags.CRMEnabled || !s.flags.CampaignsEnabled || !s.flags.CampaignPreparationEnabled || s.flags.CommerceKillSwitch {
		return 0, nil
	}
	processed := 0
	for processed < limit {
		snapshot, claimed, err := s.claimCampaignPreparation(ctx, workerID)
		if err != nil {
			return processed, err
		}
		if !claimed {
			return processed, nil
		}
		processed++
		if err = s.runCampaignPreparation(ctx, snapshot, workerID); err != nil {
			if failErr := s.failCampaignPreparation(ctx, snapshot, err); failErr != nil {
				return processed, failErr
			}
		}
	}
	return processed, nil
}

func (s *Service) claimCampaignPreparation(ctx context.Context, workerID string) (CampaignSnapshot, bool, error) {
	lease := fmt.Sprintf("%d milliseconds", s.campaignEval.LeaseTTL.Milliseconds())
	snapshot, err := scanSnapshot(s.db.QueryRow(ctx, `UPDATE crm_campaign_audience_snapshots SET
			lease_owner=$1, lease_expires_at=now() + $2::interval,
			attempt_count=attempt_count+1, next_attempt_at=now() + $2::interval,
			updated_at=now(), version=version+1
		WHERE id = (
			SELECT id FROM crm_campaign_audience_snapshots
			WHERE status='BUILDING' AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at <= now())
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+snapshotCols, workerID, lease))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignSnapshot{}, false, nil
	}
	if err != nil {
		return CampaignSnapshot{}, false, err
	}
	return snapshot, true, nil
}

// runCampaignPreparation builds one snapshot and swaps it in atomically.
//
// The order is the whole design: the audience is written while the snapshot is
// still BUILDING and therefore invisible as a prepared audience, the promotion
// is one committed transaction, and a failure at any point before that
// transaction leaves the previous prepared snapshot exactly as it was. A
// partial audience can never become current, because "current" is a pointer
// that only moves once the rows behind it are complete.
func (s *Service) runCampaignPreparation(ctx context.Context, snapshot CampaignSnapshot, workerID string) error {
	actor := Actor{VendorID: snapshot.BusinessID, CorrelationID: "campaign-audience-preparer"}

	campaign, err := s.loadCampaign(ctx, s.db, snapshot.BusinessID, snapshot.CampaignID)
	if err != nil {
		return err
	}
	if campaign.Status == CampaignArchived {
		return s.supersedeSnapshot(ctx, snapshot)
	}
	version, err := s.CampaignVersionByID(ctx, s.db, snapshot.BusinessID, snapshot.CampaignVersionID)
	if err != nil {
		return err
	}
	// The pinned generation must still hold its membership. Section 8B releases
	// a superseded generation's rows once nothing references it, so a
	// generation that has moved on cannot be prepared from — and retrying would
	// never change that, which is why this failure is terminal rather than
	// backed off.
	if snapshot.SourceSegmentGeneration != nil {
		var status string
		if err = s.db.QueryRow(ctx, `SELECT status FROM crm_segment_evaluations
			WHERE business_id=$1 AND segment_id=$2 AND generation=$3`,
			snapshot.BusinessID, snapshot.SourceSegmentID, *snapshot.SourceSegmentGeneration).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return terminalPreparationError(errCampaignAudienceNotCurrent(EvalNotEvaluated))
			}
			return err
		}
		if status != RunSuccess {
			return terminalPreparationError(errCampaignAudienceNotCurrent(status))
		}
	}

	generation := int64(0)
	if snapshot.SourceSegmentGeneration != nil {
		generation = *snapshot.SourceSegmentGeneration
	}
	// The preparation reads the whole audience the campaign's own scope
	// describes. The requesting actor's branch reach was already enforced when
	// the campaign and its segment were bound together; re-applying a session
	// scope here would make a snapshot's contents depend on who pressed the
	// button.
	campaignScope := Scope{Global: true}
	query := s.buildAudienceQuery(snapshot.BusinessID, version, generation, campaignScope, s.campaignEval.BatchSize)
	allocator := newExperimentAllocator(version)

	heartbeatCtx, stopHeartbeat := s.startPreparationHeartbeat(ctx, snapshot, workerID)
	// A previous attempt may have written part of this snapshot before dying.
	// Restarting from a clean slate is safe precisely because a BUILDING
	// snapshot is not the live audience.
	if _, err = s.db.Exec(heartbeatCtx, `DELETE FROM crm_campaign_targets WHERE business_id=$1 AND snapshot_id=$2`,
		snapshot.BusinessID, snapshot.ID); err != nil {
		_ = stopHeartbeat()
		return err
	}

	counts, err := s.writeSnapshotAudience(heartbeatCtx, snapshot, version, query, allocator)
	if heartbeatErr := stopHeartbeat(); heartbeatErr != nil {
		return heartbeatErr
	}
	if err != nil {
		return err
	}

	if err = s.promoteSnapshot(ctx, actor, snapshot, version, counts, nil); err != nil {
		return err
	}
	s.metrics.Inc("crm_campaign_prepare_success_total")
	return nil
}

// snapshotCounts is the reconciled funnel for one preparation.
type snapshotCounts struct {
	Source            int
	Excluded          int
	Privacy           int
	ServiceRecovery   int
	ConsentSuppressed int
	Target            int
	Control           int
	Treatment         int
}

// writeSnapshotAudience streams the source audience into the snapshot in
// bounded keyset batches, so a large tenant never produces one enormous
// statement or one enormous transaction.
func (s *Service) writeSnapshotAudience(ctx context.Context, snapshot CampaignSnapshot, version CampaignVersion,
	query audienceQuery, allocator experimentAllocator) (snapshotCounts, error) {
	var counts snapshotCounts
	batch := s.campaignEval.BatchSize
	cursor := ""
	now := s.now()

	for {
		candidates, err := s.scanCandidates(ctx, s.db, query, cursor)
		if err != nil {
			return counts, err
		}
		if len(candidates) == 0 {
			return counts, nil
		}

		targets := make([]campaignCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			cursor = candidate.ProfileID
			counts.Source++
			outcome, _ := candidate.classify(version.ServiceRecoverySuppression)
			switch outcome {
			case outcomeExcluded:
				counts.Excluded++
			case outcomeSuppressed:
				counts.Privacy++
			case outcomeServiceRecovery:
				counts.ServiceRecovery++
			default:
				targets = append(targets, candidate)
			}
		}

		if len(targets) > 0 {
			written, err := s.writeTargetBatch(ctx, snapshot, version, allocator, targets, now)
			if err != nil {
				return counts, err
			}
			counts.Target += written.Target
			counts.Control += written.Control
			counts.Treatment += written.Treatment
			counts.ConsentSuppressed += written.ConsentSuppressed
		}
		if len(candidates) < batch {
			return counts, nil
		}
	}
}

// writeTargetBatch inserts one batch of targets and their per-channel consent
// evidence.
//
// Target ids are derived from (snapshot, profile) rather than generated
// randomly, so re-running a partially written snapshot after a crash reproduces
// exactly the same rows instead of duplicating them under new ids.
func (s *Service) writeTargetBatch(ctx context.Context, snapshot CampaignSnapshot, version CampaignVersion,
	allocator experimentAllocator, targets []campaignCandidate, now time.Time) (snapshotCounts, error) {
	var counts snapshotCounts

	profileIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		profileIDs = append(profileIDs, target.ProfileID)
	}
	consent, err := s.consentForProfiles(ctx, s.db, snapshot.BusinessID, profileIDs)
	if err != nil {
		return counts, err
	}

	targetIDs := make([]string, 0, len(targets))
	variantKeys := make([]*string, 0, len(targets))
	controls := make([]bool, 0, len(targets))
	buckets := make([]*int, 0, len(targets))

	channelTargetIDs := make([]string, 0, len(targets)*len(version.Channels))
	channelIDs := make([]string, 0, cap(channelTargetIDs))
	channelNames := make([]string, 0, cap(channelTargetIDs))
	decisions := make([]string, 0, cap(channelTargetIDs))
	eligibles := make([]bool, 0, cap(channelTargetIDs))
	observedAts := make([]*time.Time, 0, cap(channelTargetIDs))
	sourceVersions := make([]*int64, 0, cap(channelTargetIDs))
	states := make([]string, 0, cap(channelTargetIDs))
	reasons := make([]string, 0, cap(channelTargetIDs))

	for _, target := range targets {
		targetID := campaignTargetID(snapshot.ID, target.ProfileID)
		assignment := allocator.Assign(target.ProfileID)

		targetIDs = append(targetIDs, targetID)
		if assignment.VariantKey == "" {
			variantKeys = append(variantKeys, nil)
			buckets = append(buckets, nil)
		} else {
			key, bucket := assignment.VariantKey, assignment.Bucket
			variantKeys = append(variantKeys, &key)
			buckets = append(buckets, &bucket)
		}
		controls = append(controls, assignment.IsControl)
		counts.Target++
		if assignment.IsControl {
			counts.Control++
		} else {
			counts.Treatment++
		}

		anyEligible := false
		for _, decision := range s.evaluateChannels(target, version.Channels, consent[target.ProfileID], now) {
			channelIDs = append(channelIDs, campaignChannelID(targetID, decision.Channel))
			channelTargetIDs = append(channelTargetIDs, targetID)
			channelNames = append(channelNames, decision.Channel)
			decisions = append(decisions, string(decision.Decision))
			eligibles = append(eligibles, decision.Eligible)
			observedAts = append(observedAts, decision.ObservedAt)
			sourceVersions = append(sourceVersions, decision.SourceVersion)
			states = append(states, decision.State)
			reasons = append(reasons, decision.Reason)
			if decision.Eligible {
				anyEligible = true
			}
		}
		if !anyEligible {
			counts.ConsentSuppressed++
		}
	}

	if _, err = s.db.Exec(ctx, `INSERT INTO crm_campaign_targets
		(id,business_id,snapshot_id,customer_profile_id,experiment_variant_key,is_control,experiment_bucket)
		SELECT t.id, $1, $2, t.profile_id, t.variant_key, t.is_control, t.bucket
		FROM unnest($3::text[], $4::text[], $5::text[], $6::boolean[], $7::int[])
			AS t(id, profile_id, variant_key, is_control, bucket)
		ON CONFLICT (business_id, snapshot_id, customer_profile_id) DO NOTHING`,
		snapshot.BusinessID, snapshot.ID, targetIDs, profileIDs, variantKeys, controls, buckets); err != nil {
		return counts, err
	}

	if len(channelIDs) > 0 {
		if _, err = s.db.Exec(ctx, `INSERT INTO crm_campaign_target_channels
			(id,business_id,snapshot_id,target_id,channel,snapshot_consent_decision,snapshot_consent_eligible,
			 consent_as_of,consent_source_version,snapshot_eligibility_state,reason_code)
			SELECT c.id, $1, $2, c.target_id, c.channel, c.decision, c.eligible, c.observed_at,
				c.source_version, c.state, c.reason
			FROM unnest($3::text[], $4::text[], $5::text[], $6::text[], $7::boolean[], $8::timestamptz[],
				$9::bigint[], $10::text[], $11::text[])
				AS c(id, target_id, channel, decision, eligible, observed_at, source_version, state, reason)
			ON CONFLICT (business_id, target_id, channel) DO NOTHING`,
			snapshot.BusinessID, snapshot.ID, channelIDs, channelTargetIDs, channelNames, decisions,
			eligibles, observedAts, sourceVersions, states, reasons); err != nil {
			return counts, err
		}
	}
	return counts, nil
}

// campaignTargetID derives a stable target id, so a restarted preparation
// reproduces the same rows rather than duplicating them.
func campaignTargetID(snapshotID, profileID string) string {
	return "crmct_" + shortDigest(snapshotID+":"+profileID)
}

func campaignChannelID(targetID, channel string) string {
	return "crmctc_" + shortDigest(targetID+":"+channel)
}

// startPreparationHeartbeat keeps a claimed preparation leased while its
// audience is being written. Without renewal, a preparation that legitimately
// exceeds the lease could be adopted by a second worker and built twice.
//
// The heartbeat stops before the short promotion transaction, leaving a full
// lease for it and avoiding a race with the promotion clearing lease_owner.
func (s *Service) startPreparationHeartbeat(ctx context.Context, snapshot CampaignSnapshot, workerID string) (context.Context, func() error) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	interval := s.campaignEval.LeaseTTL / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	lease := fmt.Sprintf("%d milliseconds", s.campaignEval.LeaseTTL.Milliseconds())
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				tag, err := s.db.Exec(runCtx, `UPDATE crm_campaign_audience_snapshots SET
					lease_expires_at=now() + $1::interval, updated_at=now()
					WHERE business_id=$2 AND id=$3 AND status='BUILDING' AND lease_owner=$4`,
					lease, snapshot.BusinessID, snapshot.ID, workerID)
				if err != nil {
					done <- err
					cancel()
					return
				}
				if tag.RowsAffected() != 1 {
					done <- errors.New("campaign preparation lease lost")
					cancel()
					return
				}
			}
		}
	}()
	return runCtx, func() error {
		cancel()
		return <-done
	}
}

// promoteSnapshot is the atomic hand-over from the previous prepared audience
// to the new one.
//
// The re-sweep inside this transaction closes the prepare-versus-privacy race:
// a redaction, merge, restriction, suppression or service-recovery task that
// committed while the audience was being written is seen here, and the affected
// targets are removed before the snapshot ever becomes current. The counts are
// then recomputed from the rows that actually survived, so the funnel the
// snapshot reports is the funnel it contains.
func (s *Service) promoteSnapshot(ctx context.Context, actor Actor, snapshot CampaignSnapshot, version CampaignVersion,
	counts snapshotCounts, afterCertification func()) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock every profile in the candidate audience, in stable id order.
	// ApplyPrivacy and ApplyIdentityLink take the same profile locks before
	// changing privacy or merge state, so either their transaction commits
	// first and this certification sees it, or this promotion commits first and
	// their own campaign-target suppression runs immediately afterwards.
	lockedRows, err := tx.Query(ctx, `/* crm_campaign_profile_fence */ SELECT p.id FROM vendor_customer_profiles p
		JOIN crm_campaign_targets t ON t.business_id=p.business_id AND t.customer_profile_id=p.id
		WHERE t.business_id=$1 AND t.snapshot_id=$2 ORDER BY p.id FOR UPDATE OF p`,
		snapshot.BusinessID, snapshot.ID)
	if err != nil {
		return err
	}
	for lockedRows.Next() {
		var id string
		if err = lockedRows.Scan(&id); err != nil {
			lockedRows.Close()
			return err
		}
	}
	lockedRows.Close()
	if err = lockedRows.Err(); err != nil {
		return err
	}

	// Structural re-sweep. The predicate is the same one classify() applies,
	// expressed once here so a relationship that stopped being targetable
	// mid-build cannot survive into a current snapshot.
	removedPrivacy, err := tx.Exec(ctx, `DELETE FROM crm_campaign_targets t
		WHERE t.business_id=$1 AND t.snapshot_id=$2 AND EXISTS (
			SELECT 1 FROM vendor_customer_profiles p
			WHERE p.business_id=t.business_id AND p.id=t.customer_profile_id
			  AND (p.privacy_state IN ('REDACTED','RESTRICTED','MERGED')
			    OR p.merge_state='MERGED_SOURCE'
			    OR p.status='ARCHIVED'
			    OR EXISTS (SELECT 1 FROM crm_privacy_projections pp
					WHERE pp.business_id=p.business_id AND pp.profile_id=p.id
					  AND pp.action='MARKETING_SUPPRESS' AND pp.processing_status='APPLIED')))`,
		snapshot.BusinessID, snapshot.ID)
	if err != nil {
		return err
	}
	removedRecovery := int64(0)
	if version.ServiceRecoverySuppression {
		tag, recoveryErr := tx.Exec(ctx, `DELETE FROM crm_campaign_targets t
			WHERE t.business_id=$1 AND t.snapshot_id=$2 AND EXISTS (
				SELECT 1 FROM vendor_customer_profiles p
				WHERE p.business_id=t.business_id AND p.id=t.customer_profile_id AND `+serviceRecoverySQL+`)`,
			snapshot.BusinessID, snapshot.ID)
		if recoveryErr != nil {
			return recoveryErr
		}
		removedRecovery = tag.RowsAffected()
	}

	counts.Privacy += int(removedPrivacy.RowsAffected())
	counts.ServiceRecovery += int(removedRecovery)

	// Recount from the surviving rows rather than trusting the running totals:
	// the funnel the snapshot publishes must describe the audience it holds.
	var target, control, treatment, consentSuppressed int
	var audienceHash *string
	if err = tx.QueryRow(ctx, `SELECT count(*),
			count(*) FILTER (WHERE is_control),
			count(*) FILTER (WHERE NOT is_control),
			md5(string_agg(customer_profile_id, ',' ORDER BY customer_profile_id))
		FROM crm_campaign_targets WHERE business_id=$1 AND snapshot_id=$2`,
		snapshot.BusinessID, snapshot.ID).Scan(&target, &control, &treatment, &audienceHash); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_targets t
		WHERE t.business_id=$1 AND t.snapshot_id=$2 AND NOT EXISTS (
			SELECT 1 FROM crm_campaign_target_channels ch
			WHERE ch.business_id=t.business_id AND ch.target_id=t.id
			  AND ch.snapshot_eligibility_state='ELIGIBLE_AT_SNAPSHOT')`,
		snapshot.BusinessID, snapshot.ID).Scan(&consentSuppressed); err != nil {
		return err
	}
	counts.Target, counts.Control, counts.Treatment, counts.ConsentSuppressed = target, control, treatment, consentSuppressed

	if afterCertification != nil {
		afterCertification()
	}

	updated, err := scanSnapshot(tx.QueryRow(ctx, `UPDATE crm_campaign_audience_snapshots SET
			status='CURRENT', completed_at=now(), audience_hash=$1,
			source_count=$2, segment_exclusion_count=$3, privacy_suppressed_count=$4,
			service_recovery_suppressed_count=$5, consent_suppressed_count=$6, target_count=$7,
			control_count=$8, treatment_count=$9,
			error_code=NULL, error_detail_safe=NULL, lease_owner=NULL, lease_expires_at=NULL,
			updated_at=now(), version=version+1
		WHERE business_id=$10 AND id=$11 AND status='BUILDING' RETURNING `+snapshotCols,
		audienceHash, counts.Source, counts.Excluded, counts.Privacy, counts.ServiceRecovery,
		counts.ConsentSuppressed, counts.Target, counts.Control, counts.Treatment,
		snapshot.BusinessID, snapshot.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		// The run was cancelled or adopted elsewhere while it was building.
		return nil
	}
	if err != nil {
		return err
	}

	// The previous prepared audience is retired only now, once its replacement
	// is complete and certified.
	if _, err = tx.Exec(ctx, `UPDATE crm_campaign_audience_snapshots SET status='SUPERSEDED', updated_at=now(), version=version+1
		WHERE business_id=$1 AND campaign_id=$2 AND id <> $3 AND status='CURRENT'`,
		snapshot.BusinessID, snapshot.CampaignID, snapshot.ID); err != nil {
		return err
	}
	// The campaign only points at the new snapshot inside the same transaction
	// that made it current.
	if _, err = tx.Exec(ctx, `UPDATE crm_campaigns SET status='PREPARED', current_snapshot_id=$1,
			updated_at=now(), version=version+1
		WHERE business_id=$2 AND id=$3 AND status <> 'ARCHIVED'`,
		snapshot.ID, snapshot.BusinessID, snapshot.CampaignID); err != nil {
		return err
	}

	if err = s.auditTx(ctx, tx, actor, "vendor.crm.campaign.prepared", "crm_campaign", snapshot.CampaignID, "",
		nil, map[string]any{"snapshot_ref": snapshot.ID, "campaign_version": snapshot.CampaignDefinitionVersion,
			"target_count": counts.Target, "control_count": counts.Control}, ""); err != nil {
		return err
	}
	// The event carries the audience's identity and its size, never its
	// contents. A target list on an event bus would be a customer export
	// nobody asked for.
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.campaign.prepared", "crm_campaign", snapshot.CampaignID,
		updated.Version, map[string]any{
			"campaign_ref": snapshot.CampaignID, "campaign_version": snapshot.CampaignDefinitionVersion,
			"snapshot_ref": snapshot.ID, "scope_type": version.ScopeType, "branch_ref": version.BranchID,
			"channels": version.Channels, "target_count": counts.Target, "control_count": counts.Control,
			"prepared_at": s.now(), "send_time_recheck_required": true,
		}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.metrics.Add("crm_campaign_snapshot_target_total", float64(counts.Target))
	s.metrics.Add("crm_campaign_consent_suppression_total", float64(counts.ConsentSuppressed))
	s.metrics.Add("crm_campaign_service_recovery_suppression_total", float64(counts.ServiceRecovery))
	return nil
}

// terminalPreparationError marks a failure that retrying can never fix.
type terminalError struct{ cause error }

func (e terminalError) Error() string { return e.cause.Error() }
func (e terminalError) Unwrap() error { return e.cause }

func terminalPreparationError(cause error) error { return terminalError{cause: cause} }

// failCampaignPreparation records a failed attempt and schedules a retry, or
// gives up after the configured ceiling.
//
// The previous CURRENT snapshot is never touched here. A campaign whose latest
// preparation failed reports PREPARATION_FAILED while still pointing at the
// last audience that actually worked — an empty audience would be a far more
// dangerous answer than a stale one.
func (s *Service) failCampaignPreparation(ctx context.Context, snapshot CampaignSnapshot, cause error) error {
	code := CodeCampaignPrepFailed
	if appErr := apperrors.From(cause); appErr != nil && appErr.Details != nil {
		if detail, ok := appErr.Details["code"].(string); ok && detail != "" {
			code = detail
		}
	}
	var terminal terminalError
	giveUp := errors.As(cause, &terminal) || snapshot.AttemptCount >= s.campaignEval.MaxAttempts
	backoff := fmt.Sprintf("%d seconds", int64(s.campaignEval.RetryBackoff.Seconds())*int64(maxInt(snapshot.AttemptCount, 1)))

	if giveUp {
		if _, err := s.db.Exec(ctx, `UPDATE crm_campaign_audience_snapshots SET
				status='FAILED', dead_lettered=true, completed_at=now(), error_code=$1, error_detail_safe=$2,
				lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
			WHERE business_id=$3 AND id=$4 AND status='BUILDING'`,
			code, safeFailureDetail(cause), snapshot.BusinessID, snapshot.ID); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, `UPDATE crm_campaigns SET status='PREPARATION_FAILED', updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 AND status='PREPARING'`, snapshot.BusinessID, snapshot.CampaignID); err != nil {
			return err
		}
		s.metrics.Inc("crm_campaign_prepare_failure_total", "outcome", "dead_letter")
		return nil
	}

	if _, err := s.db.Exec(ctx, `UPDATE crm_campaign_audience_snapshots SET
			error_code=$1, error_detail_safe=$2, lease_owner=NULL, lease_expires_at=NULL,
			next_attempt_at=now() + $3::interval, updated_at=now(), version=version+1
		WHERE business_id=$4 AND id=$5 AND status='BUILDING'`,
		code, safeFailureDetail(cause), backoff, snapshot.BusinessID, snapshot.ID); err != nil {
		return err
	}
	s.metrics.Inc("crm_campaign_prepare_failure_total", "outcome", "retry")
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Service) supersedeSnapshot(ctx context.Context, snapshot CampaignSnapshot) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM crm_campaign_targets WHERE business_id=$1 AND snapshot_id=$2`,
		snapshot.BusinessID, snapshot.ID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE crm_campaign_audience_snapshots SET status='SUPERSEDED', completed_at=now(),
		lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2 AND status='BUILDING'`, snapshot.BusinessID, snapshot.ID)
	return err
}

// ReplayCampaignPreparation returns a dead-lettered preparation to the durable
// queue. It is deliberately a code-level operator hook rather than a
// Vendor-facing endpoint: replaying failed infrastructure work is an
// operational action, not ordinary CRM authority.
func (s *Service) ReplayCampaignPreparation(ctx context.Context, businessID, snapshotID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE crm_campaign_audience_snapshots SET
		status='BUILDING', dead_lettered=false, attempt_count=0, next_attempt_at=now(),
		lease_owner=NULL, lease_expires_at=NULL, completed_at=NULL,
		error_code=NULL, error_detail_safe=NULL, updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2 AND status='FAILED' AND dead_lettered=true`, businessID, snapshotID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("campaign preparation %s is not dead-lettered", snapshotID)
	}
	return nil
}

// CampaignPreparationBacklog reports how many preparations are waiting, for the
// metrics line.
func (s *Service) CampaignPreparationBacklog(ctx context.Context) (int, error) {
	var depth int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_audience_snapshots
		WHERE status='BUILDING' AND next_attempt_at <= now()`).Scan(&depth)
	return depth, err
}

// ---------------------------------------------------------------------------
// Snapshot and target reads
// ---------------------------------------------------------------------------

// ListSnapshots pages a campaign's snapshot history.
func (s *Service) ListSnapshots(ctx context.Context, actor Actor, scope Scope, campaignID string, limit, offset int) ([]CampaignSnapshot, bool, error) {
	if _, err := s.GetCampaign(ctx, actor, scope, campaignID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	rows, err := s.db.Query(ctx, `SELECT `+snapshotCols+` FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2 ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`,
		actor.VendorID, campaignID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]CampaignSnapshot, 0, limit)
	for rows.Next() {
		snapshot, scanErr := scanSnapshot(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		out = append(out, snapshot)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// GetSnapshot loads one snapshot through its campaign, so tenancy and branch
// scope are decided by the campaign's own visibility rules.
func (s *Service) GetSnapshot(ctx context.Context, actor Actor, scope Scope, campaignID, snapshotID string) (CampaignSnapshot, error) {
	if _, err := s.GetCampaign(ctx, actor, scope, campaignID); err != nil {
		return CampaignSnapshot{}, err
	}
	snapshot, err := scanSnapshot(s.db.QueryRow(ctx, `SELECT `+snapshotCols+` FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2 AND id=$3`, actor.VendorID, campaignID, snapshotID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignSnapshot{}, crmError(apperrors.NotFound("Audience snapshot not found.", nil), CodeCampaignSnapshotMissing)
	}
	return snapshot, err
}

// CampaignTarget is one row of a prepared audience.
type CampaignTarget struct {
	ID                string
	ProfileID         string
	DisplayLabel      *string
	VariantKey        *string
	IsControl         bool
	Bucket            *int
	CurrentActionable bool
	SuppressionReason *string
	// ServiceRecoveryActive is evaluated live rather than stored. A customer
	// can enter and leave service recovery at any time, and a stored flag would
	// be stale the moment a complaint was resolved.
	ServiceRecoveryActive bool
	Channels              []CampaignTargetChannel
}

// Actionable is the answer a caller should act on: the stored suppression state
// and the live service-recovery signal together. It is still not permission to
// send — every channel below carries a mandatory send-time recheck.
func (t CampaignTarget) Actionable() bool {
	return t.CurrentActionable && !t.ServiceRecoveryActive
}

// CampaignTargetChannel is the immutable snapshot-time consent evidence.
type CampaignTargetChannel struct {
	Channel                  string     `json:"channel"`
	SnapshotConsentDecision  string     `json:"snapshot_consent_decision"`
	SnapshotConsentEligible  bool       `json:"snapshot_consent_eligible"`
	ConsentAsOf              *time.Time `json:"consent_as_of,omitempty"`
	ConsentSourceVersion     *int64     `json:"consent_source_version,omitempty"`
	SnapshotEligibilityState string     `json:"snapshot_eligibility_state"`
	ReasonCode               string     `json:"reason_code"`
	SendTimeRecheckRequired  bool       `json:"send_time_recheck_required"`
}

// TargetListOptions are the safe filters over a snapshot's targets.
type TargetListOptions struct {
	Limit             int
	Offset            int
	Cohort            string
	ActionableOnly    bool
	EligibleOnChannel string
}

// ListSnapshotTargets pages the relationships inside one snapshot.
//
// The rows carry a relationship reference, a cohort and consent evidence, and
// nothing else — there is no email, phone or device token field to populate,
// because Section 8E never learned one. A branch-scoped caller reaches this
// only through a branch campaign they hold, so a snapshot's membership can
// never be read across the branch wall.
func (s *Service) ListSnapshotTargets(ctx context.Context, actor Actor, scope Scope, campaignID, snapshotID string,
	opts TargetListOptions) ([]CampaignTarget, bool, error) {
	if _, err := s.GetSnapshot(ctx, actor, scope, campaignID, snapshotID); err != nil {
		return nil, false, err
	}
	limit, offset := boundPage(opts.Limit, opts.Offset)

	args := []any{actor.VendorID, snapshotID}
	where := []string{"t.business_id=$1", "t.snapshot_id=$2"}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}
	switch strings.ToUpper(strings.TrimSpace(opts.Cohort)) {
	case "":
	case "CONTROL":
		where = append(where, "t.is_control")
	case "TREATMENT":
		where = append(where, "NOT t.is_control")
	default:
		return nil, false, campaignValidation("cohort", "Cohort must be CONTROL or TREATMENT.", CodeCampaignInvalidExpiment)
	}
	if opts.EligibleOnChannel != "" {
		channel := strings.ToUpper(strings.TrimSpace(opts.EligibleOnChannel))
		if !IsValidConsentChannel(channel) {
			return nil, false, campaignValidation("channel", "Channels must be EMAIL, PUSH, IN_APP or SMS.", CodeCampaignInvalidChannel)
		}
		where = append(where, `EXISTS (SELECT 1 FROM crm_campaign_target_channels ch
			WHERE ch.business_id=t.business_id AND ch.target_id=t.id AND ch.channel=`+bind(channel)+`
			  AND ch.snapshot_eligibility_state='ELIGIBLE_AT_SNAPSHOT')`)
	}
	if opts.ActionableOnly {
		where = append(where, "t.current_actionable")
		where = append(where, `NOT EXISTS (SELECT 1 FROM vendor_customer_profiles p
			WHERE p.business_id=t.business_id AND p.id=t.customer_profile_id AND `+serviceRecoverySQL+`)`)
	}

	limitArg, offsetArg := bind(limit+1), bind(offset)
	rows, err := s.db.Query(ctx, `SELECT t.id, t.customer_profile_id, p.safe_display_label,
			t.experiment_variant_key, t.is_control, t.experiment_bucket,
			t.current_actionable, t.suppression_reason_code,
			`+serviceRecoverySQL+` AS service_recovery
		FROM crm_campaign_targets t
		JOIN vendor_customer_profiles p ON p.business_id=t.business_id AND p.id=t.customer_profile_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY t.customer_profile_id ASC LIMIT `+limitArg+` OFFSET `+offsetArg, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	targets := make([]CampaignTarget, 0, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		var target CampaignTarget
		if err = rows.Scan(&target.ID, &target.ProfileID, &target.DisplayLabel, &target.VariantKey,
			&target.IsControl, &target.Bucket, &target.CurrentActionable, &target.SuppressionReason,
			&target.ServiceRecoveryActive); err != nil {
			return nil, false, err
		}
		targets = append(targets, target)
		ids = append(ids, target.ID)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(targets) > limit
	if hasMore {
		targets = targets[:limit]
		ids = ids[:limit]
	}
	if len(ids) == 0 {
		return targets, hasMore, nil
	}

	channels, err := s.channelsForTargets(ctx, actor.VendorID, ids)
	if err != nil {
		return nil, false, err
	}
	for index := range targets {
		targets[index].Channels = channels[targets[index].ID]
	}
	return targets, hasMore, nil
}

func (s *Service) channelsForTargets(ctx context.Context, businessID string, targetIDs []string) (map[string][]CampaignTargetChannel, error) {
	rows, err := s.db.Query(ctx, `SELECT target_id, channel, snapshot_consent_decision, snapshot_consent_eligible,
		consent_as_of, consent_source_version, snapshot_eligibility_state, reason_code, send_time_recheck_required
		FROM crm_campaign_target_channels WHERE business_id=$1 AND target_id = ANY($2)
		ORDER BY target_id, channel`, businessID, targetIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]CampaignTargetChannel{}
	for rows.Next() {
		var targetID string
		var channel CampaignTargetChannel
		if err = rows.Scan(&targetID, &channel.Channel, &channel.SnapshotConsentDecision,
			&channel.SnapshotConsentEligible, &channel.ConsentAsOf, &channel.ConsentSourceVersion,
			&channel.SnapshotEligibilityState, &channel.ReasonCode, &channel.SendTimeRecheckRequired); err != nil {
			return nil, err
		}
		out[targetID] = append(out[targetID], channel)
	}
	return out, rows.Err()
}

// SnapshotChannelCounts derives the per-channel eligibility figures from the
// rows themselves, so a reported count can never drift from the evidence it
// summarises.
func (s *Service) SnapshotChannelCounts(ctx context.Context, businessID, snapshotID string) ([]CampaignChannelProjection, error) {
	rows, err := s.db.Query(ctx, `SELECT channel,
			count(*) FILTER (WHERE snapshot_eligibility_state='ELIGIBLE_AT_SNAPSHOT'),
			count(*) FILTER (WHERE snapshot_eligibility_state='INELIGIBLE_CONSENT')
		FROM crm_campaign_target_channels WHERE business_id=$1 AND snapshot_id=$2
		GROUP BY channel ORDER BY channel`, businessID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CampaignChannelProjection{}
	for rows.Next() {
		projection := CampaignChannelProjection{IneligibleReasons: map[string]int{}, SendTimeRecheckRequired: true}
		if err = rows.Scan(&projection.Channel, &projection.EligibleCount, &projection.IneligibleCount); err != nil {
			return nil, err
		}
		out = append(out, projection)
	}
	return out, rows.Err()
}

func boundPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
