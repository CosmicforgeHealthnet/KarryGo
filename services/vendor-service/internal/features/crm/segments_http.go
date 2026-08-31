package crm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 8B permission codes.
//
// Five permissions, not nine: bulk tagging is the same authority as tagging,
// preview and evaluation are the same authority as reading and managing a
// definition, and duplicating them would only create a way for a role to be
// half-granted. Member reads are separate because listing the actual customers
// in an audience is a strictly larger disclosure than reading its definition.
const (
	PermTagsRead           = "vendor.crm.tags.read"
	PermTagsManage         = "vendor.crm.tags.manage"
	PermSegmentsRead       = "vendor.crm.segments.read"
	PermSegmentsManage     = "vendor.crm.segments.manage"
	PermSegmentMembersRead = "vendor.crm.segments.members.read"
)

// SegmentationHandler exposes the Section 8B vendor surface.
type SegmentationHandler struct {
	service     *Service
	idempotency *idempotency.Store
}

// NewSegmentationHandler wires the Section 8B handler.
func NewSegmentationHandler(service *Service, store *idempotency.Store) *SegmentationHandler {
	return &SegmentationHandler{service: service, idempotency: store}
}

func (h *SegmentationHandler) actor(c *gin.Context) (Actor, error) {
	handler := Handler{service: h.service}
	return handler.actor(c)
}

// command runs a state-changing operation behind the shared Idempotency-Key
// store, matching the Section 6/7 command convention exactly: same key and same
// request replays the stored response, same key with a different request is a
// conflict, and a failure releases the key so a genuine retry can run.
func (h *SegmentationHandler) command(c *gin.Context, status int, fn func(Actor, []byte) (any, error)) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Could not read request body.", err))
		return
	}
	key := c.GetHeader("Idempotency-Key")
	endpoint := c.Request.Method + " " + c.FullPath()
	fingerprint := idempotency.Fingerprint(c.Request.Method, c.Request.URL.Path, raw)
	record, err := h.idempotency.Begin(c, actor.VendorID, endpoint, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrFingerprintMismatch) || errors.Is(err, idempotency.ErrInProgress) {
			httpx.RespondError(c, crmError(
				apperrors.Conflict("This idempotency key is already in use for another request.", err),
				"CRM_IDEMPOTENCY_CONFLICT"))
		} else {
			httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", err))
		}
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}
	data, err := fn(actor, raw)
	if err != nil {
		_ = h.idempotency.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	body, err := json.Marshal(gin.H{"success": true, "data": data})
	if err != nil {
		_ = h.idempotency.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	if err = h.idempotency.Complete(c, actor.VendorID, endpoint, key, status, body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Data(status, "application/json", body)
}

func (h *SegmentationHandler) authorized(c *gin.Context, permission string) (Actor, Scope, bool) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}
	scope, err := h.service.Authorize(c, actor, permission)
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}
	return actor, scope, true
}

// ---------------------------------------------------------------------------
// Field registry
// ---------------------------------------------------------------------------

type fieldView struct {
	Key              string   `json:"key"`
	Label            string   `json:"label"`
	Description      string   `json:"description"`
	Type             string   `json:"type"`
	Source           string   `json:"source"`
	Version          int      `json:"version"`
	Scopes           []string `json:"scopes"`
	Operators        []string `json:"operators"`
	Values           []string `json:"values,omitempty"`
	Available        bool     `json:"available"`
	NullableBehavior string   `json:"nullable_behavior"`
	BranchBehavior   string   `json:"branch_behavior"`
	PrivacyBehavior  string   `json:"privacy_behavior"`
	BranchOnly       bool     `json:"branch_only"`
}

// ListSegmentFields returns the closed set of fields a rule may reference.
//
// A builder UI must read this rather than invent field keys: the registry is
// the contract, and a field that is not here cannot be used no matter what a
// client sends. The SQL each field maps to is deliberately not exposed.
func (h *SegmentationHandler) ListSegmentFields(c *gin.Context) {
	if _, _, allowed := h.authorized(c, PermSegmentsRead); !allowed {
		return
	}
	registry := h.service.FieldRegistry()
	fields := registry.Fields()
	items := make([]fieldView, 0, len(fields))
	for _, field := range fields {
		items = append(items, fieldView{
			Key: field.Key, Label: field.Label, Description: field.Description,
			Type: string(field.Type), Source: field.Source, Version: field.Version, Scopes: field.Scopes,
			Operators: field.Operators, Values: field.EnumValues, Available: registry.AvailabilityOf(field),
			NullableBehavior: field.NullableBehavior, BranchBehavior: field.BranchBehavior,
			PrivacyBehavior: field.PrivacyBehavior, BranchOnly: field.BranchOnly,
		})
	}
	limits := h.service.RuleLimitSet()
	ok(c, gin.H{
		"schema_version": RuleSchemaVersion,
		"fields":         items,
		"limits": gin.H{
			"max_depth": limits.MaxDepth, "max_nodes": limits.MaxNodes,
			"max_or_children": limits.MaxOrChildren, "max_in_values": limits.MaxInValues,
			"max_not_depth": limits.MaxNotDepth, "max_tag_references": limits.MaxTagReferences,
		},
		"bulk": gin.H{"default_batch": DefaultBulkBatch, "max_batch": MaxBulkBatch},
	})
}

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

