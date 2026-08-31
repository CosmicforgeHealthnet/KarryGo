package pastry

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

const (
	PermRead    = "vendor.pastry.read"
	PermManage  = "vendor.pastry.manage"
	PermQuote   = "vendor.pastry.quote.manage"
	PermApprove = "vendor.pastry.quote.approve"
)

type Service struct {
	db         *pgxpool.Pool
	production *production.Service
}

func NewService(db *pgxpool.Pool, p *production.Service) *Service {
	return &Service{db: db, production: p}
}

func snapshotProduct(ctx context.Context, tx pgx.Tx, productID, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO pastry_product_versions(id,product_id,version,definition,reason)
		SELECT $1,p.id,p.version,jsonb_build_object(
			'product',to_jsonb(p),
			'option_groups',COALESCE((SELECT jsonb_agg(jsonb_build_object('attachment',to_jsonb(a),'group',to_jsonb(g),'options',COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.position,o.id) FROM pastry_options o WHERE o.group_id=g.id),'[]'::jsonb)) ORDER BY a.position,g.id) FROM pastry_product_option_groups a JOIN pastry_option_groups g ON g.id=a.group_id WHERE a.product_id=p.id),'[]'::jsonb)
		),$3 FROM pastry_products p WHERE p.id=$2`, "ppv_"+uuid.NewString(), productID, reason)
	return err
}

func recordProductPriceVersion(ctx context.Context, tx pgx.Tx, productID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO pastry_product_price_history(id,product_id,product_version,amount_minor,currency) SELECT $1,id,version,base_price_minor,currency FROM pastry_products WHERE id=$2`, "pph_"+uuid.NewString(), productID)
	return err
}

