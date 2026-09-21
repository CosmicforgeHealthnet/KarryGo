package supportusecases

// Section 15: context enrichment and action routing.
//
// Two rules govern everything here.
//
// Support reads operational truth from its OWNER, fresh, and keeps only a
// snapshot of what it saw. It never stores a trip or mission state of its own.
//
// Support ASKS; the owner DECIDES. An action request is delivered to the
// owning service through a contract that owner already exposes, and the owner
// validates it and may refuse. Where no such contract exists, the action is
// not offered -- inventing one on the owner's behalf would be Support
// acquiring authority over another service's state.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/supportcase"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
)

// actionRoute is one action Support may ask one owner for.
type actionRoute struct {
	targetService string
	deliverer     supportclients.ActionDeliverer
	// requiredParameters must be present and non-empty. Their VALUES are the
	// owner's to validate: Support does not carry a copy of another service's
	// vocabulary that could drift from the real one.
	requiredParameters []string
}

// unsupportedReason explains, precisely, why an action is not offered.
//
// Every entry names the owner and what it lacks, so "why can't support do X?"
// has an answer that points at a real contract rather than at this service.
var unsupportedReason = map[string]string{
	supportmodels.ActionRequestRecoveryEscalation + "/" + supportcase.ContextTaxiBooking:      "driver-taxi-service exposes no recovery request contract for passenger bookings",
	supportmodels.ActionRequestRecoveryEscalation + "/" + supportcase.ContextTaxiGoodsMission: "driver-taxi-service restricts Taxi Goods recovery to its operations caller",
	supportmodels.ActionRequestRecoveryEscalation + "/" + supportcase.ContextDispatchMission:  "driver-dispatch-delivery-service exposes no recovery-start contract",
	supportmodels.ActionRequestRefundReview:                                                   "refunds are decided by the owning commerce workflow through the existing case resolution; Support holds no money authority",
}

// NetworkContextService implements Section 15.
type NetworkContextService struct {
	support  *SupportService
	store    *supportrepositories.NetworkContextStore
	contexts *supportclients.ContextRegistry
	routes   map[string]actionRoute
	now      func() time.Time
}

// NetworkContextOptions configures the service.
type NetworkContextOptions struct {
	Support  *SupportService
	Store    *supportrepositories.NetworkContextStore
	Contexts *supportclients.ContextRegistry
	// HaulingRecovery delivers recovery escalations to Hauling. Nil means the
	// action is not offered on this deployment.
	HaulingRecovery supportclients.ActionDeliverer
	Now             func() time.Time
}

// NewNetworkContextService builds the service.
func NewNetworkContextService(options NetworkContextOptions) *NetworkContextService {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	routes := map[string]actionRoute{}
	if options.HaulingRecovery != nil {
		routes[supportmodels.ActionRequestRecoveryEscalation+"/"+supportcase.ContextHaulageBooking] =
			actionRoute{targetService: "driver-hauling-service", deliverer: options.HaulingRecovery,
				requiredParameters: []string{"trigger_code"}}
	}
	return &NetworkContextService{support: options.Support, store: options.Store,
		contexts: options.Contexts, routes: routes, now: now}
}

// AttachContextInput attaches references to a complaint.
type AttachContextInput struct {
	ComplaintID   string
	RequesterType supportmodels.ComplainantType
	RequesterID   string
	Admin         bool
	Refs          supportmodels.ContextRefs
}

// AttachContext records what a complaint is about.
//
// A complainant may attach context to their own complaint; an agent to any.
// Ownership is checked through the same path every other complaint read uses,
// so a complainant cannot attach context to somebody else's case.
func (s *NetworkContextService) AttachContext(ctx context.Context,
	input AttachContextInput) (supportmodels.ContextRefs, error) {
	if err := s.authorize(ctx, input.ComplaintID, input.RequesterType, input.RequesterID, input.Admin); err != nil {
		return supportmodels.ContextRefs{}, err
	}
	refs := normalizeRefs(input.Refs)
	if refs.ContextType != "" && !supportcase.ValidContextType(refs.ContextType) {
		return supportmodels.ContextRefs{}, apperrors.Validation("Check the complaint context.",
			[]apperrors.FieldViolation{{Field: "context_type", Message: "Unsupported context type."}})
	}
	if refs.ContextType != "" && refs.PrimaryRef() == "" {
		return supportmodels.ContextRefs{}, apperrors.Validation("Check the complaint context.",
			[]apperrors.FieldViolation{{Field: "mission_ref",
				Message: "A context type needs the reference of the job it names."}})
	}
	if err := s.store.SetContextRefs(ctx, input.ComplaintID, refs); err != nil {
		return supportmodels.ContextRefs{}, mapNetworkErr(err)
	}
	return refs, nil
}

