package http

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
	approvalmodels "cosmicforge/logistics/services/pricing-service/internal/features/approvals/models"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	policyusecases "cosmicforge/logistics/services/pricing-service/internal/features/policies/usecases"
	rolloutmodels "cosmicforge/logistics/services/pricing-service/internal/features/rollouts/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Approvals is the review queue this handler exposes. Declared as a port so this
// package never imports the approvals feature's repositories.
type Approvals interface {
	ListPending(ctx context.Context, limit int) ([]approvalmodels.Request, error)
}

// Rollouts is the switch registry, likewise a port.
type Rollouts interface {
	RecordRollout(ctx context.Context, rollout rolloutmodels.Rollout) error
	ModeFor(ctx context.Context, scope, capability, marketCode string) (string, error)
	RecordFreeze(ctx context.Context, freeze rolloutmodels.Freeze) error
	FreezeStateFor(ctx context.Context, marketCode string) (rolloutmodels.FreezeState, error)
	ListRollouts(ctx context.Context) ([]rolloutmodels.Rollout, error)
}

// Handler serves the administrative routes.
type Handler struct {
	policies  *policyusecases.Service
	approvals Approvals
	rollouts  Rollouts
}

// NewHandler builds the handler.
func NewHandler(policies *policyusecases.Service, approvals Approvals, rollouts Rollouts) *Handler {
	return &Handler{policies: policies, approvals: approvals, rollouts: rollouts}
}

// CreateDraft stores a new policy version as a draft.
func (h *Handler) CreateDraft(c *gin.Context) {
	var request createDraftRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	version, err := h.policies.CreateDraft(c.Request.Context(), policyusecases.DraftInput{
		Kind:           request.Kind,
		GeographyLevel: request.GeographyLevel,
		GeographyRef:   request.GeographyRef,
		SchemaVersion:  request.SchemaVersion,
		Body:           request.Body,
		PerformedBy:    request.PerformedBy,
		ActorCaller:    servicetrust.CallerFrom(c),
		ChangeReason:   request.Reason,
		CloneFrom:      request.CloneFrom,
		RequestID:      httpx.GetRequestID(c),
	})
	if err != nil {
		respond(c, err)
		return
	}
	created(c, toVersionResponse(version))
}

// UpdateDraft replaces a draft's body.
func (h *Handler) UpdateDraft(c *gin.Context) {
	var request updateDraftRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	version, err := h.policies.UpdateDraft(c.Request.Context(), c.Param("id"), request.Body,
		request.PerformedBy, servicetrust.CallerFrom(c), request.Reason, httpx.GetRequestID(c))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, toVersionResponse(version))
}

// Submit opens an approval request for a draft.
func (h *Handler) Submit(c *gin.Context) {
	var request decideRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	approvalID, err := h.policies.Submit(c.Request.Context(), c.Param("id"),
		request.PerformedBy, servicetrust.CallerFrom(c), request.Reason, httpx.GetRequestID(c))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{"approval_request_id": approvalID, "status": policymodels.StatusSubmitted})
}

// Approve accepts a submitted version. It does not make it live.
func (h *Handler) Approve(c *gin.Context) {
	h.decide(c, true)
}

// Reject returns a submitted version to its author.
func (h *Handler) Reject(c *gin.Context) {
	h.decide(c, false)
}

func (h *Handler) decide(c *gin.Context, approve bool) {
	var request decideRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	action := h.policies.Reject
	status := policymodels.StatusRejected
	if approve {
		action = h.policies.Approve
		status = policymodels.StatusApproved
	}
	if err := action(c.Request.Context(), c.Param("id"), request.PerformedBy,
		servicetrust.CallerFrom(c), request.Reason, httpx.GetRequestID(c)); err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{"status": status})
}

// Activate makes an approved version live, now or at a scheduled moment.
func (h *Handler) Activate(c *gin.Context) {
	var request activateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}

	var effectiveFrom time.Time
	if request.EffectiveFrom != "" {
		parsed, err := time.Parse(time.RFC3339, request.EffectiveFrom)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("effective_from must be an RFC 3339 timestamp.", err))
			return
		}
		effectiveFrom = parsed.UTC()
	}

	activation, err := h.policies.Activate(c.Request.Context(), policyusecases.ActivateInput{
		VersionID:     c.Param("id"),
		EffectiveFrom: effectiveFrom,
		PerformedBy:   request.PerformedBy,
		ActorCaller:   servicetrust.CallerFrom(c),
		Reason:        request.Reason,
		RequestID:     httpx.GetRequestID(c),
		Rollback:      request.Rollback,
	})
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{
		"activation_id":  activation.ID,
		"version_id":     activation.VersionID,
		"effective_from": activation.EffectiveFrom.UTC().Format(time.RFC3339),
		"action":         activation.Action,
	})
}