func snapshotProductsAttachedToGroup(ctx context.Context, tx pgx.Tx, groupID string) error {
	rows, err := tx.Query(ctx, `SELECT p.id FROM pastry_products p JOIN pastry_product_option_groups a ON a.product_id=p.id WHERE a.group_id=$1 ORDER BY p.id FOR UPDATE OF p`, groupID)
	if err != nil {
		return err
	}
	productIDs := []string{}
	for rows.Next() {
		var productID string
		if err = rows.Scan(&productID); err != nil {
			rows.Close()
			return err
		}
		productIDs = append(productIDs, productID)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, productID := range productIDs {
		if _, err = tx.Exec(ctx, `UPDATE pastry_products SET version=version+1,updated_at=now() WHERE id=$1`, productID); err != nil {
			return err
		}
		if err = recordProductPriceVersion(ctx, tx, productID); err != nil {
			return err
		}
		if err = snapshotProduct(ctx, tx, productID, "OPTIONS_UPDATED"); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) DB() *pgxpool.Pool               { return s.db }
func (s *Service) Production() *production.Service { return s.production }

type Product struct {
	ID                   string         `json:"id"`
	BranchID             string         `json:"branch_id"`
	Name                 string         `json:"name"`
	Description          string         `json:"description"`
	ProductType          string         `json:"product_type"`
	Currency             string         `json:"currency"`
	Status               string         `json:"status"`
	BasePriceMinor       int64          `json:"base_price_minor"`
	Customizable         bool           `json:"customizable"`
	ProductionClass      *string        `json:"production_class"`
	CapacityPoolID       *string        `json:"capacity_pool_id"`
	SetupBufferMinutes   int            `json:"setup_buffer_minutes"`
	CleanupBufferMinutes int            `json:"cleanup_buffer_minutes"`
	WorkloadUnitsPerItem int            `json:"workload_units_per_item"`
	Logistics            map[string]any `json:"logistics"`
	Version              int64          `json:"version"`
	Position             int            `json:"position"`
}

func (s *Service) ListProducts(ctx context.Context, business string, scope production.Scope) ([]Product, error) {
	rows, e := s.db.Query(ctx, `SELECT id,branch_id,name,description,product_type,base_price_minor,currency,customizable,production_class,capacity_pool_id,setup_buffer_minutes,cleanup_buffer_minutes,workload_units_per_item,logistics,status,version,position FROM pastry_products WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY branch_id,position,name`, business, scope.Global, scope.BranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		var p Product
		var raw []byte
		if e := rows.Scan(&p.ID, &p.BranchID, &p.Name, &p.Description, &p.ProductType, &p.BasePriceMinor, &p.Currency, &p.Customizable, &p.ProductionClass, &p.CapacityPoolID, &p.SetupBufferMinutes, &p.CleanupBufferMinutes, &p.WorkloadUnitsPerItem, &raw, &p.Status, &p.Version, &p.Position); e != nil {
			return nil, e
		}
		_ = json.Unmarshal(raw, &p.Logistics)
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) CreateProduct(ctx context.Context, business string, p Product) (string, error) {
	if p.WorkloadUnitsPerItem == 0 {
		p.WorkloadUnitsPerItem = 1
	}
	if p.BasePriceMinor < 0 || p.Name == "" || p.SetupBufferMinutes < 0 || p.SetupBufferMinutes > production.MaxBufferMinutes || p.CleanupBufferMinutes < 0 || p.CleanupBufferMinutes > production.MaxBufferMinutes || p.WorkloadUnitsPerItem < 1 || p.WorkloadUnitsPerItem > 1000 {
		return "", fmt.Errorf("invalid pastry product")
	}
	if err := validateLogisticsMetadata(p.Logistics); err != nil {
		return "", err
	}
	id := uuid.NewString()
	logistics, e := json.Marshal(p.Logistics)
	if e != nil {
		return "", e
	}
	if p.Currency == "" {
		p.Currency = "NGN"
	}
	if p.Status == "" {
		p.Status = "DRAFT"
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	if e = s.production.ValidateCapacityPoolReference(ctx, tx, business, p.BranchID, "PASTRY_BAKERY", p.CapacityPoolID); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_products(id,business_id,branch_id,name,description,product_type,base_price_minor,currency,customizable,production_class,capacity_pool_id,setup_buffer_minutes,cleanup_buffer_minutes,workload_units_per_item,logistics,status,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, id, business, p.BranchID, p.Name, p.Description, p.ProductType, p.BasePriceMinor, p.Currency, p.Customizable, p.ProductionClass, p.CapacityPoolID, p.SetupBufferMinutes, p.CleanupBufferMinutes, p.WorkloadUnitsPerItem, logistics, p.Status, p.Position); e != nil {
		return "", e
	}
	if e = snapshotProduct(ctx, tx, id, "CREATED"); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_product_price_history(id,product_id,product_version,amount_minor,currency) VALUES($1,$2,1,$3,$4)`, "pph_"+uuid.NewString(), id, p.BasePriceMinor, p.Currency); e != nil {
		return "", e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.product.created", "pastry.product.created", "pastry_product", id, p.BranchID, 1, nil, p, map[string]any{"id": id}, ""); e != nil {
			return "", e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return id, nil
}

type StandardBatchInput struct {
	ProductID        string
	Quantity         int
	WindowStart      time.Time
	WindowEnd        time.Time
	ProductionLineID string
}

func (s *Service) standardAllocation(ctx context.Context, business string, scope production.Scope, in StandardBatchInput) (string, string, production.AllocationRequest, error) {
	var branch, status string
	var pool *string
	var workload int
	if err := s.db.QueryRow(ctx, `SELECT branch_id,status,capacity_pool_id,workload_units_per_item FROM pastry_products WHERE business_id=$1 AND id=$2`, business, in.ProductID).Scan(&branch, &status, &pool, &workload); err != nil {
		return "", "", production.AllocationRequest{}, err
	}
	if !scope.Covers(branch) || status != "ACTIVE" || pool == nil || *pool == "" {
		return "", "", production.AllocationRequest{}, fmt.Errorf("active capacity-backed product not found")
	}
	if in.Quantity <= 0 || workload <= 0 || in.Quantity > production.MaxCapacity/workload || !in.WindowEnd.After(in.WindowStart) {
		return "", "", production.AllocationRequest{}, fmt.Errorf("invalid standard production request")
	}
	return branch, *pool, production.AllocationRequest{CapacityPoolID: *pool, Quantity: in.Quantity * workload, ProductionLineID: in.ProductionLineID}, nil
}

func (s *Service) PreviewStandardBatch(ctx context.Context, actor production.Actor, scope production.Scope, in StandardBatchInput) (production.CapacityPreview, error) {
	branch, _, allocation, err := s.standardAllocation(ctx, actor.VendorID, scope, in)
	if err != nil {
		return production.CapacityPreview{}, err
	}
	return s.production.PreviewCapacity(ctx, actor, scope, production.ReserveInput{BranchID: branch, Capability: "PASTRY_BAKERY", WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, Allocations: []production.AllocationRequest{allocation}, SourceType: "PASTRY_PRODUCT", SourceRef: in.ProductID})
}

func (s *Service) CreateStandardBatch(ctx context.Context, actor production.Actor, scope production.Scope, in StandardBatchInput) (string, string, error) {
	branch, _, allocation, err := s.standardAllocation(ctx, actor.VendorID, scope, in)
	if err != nil {
		return "", "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)
	reservation, _, err := s.production.ReserveInTx(ctx, tx, actor, scope, production.ReserveInput{BranchID: branch, Capability: "PASTRY_BAKERY", WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, Allocations: []production.AllocationRequest{allocation}, SourceType: "PASTRY_PRODUCT", SourceRef: in.ProductID})
	if err != nil {
		return "", "", err
	}
	jobID := "job_" + uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO production_jobs(id,business_id,branch_id,capability,reservation_id,production_line_id,source_type,source_ref,source_version,title,planned_quantity,scheduled_start,scheduled_end) SELECT $1,$2,$3,'PASTRY_BAKERY',$4,NULLIF($5,''),'PASTRY_PRODUCT',$6,version,'Pastry batch: '||name,$7,$8,$9 FROM pastry_products WHERE business_id=$2 AND id=$6`, jobID, actor.VendorID, branch, reservation.ID, in.ProductionLineID, in.ProductID, in.Quantity, in.WindowStart, in.WindowEnd); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,to_status,actor_type,actor_id,note) VALUES($1,$2,$3,'CREATED','PLANNED','vendor_staff',NULLIF($4,''),'standard pastry batch created')`, "pje_"+uuid.NewString(), actor.VendorID, jobID, actor.StaffID); err != nil {
		return "", "", err
	}
	if err = s.production.RecordMutation(ctx, tx, actor, "pastry.batch.created", "pastry.batch.created", "production_job", jobID, branch, 1, nil, map[string]any{"product_id": in.ProductID, "quantity": in.Quantity, "reservation_id": reservation.ID}, map[string]any{"job_id": jobID, "reservation_id": reservation.ID}, ""); err != nil {
		return "", "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return jobID, reservation.ID, nil
}
func (s *Service) CreateGroup(ctx context.Context, business, name, typ string, min, max int) (string, error) {
	if min < 0 || max < min {
		return "", fmt.Errorf("invalid selection bounds")
	}
	id := uuid.NewString()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_option_groups(id,business_id,name,group_type,min_select,max_select) VALUES($1,$2,$3,$4,$5,$6)`, id, business, name, typ, min, max); e != nil {
		return "", e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.option_group.created", "pastry.option_group.created", "pastry_option_group", id, "", 1, nil, map[string]any{"name": name, "group_type": typ}, map[string]any{"id": id}, ""); e != nil {
			return "", e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return id, nil
}
func (s *Service) AddOption(ctx context.Context, group, name, code string, delta int64, weight, position int, capacityClass *string) (string, error) {
	if delta < 0 || weight <= 0 || position < 0 || position > 999 {
		return "", fmt.Errorf("invalid option")
	}
	if capacityClass != nil {
		value := strings.ToUpper(strings.TrimSpace(*capacityClass))
		allowed := map[string]bool{"GENERAL": true, "HOT": true, "COLD": true, "BAKERY": true, "DECORATION": true, "PACKING": true, "CHILLING": true}
		if !allowed[value] {
			return "", fmt.Errorf("invalid option capacity class")
		}
		capacityClass = &value
	}
	id := uuid.NewString()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	var business string
	if e = tx.QueryRow(ctx, `SELECT business_id FROM pastry_option_groups WHERE id=$1`, group).Scan(&business); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_options(id,group_id,name,code,price_delta_minor,workload_weight,position,capacity_class) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, group, name, code, delta, weight, position, capacityClass); e != nil {
		return "", e
	}
	if e = snapshotProductsAttachedToGroup(ctx, tx, group); e != nil {
		return "", e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.option.created", "pastry.option.created", "pastry_option", id, "", 1, nil, map[string]any{"group_id": group, "code": code, "price_delta_minor": delta}, map[string]any{"id": id}, ""); e != nil {
			return "", e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return id, nil
}
func (s *Service) AttachGroup(ctx context.Context, product, group string, position int, required bool, expectedVersion int64) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var business, branch string
	var version int64
	if e = tx.QueryRow(ctx, `SELECT business_id,branch_id,version FROM pastry_products WHERE id=$1 FOR UPDATE`, product).Scan(&business, &branch, &version); e != nil {
		return e
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return fmt.Errorf("stale product version")
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_product_option_groups(product_id,group_id,position,required) VALUES($1,$2,$3,$4) ON CONFLICT(product_id,group_id) DO UPDATE SET position=EXCLUDED.position,required=EXCLUDED.required`, product, group, position, required); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_products SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, product, version); e != nil {
		return e
	}
	if e = recordProductPriceVersion(ctx, tx, product); e != nil {
		return e
	}
	if e = snapshotProduct(ctx, tx, product, "OPTIONS_UPDATED"); e != nil {
		return e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.product_option_group.attached", "pastry.product_option_group.attached", "pastry_product_option_group", product+":"+group, branch, version+1, map[string]any{"product_version": version}, map[string]any{"product_id": product, "group_id": group, "position": position, "required": required, "product_version": version + 1}, map[string]any{"product_id": product, "group_id": group}, ""); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

type CustomRequestInput struct {
	BranchID            string
	CustomerRef         string
	CorporateProfileRef string
	Title               string
	Notes               string
	RequiredAt          *time.Time
	ChangeCutoffAt      *time.Time
	ChangePolicy        map[string]any
	CancellationPolicy  map[string]any
	Logistics           map[string]any
	RiskFlags           []string
}

type changePolicyDefinition struct {
	MinutesBeforeRequiredAt  int    `json:"minutes_before_required_at"`
	DisallowedAfterMilestone string `json:"disallowed_after_milestone"`
}

type cancellationPolicyDefinition struct {
	ReviewMinutesBeforeRequiredAt   int    `json:"review_minutes_before_required_at"`
	RestrictMinutesBeforeRequiredAt int    `json:"restrict_minutes_before_required_at"`
	ReviewAfterMilestone            string `json:"review_after_milestone"`
	RestrictAfterMilestone          string `json:"restrict_after_milestone"`
}

func decodeBoundedPolicy(raw []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func validateStoredPolicies(changeRaw, cancellationRaw []byte) error {
	var change changePolicyDefinition
	if err := decodeBoundedPolicy(changeRaw, &change); err != nil {
		return fmt.Errorf("invalid change policy")
	}
	if change.MinutesBeforeRequiredAt < 0 || change.MinutesBeforeRequiredAt > 90*24*60 {
		return fmt.Errorf("invalid change policy cutoff")
	}
	if change.DisallowedAfterMilestone != "" {
		if _, ok := milestoneOrder[change.DisallowedAfterMilestone]; !ok {
			return fmt.Errorf("invalid change policy milestone")
		}
	}
	var cancellation cancellationPolicyDefinition
	if err := decodeBoundedPolicy(cancellationRaw, &cancellation); err != nil {
		return fmt.Errorf("invalid cancellation policy")
	}
	if cancellation.ReviewMinutesBeforeRequiredAt < 0 || cancellation.ReviewMinutesBeforeRequiredAt > 90*24*60 || cancellation.RestrictMinutesBeforeRequiredAt < 0 || cancellation.RestrictMinutesBeforeRequiredAt > 90*24*60 {
		return fmt.Errorf("invalid cancellation policy cutoff")
	}
	for _, milestone := range []string{cancellation.ReviewAfterMilestone, cancellation.RestrictAfterMilestone} {
		if milestone != "" {
			if _, ok := milestoneOrder[milestone]; !ok {
				return fmt.Errorf("invalid cancellation policy milestone")
			}
		}
	}
	return nil
}

func (s *Service) CreateRequest(ctx context.Context, business string, in CustomRequestInput) (string, error) {
	if len(in.Notes) > 4000 || len(in.CustomerRef) > 200 || len(in.CorporateProfileRef) > 200 || len(in.Title) < 2 || len(in.Title) > 200 {
		return "", fmt.Errorf("notes too long")
	}
	if in.ChangeCutoffAt != nil && in.RequiredAt != nil && in.ChangeCutoffAt.After(*in.RequiredAt) {
		return "", fmt.Errorf("change cutoff must precede required time")
	}
	changePolicy, e := json.Marshal(in.ChangePolicy)
	if e != nil || len(changePolicy) > 8192 {
		return "", fmt.Errorf("invalid change policy")
	}
	cancellationPolicy, e := json.Marshal(in.CancellationPolicy)
	if e != nil || len(cancellationPolicy) > 8192 {
		return "", fmt.Errorf("invalid cancellation policy")
	}
	if e = validateStoredPolicies(changePolicy, cancellationPolicy); e != nil {
		return "", e
	}
	if e = validateLogisticsMetadata(in.Logistics); e != nil {
		return "", e
	}
	if e = validateRiskFlags(in.RiskFlags); e != nil {
		return "", e
	}
	logistics, e := json.Marshal(in.Logistics)
	if e != nil || len(logistics) > 8192 {
		return "", fmt.Errorf("invalid logistics metadata")
	}
	riskFlags, e := json.Marshal(in.RiskFlags)
	if e != nil || len(riskFlags) > 2048 {
		return "", fmt.Errorf("invalid risk flags")
	}
	id := uuid.NewString()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_custom_requests(id,business_id,branch_id,customer_ref,corporate_profile_ref,title,notes,event_at,change_cutoff_at,change_policy,cancellation_policy,logistics,risk_flags) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13)`, id, business, in.BranchID, in.CustomerRef, in.CorporateProfileRef, in.Title, in.Notes, in.RequiredAt, in.ChangeCutoffAt, changePolicy, cancellationPolicy, logistics, riskFlags); e != nil {
		return "", e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.custom_request.created", "pastry.custom_request.created", "pastry_custom_request", id, in.BranchID, 1, nil, map[string]any{"title": in.Title, "required_at": in.RequiredAt, "logistics": in.Logistics, "risk_flags": in.RiskFlags}, map[string]any{"id": id}, ""); e != nil {
			return "", e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return id, nil
}

var allowedRiskFlags = map[string]bool{"LATE_START": true, "CAPACITY_AT_RISK": true, "CUSTOMER_CHANGE_PENDING": true, "MATERIAL_DEPENDENCY": true, "QUALITY_REVIEW_REQUIRED": true}

func validateLogisticsMetadata(v map[string]any) error {
	if len(v) > 8 {
		return fmt.Errorf("too many logistics fields")
	}
	for field, value := range v {
		switch field {
		case "temperature_class":
			x, ok := value.(string)
			if !ok || (x != "AMBIENT" && x != "CHILLED" && x != "FROZEN") {
				return fmt.Errorf("invalid temperature_class")
			}
		case "spill_risk":
			x, ok := value.(string)
			if !ok || (x != "LOW" && x != "MEDIUM" && x != "HIGH") {
				return fmt.Errorf("invalid spill_risk")
			}
		case "weight_grams":
			x, ok := value.(float64)
			if !ok || x < 0 || x > 1000000 {
				return fmt.Errorf("weight_grams is outside supported bounds")
			}
		case "fragile", "upright_required", "cold_chain_required":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", field)
			}
		case "package_size":
			x, ok := value.(string)
			if !ok || (x != "SMALL" && x != "MEDIUM" && x != "LARGE" && x != "OVERSIZE") {
				return fmt.Errorf("invalid package_size")
			}
		case "vehicle_suitability_hints":
			x, ok := value.([]any)
			if !ok || len(x) > 8 {
				return fmt.Errorf("invalid vehicle_suitability_hints")
			}
			for _, hint := range x {
				if text, ok := hint.(string); !ok || len(text) > 80 {
					return fmt.Errorf("invalid vehicle suitability hint")
				}
			}
		default:
			return fmt.Errorf("unsupported logistics field: %s", field)
		}
	}
	return nil
}

func validateRiskFlags(flags []string) error {
	if len(flags) > 12 {
		return fmt.Errorf("too many risk flags")
	}
	for _, flag := range flags {
		if !allowedRiskFlags[strings.ToUpper(flag)] {
			return fmt.Errorf("unsupported risk flag")
		}
	}
	return nil
}

func (s *Service) SetRequestMetadata(ctx context.Context, business, id string, logistics map[string]any, riskFlags []string) error {
	if err := validateLogisticsMetadata(logistics); err != nil {
		return err
	}
	if err := validateRiskFlags(riskFlags); err != nil {
		return err
	}
	lb, _ := json.Marshal(logistics)
	rb, _ := json.Marshal(riskFlags)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,version FROM pastry_custom_requests WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, id).Scan(&branch, &version); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE pastry_custom_requests SET logistics=$1,risk_flags=$2,version=version+1,updated_at=now() WHERE business_id=$3 AND id=$4 AND version=$5`, lb, rb, business, id, version); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.custom_request.metadata.updated", "pastry.custom_request.metadata.updated", "pastry_custom_request", id, branch, version+1, nil, map[string]any{"logistics": logistics, "risk_flags": riskFlags}, nil, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) UpdateRequest(ctx context.Context, business, id, title, notes string, event *time.Time, expected int64) error {
	if len(notes) > 4000 {
		return fmt.Errorf("notes too long")
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var branch, status string
	var version int64
	if e = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM pastry_custom_requests WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, id).Scan(&branch, &status, &version); e != nil {
		return e
	}
	if status != "DRAFT" && status != "SUBMITTED" {
		return fmt.Errorf("request is no longer mutable")
	}
	if expected <= 0 || expected != version {
		return fmt.Errorf("stale request version")
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET title=$1,notes=$2,event_at=$3,version=version+1,updated_at=now() WHERE business_id=$4 AND id=$5 AND version=$6`, title, notes, event, business, id, version); e != nil {
		return e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.custom_request.updated", "pastry.custom_request.updated", "pastry_custom_request", id, branch, version+1, nil, map[string]any{"title": title}, map[string]any{"id": id}, ""); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

type ChangeEligibility struct {
	Allowed           bool       `json:"allowed"`
	ReasonCode        string     `json:"reason_code"`
	CutoffAt          *time.Time `json:"cutoff_at"`
	BlockingMilestone string     `json:"blocking_milestone,omitempty"`
	EvaluatedAt       time.Time  `json:"evaluated_at"`
}

type CancellationEligibility struct {
	State             string     `json:"state"`
	ReasonCode        string     `json:"reason_code"`
	ReviewAt          *time.Time `json:"review_at,omitempty"`
	RestrictAt        *time.Time `json:"restrict_at,omitempty"`
	BlockingMilestone string     `json:"blocking_milestone,omitempty"`
	EvaluatedAt       time.Time  `json:"evaluated_at"`
}

func (s *Service) latestMilestone(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, requestID string) (string, int, error) {
	var milestone string
	var position int
	err := queryer.QueryRow(ctx, `SELECT milestone_type,position FROM pastry_milestones WHERE request_id=$1 AND state IN ('ACTIVE','BLOCKED','READY','COMPLETED') ORDER BY position DESC LIMIT 1`, requestID).Scan(&milestone, &position)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", -1, nil
	}
	return milestone, position, err
}