// Enrich reads the job's current context from its owner and stores a snapshot.
//
// The owner is asked about the COMPLAINANT, not about whoever is asking. So an
// agent enriching a case sees exactly what the complainant is party to, and a
// complaint cannot be used to read a trip the complainant was never on.
func (s *NetworkContextService) Enrich(ctx context.Context, complaintID,
	correlationID string) (supportmodels.ContextSnapshot, error) {
	complaint, err := s.support.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return supportmodels.ContextSnapshot{}, err
	}
	refs, err := s.store.GetContextRefs(ctx, complaintID)
	if err != nil {
		return supportmodels.ContextSnapshot{}, mapNetworkErr(err)
	}
	if refs.ContextType == "" || refs.PrimaryRef() == "" {
		return supportmodels.ContextSnapshot{}, apperrors.Validation(
			"This complaint names no job to look up.", nil)
	}
	reader, owner, ok := s.contexts.ReaderFor(refs.ContextType)
	if !ok {
		return supportmodels.ContextSnapshot{}, apperrors.Unavailable(
			"The service that owns this job is not configured for support context.", nil)
	}
	observed, err := reader.Read(ctx, supportcase.ContextRequest{
		ContextType: refs.ContextType, SourceRef: refs.PrimaryRef(),
		ExpectedActorRef: complaint.ComplainantID,
	})
	switch {
	case errors.Is(err, supportcase.ErrContextNotFound):
		return supportmodels.ContextSnapshot{}, apperrors.NotFound(
			"The owning service holds no such job for this complainant.", nil)
	case err != nil:
		// Owner unavailable is NOT "no such job". The complaint is still about
		// a real trip; the agent just cannot see it right now.
		return supportmodels.ContextSnapshot{}, apperrors.Unavailable(
			"The owning service could not be reached.", nil)
	}
	return s.store.AppendSnapshot(ctx, supportmodels.ContextSnapshot{
		ComplaintID: complaintID, SourceService: owner, SourceRef: refs.PrimaryRef(),
		SourceVersion: observed.SourceVersion, Context: observed, CorrelationID: correlationID,
	})
}

// ListSnapshots returns what Support has been shown about a complaint.
func (s *NetworkContextService) ListSnapshots(ctx context.Context,
	complaintID string) ([]supportmodels.ContextSnapshot, error) {
	if _, err := s.support.repo.GetComplaintByID(ctx, complaintID); err != nil {
		return nil, err
	}
	return s.store.ListSnapshots(ctx, complaintID)
}

// RequestActionInput asks an owner to act.
type RequestActionInput struct {
	ComplaintID    string
	ActionCode     string
	Parameters     map[string]string
	IdempotencyKey string
	RequestedBy    string
	CorrelationID  string
}

