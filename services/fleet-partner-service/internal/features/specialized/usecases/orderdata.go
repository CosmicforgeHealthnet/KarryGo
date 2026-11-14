package usecases

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// GetSchema returns the typed extension schema of a capability the
// organization has enabled. Organizations see only modules they have enabled
// (spec 50.2): anything else is the same safe not-found.
func (s *Service) GetSchema(ctx context.Context, t tenant.Context, code string) (models.CapabilitySchema, error) {
	if e := s.gate(false); e != nil {
		return models.CapabilitySchema{}, e
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if _, e := s.capability(ctx, t.OrganizationID, code, false); e != nil {
		return models.CapabilitySchema{}, e
	}
	schema, ok := models.Schema(code)
	if !ok {
		return models.CapabilitySchema{}, apperrors.NotFound("Resource not found.", nil)
	}
	return schema, nil
}

// orderBranch resolves an order of the caller's organization and branch scope.
func (s *Service) orderBranch(ctx context.Context, t tenant.Context, orderID string) (string, error) {
	branch, e := s.repo.OrderBranch(ctx, t.OrganizationID, orderID)
	if errors.Is(e, models.ErrNotFound) || e == nil && !t.BranchPermitted(branch) {
		return "", apperrors.NotFound("Order not found.", nil)
	}
	return branch, e
}

// CreateOrderData validates specialized data against the capability schema
// and stores it as an extension of the common order (spec 50.3 POST
// /orders/{id}/specialized-data). The order record itself is untouched.
func (s *Service) CreateOrderData(ctx context.Context, t tenant.Context, orderID string, in models.OrderDataInput) (models.Record, error) {
	if e := s.gate(true); e != nil {
		return models.Record{}, e
	}
	branch, e := s.orderBranch(ctx, t, strings.TrimSpace(orderID))
	if e != nil {
		return models.Record{}, e
	}
	return s.CreateRecord(ctx, t, models.Record{CapabilityCode: in.CapabilityCode, RecordType: in.RecordType, SubjectType: models.SubjectOrder, SubjectReference: strings.TrimSpace(orderID), BranchID: branch, Payload: in.Payload, SchemaVersion: in.SchemaVersion, IdempotencyKey: in.IdempotencyKey})
}

// ListOrderData returns the order's specialized data across the capabilities
// that are enabled for this deployment and organization. Restricted records
// are audited as sensitive reads.
func (s *Service) ListOrderData(ctx context.Context, t tenant.Context, orderID string) ([]models.Record, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	orderID = strings.TrimSpace(orderID)
	if _, e := s.orderBranch(ctx, t, orderID); e != nil {
		return nil, e
	}
	enabled := []string{}
	for code, on := range s.o.Flags.Modules {
		if on {
			enabled = append(enabled, code)
		}
	}
	out, e := s.repo.ListOrderRecords(ctx, t.OrganizationID, orderID, enabled)
	if e != nil {
		return nil, e
	}
	for _, v := range out {
		if v.RestrictedVisibility {
			if e = s.repo.AuditSensitiveRead(ctx, t.OrganizationID, v.ID, t.ActorID, "ORDER_SPECIALIZED_DATA_READ", t.CorrelationID); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