func effectivePolicyTime(requiredAt *time.Time, minutes int) *time.Time {
	if requiredAt == nil || minutes <= 0 {
		return nil
	}
	value := requiredAt.Add(-time.Duration(minutes) * time.Minute)
	return &value
}

func earlierTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil || a.Before(*b) {
		return a
	}
	return b
}

func (s *Service) changeEligibilityWith(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, business, requestID string) (ChangeEligibility, error) {
	now := s.production.Now()
	result := ChangeEligibility{Allowed: true, ReasonCode: "ALLOWED", EvaluatedAt: now}
	var status string
	var requiredAt, explicitCutoff *time.Time
	var raw []byte
	if err := queryer.QueryRow(ctx, `SELECT status,event_at,change_cutoff_at,change_policy FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, business, requestID).Scan(&status, &requiredAt, &explicitCutoff, &raw); err != nil {
		return ChangeEligibility{}, err
	}
	var policy changePolicyDefinition
	if err := decodeBoundedPolicy(raw, &policy); err != nil {
		return ChangeEligibility{}, err
	}
	result.CutoffAt = earlierTime(explicitCutoff, effectivePolicyTime(requiredAt, policy.MinutesBeforeRequiredAt))
	if status == "CANCELLED" || status == "CONVERTED" || status == "COMPLETED" || status == "DECLINED" || status == "EXPIRED" {
		result.Allowed, result.ReasonCode = false, "REQUEST_TERMINAL"
		return result, nil
	}
	if result.CutoffAt != nil && !now.Before(*result.CutoffAt) {
		result.Allowed, result.ReasonCode = false, "CUTOFF_PASSED"
		return result, nil
	}
	blockingAfter := policy.DisallowedAfterMilestone
	if blockingAfter == "" {
		blockingAfter = "DECORATION"
	}
	milestone, position, err := s.latestMilestone(ctx, queryer, requestID)
	if err != nil {
		return ChangeEligibility{}, err
	}
	if position >= milestoneOrder[blockingAfter] {
		result.Allowed, result.ReasonCode, result.BlockingMilestone = false, "MILESTONE_CUTOFF", milestone
	}
	return result, nil
}

func (s *Service) ChangeEligibility(ctx context.Context, business, requestID string) (ChangeEligibility, error) {
	return s.changeEligibilityWith(ctx, s.db, business, requestID)
}

func (s *Service) cancellationEligibilityWith(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, business, requestID string) (CancellationEligibility, error) {
	now := s.production.Now()
	result := CancellationEligibility{State: "ALLOWED", ReasonCode: "BEFORE_PRODUCTION", EvaluatedAt: now}
	var status string
	var requiredAt *time.Time
	var raw []byte
	if err := queryer.QueryRow(ctx, `SELECT status,event_at,cancellation_policy FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, business, requestID).Scan(&status, &requiredAt, &raw); err != nil {
		return CancellationEligibility{}, err
	}
	if status == "CANCELLED" || status == "COMPLETED" || status == "CONVERTED" || status == "DECLINED" || status == "EXPIRED" {
		result.State, result.ReasonCode = "RESTRICTED", "REQUEST_TERMINAL"
		return result, nil
	}
	var policy cancellationPolicyDefinition
	if err := decodeBoundedPolicy(raw, &policy); err != nil {
		return CancellationEligibility{}, err
	}
	reviewMilestone, restrictMilestone := policy.ReviewAfterMilestone, policy.RestrictAfterMilestone
	if reviewMilestone == "" {
		reviewMilestone = "BASE_PREPARED"
	}
	if restrictMilestone == "" {
		restrictMilestone = "DECORATION"
	}
	result.ReviewAt = effectivePolicyTime(requiredAt, policy.ReviewMinutesBeforeRequiredAt)
	result.RestrictAt = effectivePolicyTime(requiredAt, policy.RestrictMinutesBeforeRequiredAt)
	milestone, position, err := s.latestMilestone(ctx, queryer, requestID)
	if err != nil {
		return CancellationEligibility{}, err
	}
	if (result.RestrictAt != nil && !now.Before(*result.RestrictAt)) || position >= milestoneOrder[restrictMilestone] {
		result.State, result.ReasonCode, result.BlockingMilestone = "RESTRICTED", "PRODUCTION_RESTRICTED", milestone
		return result, nil
	}
	if status == "ACCEPTED_PENDING_ORDER" || (result.ReviewAt != nil && !now.Before(*result.ReviewAt)) || position >= milestoneOrder[reviewMilestone] {
		result.State, result.ReasonCode, result.BlockingMilestone = "REQUIRES_REVIEW", "REVIEW_REQUIRED", milestone
	}
	return result, nil
}