type scopeRequest struct {
	Type      string `json:"type"`
	BranchRef string `json:"branch_ref"`
}

type createTagRequest struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Scope       scopeRequest `json:"scope"`
}

type updateTagRequest struct {
	Name            *string `json:"name"`
	Description     *string `json:"description"`
	ExpectedVersion int64   `json:"expected_version"`
}

type bulkTagRequest struct {
	TagRef      string   `json:"tag_ref"`
	ProfileRefs []string `json:"profile_refs"`
}

type bulkProfileRequest struct {
	ProfileRefs []string `json:"profile_refs"`
}

type commandVersionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

// ListTags returns the tag catalogue.
func (h *SegmentationHandler) ListTags(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermTagsRead)
	if !allowed {
		return
	}
	opts, err := tagListOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	tags, counts, hasMore, err := h.service.ListTags(c, actor, scope, opts)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]TagView, 0, len(tags))
	for _, tag := range tags {
		view := TagViewOf(tag)
		if count, found := counts[tag.ID]; found {
			view.MemberCount = &count
		} else {
			zero := 0
			view.MemberCount = &zero
		}
		items = append(items, view)
	}
	ok(c, gin.H{"items": items, "pagination": gin.H{
		"limit": opts.bounded().Limit, "offset": opts.bounded().Offset, "has_more": hasMore,
	}})
}

// GetTag returns one tag.
func (h *SegmentationHandler) GetTag(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermTagsRead)
	if !allowed {
		return
	}
	tag, err := h.service.GetTag(c, actor, scope, c.Param("tagId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, TagViewOf(tag))
}

// CreateTag defines a tag.
func (h *SegmentationHandler) CreateTag(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTagsManage)
		if err != nil {
			return nil, err
		}
		var request createTagRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		tag, err := h.service.CreateTag(c, actor, scope, CreateTagInput{
			Name: request.Name, Description: request.Description,
			ScopeType: request.Scope.Type, BranchRef: request.Scope.BranchRef,
		})
		if err != nil {
			return nil, err
		}
		return TagViewOf(tag), nil
	})
}

// UpdateTag renames or re-describes a tag.
func (h *SegmentationHandler) UpdateTag(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTagsManage)
		if err != nil {
			return nil, err
		}
		var request updateTagRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		tag, err := h.service.UpdateTag(c, actor, scope, c.Param("tagId"), UpdateTagInput{
			Name: request.Name, Description: request.Description, ExpectedVersion: request.ExpectedVersion,
		})
		if err != nil {
			return nil, err
		}
		return TagViewOf(tag), nil
	})
}

// ArchiveTag retires a tag.
func (h *SegmentationHandler) ArchiveTag(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTagsManage)
		if err != nil {
			return nil, err
		}
		var request commandVersionRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		tag, err := h.service.ArchiveTag(c, actor, scope, c.Param("tagId"), request.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return TagViewOf(tag), nil
	})
}

// BulkAssignTags assigns one tag to many relationships.
func (h *SegmentationHandler) BulkAssignTags(c *gin.Context) { h.bulkTag(c, true) }

// BulkRemoveTags removes one tag from many relationships.
func (h *SegmentationHandler) BulkRemoveTags(c *gin.Context) { h.bulkTag(c, false) }

func (h *SegmentationHandler) bulkTag(c *gin.Context, assign bool) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTagsManage)
		if err != nil {
			return nil, err
		}
		var request bulkTagRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		if strings.TrimSpace(request.TagRef) == "" {
			return nil, tagValidation("tag_ref", "A tag is required.")
		}
		if assign {
			return h.service.BulkAssignTag(c, actor, scope, request.TagRef, request.ProfileRefs)
		}
		return h.service.BulkRemoveTag(c, actor, scope, request.TagRef, request.ProfileRefs)
	})
}

