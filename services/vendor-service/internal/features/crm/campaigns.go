package crm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "time/tzdata" // embed the IANA database so quiet-hour zone validation works on Windows and slim containers

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8E is the campaign intelligence and audience engine. It answers who a
// campaign should target and freezes that answer; it never answers how or when
// a message is delivered.
//
// The distinction is load-bearing rather than editorial. A prepared audience is
// evidence of what the consent authority said at one moment, and consent is a
// decision a customer may revise at any time. If preparation were allowed to
// read as permission, every later change of mind would arrive too late. So
// nothing in this file schedules, hands off, or marks anything as sent, and
// every eligible channel is stamped with a mandatory send-time recheck that the
// database itself will not let us clear.

// Campaign lifecycle.
//
// There is no SCHEDULED, SENDING, SENT or DELIVERED. Those states describe
// delivery, which Section 8F owns.
const (
	CampaignDraft             = "DRAFT"
	CampaignPreparing         = "PREPARING"
	CampaignPrepared          = "PREPARED"
	CampaignPreparationFailed = "PREPARATION_FAILED"
	CampaignArchived          = "ARCHIVED"
)

// Campaign objectives. All five are marketing objectives, and all five evaluate
// against the single Section 8A consent purpose, MARKETING. An objective is a
// vendor's own description of intent; it is never a consent purpose and can
// never widen what the authority granted. Transactional, order-status and
// security messages are deliberately absent: they are not CRM campaigns.
const (
	ObjectiveGeneralMarketing = "GENERAL_MARKETING"
	ObjectiveRetention        = "RETENTION"
	ObjectiveReactivation     = "REACTIVATION"
	ObjectiveLoyalty          = "LOYALTY"
	ObjectiveAnnouncement     = "ANNOUNCEMENT"
)

var campaignObjectives = map[string]bool{
	ObjectiveGeneralMarketing: true, ObjectiveRetention: true, ObjectiveReactivation: true,
	ObjectiveLoyalty: true, ObjectiveAnnouncement: true,
}

// Snapshot lifecycle. BUILDING covers queued, running and awaiting-retry: the
// audience is not current and we are still trying. FAILED means we stopped
// trying. Neither ever disturbs the previous CURRENT snapshot.
const (
	SnapshotBuilding   = "BUILDING"
	SnapshotCurrent    = "CURRENT"
	SnapshotFailed     = "FAILED"
	SnapshotSuperseded = "SUPERSEDED"
)

// Snapshot-time channel eligibility.
//
// Two values, not five. A privacy- or service-recovery-suppressed relationship
// never becomes a target at all, so there is no row for a suppressed state to
// live on; and every eligible row requires a send-time recheck, so "recheck
// required" is a property of all of them rather than a state one of them can be
// in. There is no AUTHORIZED_TO_SEND because this section cannot authorise a
// send.
const (
	ChannelEligibleAtSnapshot = "ELIGIBLE_AT_SNAPSHOT"
	ChannelIneligibleConsent  = "INELIGIBLE_CONSENT"
)

// Reasons a target stopped being actionable after its snapshot was frozen.
// These never rewrite the snapshot; they sit beside it.
const (
	SuppressionRedacted            = "PRIVACY_REDACTED"
	SuppressionRestricted          = "PRIVACY_RESTRICTED"
	SuppressionMarketingSuppressed = "PRIVACY_MARKETING_SUPPRESSED"
	SuppressionIdentityMerged      = "IDENTITY_MERGED"
)

// Preview explanation categories. They are deliberately coarse: they explain an
// outcome to vendor staff without disclosing the consent authority's reasoning,
// the underlying query, or anything about the customer.
//
// Structural suppression deliberately reuses Section 8A's own reason
// vocabulary (PRIVACY_REDACTED, PROFILE_MERGED, PROFILE_ARCHIVED and so on)
// rather than minting a coarser campaign-specific one, so an operator sees the
// same explanation here that Customer 360 already gives them.
const (
	ReasonSourceMember      = "SOURCE_MEMBER"
	ReasonExcludedBySegment = "EXCLUDED_BY_SEGMENT"
	ReasonServiceRecovery   = "SERVICE_RECOVERY_SUPPRESSED"
	ReasonControlGroup      = "CONTROL_GROUP"
	ReasonTreatmentVariant  = "TREATMENT_VARIANT"
)

// Stable Section 8E error codes.
const (
	CodeCampaignNotFound        = "CRM_CAMPAIGN_NOT_FOUND"
	CodeCampaignScopeDenied     = "CRM_CAMPAIGN_SCOPE_DENIED"
	CodeCampaignVersionConflict = "CRM_CAMPAIGN_VERSION_CONFLICT"
	CodeCampaignInvalidState    = "CRM_CAMPAIGN_INVALID_STATE"
	CodeCampaignAudienceMissing = "CRM_CAMPAIGN_AUDIENCE_NOT_FOUND"
	CodeCampaignAudienceStale   = "CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT"
	CodeCampaignAudienceScope   = "CRM_CAMPAIGN_AUDIENCE_SCOPE_INVALID"
	CodeCampaignInvalidChannel  = "CRM_CAMPAIGN_INVALID_CHANNEL"
	CodeCampaignInvalidExpiment = "CRM_CAMPAIGN_INVALID_EXPERIMENT"
	CodeCampaignInvalidFreq     = "CRM_CAMPAIGN_INVALID_FREQUENCY_POLICY"
	CodeCampaignInvalidQuiet    = "CRM_CAMPAIGN_INVALID_QUIET_HOURS"
	CodeCampaignPreparing       = "CRM_CAMPAIGN_PREPARATION_IN_PROGRESS"
	CodeCampaignPrepFailed      = "CRM_CAMPAIGN_PREPARATION_FAILED"
	CodeCampaignNameExists      = "CRM_CAMPAIGN_ALREADY_EXISTS"
	CodeCampaignsDisabled       = "CRM_CAMPAIGNS_DISABLED"
	CodeCampaignSnapshotMissing = "CRM_CAMPAIGN_SNAPSHOT_NOT_FOUND"
)