func (s *Service) CancellationEligibility(ctx context.Context, business, requestID string) (CancellationEligibility, error) {
	return s.cancellationEligibilityWith(ctx, s.db, business, requestID)
}

func (s *Service) CancelRequest(ctx context.Context, actor production.Actor, scope production.Scope, requestID, reason string, reviewApproved bool, expectedVersion int64) (CancellationEligibility, error) {
	if len(strings.TrimSpace(reason)) > 280 {
		return CancellationEligibility{}, fmt.Errorf("cancellation reason is too long")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CancellationEligibility{}, err
	}
	defer tx.Rollback(ctx)
	var branch, status string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM pastry_custom_requests WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, requestID).Scan(&branch, &status, &version); err != nil || !scope.Covers(branch) {
		return CancellationEligibility{}, fmt.Errorf("request not found")
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return CancellationEligibility{}, fmt.Errorf("stale request version")
	}
	eligibility, err := s.cancellationEligibilityWith(ctx, tx, actor.VendorID, requestID)
	if err != nil {
		return CancellationEligibility{}, err
	}
	if eligibility.State == "RESTRICTED" {
		return eligibility, fmt.Errorf("cancellation is restricted")
	}
	if eligibility.State == "REQUIRES_REVIEW" && (!reviewApproved || strings.TrimSpace(reason) == "") {
		return eligibility, fmt.Errorf("cancellation review approval and reason are required")
	}
	var unsafeJob bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_jobs j JOIN pastry_quotes q ON q.id=j.source_ref WHERE q.request_id=$1 AND j.business_id=$2 AND j.source_type='CUSTOM_CAKE_QUOTE' AND j.status IN ('IN_PROGRESS','READY','COMPLETED'))`, requestID, actor.VendorID).Scan(&unsafeJob); err != nil {
		return CancellationEligibility{}, err
	}
	if unsafeJob {
		eligibility.State, eligibility.ReasonCode = "RESTRICTED", "PRODUCTION_ALREADY_STARTED"
		return eligibility, fmt.Errorf("production already started")
	}
	reservationRows, err := tx.Query(ctx, `SELECT DISTINCT q.reservation_id FROM pastry_quotes q JOIN capacity_reservations r ON r.id=q.reservation_id WHERE q.request_id=$1 AND r.business_id=$2 AND r.status IN ('HELD','CONFIRMED')`, requestID, actor.VendorID)
	if err != nil {
		return CancellationEligibility{}, err
	}
	reservationIDs := []string{}
	for reservationRows.Next() {
		var reservationID string
		if err = reservationRows.Scan(&reservationID); err != nil {
			reservationRows.Close()
			return CancellationEligibility{}, err
		}
		reservationIDs = append(reservationIDs, reservationID)
	}
	reservationRows.Close()
	for _, reservationID := range reservationIDs {
		if _, _, err = s.production.CloseReservationInTx(ctx, tx, actor, scope, reservationID, production.ReservationCancelled, reason); err != nil {
			return CancellationEligibility{}, err
		}
	}
	jobRows, err := tx.Query(ctx, `SELECT j.id,j.status FROM production_jobs j JOIN pastry_quotes q ON q.id=j.source_ref WHERE q.request_id=$1 AND j.business_id=$2 AND j.source_type='CUSTOM_CAKE_QUOTE' AND j.status IN ('PLANNED','QUEUED','BLOCKED') FOR UPDATE`, requestID, actor.VendorID)
	if err != nil {
		return CancellationEligibility{}, err
	}
	type cancellableJob struct{ id, status string }
	jobs := []cancellableJob{}
	for jobRows.Next() {
		var job cancellableJob
		if err = jobRows.Scan(&job.id, &job.status); err != nil {
			jobRows.Close()
			return CancellationEligibility{}, err
		}
		jobs = append(jobs, job)
	}
	jobRows.Close()
	for _, job := range jobs {
		if _, err = tx.Exec(ctx, `UPDATE production_jobs SET status='CANCELLED',updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2`, actor.VendorID, job.id); err != nil {
			return CancellationEligibility{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,from_status,to_status,actor_type,actor_id,correlation_id,note) VALUES($1,$2,$3,'CANCELLED',$4,'CANCELLED','vendor_staff',NULLIF($5,''),NULLIF($6,''),$7)`, "pje_"+uuid.NewString(), actor.VendorID, job.id, job.status, actor.StaffID, actor.CorrelationID, reason); err != nil {
			return CancellationEligibility{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE pastry_quotes SET status='CANCELLED',version=version+1 WHERE request_id=$1 AND status IN ('DRAFT','PENDING_APPROVAL','APPROVED')`, requestID); err != nil {
		return CancellationEligibility{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE pastry_custom_requests SET status='CANCELLED',version=version+1,updated_at=now() WHERE business_id=$1 AND id=$2 AND version=$3`, actor.VendorID, requestID, version)
	if err != nil {
		return CancellationEligibility{}, err
	}
	if tag.RowsAffected() == 0 {
		return CancellationEligibility{}, fmt.Errorf("stale request version")
	}
	if err = s.production.RecordMutation(ctx, tx, actor, "pastry.custom_request.cancelled", "pastry.custom_request.cancelled", "pastry_custom_request", requestID, branch, version+1, map[string]any{"status": status, "version": version}, map[string]any{"status": "CANCELLED", "version": version + 1}, map[string]any{"request_id": requestID, "review_approved": reviewApproved, "reason": reason}, ""); err != nil {
		return CancellationEligibility{}, err
	}
	return eligibility, tx.Commit(ctx)
}
func (s *Service) AddSpec(ctx context.Context, request string, spec map[string]any, expectedVersion int64) (string, int, error) {
	if e := validateSpecification(spec); e != nil {
		return "", 0, e
	}
	b, e := json.Marshal(spec)
	if e != nil {
		return "", 0, e
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", 0, e
	}
	defer tx.Rollback(ctx)
	var business, branch, status string
	var requestVersion int64
	if e = tx.QueryRow(ctx, `SELECT business_id,branch_id,status,version FROM pastry_custom_requests WHERE id=$1 FOR UPDATE`, request).Scan(&business, &branch, &status, &requestVersion); e != nil {
		return "", 0, e
	}
	if expectedVersion <= 0 || expectedVersion != requestVersion {
		return "", 0, fmt.Errorf("stale request version")
	}
	if status == "ACCEPTED" || status == "ACCEPTED_PENDING_ORDER" || status == "CONVERTED" || status == "CANCELLED" || status == "COMPLETED" {
		return "", 0, fmt.Errorf("request is no longer mutable")
	}
	var rev int
	if e = tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM pastry_specification_versions WHERE request_id=$1`, request).Scan(&rev); e != nil {
		return "", 0, e
	}
	id := uuid.NewString()
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_specification_versions(id,request_id,revision,specification) VALUES($1,$2,$3,$4)`, id, request, rev, b); e != nil {
		return "", 0, e
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, request, requestVersion); e != nil {
		return "", 0, e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.specification.created", "pastry.specification.created", "pastry_custom_request", request, branch, requestVersion+1, map[string]any{"version": requestVersion}, map[string]any{"version": requestVersion + 1, "specification_revision": rev}, map[string]any{"specification_id": id}, ""); e != nil {
			return "", 0, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", 0, e
	}
	return id, rev, nil
}

func validateSpecification(spec map[string]any) error {
	allowed := map[string]bool{"occasion": true, "required_at": true, "servings": true, "theme": true, "colours": true, "inscription": true, "selected_options": true, "reference_media": true, "notes": true, "production_requirements": true, "logistics_requirements": true}
	for field := range spec {
		if !allowed[field] {
			return fmt.Errorf("unsupported specification field: %s", field)
		}
	}
	if servings, ok := spec["servings"].(float64); ok && (servings < 1 || servings > 10000 || servings != float64(int(servings))) {
		return fmt.Errorf("servings must be a whole number between 1 and 10000")
	}
	if notes, ok := spec["notes"].(string); ok && len(notes) > 4000 {
		return fmt.Errorf("specification notes are too long")
	}
	if media, ok := spec["reference_media"].([]any); ok && len(media) > 20 {
		return fmt.Errorf("too many reference media items")
	}
	if media, ok := spec["reference_media"].([]any); ok {
		for _, value := range media {
			ref, valid := value.(string)
			if !valid || strings.TrimSpace(ref) == "" || len(ref) > 200 {
				return fmt.Errorf("invalid reference media identifier")
			}
		}
	}
	raw, err := json.Marshal(spec)
	if err != nil || len(raw) > 65536 {
		return fmt.Errorf("specification is too large")
	}
	return nil
}

type QuoteCapacityInput struct {
	WindowStart       time.Time
	WindowEnd         time.Time
	Allocations       []production.AllocationRequest
	HoldFor           time.Duration
	ApprovalRequired  *bool
	VendorTerms       string
	NegotiationReason string
	NegotiationNote   string
}

