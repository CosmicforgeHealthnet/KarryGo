package movementusecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	movementmodels "cosmicforge/logistics/services/customer-service/internal/features/movement_history/models"
	movementrepositories "cosmicforge/logistics/services/customer-service/internal/features/movement_history/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/pagination"
)

type Repository interface {
	Project(ctx context.Context, envelope events.Envelope, incoming movementmodels.Movement) (string, error)
	ApplyGrouping(ctx context.Context, envelope events.Envelope, grouping events.MovementGrouping) (string, int, error)
	ProjectedVersions(ctx context.Context, authority string, sourceRefs []string) (map[string]int64, error)
	Reject(ctx context.Context, envelope events.Envelope, reason string) error
	List(ctx context.Context, customerID string, filter movementrepositories.Filter) ([]movementmodels.Movement, int, error)
	Get(ctx context.Context, customerID, authority, sourceRef string) (movementmodels.Movement, error)
}

// CallerAuthorities binds each HMAC caller to the only movement authority
// it may publish for. Taxi cannot write Dispatch history, and Customer
// Service writes none of these authorities' state itself.
type CallerAuthorities map[string]string

// ParseCallerAuthorities reads "svc=TAXI;svc2=DISPATCH".
func ParseCallerAuthorities(raw string) (CallerAuthorities, error) {
	out := CallerAuthorities{}
	for _, entry := range strings.Split(raw, ";") {
		if entry = strings.TrimSpace(entry); entry == "" {
			continue
		}
		name, authority, ok := strings.Cut(entry, "=")
		name, authority = strings.TrimSpace(name), strings.ToUpper(strings.TrimSpace(authority))
		if !ok || name == "" || (authority != events.MovementAuthorityTaxi && authority != events.MovementAuthorityDispatch &&
			authority != events.MovementAuthorityHauling && authority != events.MovementAuthorityVendorCommerce) {
			return nil, fmt.Errorf("invalid movement caller entry %q", entry)
		}
		out[name] = authority
	}
	return out, nil
}

type Options struct {
	Repository      Repository
	Callers         CallerAuthorities
	ConsumerEnabled bool
	ReadEnabled     bool
	// GroupingCallers are the services allowed to publish commerce grouping.
	// Grouping is commercial context only; a grouping caller can never
	// publish movement state.
	GroupingCallers []string
	// Metrics receives bounded ingestion counters (Customer V4 §27.2).
	Metrics *observability.Metrics
}

type Service struct {
	repo            Repository
	callers         CallerAuthorities
	consumerEnabled bool
	readEnabled     bool
	groupingCallers map[string]bool
	metrics         *observability.Metrics
}

func NewService(options Options) *Service {
	grouping := map[string]bool{}
	for _, caller := range options.GroupingCallers {
		if caller = strings.TrimSpace(caller); caller != "" {
			grouping[caller] = true
		}
	}
	return &Service{repo: options.Repository, callers: options.Callers, consumerEnabled: options.ConsumerEnabled,
		readEnabled: options.ReadEnabled, groupingCallers: grouping, metrics: options.Metrics}
}

// IngestGrouping records the commercial grouping of one purchase.
//
// A grouping caller is deliberately NOT a movement authority: it may say
// which movements belong together, and can never create a movement, change
// its state, or name who executes it.
func (s *Service) IngestGrouping(ctx context.Context, caller string, envelope events.Envelope) (IngestResult, error) {
	if !s.consumerEnabled {
		return IngestResult{}, apperrors.Unavailable("Movement event consumption is disabled.", nil)
	}
	caller = strings.TrimSpace(caller)
	if !s.groupingCallers[caller] {
		return IngestResult{}, apperrors.Forbidden("This service may not publish commerce grouping.", nil)
	}
	if err := envelope.Validate(); err != nil {
		return IngestResult{}, apperrors.BadRequest("Grouping event envelope is invalid.", err)
	}
	if envelope.SourceService != caller {
		_ = s.repo.Reject(ctx, envelope, "SOURCE_SERVICE_MISMATCH")
		return IngestResult{}, apperrors.Forbidden("Event source_service does not match the authenticated caller.", nil)
	}
	if envelope.EventName != events.CommerceGroupingEventName {
		_ = s.repo.Reject(ctx, envelope, "EVENT_NAME_NOT_ACCEPTED")
		return IngestResult{}, apperrors.BadRequest("This ingress accepts commerce grouping events only.", nil)
	}
	var grouping events.MovementGrouping
	if err := json.Unmarshal(envelope.Payload, &grouping); err != nil {
		return IngestResult{}, apperrors.BadRequest("Grouping payload is invalid.", err)
	}
	if err := grouping.Validate(); err != nil {
		_ = s.repo.Reject(ctx, envelope, "GROUPING_INCOMPLETE")
		return IngestResult{}, apperrors.BadRequest("Grouping payload is incomplete.", err)
	}
	if _, err := uuid.Parse(grouping.CustomerID); err != nil {
		_ = s.repo.Reject(ctx, envelope, "CUSTOMER_ID_INVALID")
		return IngestResult{}, apperrors.BadRequest("Grouping customer_id is invalid.", err)
	}
	outcome, _, err := s.repo.ApplyGrouping(ctx, envelope, grouping)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{EventID: envelope.EventID, Outcome: outcome}, nil
}