// ---------------------------------------------------------------------------
// Segments
// ---------------------------------------------------------------------------

type createSegmentRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	SegmentType string          `json:"segment_type"`
	Scope       scopeRequest    `json:"scope"`
	Rule        json.RawMessage `json:"rule"`
}

type updateSegmentRequest struct {
	Name            *string         `json:"name"`
	Description     *string         `json:"description"`
	Rule            json.RawMessage `json:"rule"`
	ChangeReason    string          `json:"change_reason"`
	ExpectedVersion int64           `json:"expected_version"`
}

type previewRequest struct {
	Scope       scopeRequest    `json:"scope"`
	Rule        json.RawMessage `json:"rule"`
	SampleLimit int             `json:"sample_limit"`
}

// ListSegments returns the segment catalogue.
func (h *SegmentationHandler) ListSegments(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermSegmentsRead)
	if !allowed {
		return
	}
	opts, err := segmentListOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	segments, hasMore, err := h.service.ListSegments(c, actor, scope, opts)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]SegmentView, 0, len(segments))
	for _, segment := range segments {
		items = append(items, h.service.SegmentViewOf(c, segment, nil))
	}
	ok(c, gin.H{"items": items, "pagination": gin.H{
		"limit": opts.bounded().Limit, "offset": opts.bounded().Offset, "has_more": hasMore,
	}})
}

// GetSegment returns one segment with its current definition.
func (h *SegmentationHandler) GetSegment(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermSegmentsRead)
	if !allowed {
		return
	}
	segment, err := h.service.GetSegment(c, actor, scope, c.Param("segmentId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	version, err := h.service.CurrentVersion(c, h.service.db, actor.VendorID, segment.ID, segment.CurrentDefinitionVersion)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, h.service.SegmentViewOf(c, segment, &version))
}

// CreateSegment defines a segment.
func (h *SegmentationHandler) CreateSegment(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsManage)
		if err != nil {
			return nil, err
		}
		var request createSegmentRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		segment, err := h.service.CreateSegment(c, actor, scope, CreateSegmentInput{
			Name: request.Name, Description: request.Description, SegmentType: request.SegmentType,
			ScopeType: request.Scope.Type, BranchRef: request.Scope.BranchRef, Rule: request.Rule,
		})
		if err != nil {
			return nil, err
		}
		return h.service.SegmentViewOf(c, segment, nil), nil
	})
}

// UpdateSegment edits a segment.
func (h *SegmentationHandler) UpdateSegment(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsManage)
		if err != nil {
			return nil, err
		}
		var request updateSegmentRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		segment, err := h.service.UpdateSegment(c, actor, scope, c.Param("segmentId"), UpdateSegmentInput{
			Name: request.Name, Description: request.Description, Rule: request.Rule,
			ChangeReason: request.ChangeReason, ExpectedVersion: request.ExpectedVersion,
		})
		if err != nil {
			return nil, err
		}
		return h.service.SegmentViewOf(c, segment, nil), nil
	})
}

// PreviewSegment evaluates an unsaved rule.
//
// Preview is a read of hypothetical membership, so it needs the segment read
// permission rather than manage: an analyst may explore audiences without being
// able to create one.
func (h *SegmentationHandler) PreviewSegment(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsRead)
		if err != nil {
			return nil, err
		}
		var request previewRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		return h.service.PreviewSegment(c, actor, scope, PreviewInput{
			ScopeType: request.Scope.Type, BranchRef: request.Scope.BranchRef,
			Rule: request.Rule, SampleLimit: request.SampleLimit,
		})
	})
}

// ActivateSegment moves a draft segment into service.
func (h *SegmentationHandler) ActivateSegment(c *gin.Context) {
	h.setSegmentStatus(c, SegmentActive)
}

// ArchiveSegment retires a segment.
func (h *SegmentationHandler) ArchiveSegment(c *gin.Context) {
	h.setSegmentStatus(c, SegmentArchived)
}

func (h *SegmentationHandler) setSegmentStatus(c *gin.Context, target string) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsManage)
		if err != nil {
			return nil, err
		}
		var request commandVersionRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		segment, err := h.service.SetSegmentStatus(c, actor, scope, c.Param("segmentId"), target, request.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return h.service.SegmentViewOf(c, segment, nil), nil
	})
}