// ensureCurrentEffectiveSpecification rejects proposed/declined changes and
// any immutable specification superseded by a later direct or approved
// revision. Quote creation, revision, approval and acceptance all call this
// inside their existing request/quote transaction so no stale-spec window is
// left between validation and mutation.
func ensureCurrentEffectiveSpecification(ctx context.Context, tx pgx.Tx, requestID, specificationID string) error {
	var currentID string
	err := tx.QueryRow(ctx, `
		SELECT s.id
		FROM pastry_specification_versions s
		LEFT JOIN pastry_change_requests c
		  ON c.request_id=s.request_id AND c.to_revision=s.revision
		WHERE s.request_id=$1
		  AND (c.id IS NULL OR c.status='APPROVED')
		ORDER BY s.revision DESC
		LIMIT 1`, requestID).Scan(&currentID)
	if err != nil {
		return fmt.Errorf("current specification not found: %w", err)
	}
	if currentID != specificationID {
		return apperrors.Conflict("The specification is superseded or is not an approved revision.", nil)
	}
	return nil
}

func (s *Service) CreateQuote(ctx context.Context, actor production.Actor, scope production.Scope, request, spec string, amount int64, expires time.Time, capacity QuoteCapacityInput, expectedRequestVersion int64) (string, string, error) {
	if amount < 0 || !expires.After(s.production.Now()) {
		return "", "", fmt.Errorf("invalid quote")
	}
	id := uuid.NewString()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", "", e
	}
	defer tx.Rollback(ctx)
	var business, branch, requestStatus string
	var requestVersion int64
	if e = tx.QueryRow(ctx, `SELECT business_id,branch_id,status,version FROM pastry_custom_requests WHERE id=$1 FOR UPDATE`, request).Scan(&business, &branch, &requestStatus, &requestVersion); e != nil {
		return "", "", e
	}
	if actor.VendorID != business || !scope.Covers(branch) {
		return "", "", fmt.Errorf("forbidden")
	}
	if expectedRequestVersion <= 0 || expectedRequestVersion != requestVersion {
		return "", "", fmt.Errorf("stale request version")
	}
	if requestStatus == "CANCELLED" || requestStatus == "CONVERTED" || requestStatus == "COMPLETED" || requestStatus == "ACCEPTED_PENDING_ORDER" {
		return "", "", fmt.Errorf("request cannot be quoted")
	}
	if len(capacity.VendorTerms) > 4000 || len(capacity.NegotiationReason) > 80 || len(capacity.NegotiationNote) > 1000 {
		return "", "", fmt.Errorf("quote terms or negotiation note is too long")
	}
	var quoteRevision int
	if e = tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM pastry_quotes WHERE request_id=$1`, request).Scan(&quoteRevision); e != nil {
		return "", "", e
	}
	var activeQuoteExists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pastry_quotes WHERE request_id=$1 AND status IN ('DRAFT','PENDING_APPROVAL','APPROVED'))`, request).Scan(&activeQuoteExists); e != nil {
		return "", "", e
	}
	if activeQuoteExists {
		return "", "", fmt.Errorf("an active quote already exists; revise it instead")
	}
	if e = ensureCurrentEffectiveSpecification(ctx, tx, request, spec); e != nil {
		return "", "", e
	}
	if len(capacity.Allocations) > 0 && (!capacity.WindowEnd.After(capacity.WindowStart) || capacity.WindowStart.IsZero()) {
		return "", "", fmt.Errorf("invalid production window")
	}
	var legacyPool *string
	if len(capacity.Allocations) > 0 {
		legacyPool = &capacity.Allocations[0].CapacityPoolID
	}
	approvalRequired := true
	if capacity.ApprovalRequired != nil {
		approvalRequired = *capacity.ApprovalRequired
	}
	quoteStatus, requestQuoteStatus := "PENDING_APPROVAL", "QUOTE_PENDING"
	if !approvalRequired {
		quoteStatus, requestQuoteStatus = "APPROVED", "QUOTED"
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET status=$1,version=version+1,updated_at=now() WHERE id=$2 AND version=$3`, requestQuoteStatus, request, requestVersion); e != nil {
		return "", "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_quotes(id,request_id,revision,specification_version_id,amount_minor,expires_at,status,approval_required,capacity_pool_id,production_start,production_end,vendor_terms,negotiation_reason,negotiation_note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, request, quoteRevision, spec, amount, expires, quoteStatus, approvalRequired, legacyPool, nilTime(capacity.WindowStart), nilTime(capacity.WindowEnd), capacity.VendorTerms, capacity.NegotiationReason, capacity.NegotiationNote); e != nil {
		return "", "", e
	}
	for _, allocation := range capacity.Allocations {
		if allocation.Quantity <= 0 {
			return "", "", fmt.Errorf("invalid capacity requirement")
		}
		if _, e = tx.Exec(ctx, `INSERT INTO pastry_quote_capacity_requirements(quote_id,capacity_pool_id,production_line_id,quantity) VALUES($1,$2,NULLIF($3,''),$4)`, id, allocation.CapacityPoolID, allocation.ProductionLineID, allocation.Quantity); e != nil {
			return "", "", e
		}
	}
	reservationID := ""
	if len(capacity.Allocations) > 0 {
		holdFor := capacity.HoldFor
		remaining := expires.Sub(s.production.Now())
		if holdFor <= 0 || holdFor > remaining {
			holdFor = remaining
			if holdFor > 30*time.Minute {
				holdFor = 30 * time.Minute
			}
		}
		reservation, _, reserveErr := s.production.ReserveInTx(ctx, tx, actor, scope, production.ReserveInput{BranchID: branch, Capability: "PASTRY_BAKERY", WindowStart: capacity.WindowStart, WindowEnd: capacity.WindowEnd, Allocations: capacity.Allocations, SourceType: "CUSTOM_CAKE_QUOTE", SourceRef: id, HoldFor: holdFor})
		if reserveErr != nil {
			return "", "", reserveErr
		}
		reservationID = reservation.ID
		if _, e = tx.Exec(ctx, `UPDATE pastry_quotes SET reservation_id=$1 WHERE id=$2`, reservationID, id); e != nil {
			return "", "", e
		}
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.created", "pastry.quote.created", "pastry_quote", id, branch, 1, nil, map[string]any{"request_id": request, "revision": quoteRevision, "specification_version_id": spec, "amount_minor": amount, "approval_required": approvalRequired, "status": quoteStatus}, map[string]any{"id": id, "revision": quoteRevision}, ""); e != nil {
			return "", "", e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", "", e
	}
	return id, reservationID, nil
}

func nilTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
func (s *Service) ApproveQuote(ctx context.Context, actor production.Actor, scope production.Scope, id string, expectedVersion int64) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var business, branch, status, requestID, specificationID string
	var expires time.Time
	var version, requestVersion int64
	if e = tx.QueryRow(ctx, `SELECT r.business_id,r.branch_id,q.status,q.expires_at,q.version,r.id,r.version,q.specification_version_id FROM pastry_quotes q JOIN pastry_custom_requests r ON r.id=q.request_id WHERE q.id=$1 FOR UPDATE`, id).Scan(&business, &branch, &status, &expires, &version, &requestID, &requestVersion, &specificationID); e != nil {
		return e
	}
	if business != actor.VendorID || !scope.Covers(branch) {
		return fmt.Errorf("quote not found")
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return fmt.Errorf("stale quote version")
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" || !expires.After(s.production.Now()) {
		return fmt.Errorf("quote cannot be approved")
	}
	if e = ensureCurrentEffectiveSpecification(ctx, tx, requestID, specificationID); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE pastry_quotes SET status='APPROVED',approved_by=NULLIF($2,''),approved_at=now(),version=version+1 WHERE id=$1 AND version=$3 AND status IN ('DRAFT','PENDING_APPROVAL')`, id, actor.StaffID, version)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("stale quote version")
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET status='QUOTED',version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, requestID, requestVersion); e != nil {
		return e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.approved", "pastry.quote.approved", "pastry_quote", id, branch, version+1, map[string]any{"status": status, "version": version}, map[string]any{"status": "APPROVED", "approved_by": actor.StaffID, "version": version + 1}, nil, ""); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *Service) AcceptQuote(ctx context.Context, actor production.Actor, scope production.Scope, id string, expectedVersion int64) (string, string, error) {
	var request, spec, business, branch, currency, status, vendorTerms string
	var amount int64
	var quoteVersion int64
	var expires time.Time
	var pool, existingReservation *string
	var productionStart, productionEnd *time.Time
	var logisticsSnapshot, riskFlags []byte
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", "", e
	}
	defer tx.Rollback(ctx)
	if e = tx.QueryRow(ctx, `SELECT q.request_id,q.specification_version_id,q.amount_minor,q.currency,q.expires_at,r.business_id,r.branch_id,q.capacity_pool_id,q.reservation_id,q.production_start,q.production_end,q.status,q.vendor_terms,r.logistics,r.risk_flags,q.version FROM pastry_quotes q JOIN pastry_custom_requests r ON r.id=q.request_id WHERE q.id=$1 FOR UPDATE`, id).Scan(&request, &spec, &amount, &currency, &expires, &business, &branch, &pool, &existingReservation, &productionStart, &productionEnd, &status, &vendorTerms, &logisticsSnapshot, &riskFlags, &quoteVersion); e != nil {
		return "", "", e
	}
	if actor.VendorID != business || !scope.Covers(branch) {
		return "", "", fmt.Errorf("forbidden")
	}
	if !expires.After(s.production.Now()) {
		if existingReservation != nil && *existingReservation != "" {
			if _, _, e = s.production.CloseReservationInTx(ctx, tx, actor, scope, *existingReservation, production.ReservationExpired, "quote expired"); e != nil {
				return "", "", e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE pastry_quotes SET status='EXPIRED',version=version+1 WHERE id=$1 AND version=$2`, id, quoteVersion); e != nil {
			return "", "", e
		}
		if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET status='EXPIRED',version=version+1,updated_at=now() WHERE id=$1`, request); e != nil {
			return "", "", e
		}
		if e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.expired", "pastry.quote.expired", "pastry_quote", id, branch, quoteVersion+1, map[string]any{"status": status, "version": quoteVersion}, map[string]any{"status": "EXPIRED", "version": quoteVersion + 1}, map[string]any{"quote_id": id}, "quote expired"); e != nil {
			return "", "", e
		}
		if e = tx.Commit(ctx); e != nil {
			return "", "", e
		}
		return "", "", fmt.Errorf("quote expired")
	}
	if status == "ACCEPTED" {
		var intent, reservation string
		e = tx.QueryRow(ctx, `SELECT i.id,COALESCE(q.reservation_id,'') FROM pastry_order_conversion_intents i JOIN pastry_quotes q ON q.id=i.quote_id WHERE i.quote_id=$1`, id).Scan(&intent, &reservation)
		return intent, reservation, e
	}
	if expectedVersion <= 0 || expectedVersion != quoteVersion {
		return "", "", fmt.Errorf("stale quote version")
	}
	if status != "APPROVED" {
		return "", "", fmt.Errorf("quote not approved")
	}
	if e = ensureCurrentEffectiveSpecification(ctx, tx, request, spec); e != nil {
		return "", "", e
	}
	allocations := []production.AllocationRequest{}
	rows, queryErr := tx.Query(ctx, `SELECT capacity_pool_id,COALESCE(production_line_id,''),quantity FROM pastry_quote_capacity_requirements WHERE quote_id=$1 ORDER BY capacity_pool_id`, id)
	if queryErr != nil {
		return "", "", queryErr
	}
	for rows.Next() {
		var allocation production.AllocationRequest
		if queryErr = rows.Scan(&allocation.CapacityPoolID, &allocation.ProductionLineID, &allocation.Quantity); queryErr != nil {
			rows.Close()
			return "", "", queryErr
		}
		allocations = append(allocations, allocation)
	}
	rows.Close()
	if len(allocations) == 0 && pool != nil && *pool != "" {
		allocations = append(allocations, production.AllocationRequest{CapacityPoolID: *pool, Quantity: 1})
	}
	var reservationID string
	var capacitySnapshot []production.AllocationView
	if existingReservation != nil && *existingReservation != "" {
		var reservationVersion int64
		if e = tx.QueryRow(ctx, `SELECT version FROM capacity_reservations WHERE business_id=$1 AND id=$2`, actor.VendorID, *existingReservation).Scan(&reservationVersion); e != nil {
			return "", "", e
		}
		confirmed, granted, confirmErr := s.production.ConfirmInTx(ctx, tx, actor, scope, *existingReservation, reservationVersion)
		if confirmErr != nil {
			return "", "", confirmErr
		}
		reservationID, capacitySnapshot = confirmed.ID, granted
	} else {
		if len(allocations) == 0 || productionStart == nil || productionEnd == nil {
			return "", "", fmt.Errorf("quote has no capacity reservation or production requirements")
		}
		reserved, granted, reserveErr := s.production.ReserveInTx(ctx, tx, actor, scope, production.ReserveInput{BranchID: branch, Capability: "PASTRY_BAKERY", WindowStart: *productionStart, WindowEnd: *productionEnd, Allocations: allocations, SourceType: "CUSTOM_CAKE_QUOTE", SourceRef: id})
		if reserveErr != nil {
			return "", "", reserveErr
		}
		reservationID, capacitySnapshot = reserved.ID, granted
	}
	snap, intent := uuid.NewString(), uuid.NewString()
	var specJSON []byte
	if e = tx.QueryRow(ctx, `SELECT specification FROM pastry_specification_versions WHERE id=$1`, spec).Scan(&specJSON); e != nil {
		return "", "", e
	}
	capacityJSON, _ := json.Marshal(capacitySnapshot)
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_accepted_snapshots(id,quote_id,request_id,quote_revision,specification,amount_minor,currency,logistics_snapshot,risk_flags,capacity_snapshot,vendor_terms,accepted_at) SELECT $1,$2,$3,revision,$4,$5,$6,$7,$8,$9,$10,now() FROM pastry_specification_versions WHERE id=$11`, snap, id, request, specJSON, amount, currency, logisticsSnapshot, riskFlags, capacityJSON, vendorTerms, spec); e != nil {
		return "", "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_order_conversion_intents(id,request_id,quote_id,snapshot_id,idempotency_key) VALUES($1,$2,$3,$4,$5)`, intent, request, id, snap, "quote-"+id); e != nil {
		return "", "", e
	}
	jobID := "job_" + uuid.NewString()
	if _, e = tx.Exec(ctx, `INSERT INTO production_jobs(id,business_id,branch_id,capability,reservation_id,source_type,source_ref,source_version,title,planned_quantity,risk_flags,scheduled_start,scheduled_end) SELECT $1,$2,$3,'PASTRY_BAKERY',$4,'CUSTOM_CAKE_QUOTE',$5,revision,$6,1,$7,$8,$9 FROM pastry_quotes WHERE id=$5`, jobID, actor.VendorID, branch, reservationID, id, "Custom cake "+id, riskFlags, productionStart, productionEnd); e != nil {
		return "", "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,to_status,actor_type,actor_id,note) VALUES($1,$2,$3,'CREATED','PLANNED','vendor_staff',NULLIF($4,''),'custom cake quote accepted')`, "pje_"+uuid.NewString(), actor.VendorID, jobID, actor.StaffID); e != nil {
		return "", "", e
	}
	for pos, milestone := range []string{"DESIGN_APPROVED", "BASE_PREPARED", "DECORATION", "CHILLING", "PACKED", "READY"} {
		mid := "milestone_" + uuid.NewString()
		if _, e = tx.Exec(ctx, `INSERT INTO pastry_milestones(id,request_id,milestone_type,state,position) VALUES($1,$2,$3,CASE WHEN $4=0 THEN 'ACTIVE' ELSE 'PENDING' END,$4) ON CONFLICT(request_id,milestone_type) DO NOTHING`, mid, request, milestone, pos); e != nil {
			return "", "", e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_quotes SET status='ACCEPTED',accepted_at=now(),reservation_id=$1,version=version+1 WHERE id=$2 AND status='APPROVED' AND version=$3`, reservationID, id, quoteVersion); e != nil {
		return "", "", e
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET status='ACCEPTED_PENDING_ORDER',version=version+1,updated_at=now() WHERE id=$1`, request); e != nil {
		return "", "", e
	}
	if e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.accepted", "pastry.quote.accepted", "pastry_accepted_snapshot", snap, branch, 1, nil, map[string]any{"quote_id": id, "request_id": request, "reservation_id": reservationID, "conversion_intent_id": intent, "production_job_id": jobID}, map[string]any{"quote_id": id, "snapshot_id": snap, "conversion_intent_id": intent, "reservation_id": reservationID, "production_job_id": jobID}, ""); e != nil {
		return "", "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return "", "", e
	}
	return intent, reservationID, nil
}