type IngestResult struct {
	EventID string `json:"event_id"`
	Outcome string `json:"outcome"`
}

// Ingest projects one movement event. Duplicate, stale and
// terminal-protected events succeed so the producer's outbox acknowledges
// them. Authority violations are recorded and refused.
func (s *Service) Ingest(ctx context.Context, caller string, envelope events.Envelope) (result IngestResult, err error) {
	defer func() { s.observeIngest(envelope, result, err) }()
	return s.ingest(ctx, caller, envelope)
}

// observeIngest records bounded ingestion evidence: authority and outcome are
// closed vocabularies; lag is authority occurrence to projection.
func (s *Service) observeIngest(envelope events.Envelope, result IngestResult, err error) {
	if s.metrics == nil {
		return
	}
	authority := strings.ToUpper(strings.TrimSpace(authorityOf(envelope)))
	switch authority {
	case events.MovementAuthorityTaxi, events.MovementAuthorityDispatch, events.MovementAuthorityHauling, events.MovementAuthorityVendorCommerce:
	default:
		authority = "UNKNOWN"
	}
	outcome := strings.ToLower(result.Outcome)
	if err != nil {
		outcome = string(observability.OutcomeForStatus(apperrors.From(err).Status))
	}
	switch outcome {
	case "applied", "stale", "terminal_protected", "duplicate", "validation", "conflict", "authorization_denied", "dependency_unavailable", "not_found":
	default:
		outcome = "other"
	}
	labels := map[string]string{"authority": authority, "outcome": outcome}
	s.metrics.IncCounter("karry_movement_ingest_total", labels, 1)
	if err == nil && !envelope.OccurredAt.IsZero() {
		if lag := time.Since(envelope.OccurredAt); lag >= 0 {
			s.metrics.ObserveDuration("karry_movement_ingest_lag", map[string]string{"authority": authority}, lag)
		}
	}
}

func authorityOf(envelope events.Envelope) string {
	var summary struct {
		Authority string `json:"authority"`
	}
	_ = json.Unmarshal(envelope.Payload, &summary)
	return summary.Authority
}

func (s *Service) ingest(ctx context.Context, caller string, envelope events.Envelope) (IngestResult, error) {
	if !s.consumerEnabled {
		return IngestResult{}, apperrors.Unavailable("Movement event consumption is disabled.", nil)
	}
	authority, allowed := s.callers[strings.TrimSpace(caller)]
	if !allowed {
		return IngestResult{}, apperrors.Forbidden("This service may not publish customer movements.", nil)
	}
	if err := envelope.Validate(); err != nil {
		return IngestResult{}, apperrors.BadRequest("Movement event envelope is invalid.", err)
	}
	if envelope.SourceService != caller {
		_ = s.repo.Reject(ctx, envelope, "SOURCE_SERVICE_MISMATCH")
		return IngestResult{}, apperrors.Forbidden("Event source_service does not match the authenticated caller.", nil)
	}
	incoming, err := movementmodels.FromEnvelope(envelope)
	if err != nil {
		return IngestResult{}, apperrors.BadRequest("Movement event payload is invalid.", err)
	}
	if _, err := uuid.Parse(incoming.CustomerID); err != nil {
		_ = s.repo.Reject(ctx, envelope, "CUSTOMER_ID_INVALID")
		return IngestResult{}, apperrors.BadRequest("Movement customer_id is invalid.", err)
	}
	if incoming.Authority != authority {
		_ = s.repo.Reject(ctx, envelope, "AUTHORITY_NOT_OWNED_BY_CALLER")
		return IngestResult{}, apperrors.Forbidden("This service may not publish movements for "+incoming.Authority+".", nil)
	}
	outcome, err := s.repo.Project(ctx, envelope, incoming)
	if errors.Is(err, movementmodels.ErrCustomerMismatch) {
		_ = s.repo.Reject(ctx, envelope, "CUSTOMER_MISMATCH")
		return IngestResult{}, apperrors.Conflict("Movement customer does not match the existing projection.", err)
	}
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{EventID: envelope.EventID, Outcome: outcome}, nil
}

