package crm

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 8A permission codes.
const (
	PermCustomersRead   = "vendor.crm.customers.read"
	PermCustomersManage = "vendor.crm.customers.manage"
	PermCustomer360Read = "vendor.crm.customer360.read"
	PermConsentRead     = "vendor.crm.consent.read"
	PermPrivacyRead     = "vendor.crm.privacy.read"
)

// Handler exposes the vendor-facing CRM read surface.
//
// The surface is read-oriented on purpose. There is no create-customer route,
// no consent mutation, no privacy mutation and no merge route: those are
// authority decisions, and exposing them to vendor staff would make the
// authority boundary advisory rather than real.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) actor(c *gin.Context) (Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	return Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
	}, nil
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// ListCustomers returns relationships visible to the caller's scope.
func (h *Handler) ListCustomers(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	scope, err := h.service.Authorize(c, actor, PermCustomersRead)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	opts, err := listOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	profiles, hasMore, err := h.service.ListProfiles(c, actor, scope, opts)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	items := make([]gin.H, 0, len(profiles))
	for _, profile := range profiles {
		consent, consentErr := h.service.ConsentSummaryFor(c, actor, profile)
		if consentErr != nil {
			httpx.RespondError(c, consentErr)
			return
		}
		summary := h.service.commercialSummary(profile, nil, scope)
		if !scope.Global {
			// A branch-scoped caller's list figures must come from their own
			// branches, so they are recomputed per row rather than copied from
			// the business-wide projection.
			relations, relErr := h.service.branchRelations(c, h.service.db, actor.VendorID, profile.ID)
			if relErr != nil {
				httpx.RespondError(c, relErr)
				return
			}
			visible := relations[:0:0]
			for _, relation := range relations {
				if containsString(scope.BranchIDs, relation.BranchID) {
					visible = append(visible, relation)
				}
			}
			summary = h.service.commercialSummary(profile, visible, scope)
		}
		items = append(items, gin.H{
			"profile":               ViewOf(profile),
			"commercial":            summary,
			"marketing_eligibility": marketingEligibilitySummary(consent),
		})
	}
	ok(c, gin.H{"items": items, "pagination": gin.H{
		"limit": opts.bounded().Limit, "offset": opts.bounded().Offset, "has_more": hasMore,
	}})
}

// GetCustomer360 returns the composed relationship view.
func (h *Handler) GetCustomer360(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	scope, err := h.service.Authorize(c, actor, PermCustomer360Read)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	profile, err := h.service.GetProfile(c, actor, scope, c.Param("profileId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	view, err := h.service.Customer360For(c, actor, scope, profile)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, view)
}

// GetConsent returns the projected consent and eligibility view.
func (h *Handler) GetConsent(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	scope, err := h.service.Authorize(c, actor, PermConsentRead)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	profile, err := h.service.GetProfile(c, actor, scope, c.Param("profileId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	summary, err := h.service.ConsentSummaryFor(c, actor, profile)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, summary)
}

// GetPrivacy returns the projected privacy state.
func (h *Handler) GetPrivacy(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	scope, err := h.service.Authorize(c, actor, PermPrivacyRead)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	profile, err := h.service.GetProfile(c, actor, scope, c.Param("profileId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	summary, err := h.service.PrivacySummaryFor(c, actor, profile)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, summary)
}

// Archive hides an inactive relationship from ordinary working views.
func (h *Handler) Archive(c *gin.Context) { h.setArchived(c, true) }

// Restore returns an archived relationship to the active working set.
func (h *Handler) Restore(c *gin.Context) { h.setArchived(c, false) }

func (h *Handler) setArchived(c *gin.Context, archived bool) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	scope, err := h.service.Authorize(c, actor, PermCustomersManage)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	expectedVersion, err := expectedVersionFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	profile, err := h.service.SetArchived(c, actor, scope, c.Param("profileId"), archived, expectedVersion)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, ViewOf(profile))
}

// marketingEligibilitySummary collapses per-channel eligibility into the
// compact form the list view needs, without losing the reason.
func marketingEligibilitySummary(consent ConsentSummary) gin.H {
	eligible := make([]string, 0, len(consent.Marketing))
	for _, channel := range MarketingChannels {
		if entry, found := consent.Marketing[channel]; found && entry.Eligible {
			eligible = append(eligible, string(channel))
		}
	}
	return gin.H{
		"integration_available": consent.IntegrationAvailable,
		"eligible_channels":     eligible,
		"any_channel_eligible":  len(eligible) > 0,
	}
}

func expectedVersionFrom(c *gin.Context) (int64, error) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("expected_version"))
	}
	if raw == "" {
		return 0, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "expected_version", Message: "Expected version is required."},
		})
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "expected_version", Message: "Expected version must be a positive number."},
		})
	}
	return value, nil
}