func (s *Service) ExpireQuotes(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, e := s.db.Query(ctx, `SELECT q.id,r.business_id,r.branch_id,q.reservation_id FROM pastry_quotes q JOIN pastry_custom_requests r ON r.id=q.request_id WHERE q.status IN ('DRAFT','PENDING_APPROVAL','APPROVED') AND q.expires_at<=now() ORDER BY q.expires_at LIMIT $1`, limit)
	if e != nil {
		return 0, e
	}
	type dueQuote struct {
		id, business, branch string
		reservation          *string
	}
	due := []dueQuote{}
	for rows.Next() {
		var q dueQuote
		if e = rows.Scan(&q.id, &q.business, &q.branch, &q.reservation); e != nil {
			rows.Close()
			return 0, e
		}
		due = append(due, q)
	}
	rows.Close()
	count := 0
	for _, q := range due {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return count, beginErr
		}
		var status string
		var version int64
		if e = tx.QueryRow(ctx, `SELECT status,version FROM pastry_quotes WHERE id=$1 FOR UPDATE`, q.id).Scan(&status, &version); e == nil && status != "EXPIRED" && status != "ACCEPTED" {
			actor := production.Actor{VendorID: q.business}
			if q.reservation != nil && *q.reservation != "" {
				_, _, e = s.production.CloseReservationInTx(ctx, tx, actor, production.Scope{Global: true}, *q.reservation, production.ReservationExpired, "quote expired")
			}
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE pastry_quotes SET status='EXPIRED',version=version+1 WHERE id=$1 AND version=$2`, q.id, version)
			}
			if e == nil {
				e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.expired", "pastry.quote.expired", "pastry_quote", q.id, q.branch, version+1, map[string]any{"status": status, "version": version}, map[string]any{"status": "EXPIRED", "version": version + 1}, map[string]any{"quote_id": q.id}, "quote expired")
			}
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
		_ = tx.Rollback(ctx)
		if e != nil {
			return count, e
		}
		count++
	}
	return count, nil
}

type SpecificationDifference struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

func (s *Service) SpecificationDiff(ctx context.Context, fromID, toID string) ([]SpecificationDifference, error) {
	var fromRaw, toRaw []byte
	if err := s.db.QueryRow(ctx, `SELECT specification FROM pastry_specification_versions WHERE id=$1`, fromID).Scan(&fromRaw); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(ctx, `SELECT specification FROM pastry_specification_versions WHERE id=$1`, toID).Scan(&toRaw); err != nil {
		return nil, err
	}
	var from, to map[string]any
	if json.Unmarshal(fromRaw, &from) != nil || json.Unmarshal(toRaw, &to) != nil {
		return nil, fmt.Errorf("invalid specification")
	}
	fields := []string{"occasion", "required_at", "servings", "theme", "colours", "inscription", "selected_options", "reference_media", "notes", "production_requirements", "logistics_requirements"}
	out := []SpecificationDifference{}
	for _, field := range fields {
		a, ao := from[field]
		b, bo := to[field]
		if !ao && !bo {
			continue
		}
		ab, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		if string(ab) != string(bb) {
			out = append(out, SpecificationDifference{Field: field, Before: a, After: b})
		}
	}
	return out, nil
}

func (s *Service) RequestChange(ctx context.Context, request, reason string, spec map[string]any, expectedVersion int64) (string, string, int, error) {
	if e := validateSpecification(spec); e != nil {
		return "", "", 0, e
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", "", 0, e
	}
	defer tx.Rollback(ctx)
	var business, branch string
	var requestVersion int64
	if e = tx.QueryRow(ctx, `SELECT business_id,branch_id,version FROM pastry_custom_requests WHERE id=$1 FOR UPDATE`, request).Scan(&business, &branch, &requestVersion); e != nil {
		return "", "", 0, e
	}
	if expectedVersion <= 0 || expectedVersion != requestVersion {
		return "", "", 0, fmt.Errorf("stale request version")
	}
	eligibility, eligibilityErr := s.changeEligibilityWith(ctx, tx, business, request)
	if eligibilityErr != nil {
		return "", "", 0, eligibilityErr
	}
	if !eligibility.Allowed {
		return "", "", 0, fmt.Errorf("request change is not allowed: %s", eligibility.ReasonCode)
	}
	var from int
	if e = tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0) FROM pastry_specification_versions WHERE request_id=$1`, request).Scan(&from); e != nil {
		return "", "", 0, e
	}
	b, e := json.Marshal(spec)
	if e != nil {
		return "", "", 0, e
	}
	specID := uuid.NewString()
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_specification_versions(id,request_id,revision,specification) VALUES($1,$2,$3,$4)`, specID, request, from+1, b); e != nil {
		return "", "", 0, e
	}
	changeID := uuid.NewString()
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_change_requests(id,request_id,from_revision,to_revision,reason,status) VALUES($1,$2,$3,$4,$5,'OPEN')`, changeID, request, from, from+1, reason); e != nil {
		return "", "", 0, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_change_request_history(id,change_request_id,from_status,to_status,note) VALUES($1,$2,NULL,'OPEN',$3)`, uuid.NewString(), changeID, reason); e != nil {
		return "", "", 0, e
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, request, requestVersion); e != nil {
		return "", "", 0, e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "pastry.change_request.created", "pastry.change_request.created", "pastry_change_request", changeID, branch, 1, nil, map[string]any{"request_id": request, "from_revision": from, "to_revision": from + 1, "request_version": requestVersion + 1}, map[string]any{"id": changeID, "specification_id": specID}, ""); e != nil {
			return "", "", 0, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", "", 0, e
	}
	return changeID, specID, from + 1, nil
}

// DecideChange records an immutable decision.  The proposed specification is
// already a new immutable revision; approval only changes the workflow state
// and never rewrites either version.
func (s *Service) DecideChange(ctx context.Context, business, changeID, decision, actorID, note string, expectedVersion int64) error {
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if decision != "APPROVED" && decision != "DECLINED" {
		return fmt.Errorf("invalid change decision")
	}
	if len(note) > 280 {
		return fmt.Errorf("decision note is too long")
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var request, fromStatus, branch string
	var version int64
	if e = tx.QueryRow(ctx, `SELECT c.request_id,c.status,r.branch_id,c.version FROM pastry_change_requests c JOIN pastry_custom_requests r ON r.id=c.request_id WHERE c.id=$1 AND r.business_id=$2 FOR UPDATE`, changeID, business).Scan(&request, &fromStatus, &branch, &version); e != nil {
		return e
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return fmt.Errorf("stale change request version")
	}
	if fromStatus != "OPEN" {
		return fmt.Errorf("change request is already decided")
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_change_requests SET status=$1,decided_at=now(),decided_by=NULLIF($2,''),decision_note=$3,updated_at=now(),version=version+1 WHERE id=$4 AND version=$5`, decision, actorID, note, changeID, version); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_change_request_history(id,change_request_id,from_status,to_status,actor_id,note) VALUES($1,$2,$3,$4,$5,$6)`, uuid.NewString(), changeID, fromStatus, decision, actorID, note); e != nil {
		return e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, production.Actor{StaffID: actorID, VendorID: business}, "pastry.change_request."+strings.ToLower(decision), "pastry.change_request."+strings.ToLower(decision), "pastry_change_request", changeID, branch, version+1, map[string]any{"status": fromStatus, "version": version}, map[string]any{"status": decision, "version": version + 1}, map[string]any{"request_id": request, "note": note}, ""); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) ListChangeHistory(ctx context.Context, business, changeID string) ([]map[string]any, error) {
	rows, e := s.db.Query(ctx, `SELECT h.from_status,h.to_status,h.actor_id,h.note,h.occurred_at FROM pastry_change_request_history h JOIN pastry_change_requests c ON c.id=h.change_request_id JOIN pastry_custom_requests r ON r.id=c.request_id WHERE h.change_request_id=$1 AND r.business_id=$2 ORDER BY h.occurred_at,h.id`, changeID, business)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var from, to, actor, note string
		var at time.Time
		if e = rows.Scan(&from, &to, &actor, &note, &at); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"from_status": from, "to_status": to, "actor_id": actor, "note": note, "occurred_at": at})
	}
	return out, rows.Err()
}

func (s *Service) ReviseQuote(ctx context.Context, actor production.Actor, scope production.Scope, quoteID, specID string, amount int64, expires time.Time, reason, note, vendorTerms string, expectedVersion int64) (string, error) {
	if amount < 0 || !expires.After(s.production.Now()) || len(reason) > 80 || len(note) > 1000 || len(vendorTerms) > 4000 {
		return "", fmt.Errorf("invalid quote revision")
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	var request, business, branch string
	var status string
	var reservation *string
	var start, end *time.Time
	var pool *string
	var quoteVersion, requestVersion int64
	var approvalRequired bool
	if e = tx.QueryRow(ctx, `SELECT q.request_id,q.status,r.business_id,r.branch_id,q.reservation_id,q.production_start,q.production_end,q.capacity_pool_id,q.version,r.version,q.approval_required FROM pastry_quotes q JOIN pastry_custom_requests r ON r.id=q.request_id WHERE q.id=$1 FOR UPDATE`, quoteID).Scan(&request, &status, &business, &branch, &reservation, &start, &end, &pool, &quoteVersion, &requestVersion, &approvalRequired); e != nil {
		return "", e
	}
	if business != actor.VendorID || !scope.Covers(branch) {
		return "", fmt.Errorf("quote not found")
	}
	if expectedVersion <= 0 || expectedVersion != quoteVersion {
		return "", fmt.Errorf("stale quote version")
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" && status != "APPROVED" {
		return "", fmt.Errorf("quote cannot be revised")
	}
	if e = ensureCurrentEffectiveSpecification(ctx, tx, request, specID); e != nil {
		return "", e
	}
	var specificationRevision int
	if e = tx.QueryRow(ctx, `SELECT revision FROM pastry_specification_versions WHERE id=$1 AND request_id=$2`, specID, request).Scan(&specificationRevision); e != nil {
		return "", e
	}
	var revision int
	if e = tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM pastry_quotes WHERE request_id=$1`, request).Scan(&revision); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_quotes SET status='SUPERSEDED',superseded_at=now(),version=version+1 WHERE id=$1 AND version=$2`, quoteID, quoteVersion); e != nil {
		return "", e
	}
	newStatus, requestStatus := "PENDING_APPROVAL", "QUOTE_PENDING"
	if !approvalRequired {
		newStatus, requestStatus = "APPROVED", "QUOTED"
	}
	newID := uuid.NewString()
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_quotes(id,request_id,revision,specification_version_id,amount_minor,expires_at,status,approval_required,capacity_pool_id,reservation_id,production_start,production_end,vendor_terms,negotiation_reason,negotiation_note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULL,$10,$11,$12,$13,$14)`, newID, request, revision, specID, amount, expires, newStatus, approvalRequired, pool, start, end, vendorTerms, reason, note); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_quote_capacity_requirements(quote_id,capacity_pool_id,production_line_id,quantity) SELECT $1,capacity_pool_id,production_line_id,quantity FROM pastry_quote_capacity_requirements WHERE quote_id=$2`, newID, quoteID); e != nil {
		return "", e
	}
	allocations := []production.AllocationRequest{}
	allocationRows, allocationErr := tx.Query(ctx, `SELECT capacity_pool_id,COALESCE(production_line_id,''),quantity FROM pastry_quote_capacity_requirements WHERE quote_id=$1 ORDER BY capacity_pool_id`, newID)
	if allocationErr != nil {
		return "", allocationErr
	}
	for allocationRows.Next() {
		var allocation production.AllocationRequest
		if allocationErr = allocationRows.Scan(&allocation.CapacityPoolID, &allocation.ProductionLineID, &allocation.Quantity); allocationErr != nil {
			allocationRows.Close()
			return "", allocationErr
		}
		allocations = append(allocations, allocation)
	}
	allocationRows.Close()
	newReservationID := ""
	if reservation != nil && *reservation != "" {
		var reservationStatus string
		var holdExpires *time.Time
		if e = tx.QueryRow(ctx, `SELECT status,hold_expires_at FROM capacity_reservations WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, *reservation).Scan(&reservationStatus, &holdExpires); e != nil {
			return "", e
		}
		if reservationStatus == "HELD" && holdExpires != nil && holdExpires.After(s.production.Now()) {
			newHoldExpiry := expires
			maximumHoldExpiry := s.production.Now().Add(30 * time.Minute)
			if newHoldExpiry.After(maximumHoldExpiry) {
				newHoldExpiry = maximumHoldExpiry
			}
			if _, e = tx.Exec(ctx, `UPDATE capacity_reservations SET source_ref=$1,hold_expires_at=$2,updated_at=now(),version=version+1 WHERE business_id=$3 AND id=$4 AND status='HELD'`, newID, newHoldExpiry, business, *reservation); e != nil {
				return "", e
			}
			newReservationID = *reservation
		}
	}
	if newReservationID == "" && len(allocations) > 0 {
		if start == nil || end == nil {
			return "", fmt.Errorf("quote has capacity requirements without a production window")
		}
		holdFor := expires.Sub(s.production.Now())
		if holdFor > 30*time.Minute {
			holdFor = 30 * time.Minute
		}
		reserved, _, reserveErr := s.production.ReserveInTx(ctx, tx, actor, scope, production.ReserveInput{BranchID: branch, Capability: "PASTRY_BAKERY", WindowStart: *start, WindowEnd: *end, Allocations: allocations, SourceType: "CUSTOM_CAKE_QUOTE", SourceRef: newID, HoldFor: holdFor})
		if reserveErr != nil {
			return "", reserveErr
		}
		newReservationID = reserved.ID
	}
	if newReservationID != "" {
		if _, e = tx.Exec(ctx, `UPDATE pastry_quotes SET reservation_id=$1 WHERE id=$2`, newReservationID, newID); e != nil {
			return "", e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET status=$1,version=version+1,updated_at=now() WHERE id=$2 AND version=$3`, requestStatus, request, requestVersion); e != nil {
		return "", e
	}
	if e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.revised", "pastry.quote.revised", "pastry_quote", newID, branch, 1, map[string]any{"quote_id": quoteID, "status": status}, map[string]any{"quote_id": newID, "revision": revision, "specification_revision": specificationRevision, "amount_minor": amount, "approval_required": approvalRequired, "status": newStatus}, map[string]any{"previous_quote_id": quoteID, "quote_id": newID, "revision": revision}, reason); e != nil {
		return "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return newID, nil
}
func (s *Service) DeclineQuote(ctx context.Context, actor production.Actor, scope production.Scope, id string, expectedVersion int64) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var business, branch, status, requestID string
	var reservation *string
	var version, requestVersion int64
	if e = tx.QueryRow(ctx, `SELECT r.business_id,r.branch_id,q.status,q.reservation_id,q.version,r.id,r.version FROM pastry_quotes q JOIN pastry_custom_requests r ON r.id=q.request_id WHERE q.id=$1 FOR UPDATE`, id).Scan(&business, &branch, &status, &reservation, &version, &requestID, &requestVersion); e != nil {
		return e
	}
	if business != actor.VendorID || !scope.Covers(branch) {
		return fmt.Errorf("quote not found")
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return fmt.Errorf("stale quote version")
	}
	if status != "DRAFT" && status != "PENDING_APPROVAL" && status != "APPROVED" {
		return fmt.Errorf("quote cannot be declined")
	}
	if reservation != nil && *reservation != "" {
		if _, _, e = s.production.CloseReservationInTx(ctx, tx, actor, scope, *reservation, production.ReservationCancelled, "quote declined"); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_quotes SET status='DECLINED',version=version+1 WHERE id=$1 AND version=$2 AND status IN ('DRAFT','PENDING_APPROVAL','APPROVED')`, id, version); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_custom_requests SET status='DECLINED',version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, requestID, requestVersion); e != nil {
		return e
	}
	if s.production != nil {
		if e = s.production.RecordMutation(ctx, tx, actor, "pastry.quote.declined", "pastry.quote.declined", "pastry_quote", id, branch, version+1, map[string]any{"status": status, "version": version}, map[string]any{"status": "DECLINED", "version": version + 1}, nil, ""); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

var milestoneOrder = map[string]int{"DESIGN_APPROVED": 0, "BASE_PREPARED": 1, "DECORATION": 2, "CHILLING": 3, "PACKED": 4, "READY": 5}
var milestoneStates = map[string]bool{"PENDING": true, "ACTIVE": true, "BLOCKED": true, "READY": true, "COMPLETED": true, "CANCELLED": true}

func validMilestoneTransition(milestone string, args ...string) bool {
	// Keep the small pure helper backwards-compatible with the original unit
	// contract (milestone -> next milestone) while the service path supplies
	// current state and target state explicitly.
	if len(args) == 1 {
		if args[0] == "BLOCKED" || args[0] == "CANCELLED" {
			return true
		}
		next, ok := milestoneOrder[args[0]]
		cur, okCur := milestoneOrder[milestone]
		return ok && okCur && next == cur+1
	}
	if len(args) != 2 {
		return false
	}
	from, state := args[0], args[1]
	if _, ok := milestoneOrder[milestone]; !ok || !milestoneStates[state] {
		return false
	}
	if state == "BLOCKED" || state == "CANCELLED" {
		return from != "COMPLETED"
	}
	if from == state {
		return true
	}
	if from == "PENDING" {
		return state == "ACTIVE"
	}
	if from == "BLOCKED" {
		return state == "ACTIVE" || state == "CANCELLED"
	}
	if from == "ACTIVE" {
		return state == "COMPLETED" || (milestone == "READY" && state == "READY")
	}
	if from == "READY" {
		return state == "COMPLETED"
	}
	return false
}

func (s *Service) TransitionMilestone(ctx context.Context, actor production.Actor, request, milestone, state, note string, expectedVersion int64) (int64, error) {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var id, current, branch, business string
	var version int64
	if e = tx.QueryRow(ctx, `SELECT m.id,m.state,m.version,r.branch_id,r.business_id FROM pastry_milestones m JOIN pastry_custom_requests r ON r.id=m.request_id WHERE m.request_id=$1 AND m.milestone_type=$2 FOR UPDATE`, request, milestone).Scan(&id, &current, &version, &branch, &business); e != nil {
		return 0, e
	}
	if actor.VendorID != business {
		return 0, fmt.Errorf("milestone not found")
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return 0, fmt.Errorf("stale milestone version")
	}
	if current == state {
		return version, tx.Commit(ctx)
	}
	if !validMilestoneTransition(milestone, current, state) {
		return 0, fmt.Errorf("invalid milestone transition")
	}
	if state == "ACTIVE" {
		var previousIncomplete bool
		position := milestoneOrder[milestone]
		if position > 0 {
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pastry_milestones WHERE request_id=$1 AND position<$2 AND state NOT IN ('COMPLETED','READY'))`, request, position).Scan(&previousIncomplete)
		}
		if previousIncomplete {
			return 0, fmt.Errorf("previous milestone is not complete")
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE pastry_milestones SET state=$1,version=version+1 WHERE id=$2 AND version=$3`, state, id, version); e != nil {
		return 0, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pastry_milestone_history(id,milestone_id,from_state,to_state,actor_id,correlation_id,note) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7)`, uuid.NewString(), id, current, state, actor.StaffID, actor.CorrelationID, note); e != nil {
		return 0, e
	}
	if milestone == "READY" && (state == "READY" || state == "COMPLETED") {
		jobRows, jobErr := tx.Query(ctx, `SELECT j.id,j.status FROM production_jobs j JOIN pastry_quotes q ON q.id=j.source_ref WHERE q.request_id=$1 AND j.business_id=$2 AND j.source_type='CUSTOM_CAKE_QUOTE' AND j.status IN ('PLANNED','QUEUED','IN_PROGRESS','BLOCKED') FOR UPDATE`, request, business)
		if jobErr != nil {
			return 0, jobErr
		}
		type jobState struct{ id, status string }
		jobs := []jobState{}
		for jobRows.Next() {
			var job jobState
			if jobErr = jobRows.Scan(&job.id, &job.status); jobErr != nil {
				jobRows.Close()
				return 0, jobErr
			}
			jobs = append(jobs, job)
		}
		jobRows.Close()
		for _, job := range jobs {
			if _, e = tx.Exec(ctx, `UPDATE production_jobs SET status='READY',ready_at=COALESCE(ready_at,now()),updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2`, business, job.id); e != nil {
				return 0, e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,from_status,to_status,actor_type,actor_id,correlation_id,note) VALUES($1,$2,$3,'READY',$4,'READY','vendor_staff',NULLIF($5,''),NULLIF($6,''),'pastry READY milestone reached')`, "pje_"+uuid.NewString(), business, job.id, job.status, actor.StaffID, actor.CorrelationID); e != nil {
				return 0, e
			}
		}
	}
	if e = s.production.RecordMutation(ctx, tx, actor, "pastry.milestone."+strings.ToLower(state), "pastry.milestone."+strings.ToLower(state), "pastry_milestone", id, branch, version+1, map[string]any{"state": current}, map[string]any{"state": state}, map[string]any{"request_id": request, "milestone": milestone, "state": state}, note); e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return version + 1, nil
}

var _ = pgx.ErrNoRows