// ProjectedVersions answers a reconciling authority with what this
// projection currently holds for its OWN movements. An authority can never
// read another authority's movements through it, and the answer carries no
// customer data: only refs the caller already named and the versions it
// published.
func (s *Service) ProjectedVersions(ctx context.Context, caller string, sourceRefs []string) (map[string]int64, error) {
	return s.ProjectedVersionsFor(ctx, caller, "", sourceRefs)
}

// ProjectedVersionsFor lets a caller that is both a grouping publisher and a
// movement authority (Vendor: grouping plus VENDOR_COMMERCE purchase rows)
// name which of its own two projections it is reconciling. It can never name
// an authority it is not mapped to.
func (s *Service) ProjectedVersionsFor(ctx context.Context, caller, requested string, sourceRefs []string) (map[string]int64, error) {
	if !s.consumerEnabled {
		return nil, apperrors.Unavailable("Movement event consumption is disabled.", nil)
	}
	caller = strings.TrimSpace(caller)
	requested = strings.ToUpper(strings.TrimSpace(requested))
	authority, allowed := s.callers[caller]
	grouping := s.groupingCallers[caller]
	switch {
	case requested == "" && grouping, requested == "COMMERCE" && grouping:
		// Vendor may reconcile grouping versions, never logistics movement state.
		authority, allowed = "COMMERCE", true
	case requested != "" && requested != authority:
		allowed = false
	}
	if !allowed {
		return nil, apperrors.Forbidden("This service may not reconcile customer movements.", nil)
	}
	cleaned := make([]string, 0, len(sourceRefs))
	for _, ref := range sourceRefs {
		if ref = strings.TrimSpace(ref); ref != "" {
			cleaned = append(cleaned, ref)
		}
	}
	if len(cleaned) == 0 || len(cleaned) > 500 {
		return nil, apperrors.BadRequest("Send between 1 and 500 source references.", nil)
	}
	return s.repo.ProjectedVersions(ctx, authority, cleaned)
}

type ListQuery struct {
	Authority, ProductCode, MovementType, State, ParentContextRef, From, To string
	Page, Limit                                                             int
}

func (s *Service) List(ctx context.Context, customerID string, query ListQuery) ([]movementmodels.Movement, pagination.Page, error) {
	if !s.readEnabled {
		return nil, pagination.Page{}, apperrors.Unavailable("Movement history is not available yet.", nil)
	}
	page := pagination.Normalize(query.Page, query.Limit)
	filter := movementrepositories.Filter{
		Authority: strings.ToUpper(strings.TrimSpace(query.Authority)), ProductCode: strings.TrimSpace(query.ProductCode),
		MovementType: strings.ToUpper(strings.TrimSpace(query.MovementType)), State: strings.ToUpper(strings.TrimSpace(query.State)),
		ParentContextRef: strings.TrimSpace(query.ParentContextRef), Page: page.Number, Limit: page.Limit,
	}
	var fields []apperrors.FieldViolation
	for name, raw := range map[string]string{"from": query.From, "to": query.To} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
		if err != nil {
			fields = append(fields, apperrors.FieldViolation{Field: name, Message: "Use an RFC3339 timestamp."})
			continue
		}
		if name == "from" {
			filter.From = &parsed
		} else {
			filter.To = &parsed
		}
	}
	if len(fields) > 0 {
		return nil, page, apperrors.Validation("Check the movement filters.", fields)
	}
	items, total, err := s.repo.List(ctx, customerID, filter)
	page.Total = total
	return items, page, err
}

func (s *Service) Get(ctx context.Context, customerID, authority, sourceRef string) (movementmodels.Movement, error) {
	if !s.readEnabled {
		return movementmodels.Movement{}, apperrors.Unavailable("Movement history is not available yet.", nil)
	}
	return s.repo.Get(ctx, customerID, strings.ToUpper(strings.TrimSpace(authority)), strings.TrimSpace(sourceRef))
}
