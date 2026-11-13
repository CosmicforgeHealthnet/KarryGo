package specialized

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// GetSchema returns the typed extension schema of a capability the
// organization has enabled. Organizations see only modules they have enabled
// (spec 50.2): anything else is the same safe not-found.
func (s *Service) GetSchema(ctx context.Context, t tenant.Context, code string) (CapabilitySchema, error) {
	if e := s.gate(false); e != nil {
		return CapabilitySchema{}, e
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if _, e := s.capability(ctx, t.OrganizationID, code, false); e != nil {
		return CapabilitySchema{}, e
	}
	schema, ok := Schema(code)
	if !ok {
		return CapabilitySchema{}, apperrors.NotFound("Resource not found.", nil)
	}
	return schema, nil
}

// orderBranch resolves an order of the caller's organization and branch scope.
func (s *Service) orderBranch(ctx context.Context, t tenant.Context, orderID string) (string, error) {
	var branch string
	e := s.o.Pool.QueryRow(ctx, `SELECT COALESCE(branch_id,'') FROM fleet_partner_private_orders WHERE organization_id=$1 AND id=$2`, t.OrganizationID, orderID).Scan(&branch)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(branch) {
		return "", apperrors.NotFound("Order not found.", nil)
	}
	return branch, e
}

// OrderDataInput attaches one capability record to a common order.
type OrderDataInput struct {
	CapabilityCode, RecordType string
	SchemaVersion              int
	Payload                    []byte
	IdempotencyKey             string
}

// CreateOrderData validates specialized data against the capability schema
// and stores it as an extension of the common order (spec 50.3 POST
// /orders/{id}/specialized-data). The order record itself is untouched.
func (s *Service) CreateOrderData(ctx context.Context, t tenant.Context, orderID string, in OrderDataInput) (Record, error) {
	if e := s.gate(true); e != nil {
		return Record{}, e
	}
	branch, e := s.orderBranch(ctx, t, strings.TrimSpace(orderID))
	if e != nil {
		return Record{}, e
	}
	return s.CreateRecord(ctx, t, Record{CapabilityCode: in.CapabilityCode, RecordType: in.RecordType, SubjectType: SubjectOrder, SubjectReference: strings.TrimSpace(orderID), BranchID: branch, Payload: in.Payload, SchemaVersion: in.SchemaVersion, IdempotencyKey: in.IdempotencyKey})
}

// ListOrderData returns the order's specialized data across the capabilities
// that are enabled for this deployment and organization. Restricted records
// are audited as sensitive reads.
func (s *Service) ListOrderData(ctx context.Context, t tenant.Context, orderID string) ([]Record, error) {
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
	rows, e := s.o.Pool.Query(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records r WHERE r.organization_id=$1 AND r.subject_type=$2 AND r.subject_reference=$3 AND r.capability_code=ANY($4::text[]) AND EXISTS(SELECT 1 FROM fleet_partner_organization_capabilities c WHERE c.organization_id=r.organization_id AND c.capability=r.capability_code AND c.status='ENABLED') ORDER BY r.created_at,r.id`, t.OrganizationID, SubjectOrder, orderID, enabled)
	if e != nil {
		return nil, e
	}
	out := []Record{}
	for rows.Next() {
		v, x := scanRecord(rows)
		if x != nil {
			rows.Close()
			return nil, x
		}
		out = append(out, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return nil, e
	}
	for _, v := range out {
		if v.RestrictedVisibility {
			if _, e = s.o.Pool.Exec(ctx, `INSERT INTO fleet_partner_sensitive_access_audit(id,organization_id,specialized_record_id,actor_reference,permission_code,purpose,correlation_id)VALUES($1,$2,$3,$4,'specialized.read','ORDER_SPECIALIZED_DATA_READ',$5)`, ids.New("saa"), t.OrganizationID, v.ID, t.ActorID, t.CorrelationID); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