// CampaignLimits bounds a campaign definition. They are configuration rather
// than scattered literals because these are the knobs an operator reaches for.
const (
	MaxExclusionSegments = 10
	MaxTreatmentVariants = 5
	MaxControlBps        = 5000
	TotalAllocationBps   = 10000
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Campaign is the mutable head of a campaign.
type Campaign struct {
	ID             string
	BusinessID     string
	Name           string
	NormalizedName string
	Objective      string
	ScopeType      string
	BranchID       *string
	Status         string

	CurrentDefinitionVersion int64
	CurrentSnapshotID        *string

	CreatedByStaffRef *string
	UpdatedByStaffRef *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ArchivedAt        *time.Time
	Version           int64
}

// Scope renders the campaign's reach in the shared segment-scope shape, so the
// same authorization helper Sections 8B and 8C use applies unchanged.
func (c Campaign) Scope() SegmentScope {
	return SegmentScope{Type: c.ScopeType, BranchID: derefOr(c.BranchID)}
}

// FrequencyPolicy is a campaign's contact-frequency intent.
//
// It is stored and versioned here and enforced nowhere here. Enforcing it needs
// real delivery facts, and this service has none: no campaign has ever been
// sent, so there is no contact history to count against. Claiming eligibility
// from an empty history would be the most dangerous possible answer, so the
// policy travels with a mandatory send-time recheck instead.
type FrequencyPolicy struct {
	Enabled       bool
	Cap           int
	WindowMinutes int
}

// QuietHoursPolicy is a campaign's do-not-disturb window.
//
// Minutes-of-day plus a mandatory IANA zone. A wall-clock time with no zone is
// exactly the ambiguity the policy exists to remove, and integers make a
// cross-midnight window (22:00 -> 07:00) a plain comparison rather than a
// special case.
type QuietHoursPolicy struct {
	Enabled     bool
	StartMinute int
	EndMinute   int
	Timezone    string
}

// Blocks reports whether an instant falls inside the quiet window.
//
// Section 8E never calls this to gate anything: it exists so a preview can
// answer an explicit "what if we sent at this time" question, and so Section 8F
// inherits a tested rule rather than reimplementing one.
func (q QuietHoursPolicy) Blocks(at time.Time) (bool, error) {
	if !q.Enabled {
		return false, nil
	}
	location, err := time.LoadLocation(q.Timezone)
	if err != nil {
		return false, errCampaignQuietHours("quiet_hours.timezone", "Quiet hours must use a valid IANA timezone.")
	}
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	if q.StartMinute < q.EndMinute {
		return minute >= q.StartMinute && minute < q.EndMinute, nil
	}
	// Cross-midnight: the window is the union of the two ends of the day.
	return minute >= q.StartMinute || minute < q.EndMinute, nil
}

// ExperimentVariant is one cohort of a campaign experiment.
type ExperimentVariant struct {
	ID                string
	BusinessID        string
	CampaignVersionID string
	VariantKey        string
	DisplayName       string
	AllocationBps     int
	IsControl         bool
	CreatedAt         time.Time
}

// CampaignVersion is one immutable campaign definition.
type CampaignVersion struct {
	ID                string
	BusinessID        string
	CampaignID        string
	DefinitionVersion int64

	Name                string
	Objective           string
	ScopeType           string
	BranchID            *string
	Channels            []string
	SourceSegmentID     string
	ExclusionSegmentIDs []string

	Frequency  FrequencyPolicy
	QuietHours QuietHoursPolicy

	ServiceRecoverySuppression bool

	ExperimentEnabled  bool
	ExperimentTotalBps int
	Variants           []ExperimentVariant

	DefinitionHash    string
	CreatedByStaffRef *string
	CreatedAt         time.Time
	ChangeReason      *string
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

func errCampaignNotFound() error {
	// Indistinguishable from "exists but you may not see it", so probing ids
	// discloses nothing about another tenant's or another branch's campaigns.
	return crmError(apperrors.NotFound("Campaign not found.", nil), CodeCampaignNotFound)
}
func errCampaignExists() error {
	return crmError(apperrors.Conflict("A campaign with this name already exists.", nil), CodeCampaignNameExists)
}
func errCampaignState(message string) error {
	return crmError(apperrors.Conflict(message, nil), CodeCampaignInvalidState)
}
func errCampaignVersionConflict() error {
	return crmError(apperrors.Conflict("This campaign changed since you loaded it. Reload and try again.", nil),
		CodeCampaignVersionConflict)
}
func errCampaignsDisabled() error {
	return crmError(apperrors.Unavailable("Campaign management is temporarily unavailable.", nil), CodeCampaignsDisabled)
}
func campaignValidation(field, message, code string) error {
	err := apperrors.Validation(message, []apperrors.FieldViolation{{Field: field, Message: message}})
	if code != "" {
		err.Details = map[string]any{"code": code}
	}
	return err
}
func errCampaignQuietHours(field, message string) error {
	return campaignValidation(field, message, CodeCampaignInvalidQuiet)
}

// assertCampaignWrites gates every Section 8E command.
//
// Reads stay available when writes are frozen, so a rollout can stop changes
// without blinding operators, and the platform kill switch overrides everything.
func (s *Service) assertCampaignWrites() error {
	if !s.flags.CRMEnabled || !s.flags.CampaignsEnabled || s.flags.CommerceKillSwitch {
		return errCampaignsDisabled()
	}
	if !s.flags.WritesEnabled {
		return errWritesDisabled()
	}
	return nil
}

func (s *Service) assertCampaignsAvailable() error {
	if !s.flags.CRMEnabled || !s.flags.CampaignsEnabled || s.flags.CommerceKillSwitch {
		return errCampaignsDisabled()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Definition validation
// ---------------------------------------------------------------------------

// CampaignDefinition is the validated, normalised form of everything a client
// may say about what a campaign targets. Nothing downstream re-parses client
// input.
type CampaignDefinition struct {
	Name                       string
	NormalizedName             string
	Objective                  string
	ScopeType                  string
	BranchID                   *string
	Channels                   []string
	SourceSegmentID            string
	ExclusionSegmentIDs        []string
	Frequency                  FrequencyPolicy
	QuietHours                 QuietHoursPolicy
	ServiceRecoverySuppression bool
	ExperimentEnabled          bool
	Variants                   []ExperimentVariant
}

// Hash is the canonical definition digest. Two definitions that differ only in
// the order a client listed their channels, exclusions or variants produce the
// same value, so an edit that changes nothing does not mint a new version.
func (d CampaignDefinition) Hash() string {
	variants := make([]map[string]any, 0, len(d.Variants))
	for _, variant := range d.orderedVariants() {
		variants = append(variants, map[string]any{
			"key": variant.VariantKey, "bps": variant.AllocationBps, "control": variant.IsControl,
			"name": variant.DisplayName,
		})
	}
	channels := append([]string(nil), d.Channels...)
	sort.Strings(channels)
	exclusions := append([]string(nil), d.ExclusionSegmentIDs...)
	sort.Strings(exclusions)

	document := map[string]any{
		"name": d.Name, "objective": d.Objective, "scope": d.ScopeType, "branch": derefOr(d.BranchID),
		"channels": channels, "source_segment": d.SourceSegmentID, "exclusions": exclusions,
		"frequency": map[string]any{
			"enabled": d.Frequency.Enabled, "cap": d.Frequency.Cap, "window": d.Frequency.WindowMinutes,
		},
		"quiet_hours": map[string]any{
			"enabled": d.QuietHours.Enabled, "start": d.QuietHours.StartMinute,
			"end": d.QuietHours.EndMinute, "timezone": d.QuietHours.Timezone,
		},
		"service_recovery": d.ServiceRecoverySuppression,
		"experiment":       map[string]any{"enabled": d.ExperimentEnabled, "variants": variants},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return ""
	}
	return hashRuleBytes(raw)
}

// orderedVariants is the single canonical ordering of an experiment's cohorts:
// control first, then variant key ascending. Assignment ranges are laid out in
// this order, so the same definition always produces the same buckets no matter
// what order a client sent the variants in.
func (d CampaignDefinition) orderedVariants() []ExperimentVariant {
	out := append([]ExperimentVariant(nil), d.Variants...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsControl != out[j].IsControl {
			return out[i].IsControl
		}
		return out[i].VariantKey < out[j].VariantKey
	})
	return out
}

// CampaignDefinitionInput is the raw client-supplied definition.
type CampaignDefinitionInput struct {
	Name                       string
	Objective                  string
	ScopeType                  string
	BranchRef                  string
	Channels                   []string
	SourceSegmentRef           string
	ExclusionSegmentRefs       []string
	FrequencyCapCount          *int
	FrequencyCapWindowMinutes  *int
	QuietHoursEnabled          *bool
	QuietHoursStart            string
	QuietHoursEnd              string
	QuietHoursTimezone         string
	ServiceRecoverySuppression *bool
	ExperimentEnabled          *bool
	Variants                   []ExperimentVariantInput
}

// ExperimentVariantInput is one client-supplied cohort.
type ExperimentVariantInput struct {
	VariantKey    string
	DisplayName   string
	AllocationBps int
	IsControl     bool
}

// validateCampaignDefinition turns untrusted input into a definition or a
// stable validation error. There is deliberately no best-effort mode.
func validateCampaignDefinition(in CampaignDefinitionInput) (CampaignDefinition, error) {
	var d CampaignDefinition
	var err error

	if d.Name, d.NormalizedName, err = validateCampaignName(in.Name); err != nil {
		return d, err
	}
	d.Objective = strings.ToUpper(strings.TrimSpace(in.Objective))
	if d.Objective == "" {
		d.Objective = ObjectiveGeneralMarketing
	}
	if !campaignObjectives[d.Objective] {
		return d, campaignValidation("objective", "Unsupported campaign objective.", CodeCampaignInvalidState)
	}

	d.ScopeType = strings.ToUpper(strings.TrimSpace(in.ScopeType))
	if d.ScopeType == "" {
		d.ScopeType = ScopeBusiness
	}
	if d.ScopeType != ScopeBusiness && d.ScopeType != ScopeBranch {
		return d, campaignValidation("scope_type", "Scope must be BUSINESS or BRANCH.", CodeCampaignScopeDenied)
	}
	if branch := strings.TrimSpace(in.BranchRef); branch != "" {
		d.BranchID = &branch
	}
	if d.ScopeType == ScopeBranch && d.BranchID == nil {
		return d, campaignValidation("branch_ref", "A branch is required for a branch-scoped campaign.", CodeCampaignScopeDenied)
	}
	if d.ScopeType == ScopeBusiness {
		d.BranchID = nil
	}

	if d.Channels, err = validateCampaignChannels(in.Channels); err != nil {
		return d, err
	}

	d.SourceSegmentID = strings.TrimSpace(in.SourceSegmentRef)
	if d.SourceSegmentID == "" {
		return d, campaignValidation("source_segment_ref", "A source audience segment is required.", CodeCampaignAudienceMissing)
	}
	if !safeToken.MatchString(d.SourceSegmentID) {
		return d, campaignValidation("source_segment_ref", "The source segment reference is not valid.", CodeCampaignAudienceMissing)
	}
	if d.ExclusionSegmentIDs, err = validateExclusionSegments(in.ExclusionSegmentRefs, d.SourceSegmentID); err != nil {
		return d, err
	}

	if d.Frequency, err = validateFrequencyPolicy(in.FrequencyCapCount, in.FrequencyCapWindowMinutes); err != nil {
		return d, err
	}
	if d.QuietHours, err = validateQuietHours(in.QuietHoursEnabled, in.QuietHoursStart, in.QuietHoursEnd, in.QuietHoursTimezone); err != nil {
		return d, err
	}

	// Service-recovery suppression defaults ON. A marketing campaign that
	// reaches a customer in the middle of an unresolved complaint is the
	// failure this policy exists to prevent, so opting out has to be a choice
	// somebody made rather than one they forgot to make.
	d.ServiceRecoverySuppression = true
	if in.ServiceRecoverySuppression != nil {
		d.ServiceRecoverySuppression = *in.ServiceRecoverySuppression
	}

	if d.ExperimentEnabled, d.Variants, err = validateExperiment(in.ExperimentEnabled, in.Variants); err != nil {
		return d, err
	}
	return d, nil
}

func validateCampaignName(name string) (display, normalized string, err error) {
	display = strings.TrimSpace(name)
	if display == "" {
		return "", "", campaignValidation("name", "A campaign name is required.", "")
	}
	if len([]rune(display)) < 2 || len([]rune(display)) > 120 {
		return "", "", campaignValidation("name", "A campaign name must be between 2 and 120 characters.", "")
	}
	for _, r := range display {
		if r < 0x20 || r == 0x7f {
			return "", "", campaignValidation("name", "A campaign name cannot contain control characters.", "")
		}
	}
	if strings.ContainsAny(display, "<>") {
		return "", "", campaignValidation("name", "A campaign name cannot contain markup characters.", "")
	}
	normalized = NormalizeTagName(display)
	if normalized == "" {
		return "", "", campaignValidation("name", "A campaign name must contain visible characters.", "")
	}
	return display, normalized, nil
}

// validateCampaignChannels accepts only the Section 8A consent channels.
//
// A channel outside that set could never have a consent decision projected for
// it, so a campaign naming one could never lawfully target it. Rejecting it
// here is more honest than accepting it and finding nobody eligible.
func validateCampaignChannels(values []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		channel := strings.ToUpper(strings.TrimSpace(value))
		if channel == "" || seen[channel] {
			continue
		}
		if !IsValidConsentChannel(channel) {
			return nil, campaignValidation("channels", "Channels must be EMAIL, PUSH, IN_APP or SMS.", CodeCampaignInvalidChannel)
		}
		seen[channel] = true
		out = append(out, channel)
	}
	if len(out) == 0 {
		return nil, campaignValidation("channels", "At least one channel is required.", CodeCampaignInvalidChannel)
	}
	sort.Strings(out)
	return out, nil
}

func validateExclusionSegments(refs []string, sourceID string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		if !safeToken.MatchString(trimmed) {
			return nil, campaignValidation("exclusion_segment_refs", "One or more exclusion segment references are not valid.",
				CodeCampaignAudienceMissing)
		}
		if trimmed == sourceID {
			// Excluding the source by itself is an audience of nobody expressed
			// as a mistake; refusing it is kinder than silently preparing zero.
			return nil, campaignValidation("exclusion_segment_refs",
				"The source segment cannot also be an exclusion segment.", CodeCampaignAudienceScope)
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	if len(out) > MaxExclusionSegments {
		return nil, campaignValidation("exclusion_segment_refs",
			fmt.Sprintf("Use at most %d exclusion segments.", MaxExclusionSegments), CodeCampaignAudienceScope)
	}
	sort.Strings(out)
	return out, nil
}

func validateFrequencyPolicy(cap, window *int) (FrequencyPolicy, error) {
	if cap == nil && window == nil {
		return FrequencyPolicy{}, nil
	}
	if cap == nil || window == nil {
		return FrequencyPolicy{}, campaignValidation("frequency_cap",
			"A frequency cap needs both a maximum contact count and a window.", CodeCampaignInvalidFreq)
	}
	if *cap < 1 || *cap > 1000 {
		return FrequencyPolicy{}, campaignValidation("frequency_cap_count",
			"The frequency cap must be between 1 and 1000 contacts.", CodeCampaignInvalidFreq)
	}
	if *window < 1 || *window > 525600 {
		return FrequencyPolicy{}, campaignValidation("frequency_cap_window_minutes",
			"The frequency window must be between 1 minute and 1 year.", CodeCampaignInvalidFreq)
	}
	return FrequencyPolicy{Enabled: true, Cap: *cap, WindowMinutes: *window}, nil
}

func validateQuietHours(enabled *bool, start, end, timezone string) (QuietHoursPolicy, error) {
	if enabled == nil || !*enabled {
		return QuietHoursPolicy{}, nil
	}
	startMinute, err := parseMinuteOfDay(start, "quiet_hours_start")
	if err != nil {
		return QuietHoursPolicy{}, err
	}
	endMinute, err := parseMinuteOfDay(end, "quiet_hours_end")
	if err != nil {
		return QuietHoursPolicy{}, err
	}
	if startMinute == endMinute {
		// A zero-length window is a policy that does nothing; a 24-hour window
		// is a campaign that can never be sent. Both are almost certainly a
		// mistake, and neither is expressible.
		return QuietHoursPolicy{}, errCampaignQuietHours("quiet_hours_end",
			"Quiet hours must start and end at different times.")
	}
	zone := strings.TrimSpace(timezone)
	if zone == "" {
		return QuietHoursPolicy{}, errCampaignQuietHours("quiet_hours_timezone",
			"A timezone is required when quiet hours are enabled.")
	}
	if len(zone) > 64 {
		return QuietHoursPolicy{}, errCampaignQuietHours("quiet_hours_timezone", "That timezone is not recognised.")
	}
	// UTC and Local are accepted by LoadLocation but are not IANA zones a
	// vendor should be scheduling customer contact against; requiring a real
	// zone name is what makes the stored policy unambiguous.
	if zone == "Local" {
		return QuietHoursPolicy{}, errCampaignQuietHours("quiet_hours_timezone",
			"Quiet hours must name an IANA timezone, such as Africa/Lagos.")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return QuietHoursPolicy{}, errCampaignQuietHours("quiet_hours_timezone",
			"Quiet hours must name an IANA timezone, such as Africa/Lagos.")
	}
	return QuietHoursPolicy{Enabled: true, StartMinute: startMinute, EndMinute: endMinute, Timezone: zone}, nil
}

// parseMinuteOfDay accepts "HH:MM" in 24-hour form.
func parseMinuteOfDay(value, field string) (int, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.Parse("15:04", trimmed)
	if err != nil {
		return 0, errCampaignQuietHours(field, "Use a 24-hour time such as 22:00.")
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// formatMinuteOfDay renders a stored minute back as "HH:MM".
func formatMinuteOfDay(minute int) string {
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

// validateExperiment enforces the allocation invariants.
//
// Every rejection here is a case where the experiment would still "work" but
// would silently mean something other than what its author wrote: an allocation
// that does not sum leaves a cohort nobody defined, two controls make the
// holdout ambiguous, and a duplicate key makes two cohorts indistinguishable in
// every later analysis.
func validateExperiment(enabled *bool, inputs []ExperimentVariantInput) (bool, []ExperimentVariant, error) {
	on := enabled != nil && *enabled
	if !on {
		if len(inputs) > 0 {
			return false, nil, campaignValidation("experiment.variants",
				"Enable the experiment before defining variants.", CodeCampaignInvalidExpiment)
		}
		return false, nil, nil
	}
	if len(inputs) == 0 {
		return false, nil, campaignValidation("experiment.variants",
			"An experiment needs at least one variant.", CodeCampaignInvalidExpiment)
	}

	seen := map[string]bool{}
	controls, treatments, total := 0, 0, 0
	out := make([]ExperimentVariant, 0, len(inputs))
	for _, input := range inputs {
		key := strings.ToUpper(strings.TrimSpace(input.VariantKey))
		if !variantKeyPattern.MatchString(key) {
			return false, nil, campaignValidation("experiment.variants.variant_key",
				"A variant key must be 1-32 characters of A-Z, 0-9 or underscore.", CodeCampaignInvalidExpiment)
		}
		if seen[key] {
			return false, nil, campaignValidation("experiment.variants.variant_key",
				"Variant keys must be unique.", CodeCampaignInvalidExpiment)
		}
		seen[key] = true

		name := strings.TrimSpace(input.DisplayName)
		if name == "" {
			name = key
		}
		if len([]rune(name)) > 80 {
			return false, nil, campaignValidation("experiment.variants.display_name",
				"A variant name must be at most 80 characters.", CodeCampaignInvalidExpiment)
		}
		if input.AllocationBps < 0 || input.AllocationBps > TotalAllocationBps {
			return false, nil, campaignValidation("experiment.variants.allocation_bps",
				"Allocation must be between 0 and 10000 basis points.", CodeCampaignInvalidExpiment)
		}
		if input.IsControl {
			controls++
			if input.AllocationBps > MaxControlBps {
				return false, nil, campaignValidation("experiment.variants.allocation_bps",
					"The control cohort cannot exceed 50% of the audience.", CodeCampaignInvalidExpiment)
			}
		} else {
			treatments++
		}
		total += input.AllocationBps
		out = append(out, ExperimentVariant{
			VariantKey: key, DisplayName: name, AllocationBps: input.AllocationBps, IsControl: input.IsControl,
		})
	}
	if controls > 1 {
		return false, nil, campaignValidation("experiment.variants.is_control",
			"An experiment can have at most one control cohort.", CodeCampaignInvalidExpiment)
	}
	if treatments < 1 {
		return false, nil, campaignValidation("experiment.variants",
			"An experiment needs at least one treatment variant.", CodeCampaignInvalidExpiment)
	}
	if treatments > MaxTreatmentVariants {
		return false, nil, campaignValidation("experiment.variants",
			fmt.Sprintf("An experiment can have at most %d treatment variants.", MaxTreatmentVariants),
			CodeCampaignInvalidExpiment)
	}
	if total != TotalAllocationBps {
		return false, nil, campaignValidation("experiment.variants.allocation_bps",
			"Variant allocations must total exactly 10000 basis points.", CodeCampaignInvalidExpiment)
	}
	return true, out, nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

const campaignCols = `id,business_id,name,normalized_name,objective,scope_type,branch_id,status,
current_definition_version,current_snapshot_id,created_by_staff_ref,updated_by_staff_ref,
created_at,updated_at,archived_at,version`

func scanCampaign(row rowScanner) (Campaign, error) {
	var c Campaign
	err := row.Scan(&c.ID, &c.BusinessID, &c.Name, &c.NormalizedName, &c.Objective, &c.ScopeType, &c.BranchID,
		&c.Status, &c.CurrentDefinitionVersion, &c.CurrentSnapshotID, &c.CreatedByStaffRef, &c.UpdatedByStaffRef,
		&c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version)
	return c, err
}

const campaignVersionCols = `id,business_id,campaign_id,definition_version,name,objective,scope_type,branch_id,
channels,source_segment_id,exclusion_segment_ids,frequency_cap_count,frequency_cap_window_minutes,
quiet_hours_enabled,quiet_hours_start_minute,quiet_hours_end_minute,quiet_hours_timezone,
service_recovery_suppression_enabled,experiment_enabled,experiment_total_bps,definition_hash,
created_by_staff_ref,created_at,change_reason`

func scanCampaignVersion(row rowScanner) (CampaignVersion, error) {
	var v CampaignVersion
	var capCount, capWindow *int
	var startMinute, endMinute *int16
	var timezone *string
	err := row.Scan(&v.ID, &v.BusinessID, &v.CampaignID, &v.DefinitionVersion, &v.Name, &v.Objective,
		&v.ScopeType, &v.BranchID, &v.Channels, &v.SourceSegmentID, &v.ExclusionSegmentIDs,
		&capCount, &capWindow, &v.QuietHours.Enabled, &startMinute, &endMinute, &timezone,
		&v.ServiceRecoverySuppression, &v.ExperimentEnabled, &v.ExperimentTotalBps, &v.DefinitionHash,
		&v.CreatedByStaffRef, &v.CreatedAt, &v.ChangeReason)
	if err != nil {
		return CampaignVersion{}, err
	}
	if capCount != nil && capWindow != nil {
		v.Frequency = FrequencyPolicy{Enabled: true, Cap: *capCount, WindowMinutes: *capWindow}
	}
	if startMinute != nil {
		v.QuietHours.StartMinute = int(*startMinute)
	}
	if endMinute != nil {
		v.QuietHours.EndMinute = int(*endMinute)
	}
	v.QuietHours.Timezone = derefOr(timezone)
	return v, nil
}

// CampaignListOptions are the safe filters over the campaign list. Sorting is a
// closed set; there is no client-chosen sort column anywhere.
type CampaignListOptions struct {
	Limit       int
	Offset      int
	Status      string
	ScopeType   string
	BranchRef   string
	Objective   string
	Channel     string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Search      string
}

func (o CampaignListOptions) bounded() CampaignListOptions {
	if o.Limit <= 0 {
		o.Limit = DefaultPageSize
	}
	if o.Limit > MaxPageSize {
		o.Limit = MaxPageSize
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// ListCampaigns returns campaigns visible to the actor's scope.
//
// A branch-scoped actor never sees a BUSINESS-scope campaign. Its name and
// audience size describe the whole business, which is a disclosure a branch
// grant does not carry — the same rule Section 8B applies to segments.
func (s *Service) ListCampaigns(ctx context.Context, actor Actor, scope Scope, opts CampaignListOptions) ([]Campaign, bool, error) {
	if err := s.assertCampaignsAvailable(); err != nil {
		return nil, false, err
	}
	opts = opts.bounded()
	where := []string{"c.business_id = $1"}
	args := []any{actor.VendorID}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}

	if !scope.Global {
		where = append(where, "c.scope_type = 'BRANCH' AND c.branch_id = ANY("+bind(scope.BranchIDs)+")")
	}
	if opts.Status != "" {
		status := strings.ToUpper(strings.TrimSpace(opts.Status))
		if !campaignStatuses[status] {
			return nil, false, campaignValidation("status", "Unsupported campaign status.", CodeCampaignInvalidState)
		}
		where = append(where, "c.status = "+bind(status))
	}
	if opts.ScopeType != "" {
		scopeType := strings.ToUpper(strings.TrimSpace(opts.ScopeType))
		if scopeType != ScopeBusiness && scopeType != ScopeBranch {
			return nil, false, campaignValidation("scope_type", "Scope must be BUSINESS or BRANCH.", CodeCampaignScopeDenied)
		}
		where = append(where, "c.scope_type = "+bind(scopeType))
	}
	if opts.BranchRef != "" {
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, false, errScopeDenied(CodeCampaignScopeDenied)
		}
		where = append(where, "c.branch_id = "+bind(opts.BranchRef))
	}
	if opts.Objective != "" {
		objective := strings.ToUpper(strings.TrimSpace(opts.Objective))
		if !campaignObjectives[objective] {
			return nil, false, campaignValidation("objective", "Unsupported campaign objective.", CodeCampaignInvalidState)
		}
		where = append(where, "c.objective = "+bind(objective))
	}
	if opts.Channel != "" {
		channel := strings.ToUpper(strings.TrimSpace(opts.Channel))
		if !IsValidConsentChannel(channel) {
			return nil, false, campaignValidation("channel", "Channels must be EMAIL, PUSH, IN_APP or SMS.", CodeCampaignInvalidChannel)
		}
		// Filtering on the live definition rather than on a denormalised copy,
		// so the filter cannot disagree with what the campaign currently is.
		where = append(where, `EXISTS (SELECT 1 FROM crm_campaign_versions v
			WHERE v.business_id = c.business_id AND v.campaign_id = c.id
			  AND v.definition_version = c.current_definition_version
			  AND `+bind(channel)+` = ANY(v.channels))`)
	}
	if opts.CreatedFrom != nil {
		where = append(where, "c.created_at >= "+bind(opts.CreatedFrom.UTC()))
	}
	if opts.CreatedTo != nil {
		where = append(where, "c.created_at <= "+bind(opts.CreatedTo.UTC()))
	}
	if opts.Search != "" {
		where = append(where, "c.normalized_name LIKE "+bind("%"+escapeLike(NormalizeTagName(opts.Search))+"%")+" ESCAPE '\\'")
	}

	limit := bind(opts.Limit + 1)
	offset := bind(opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+campaignCols+` FROM crm_campaigns c
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.updated_at DESC, c.id ASC LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	campaigns := make([]Campaign, 0, opts.Limit)
	for rows.Next() {
		campaign, scanErr := scanCampaign(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		campaigns = append(campaigns, campaign)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(campaigns) > opts.Limit
	if hasMore {
		campaigns = campaigns[:opts.Limit]
	}
	return campaigns, hasMore, nil
}

var campaignStatuses = map[string]bool{
	CampaignDraft: true, CampaignPreparing: true, CampaignPrepared: true,
	CampaignPreparationFailed: true, CampaignArchived: true,
}

// GetCampaign loads one campaign, enforcing tenancy and scope.
func (s *Service) GetCampaign(ctx context.Context, actor Actor, scope Scope, campaignID string) (Campaign, error) {
	if err := s.assertCampaignsAvailable(); err != nil {
		return Campaign{}, err
	}
	campaign, err := s.loadCampaign(ctx, s.db, actor.VendorID, campaignID)
	if err != nil {
		return Campaign{}, err
	}
	if err = s.assertCampaignVisible(scope, campaign); err != nil {
		return Campaign{}, err
	}
	return campaign, nil
}

func (s *Service) loadCampaign(ctx context.Context, q querier, businessID, campaignID string) (Campaign, error) {
	campaign, err := scanCampaign(q.QueryRow(ctx,
		`SELECT `+campaignCols+` FROM crm_campaigns WHERE business_id=$1 AND id=$2`, businessID, campaignID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, errCampaignNotFound()
	}
	return campaign, err
}

func (s *Service) loadCampaignForUpdate(ctx context.Context, tx pgx.Tx, businessID, campaignID string) (Campaign, error) {
	campaign, err := scanCampaign(tx.QueryRow(ctx,
		`SELECT `+campaignCols+` FROM crm_campaigns WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessID, campaignID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, errCampaignNotFound()
	}
	return campaign, err
}

func (s *Service) assertCampaignVisible(scope Scope, campaign Campaign) error {
	if scope.Global {
		return nil
	}
	if campaign.ScopeType == ScopeBranch && campaign.BranchID != nil && containsString(scope.BranchIDs, *campaign.BranchID) {
		return nil
	}
	return errCampaignNotFound()
}

// CampaignVersionAt loads one immutable definition, including its cohorts.
func (s *Service) CampaignVersionAt(ctx context.Context, q querier, businessID, campaignID string, definitionVersion int64) (CampaignVersion, error) {
	version, err := scanCampaignVersion(q.QueryRow(ctx, `SELECT `+campaignVersionCols+` FROM crm_campaign_versions
		WHERE business_id=$1 AND campaign_id=$2 AND definition_version=$3`, businessID, campaignID, definitionVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignVersion{}, errCampaignNotFound()
	}
	if err != nil {
		return CampaignVersion{}, err
	}
	if version.Variants, err = s.loadVariants(ctx, q, businessID, version.ID); err != nil {
		return CampaignVersion{}, err
	}
	return version, nil
}

// CampaignVersionByID loads one immutable definition by its own id, which is
// how a snapshot resolves the exact definition it was built from.
func (s *Service) CampaignVersionByID(ctx context.Context, q querier, businessID, versionID string) (CampaignVersion, error) {
	version, err := scanCampaignVersion(q.QueryRow(ctx, `SELECT `+campaignVersionCols+` FROM crm_campaign_versions
		WHERE business_id=$1 AND id=$2`, businessID, versionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignVersion{}, errCampaignNotFound()
	}
	if err != nil {
		return CampaignVersion{}, err
	}
	if version.Variants, err = s.loadVariants(ctx, q, businessID, version.ID); err != nil {
		return CampaignVersion{}, err
	}
	return version, nil
}

func (s *Service) loadVariants(ctx context.Context, q querier, businessID, versionID string) ([]ExperimentVariant, error) {
	rows, err := q.Query(ctx, `SELECT id,business_id,campaign_version_id,variant_key,display_name,allocation_bps,is_control,created_at
		FROM crm_campaign_experiment_variants WHERE business_id=$1 AND campaign_version_id=$2
		ORDER BY is_control DESC, variant_key ASC`, businessID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ExperimentVariant, 0, 6)
	for rows.Next() {
		var v ExperimentVariant
		if err = rows.Scan(&v.ID, &v.BusinessID, &v.CampaignVersionID, &v.VariantKey, &v.DisplayName,
			&v.AllocationBps, &v.IsControl, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// CreateCampaign defines a campaign in DRAFT together with definition version 1.
func (s *Service) CreateCampaign(ctx context.Context, actor Actor, scope Scope, in CampaignDefinitionInput) (Campaign, CampaignVersion, error) {
	if err := s.assertCampaignWrites(); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	definition, err := validateCampaignDefinition(in)
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = authorizeObjectScope(scope, definition.ScopeType, definition.BranchID, CodeCampaignScopeDenied); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if definition.BranchID != nil {
		owned, ownErr := s.branchBelongsToBusiness(ctx, tx, actor.VendorID, *definition.BranchID)
		if ownErr != nil {
			return Campaign{}, CampaignVersion{}, ownErr
		}
		if !owned {
			return Campaign{}, CampaignVersion{}, crmError(
				apperrors.Forbidden("That branch does not belong to this business.", nil), CodeCrossTenant)
		}
	}
	if err = s.assertAudienceSegmentsUsable(ctx, tx, actor, scope, definition); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}

	campaign, err := scanCampaign(tx.QueryRow(ctx, `INSERT INTO crm_campaigns
		(id,business_id,name,normalized_name,objective,scope_type,branch_id,status,
		 current_definition_version,created_by_staff_ref,updated_by_staff_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,'DRAFT',1,NULLIF($8,''),NULLIF($8,''))
		RETURNING `+campaignCols,
		newID("crmcmp_"), actor.VendorID, definition.Name, definition.NormalizedName, definition.Objective,
		definition.ScopeType, definition.BranchID, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return Campaign{}, CampaignVersion{}, errCampaignExists()
		}
		return Campaign{}, CampaignVersion{}, err
	}

	version, err := s.insertCampaignVersion(ctx, tx, actor, campaign, 1, definition, "created")
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.campaign.created", "crm_campaign", campaign.ID,
		derefOr(campaign.BranchID), nil, safeCampaignAudit(campaign, definition), ""); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = s.publishCampaign(ctx, tx, actor, "vendor.crm.campaign.created", campaign, definition); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	s.metrics.Inc("crm_campaign_create_total", "scope", definition.ScopeType, "objective", definition.Objective)
	return campaign, version, nil
}

// insertCampaignVersion writes one immutable definition revision and its
// cohorts inside the caller's transaction.
//
// The variants are written in the same transaction as the version row that
// declares their total, which is what makes the "exactly 10,000 basis points"
// check on that row a real invariant: versions are never updated, so an
// allocation that was correct when written stays correct forever.
func (s *Service) insertCampaignVersion(ctx context.Context, tx pgx.Tx, actor Actor, campaign Campaign,
	definitionVersion int64, d CampaignDefinition, reason string) (CampaignVersion, error) {
	var capCount, capWindow *int
	if d.Frequency.Enabled {
		capCount, capWindow = &d.Frequency.Cap, &d.Frequency.WindowMinutes
	}
	var startMinute, endMinute *int16
	var timezone *string
	if d.QuietHours.Enabled {
		start, end := int16(d.QuietHours.StartMinute), int16(d.QuietHours.EndMinute)
		zone := d.QuietHours.Timezone
		startMinute, endMinute, timezone = &start, &end, &zone
	}
	total := 0
	if d.ExperimentEnabled {
		total = TotalAllocationBps
	}
	var reasonPtr *string
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		reasonPtr = &trimmed
	}

	version, err := scanCampaignVersion(tx.QueryRow(ctx, `INSERT INTO crm_campaign_versions
		(id,business_id,campaign_id,definition_version,name,objective,scope_type,branch_id,channels,
		 source_segment_id,exclusion_segment_ids,frequency_cap_count,frequency_cap_window_minutes,
		 quiet_hours_enabled,quiet_hours_start_minute,quiet_hours_end_minute,quiet_hours_timezone,
		 service_recovery_suppression_enabled,experiment_enabled,experiment_total_bps,definition_hash,
		 created_by_staff_ref,change_reason)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,NULLIF($22,''),$23)
		RETURNING `+campaignVersionCols,
		newID("crmcv_"), actor.VendorID, campaign.ID, definitionVersion, d.Name, d.Objective, d.ScopeType,
		d.BranchID, d.Channels, d.SourceSegmentID, d.ExclusionSegmentIDs, capCount, capWindow,
		d.QuietHours.Enabled, startMinute, endMinute, timezone, d.ServiceRecoverySuppression,
		d.ExperimentEnabled, total, d.Hash(), actor.StaffID, reasonPtr))
	if err != nil {
		return CampaignVersion{}, err
	}

	for _, variant := range d.orderedVariants() {
		if _, err = tx.Exec(ctx, `INSERT INTO crm_campaign_experiment_variants
			(id,business_id,campaign_version_id,variant_key,display_name,allocation_bps,is_control)
			VALUES($1,$2,$3,$4,$5,$6,$7)`,
			newID("crmcev_"), actor.VendorID, version.ID, variant.VariantKey, variant.DisplayName,
			variant.AllocationBps, variant.IsControl); err != nil {
			return CampaignVersion{}, err
		}
	}
	version.Variants, err = s.loadVariants(ctx, tx, actor.VendorID, version.ID)
	return version, err
}

// UpdateCampaignInput edits a campaign. Scope is immutable: changing it would
// reinterpret every stored version and every snapshot built from them.
type UpdateCampaignInput struct {
	Definition      CampaignDefinitionInput
	ChangeReason    string
	ExpectedVersion int64
}

// UpdateCampaign replaces a campaign's definition, minting a new immutable
// version when anything meaningful changed.
//
// A campaign that is already PREPARED keeps its prepared snapshot and that
// snapshot keeps pointing at the exact version it was built from. The campaign
// simply returns to DRAFT: the new definition has never been prepared, and
// saying otherwise would let an edit inherit an audience it never described.
func (s *Service) UpdateCampaign(ctx context.Context, actor Actor, scope Scope, campaignID string, in UpdateCampaignInput) (Campaign, CampaignVersion, error) {
	if err := s.assertCampaignWrites(); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if in.ExpectedVersion < 1 {
		return Campaign{}, CampaignVersion{}, campaignValidation("expected_version",
			"Expected version is required.", CodeCampaignVersionConflict)
	}
	definition, err := validateCampaignDefinition(in.Definition)
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadCampaignForUpdate(ctx, tx, actor.VendorID, campaignID)
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = s.assertCampaignVisible(scope, current); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = authorizeObjectScope(scope, current.ScopeType, current.BranchID, CodeCampaignScopeDenied); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if current.Status == CampaignArchived {
		return Campaign{}, CampaignVersion{}, errCampaignState("An archived campaign cannot be changed.")
	}
	if current.Version != in.ExpectedVersion {
		return Campaign{}, CampaignVersion{}, errCampaignVersionConflict()
	}
	// Scope is fixed at creation. An edit that moved a business campaign to a
	// branch (or the reverse) would silently reinterpret every prepared
	// audience built from the old scope.
	if definition.ScopeType != current.ScopeType || derefOr(definition.BranchID) != derefOr(current.BranchID) {
		return Campaign{}, CampaignVersion{}, campaignValidation("scope_type",
			"A campaign's scope cannot be changed after creation.", CodeCampaignScopeDenied)
	}
	if err = s.assertAudienceSegmentsUsable(ctx, tx, actor, scope, definition); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}

	existing, err := s.CampaignVersionAt(ctx, tx, actor.VendorID, campaignID, current.CurrentDefinitionVersion)
	if err != nil {
		return Campaign{}, CampaignVersion{}, err
	}

	definitionVersion := current.CurrentDefinitionVersion
	status := current.Status
	changed := existing.DefinitionHash != definition.Hash()
	if changed {
		definitionVersion++
		// The new definition has never been prepared. PREPARING is left alone:
		// a run in flight belongs to version N and finishes against version N.
		if status == CampaignPrepared || status == CampaignPreparationFailed {
			status = CampaignDraft
		}
	}

	updated, err := scanCampaign(tx.QueryRow(ctx, `UPDATE crm_campaigns
		SET name=$1, normalized_name=$2, objective=$3, status=$4, current_definition_version=$5,
			updated_by_staff_ref=NULLIF($6,''), updated_at=now(), version=version+1
		WHERE business_id=$7 AND id=$8 AND version=$9 AND status <> 'ARCHIVED'
		RETURNING `+campaignCols,
		definition.Name, definition.NormalizedName, definition.Objective, status, definitionVersion,
		actor.StaffID, actor.VendorID, campaignID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, CampaignVersion{}, errCampaignVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return Campaign{}, CampaignVersion{}, errCampaignExists()
		}
		return Campaign{}, CampaignVersion{}, err
	}

	version := existing
	if changed {
		reason := strings.TrimSpace(in.ChangeReason)
		if reason == "" {
			reason = "definition updated"
		}
		if version, err = s.insertCampaignVersion(ctx, tx, actor, updated, definitionVersion, definition, reason); err != nil {
			return Campaign{}, CampaignVersion{}, err
		}
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.campaign.updated", "crm_campaign", campaignID,
		derefOr(current.BranchID),
		map[string]any{"name": current.Name, "definition_version": current.CurrentDefinitionVersion, "status": current.Status},
		map[string]any{"name": updated.Name, "definition_version": definitionVersion, "status": updated.Status},
		in.ChangeReason); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = s.publishCampaign(ctx, tx, actor, "vendor.crm.campaign.updated", updated, definition); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Campaign{}, CampaignVersion{}, err
	}
	return updated, version, nil
}

// ArchiveCampaign retires a campaign.
//
// ARCHIVED is terminal. A retired campaign that could come back would come back
// with the audience it froze months ago, which is precisely the stale-audience
// reuse the whole section is built to prevent.
func (s *Service) ArchiveCampaign(ctx context.Context, actor Actor, scope Scope, campaignID string, expectedVersion int64) (Campaign, error) {
	if err := s.assertCampaignWrites(); err != nil {
		return Campaign{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadCampaignForUpdate(ctx, tx, actor.VendorID, campaignID)
	if err != nil {
		return Campaign{}, err
	}
	if err = s.assertCampaignVisible(scope, current); err != nil {
		return Campaign{}, err
	}
	if err = authorizeObjectScope(scope, current.ScopeType, current.BranchID, CodeCampaignScopeDenied); err != nil {
		return Campaign{}, err
	}
	if current.Status == CampaignArchived {
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return Campaign{}, errCampaignVersionConflict()
	}

	updated, err := scanCampaign(tx.QueryRow(ctx, `UPDATE crm_campaigns
		SET status='ARCHIVED', archived_at=now(), updated_by_staff_ref=NULLIF($1,''), updated_at=now(), version=version+1
		WHERE business_id=$2 AND id=$3 AND version=$4 AND status <> 'ARCHIVED' RETURNING `+campaignCols,
		actor.StaffID, actor.VendorID, campaignID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, errCampaignVersionConflict()
	}
	if err != nil {
		return Campaign{}, err
	}
	// A preparation still in flight for a campaign nobody will use is wasted
	// work at best and a resurrected audience at worst.
	if _, err = tx.Exec(ctx, `UPDATE crm_campaign_audience_snapshots
		SET status='SUPERSEDED', completed_at=now(), lease_owner=NULL, lease_expires_at=NULL,
			updated_at=now(), version=version+1
		WHERE business_id=$1 AND campaign_id=$2 AND status='BUILDING'`, actor.VendorID, campaignID); err != nil {
		return Campaign{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.campaign.archived", "crm_campaign", campaignID,
		derefOr(current.BranchID), map[string]any{"status": current.Status},
		map[string]any{"status": CampaignArchived}, ""); err != nil {
		return Campaign{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.campaign.archived", "crm_campaign", campaignID, updated.Version,
		map[string]any{"campaign_ref": campaignID, "status": CampaignArchived, "scope_type": updated.ScopeType,
			"branch_ref": updated.BranchID}); err != nil {
		return Campaign{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Campaign{}, err
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Audience source validation
// ---------------------------------------------------------------------------

// assertAudienceSegmentsUsable checks every segment a definition names.
//
// This is where a campaign's audience boundary is actually enforced. A branch
// campaign that could name a business-wide segment would be a lawful route to
// business-wide customer data for a branch manager who cannot otherwise see it,
// and no amount of later filtering would undo the count it disclosed.
func (s *Service) assertAudienceSegmentsUsable(ctx context.Context, q querier, actor Actor, scope Scope, d CampaignDefinition) error {
	refs := append([]string{d.SourceSegmentID}, d.ExclusionSegmentIDs...)
	for _, ref := range refs {
		segment, err := s.loadSegment(ctx, q, actor.VendorID, ref)
		if errors.Is(err, pgx.ErrNoRows) {
			return errCampaignAudienceMissing()
		}
		if err != nil {
			if isCRMCode(err, CodeSegmentNotFound) {
				return errCampaignAudienceMissing()
			}
			return err
		}
		// Visibility first: an unauthorised segment is reported missing, so the
		// error cannot be used to confirm that it exists.
		if err = s.assertSegmentVisible(scope, segment); err != nil {
			return errCampaignAudienceMissing()
		}
		if segment.Status == SegmentArchived {
			return crmError(apperrors.Conflict("That audience segment is archived.", nil), CodeCampaignAudienceScope)
		}
		// A BRANCH campaign may only reason over its own branch's audience. A
		// BUSINESS campaign is created by a business-wide grant and may use any
		// segment that grant can already see.
		if d.ScopeType == ScopeBranch {
			if segment.ScopeType != ScopeBranch || derefOr(segment.BranchID) != derefOr(d.BranchID) {
				return crmError(apperrors.Forbidden(
					"A branch campaign can only use an audience segment for its own branch.", nil),
					CodeCampaignAudienceScope)
			}
		}
	}
	return nil
}

func errCampaignAudienceMissing() error {
	return crmError(apperrors.NotFound("Audience segment not found.", nil), CodeCampaignAudienceMissing)
}

// isCRMCode reports whether an error carries a specific stable CRM code.
func isCRMCode(err error, code string) bool {
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Details == nil {
		return false
	}
	value, _ := appErr.Details["code"].(string)
	return value == code
}

// ---------------------------------------------------------------------------
// Audit and events
// ---------------------------------------------------------------------------

// safeCampaignAudit is the audit payload: campaign metadata and counts only.
// No target list, no customer reference, no consent payload, no preview sample.
func safeCampaignAudit(c Campaign, d CampaignDefinition) map[string]any {
	return map[string]any{
		"campaign_ref": c.ID, "name": c.Name, "objective": c.Objective, "scope_type": c.ScopeType,
		"branch_ref": c.BranchID, "status": c.Status, "definition_version": c.CurrentDefinitionVersion,
		"channels": d.Channels, "experiment_enabled": d.ExperimentEnabled,
	}
}

func (s *Service) publishCampaign(ctx context.Context, tx pgx.Tx, actor Actor, event string, c Campaign, d CampaignDefinition) error {
	return s.publishTx(ctx, tx, actor, event, "crm_campaign", c.ID, c.Version, map[string]any{
		"campaign_ref": c.ID, "definition_version": c.CurrentDefinitionVersion, "status": c.Status,
		"scope_type": c.ScopeType, "branch_ref": c.BranchID, "objective": c.Objective,
		"channels": d.Channels, "definition_hash": d.Hash(),
	})
}