// GetVersion returns one version and its body.
func (h *Handler) GetVersion(c *gin.Context) {
	version, err := h.policies.GetVersion(c.Request.Context(), c.Param("id"))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, toVersionResponse(version))
}

// ListVersions returns a lineage's editions.
func (h *Handler) ListVersions(c *gin.Context) {
	versions, err := h.policies.ListVersions(c.Request.Context(), c.Param("id"), 50)
	if err != nil {
		respond(c, err)
		return
	}
	out := make([]versionResponse, 0, len(versions))
	for _, version := range versions {
		out = append(out, toVersionResponse(version))
	}
	ok(c, gin.H{"versions": out})
}

// Resolve answers "which version would price this place right now", which is the
// first question asked when a fare looks wrong.
func (h *Handler) Resolve(c *gin.Context) {
	var request resolveRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the query.", err))
		return
	}
	asOf := time.Now().UTC()
	if request.AsOf != "" {
		parsed, err := time.Parse(time.RFC3339, request.AsOf)
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("as_of must be an RFC 3339 timestamp.", err))
			return
		}
		asOf = parsed.UTC()
	}

	resolution, err := h.policies.Resolve(c.Request.Context(), request.Kind, policymodels.GeographyChain{
		ZoneID: request.ZoneID, MarketID: request.MarketID,
		StateID: request.StateID, CountryCode: request.CountryCode,
	}, asOf)
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{
		"policy_id":       resolution.Policy.ID,
		"geography_level": resolution.Policy.GeographyLevel,
		"geography_ref":   resolution.Policy.GeographyRef,
		"version":         toVersionResponse(resolution.Version),
		"activation_id":   resolution.Activation.ID,
		"effective_from":  resolution.Activation.EffectiveFrom.UTC().Format(time.RFC3339),
		"resolved_as_of":  asOf.Format(time.RFC3339),
	})
}

// Simulate prices a request against a specific version WITHOUT storing anything.
//
// This is how a rate change is checked before anyone approves it: run it on real
// past trips and see what would have changed.
func (h *Handler) Simulate(c *gin.Context) {
	var request simulateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}

	version, err := h.policies.GetVersion(c.Request.Context(), request.VersionID)
	if err != nil {
		respond(c, err)
		return
	}
	tariff, err := engine.DecodeTariff(version.Kind, version.SchemaVersion, version.Body)
	if err != nil {
		respond(c, err)
		return
	}

	commission, commissionSource, err := h.commissionFor(c, request, version.PolicyID)
	if err != nil {
		respond(c, err)
		return
	}

	result, err := engine.Evaluate(engine.Request{
		Scope:                  engine.Scope(request.Scope),
		Channel:                engine.Channel(request.Channel),
		Class:                  request.Class,
		DistanceMeters:         request.DistanceMeters,
		DurationSeconds:        request.DurationSeconds,
		TrafficDurationSeconds: request.TrafficDurationSeconds,
		RouteConfidence:        request.RouteConfidence,
		RouteDegraded:          request.RouteDegraded,
		PackageSize:            request.PackageSize,
		ServiceTier:            request.ServiceTier,
		WeightGrams:            request.WeightGrams,
		Helpers:                request.Helpers,
		AsOf:                   time.Now().UTC(),
	}, engine.PolicySet{Tariff: tariff, Commission: commission})
	if err != nil {
		respond(c, err)
		return
	}

	ok(c, gin.H{
		"simulated":         true,
		"version_id":        version.ID,
		"version_status":    version.Status,
		"commission_source": commissionSource,
		"result":            result,
	})
}

// commissionFor loads the commission a simulation should use.
//
// With no commission named it resolves the one ACTIVE for the same place as the
// tariff, because an operator simulating a fare wants the real split. Falling
// back to zero would quietly show the provider receiving the whole fare, which is
// exactly the kind of misleading number this service exists to prevent -- so when
// nothing can be resolved the response says so rather than implying 0%.
func (h *Handler) commissionFor(c *gin.Context, request simulateRequest, tariffPolicyID string) (engine.CommissionV1, string, error) {
	versionID := request.CommissionVersionID
	source := "explicit"

	if versionID == "" {
		policy, err := h.policies.GetPolicy(c.Request.Context(), tariffPolicyID)
		if err != nil {
			return engine.CommissionV1{}, "", err
		}
		resolution, err := h.policies.Resolve(c.Request.Context(), engine.KindCommission,
			chainFor(policy.GeographyLevel, policy.GeographyRef), time.Now().UTC())
		if err != nil {
			// No active commission for this place: price the economic fare and
			// label the gap instead of pretending the fee is zero.
			return engine.CommissionV1{
				SchemaVersion: 1,
				AppliedTo:     engine.CommissionOnCustomerPayable,
				Rounding:      "FLOOR",
			}, "none_active_economic_fare_only", nil
		}
		versionID = resolution.Version.ID
		source = "active"
	}

	version, err := h.policies.GetVersion(c.Request.Context(), versionID)
	if err != nil {
		return engine.CommissionV1{}, "", err
	}
	body, err := engine.Decode(version.Kind, version.SchemaVersion, version.Body)
	if err != nil {
		return engine.CommissionV1{}, "", err
	}
	commission, isCommission := body.(engine.CommissionV1)
	if !isCommission {
		return engine.CommissionV1{}, "", apperrors.BadRequest("commission_version_id is not a commission policy.", nil)
	}
	return commission, source + ":" + versionID, nil
}