// MaterializeSegment enqueues a membership rebuild.
func (h *SegmentationHandler) MaterializeSegment(c *gin.Context) {
	h.command(c, http.StatusAccepted, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsManage)
		if err != nil {
			return nil, err
		}
		evaluation, err := h.service.RequestMaterialization(c, actor, scope, c.Param("segmentId"))
		if err != nil {
			return nil, err
		}
		return EvaluationViewOf(evaluation), nil
	})
}

// ListSegmentMembers pages a segment's current membership.
func (h *SegmentationHandler) ListSegmentMembers(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermSegmentMembersRead)
	if !allowed {
		return
	}
	segment, err := h.service.GetSegment(c, actor, scope, c.Param("segmentId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	members, hasMore, err := h.service.ListSegmentMembers(c, actor, scope, segment, limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]MemberView, 0, len(members))
	for _, member := range members {
		items = append(items, MemberViewOf(member))
	}
	ok(c, gin.H{
		"items": items,
		"membership": gin.H{
			"evaluation_status":  segment.EvaluationStatus,
			"member_count":       segment.CurrentMemberCount,
			"definition_version": segment.CurrentDefinitionVersion,
			"last_evaluated_at":  segment.LastEvaluatedAt,
			"stale_since":        segment.StaleSince,
		},
		"pagination": gin.H{"limit": limit, "offset": offset, "has_more": hasMore},
	})
}

// BulkAddMembers adds explicit members to a static segment.
func (h *SegmentationHandler) BulkAddMembers(c *gin.Context) { h.bulkMembers(c, true) }

// BulkRemoveMembers removes explicit members from a static segment.
func (h *SegmentationHandler) BulkRemoveMembers(c *gin.Context) { h.bulkMembers(c, false) }

func (h *SegmentationHandler) bulkMembers(c *gin.Context, add bool) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsManage)
		if err != nil {
			return nil, err
		}
		var request bulkProfileRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		return h.service.BulkStaticMembership(c, actor, scope, c.Param("segmentId"), request.ProfileRefs, add)
	})
}

// BulkAddExclusions excludes relationships from a dynamic segment.
func (h *SegmentationHandler) BulkAddExclusions(c *gin.Context) { h.bulkExclusions(c, true) }

// BulkRemoveExclusions lifts manual exclusions.
func (h *SegmentationHandler) BulkRemoveExclusions(c *gin.Context) { h.bulkExclusions(c, false) }

func (h *SegmentationHandler) bulkExclusions(c *gin.Context, add bool) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermSegmentsManage)
		if err != nil {
			return nil, err
		}
		var request bulkProfileRequest
		if err = decodeBody(raw, &request); err != nil {
			return nil, err
		}
		return h.service.BulkExclusion(c, actor, scope, c.Param("segmentId"), request.ProfileRefs, add)
	})
}

// ---------------------------------------------------------------------------
// Request parsing
// ---------------------------------------------------------------------------

// decodeBody rejects unknown members so a misspelled field is a visible error
// rather than a silently ignored instruction.
func decodeBody(raw []byte, target any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return apperrors.BadRequest("The request body could not be read.", err)
	}
	return nil
}

func tagListOptionsFrom(c *gin.Context) (TagListOptions, error) {
	opts := TagListOptions{
		ScopeType: strings.ToUpper(strings.TrimSpace(c.Query("scope_type"))),
		BranchRef: strings.TrimSpace(c.Query("branch_ref")),
		Status:    strings.ToUpper(strings.TrimSpace(c.Query("status"))),
		Search:    strings.TrimSpace(c.Query("search")),
	}
	var violations []apperrors.FieldViolation
	if opts.ScopeType != "" && opts.ScopeType != ScopeBusiness && opts.ScopeType != ScopeBranch {
		violations = append(violations, apperrors.FieldViolation{Field: "scope_type", Message: "Scope must be BUSINESS or BRANCH."})
	}
	if opts.Status != "" && opts.Status != TagActive && opts.Status != TagArchived {
		violations = append(violations, apperrors.FieldViolation{Field: "status", Message: "Status must be ACTIVE or ARCHIVED."})
	}
	if len(opts.Search) > 80 {
		violations = append(violations, apperrors.FieldViolation{Field: "search", Message: "Search text is too long."})
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		return TagListOptions{}, err
	}
	opts.Limit, opts.Offset = limit, offset
	if len(violations) > 0 {
		return TagListOptions{}, apperrors.Validation("Check your details.", violations)
	}
	return opts, nil
}