func listOptionsFrom(c *gin.Context) (ListOptions, error) {
	var violations []apperrors.FieldViolation
	opts := ListOptions{
		SubjectType:               strings.TrimSpace(c.Query("subject_type")),
		Status:                    strings.TrimSpace(c.Query("status")),
		PrivacyState:              strings.TrimSpace(c.Query("privacy_state")),
		BranchRef:                 strings.TrimSpace(c.Query("branch_ref")),
		CapabilityType:            strings.TrimSpace(c.Query("capability_type")),
		CommercialProjectionState: strings.TrimSpace(c.Query("commercial_projection_state")),
		AcquisitionSourceType:     strings.TrimSpace(c.Query("acquisition_source_type")),
		Sort:                      strings.TrimSpace(c.Query("sort")),
		Direction:                 strings.TrimSpace(c.Query("direction")),
	}

	if opts.SubjectType != "" && !IsValidSubjectType(opts.SubjectType) {
		violations = append(violations, apperrors.FieldViolation{Field: "subject_type", Message: "Unsupported subject type."})
	}
	switch ProfileStatus(opts.Status) {
	case "", StatusActive, StatusArchived:
	default:
		violations = append(violations, apperrors.FieldViolation{Field: "status", Message: "Unsupported status."})
	}
	// REDACTED is deliberately not accepted: redacted relationships are not
	// part of the ordinary working set and cannot be filtered into view.
	switch PrivacyState(opts.PrivacyState) {
	case "", PrivacyActive, PrivacyRestricted:
	default:
		violations = append(violations, apperrors.FieldViolation{Field: "privacy_state", Message: "Unsupported privacy state filter."})
	}
	if opts.Sort != "" {
		if _, allowed := allowedProfileSorts[opts.Sort]; !allowed {
			violations = append(violations, apperrors.FieldViolation{Field: "sort", Message: "Unsupported sort field."})
		}
	}
	if opts.Direction != "" && strings.ToLower(opts.Direction) != "asc" && strings.ToLower(opts.Direction) != "desc" {
		violations = append(violations, apperrors.FieldViolation{Field: "direction", Message: "Direction must be asc or desc."})
	}

	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			violations = append(violations, apperrors.FieldViolation{Field: "limit", Message: "Limit must be from 1 to 100."})
		} else {
			opts.Limit = value
		}
	}
	if raw := c.Query("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			violations = append(violations, apperrors.FieldViolation{Field: "offset", Message: "Offset must be zero or greater."})
		} else {
			opts.Offset = value
		}
	}

	opts.MinCompletedOrders, violations = intQuery(c, "min_completed_orders", violations)
	opts.MaxCompletedOrders, violations = intQuery(c, "max_completed_orders", violations)
	opts.MinNetSpendMinor, violations = int64Query(c, "min_net_spend_minor", violations)
	opts.MaxNetSpendMinor, violations = int64Query(c, "max_net_spend_minor", violations)
	opts.LastOrderAfter, violations = timeQuery(c, "last_order_after", violations)
	opts.LastOrderBefore, violations = timeQuery(c, "last_order_before", violations)

	if len(violations) > 0 {
		return ListOptions{}, apperrors.Validation("Check your details.", violations)
	}
	return opts, nil
}

func intQuery(c *gin.Context, name string, violations []apperrors.FieldViolation) (*int, []apperrors.FieldViolation) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, violations
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return nil, append(violations, apperrors.FieldViolation{Field: name, Message: "Value must be zero or greater."})
	}
	return &value, violations
}

func int64Query(c *gin.Context, name string, violations []apperrors.FieldViolation) (*int64, []apperrors.FieldViolation) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, violations
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return nil, append(violations, apperrors.FieldViolation{Field: name, Message: "Value must be zero or greater."})
	}
	return &value, violations
}

func timeQuery(c *gin.Context, name string, violations []apperrors.FieldViolation) (*time.Time, []apperrors.FieldViolation) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, violations
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, append(violations, apperrors.FieldViolation{Field: name, Message: "Value must be an RFC3339 timestamp."})
	}
	return &value, violations
}

// RegisterRoutes wires the vendor-facing CRM surface under the authenticated
// API group. Nothing is registered when the feature is disabled, so a
// disabled CRM has no attack surface at all rather than routes that 503.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler, flags Flags) {
	if !flags.CRMEnabled {
		return
	}
	customers := group.Group("/crm/customers")
	customers.GET("", handler.ListCustomers)
	customers.GET("/:profileId", handler.GetCustomer360)
	customers.GET("/:profileId/consent", handler.GetConsent)
	customers.GET("/:profileId/privacy", handler.GetPrivacy)
	customers.POST("/:profileId/archive", handler.Archive)
	customers.POST("/:profileId/restore", handler.Restore)
}