// chainFor turns one policy's geography into a single-candidate chain.
func chainFor(level, ref string) policymodels.GeographyChain {
	switch level {
	case policymodels.LevelZone:
		return policymodels.GeographyChain{ZoneID: ref}
	case policymodels.LevelMarket:
		return policymodels.GeographyChain{MarketID: ref}
	case policymodels.LevelState:
		return policymodels.GeographyChain{StateID: ref}
	default:
		return policymodels.GeographyChain{CountryCode: ref}
	}
}

// ListApprovals returns the review queue.
func (h *Handler) ListApprovals(c *gin.Context) {
	pending, err := h.approvals.ListPending(c.Request.Context(), 50)
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{"pending": pending})
}

// SetRollout moves a market between LEGACY, SHADOW and CENTRAL.
func (h *Handler) SetRollout(c *gin.Context) {
	var request rolloutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	if err := rolloutmodels.ValidateRollout(request.Scope, request.Capability,
		request.MarketCode, request.Mode, request.PerformedBy, request.Reason); err != nil {
		respond(c, err)
		return
	}
	if err := h.rollouts.RecordRollout(c.Request.Context(), rolloutmodels.Rollout{
		ID: ids.New(ids.PrefixRollout), Scope: request.Scope, Capability: request.Capability,
		MarketCode: request.MarketCode, Mode: request.Mode,
		PerformedBy: request.PerformedBy, Reason: request.Reason,
	}); err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{"scope": request.Scope, "capability": request.Capability,
		"market_code": request.MarketCode, "mode": request.Mode})
}

// ListRollouts shows which markets are switched where.
func (h *Handler) ListRollouts(c *gin.Context) {
	rollouts, err := h.rollouts.ListRollouts(c.Request.Context())
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{"rollouts": rollouts})
}

// Freeze holds a market's prices still. Freezing is protective, so one operator
// may do it; unfreezing is the act that lets prices move again.
func (h *Handler) Freeze(c *gin.Context) {
	h.setFreeze(c, rolloutmodels.ActionFreeze)
}

// Unfreeze lets a market's prices move again.
func (h *Handler) Unfreeze(c *gin.Context) {
	h.setFreeze(c, rolloutmodels.ActionUnfreeze)
}

func (h *Handler) setFreeze(c *gin.Context, action string) {
	var request freezeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
		return
	}
	if request.PerformedBy == "" || request.Reason == "" {
		httpx.Abort(c, apperrors.BadRequest("performed_by and reason are required.", nil))
		return
	}
	if err := h.rollouts.RecordFreeze(c.Request.Context(), rolloutmodels.Freeze{
		ID: ids.New(ids.PrefixFreeze), MarketCode: c.Param("code"), Action: action,
		PerformedBy: request.PerformedBy, Reason: request.Reason,
	}); err != nil {
		respond(c, err)
		return
	}
	ok(c, gin.H{"market_code": c.Param("code"), "action": action})
}

// FreezeState reports whether a market is frozen.
func (h *Handler) FreezeState(c *gin.Context) {
	state, err := h.rollouts.FreezeStateFor(c.Request.Context(), c.Param("code"))
	if err != nil {
		respond(c, err)
		return
	}
	response := gin.H{"market_code": c.Param("code"), "frozen": state.Frozen}
	if state.Frozen {
		response["since"] = state.Since.UTC().Format(time.RFC3339)
		response["reason"] = state.Reason
	}
	ok(c, response)
}

func toVersionResponse(version policymodels.Version) versionResponse {
	return versionResponse{
		ID:            version.ID,
		PolicyID:      version.PolicyID,
		VersionNo:     version.VersionNo,
		Kind:          version.Kind,
		SchemaVersion: version.SchemaVersion,
		Status:        version.Status,
		Body:          json.RawMessage(version.Body),
		BodyHash:      version.BodyHash,
		CreatedBy:     version.CreatedBy,
		ChangeReason:  version.ChangeReason,
		CreatedAt:     version.CreatedAt.UTC().Format(time.RFC3339),
	}
}