// RequestAction records the request once and delivers it to its owner.
func (s *NetworkContextService) RequestAction(ctx context.Context,
	input RequestActionInput) (supportmodels.ActionRequest, error) {
	actionCode := strings.ToUpper(strings.TrimSpace(input.ActionCode))
	key := strings.TrimSpace(input.IdempotencyKey)
	requestedBy := strings.TrimSpace(input.RequestedBy)
	if key == "" {
		return supportmodels.ActionRequest{}, apperrors.Validation("An idempotency key is required.",
			[]apperrors.FieldViolation{{Field: "Idempotency-Key", Message: "Required."}})
	}
	if requestedBy == "" {
		return supportmodels.ActionRequest{}, apperrors.Validation("The requesting agent is required.", nil)
	}
	if _, err := s.support.repo.GetComplaintByID(ctx, input.ComplaintID); err != nil {
		return supportmodels.ActionRequest{}, err
	}
	refs, err := s.store.GetContextRefs(ctx, input.ComplaintID)
	if err != nil {
		return supportmodels.ActionRequest{}, mapNetworkErr(err)
	}
	route, ok := s.routes[actionCode+"/"+refs.ContextType]
	if !ok {
		reason := unsupportedReason[actionCode+"/"+refs.ContextType]
		if reason == "" {
			reason = unsupportedReason[actionCode]
		}
		if reason == "" {
			reason = "the owning service exposes no contract for this action"
		}
		return supportmodels.ActionRequest{}, apperrors.New(422, apperrors.Code("action_not_supported_by_owner"),
			"This action cannot be requested: "+reason+".", nil)
	}
	target := refs.PrimaryRef()
	if target == "" {
		return supportmodels.ActionRequest{}, apperrors.Validation(
			"This complaint names no job to act on.", nil)
	}
	params := map[string]string{}
	for name, value := range input.Parameters {
		params[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	for _, required := range route.requiredParameters {
		if params[required] == "" {
			return supportmodels.ActionRequest{}, apperrors.Validation("Check the action parameters.",
				[]apperrors.FieldViolation{{Field: required, Message: "Required."}})
		}
	}

	action, created, err := s.store.CreateActionRequest(ctx, supportmodels.ActionRequest{
		ComplaintID: input.ComplaintID, TargetService: route.targetService,
		ActionCode: actionCode, TargetRef: target, Parameters: params,
		IdempotencyKey: key, RequestedBy: requestedBy,
		RequestFingerprint: actionFingerprint(input.ComplaintID, actionCode, target, params),
	})
	if errors.Is(err, supportrepositories.ErrActionKeyReused) {
		return supportmodels.ActionRequest{}, apperrors.Conflict(
			"That idempotency key was already used for a different action.", nil)
	}
	if err != nil {
		return supportmodels.ActionRequest{}, err
	}
	if !created {
		// A replay: the original request, exactly as it now stands. Delivering
		// again here would be a second attempt the caller did not ask for.
		return action, nil
	}
	return s.deliver(ctx, route, action, input.CorrelationID)
}

// DeliverPending re-sends requests whose earlier attempt was ambiguous.
func (s *NetworkContextService) DeliverPending(ctx context.Context, limit int) (int, error) {
	claimed, err := s.store.ClaimDeliverable(ctx, limit, time.Minute)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, action := range claimed {
		refs, refErr := s.store.GetContextRefs(ctx, action.ComplaintID)
		if refErr != nil {
			continue
		}
		route, ok := s.routes[action.ActionCode+"/"+refs.ContextType]
		if !ok {
			// The route was withdrawn from this deployment. Park it rather
			// than guess at another owner.
			_, _ = s.store.RecordActionOutcome(ctx, action.ID, supportmodels.ActionDeadLetter,
				"", "ROUTE_UNAVAILABLE", time.Hour)
			continue
		}
		if _, deliverErr := s.deliver(ctx, route, action, ""); deliverErr == nil {
			delivered++
		}
	}
	return delivered, nil
}

// deliver makes one attempt and records the owner's answer.
//
// The operation reference sent to the owner is derived from the request id, so
// every attempt for one request is the SAME operation to the owner. That is
// what makes a retry after an ambiguous outcome safe: the owner replays rather
// than acting twice.
func (s *NetworkContextService) deliver(ctx context.Context, route actionRoute,
	action supportmodels.ActionRequest, correlation string) (supportmodels.ActionRequest, error) {
	outcome, err := route.deliverer.Deliver(ctx, supportclients.ActionDelivery{
		TargetRef: action.TargetRef, OperationRef: "support-action-" + action.ID,
		Parameters: action.Parameters, Correlation: correlation,
	})
	status, retry := supportmodels.ActionUnknown, 30*time.Second
	switch {
	case err != nil:
		status = supportmodels.ActionUnknown
	case outcome.Definite && outcome.Accepted:
		status = supportmodels.ActionAccepted
	case outcome.Definite:
		status = supportmodels.ActionRefused
	}
	updated, recordErr := s.store.RecordActionOutcome(ctx, action.ID, status,
		outcome.ResultRef, outcome.Code, retry)
	if errors.Is(recordErr, supportrepositories.ErrNetworkNotFound) {
		// Already terminal -- a concurrent delivery got there first. Its
		// answer stands.
		return s.store.GetActionRequest(ctx, action.ComplaintID, action.ID)
	}
	if recordErr != nil {
		return action, recordErr
	}
	return updated, nil
}

// ListActions returns a complaint's action requests.
func (s *NetworkContextService) ListActions(ctx context.Context,
	complaintID string) ([]supportmodels.ActionRequest, error) {
	if _, err := s.support.repo.GetComplaintByID(ctx, complaintID); err != nil {
		return nil, err
	}
	return s.store.ListActionRequests(ctx, complaintID)
}

func (s *NetworkContextService) authorize(ctx context.Context, complaintID string,
	requesterType supportmodels.ComplainantType, requesterID string, admin bool) error {
	if admin {
		_, err := s.support.repo.GetComplaintByID(ctx, complaintID)
		return err
	}
	_, err := s.support.GetComplaint(ctx, complaintID, requesterType, requesterID)
	return err
}

func normalizeRefs(refs supportmodels.ContextRefs) supportmodels.ContextRefs {
	trim := strings.TrimSpace
	return supportmodels.ContextRefs{
		ContextType: strings.ToUpper(trim(refs.ContextType)), OrderRef: trim(refs.OrderRef),
		FulfilmentRef: trim(refs.FulfilmentRef), MissionRef: trim(refs.MissionRef),
		ProviderRef: trim(refs.ProviderRef), AssetRef: trim(refs.AssetRef),
		GuaranteeBreachRef: trim(refs.GuaranteeBreachRef), RecoveryCaseRef: trim(refs.RecoveryCaseRef),
	}
}

// actionFingerprint identifies what was asked, independent of map ordering.
func actionFingerprint(complaintID, actionCode, target string, params map[string]string) string {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := []string{complaintID, actionCode, target}
	for _, name := range names {
		parts = append(parts, name+"="+params[name])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func mapNetworkErr(err error) error {
	if errors.Is(err, supportrepositories.ErrNetworkNotFound) {
		return apperrors.NotFound("Complaint not found.", nil)
	}
	return fmt.Errorf("support network context: %w", err)
}