func segmentListOptionsFrom(c *gin.Context) (SegmentListOptions, error) {
	opts := SegmentListOptions{
		SegmentType:      strings.ToUpper(strings.TrimSpace(c.Query("segment_type"))),
		ScopeType:        strings.ToUpper(strings.TrimSpace(c.Query("scope_type"))),
		BranchRef:        strings.TrimSpace(c.Query("branch_ref")),
		Status:           strings.ToUpper(strings.TrimSpace(c.Query("status"))),
		EvaluationStatus: strings.ToUpper(strings.TrimSpace(c.Query("evaluation_status"))),
		Search:           strings.TrimSpace(c.Query("search")),
	}
	var violations []apperrors.FieldViolation
	if opts.SegmentType != "" && opts.SegmentType != SegmentStatic && opts.SegmentType != SegmentDynamic {
		violations = append(violations, apperrors.FieldViolation{Field: "segment_type", Message: "Segment type must be STATIC or DYNAMIC."})
	}
	if opts.ScopeType != "" && opts.ScopeType != ScopeBusiness && opts.ScopeType != ScopeBranch {
		violations = append(violations, apperrors.FieldViolation{Field: "scope_type", Message: "Scope must be BUSINESS or BRANCH."})
	}
	switch opts.Status {
	case "", SegmentDraft, SegmentActive, SegmentArchived:
	default:
		violations = append(violations, apperrors.FieldViolation{Field: "status", Message: "Unsupported segment status."})
	}
	switch opts.EvaluationStatus {
	case "", EvalNotEvaluated, EvalQueued, EvalRunning, EvalCurrent, EvalStale, EvalFailed:
	default:
		violations = append(violations, apperrors.FieldViolation{Field: "evaluation_status", Message: "Unsupported evaluation status."})
	}
	if len(opts.Search) > 120 {
		violations = append(violations, apperrors.FieldViolation{Field: "search", Message: "Search text is too long."})
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		return SegmentListOptions{}, err
	}
	opts.Limit, opts.Offset = limit, offset
	if len(violations) > 0 {
		return SegmentListOptions{}, apperrors.Validation("Check your details.", violations)
	}
	return opts, nil
}

func pageFrom(c *gin.Context) (int, int, error) {
	limit, offset := DefaultPageSize, 0
	var violations []apperrors.FieldViolation
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > MaxPageSize {
			violations = append(violations, apperrors.FieldViolation{
				Field: "limit", Message: "Limit must be from 1 to " + itoa(MaxPageSize) + ".",
			})
		} else {
			limit = value
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			violations = append(violations, apperrors.FieldViolation{Field: "offset", Message: "Offset must be zero or greater."})
		} else {
			offset = value
		}
	}
	if len(violations) > 0 {
		return 0, 0, apperrors.Validation("Check your details.", violations)
	}
	return limit, offset, nil
}

// RegisterSegmentationRoutes wires the Section 8B surface.
//
// Nothing is registered when segmentation is disabled, so a disabled feature has
// no attack surface at all rather than routes that return 503.
func RegisterSegmentationRoutes(group *gin.RouterGroup, handler *SegmentationHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.SegmentationEnabled {
		return
	}

	crm := group.Group("/crm")
	crm.GET("/segment-fields", handler.ListSegmentFields)

	tags := crm.Group("/tags")
	tags.GET("", handler.ListTags)
	tags.POST("", handler.CreateTag)
	tags.POST("/bulk-assign", handler.BulkAssignTags)
	tags.POST("/bulk-remove", handler.BulkRemoveTags)
	tags.GET("/:tagId", handler.GetTag)
	tags.PATCH("/:tagId", handler.UpdateTag)
	tags.POST("/:tagId/archive", handler.ArchiveTag)

	segments := crm.Group("/segments")
	segments.GET("", handler.ListSegments)
	segments.POST("", handler.CreateSegment)
	segments.POST("/preview", handler.PreviewSegment)
	segments.GET("/:segmentId", handler.GetSegment)
	segments.PATCH("/:segmentId", handler.UpdateSegment)
	segments.POST("/:segmentId/activate", handler.ActivateSegment)
	segments.POST("/:segmentId/archive", handler.ArchiveSegment)
	segments.POST("/:segmentId/materialize", handler.MaterializeSegment)
	segments.GET("/:segmentId/members", handler.ListSegmentMembers)
	segments.POST("/:segmentId/members/bulk-add", handler.BulkAddMembers)
	segments.POST("/:segmentId/members/bulk-remove", handler.BulkRemoveMembers)
	segments.POST("/:segmentId/exclusions/bulk-add", handler.BulkAddExclusions)
	segments.POST("/:segmentId/exclusions/bulk-remove", handler.BulkRemoveExclusions)
}
