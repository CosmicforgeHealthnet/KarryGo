package production

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler is the authenticated Vendor management surface over the shared
// production engine. Domain-specific Cloud Kitchen and Pastry handlers build
// on this service; they do not create another capacity authority.
type Handler struct {
	service     *Service
	idempotency *idempotency.Store
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) UseIdempotency(store *idempotency.Store) *Handler {
	h.idempotency = store
	return h
}

// mutate is the narrow transaction boundary for management handlers.  The
// domain write, audit row and outbox event either all commit or all roll back.
func (h *Handler) mutate(c *gin.Context, actor Actor, branch, action, aggregateType, aggregateID string, version int64, before, after, payload any, fn func(pgx.Tx) error) error {
	tx, err := h.service.db.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	if err = fn(tx); err != nil {
		return err
	}
	if err = h.service.RecordMutation(c, tx, actor, action, action, aggregateType, aggregateID, branch, version, before, after, payload, ""); err != nil {
		return err
	}
	return tx.Commit(c)
}

func (h *Handler) actor(c *gin.Context) (Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	return Actor{StaffID: staff.StaffID, VendorID: staff.VendorID,
		SessionID: staff.SessionID, CorrelationID: httpx.GetRequestID(c),
		CausationID: c.GetHeader("X-Causation-ID"), CommandRef: c.GetHeader("Idempotency-Key")}, nil
}

func productionOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func bindExpectedVersion(c *gin.Context) (int64, error) {
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ExpectedVersion <= 0 {
		return 0, apperrors.BadRequest("A positive expected_version is required.", err)
	}
	return in.ExpectedVersion, nil
}

func (h *Handler) authorized(c *gin.Context, permission string) (Actor, Scope, bool) {
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

type poolRequest struct {
	BranchID             string  `json:"branch_id"`
	Capability           string  `json:"capability"`
	Name                 string  `json:"name"`
	Code                 string  `json:"code"`
	ScopeType            string  `json:"scope_type"`
	ScopeRef             *string `json:"scope_ref"`
	UnitType             string  `json:"unit_type"`
	DefaultCapacity      int     `json:"default_capacity"`
	SetupBufferMinutes   int     `json:"setup_buffer_minutes"`
	CleanupBufferMinutes int     `json:"cleanup_buffer_minutes"`
	ExpectedVersion      int64   `json:"expected_version"`
}

type lineRequest struct {
	BranchID             string  `json:"branch_id"`
	Capability           string  `json:"capability"`
	Name                 string  `json:"name"`
	Code                 string  `json:"code"`
	ProductionClass      string  `json:"production_class"`
	CapacityPoolID       *string `json:"capacity_pool_id"`
	SetupBufferMinutes   int     `json:"setup_buffer_minutes"`
	CleanupBufferMinutes int     `json:"cleanup_buffer_minutes"`
	ExpectedVersion      int64   `json:"expected_version"`
}

type ruleRequest struct {
	BranchID           string  `json:"branch_id"`
	Capability         string  `json:"capability"`
	CapacityPoolID     string  `json:"capacity_pool_id"`
	ProductionLineID   *string `json:"production_line_id"`
	ItemFamily         *string `json:"item_family"`
	MaxQuantity        int     `json:"max_quantity"`
	DaysOfWeek         []int16 `json:"days_of_week"`
	StartLocalTime     string  `json:"start_local_time"`
	EndLocalTime       string  `json:"end_local_time"`
	EffectiveStartDate string  `json:"effective_start_date"`
	EffectiveEndDate   string  `json:"effective_end_date"`
	Specificity        int     `json:"specificity"`
	ExpectedVersion    int64   `json:"expected_version"`
}
type blackoutRequest struct {
	BranchID         string    `json:"branch_id"`
	Capability       *string   `json:"capability"`
	CapacityPoolID   *string   `json:"capacity_pool_id"`
	ProductionLineID *string   `json:"production_line_id"`
	StartAt          time.Time `json:"start_at"`
	EndAt            time.Time `json:"end_at"`
	ReasonCode       string    `json:"reason_code"`
	Note             string    `json:"note"`
	ExpectedVersion  int64     `json:"expected_version"`
}

type slotRequest struct {
	BranchID         string    `json:"branch_id"`
	Capability       string    `json:"capability"`
	CapacityPoolID   string    `json:"capacity_pool_id"`
	ProductionLineID *string   `json:"production_line_id"`
	StartAt          time.Time `json:"start_at"`
	EndAt            time.Time `json:"end_at"`
	CapacityOverride *int      `json:"capacity_override"`
	Label            string    `json:"label"`
	ExpectedVersion  int64     `json:"expected_version"`
}

func (h *Handler) listBlackouts(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,capability,capacity_pool_id,production_line_id,start_at,end_at,reason_code,note,status,created_at,updated_at,version FROM production_blackouts WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY start_at DESC`, actor.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, reason, note, status string
		var cap, pool, line *string
		var start, end, created, updated time.Time
		var ver int64
		if err = rows.Scan(&id, &branch, &cap, &pool, &line, &start, &end, &reason, &note, &status, &created, &updated, &ver); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "branch_id": branch, "capability": cap, "capacity_pool_id": pool, "production_line_id": line, "start_at": start, "end_at": end, "reason_code": reason, "note": note, "status": status, "created_at": created, "updated_at": updated, "version": ver})
	}
	productionOK(c, out)
}
func (h *Handler) createBlackout(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in blackoutRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if !scope.Covers(in.BranchID) {
		httpx.RespondError(c, errForbidden())
		return
	}
	if !in.EndAt.After(in.StartAt) {
		httpx.RespondError(c, coded(apperrors.Validation("Check your details.", nil), CodeInvalidWindow))
		return
	}
	id := newID("bo_")
	err := h.mutate(c, actor, in.BranchID, "production.blackout.created", "production_blackout", id, 1, nil, in, map[string]any{"id": id}, func(tx pgx.Tx) error {
		if (in.CapacityPoolID != nil || in.ProductionLineID != nil) && in.Capability == nil {
			return apperrors.BadRequest("capability is required for a pool or production-line blackout.", nil)
		}
		if in.Capability != nil {
			capability := strings.ToUpper(*in.Capability)
			in.Capability = &capability
			if err := h.service.ValidateCapacityPoolReference(c, tx, actor.VendorID, in.BranchID, capability, in.CapacityPoolID); err != nil {
				return err
			}
			if err := h.service.ValidateProductionLineReference(c, tx, actor.VendorID, in.BranchID, capability, in.ProductionLineID); err != nil {
				return err
			}
		}
		if err := lockBlackoutPools(c, tx, actor.VendorID, in.BranchID, in.Capability, in.CapacityPoolID, in.ProductionLineID); err != nil {
			return err
		}
		_, e := tx.Exec(c, `INSERT INTO production_blackouts(id,business_id,branch_id,capability,capacity_pool_id,production_line_id,start_at,end_at,reason_code,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, actor.VendorID, in.BranchID, in.Capability, in.CapacityPoolID, in.ProductionLineID, in.StartAt, in.EndAt, in.ReasonCode, in.Note)
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Blackout", CodeBlackoutNotFound))
		return
	}
	productionOK(c, gin.H{"id": id, "version": int64(1)})
}

// lockBlackoutPools serializes blackout publication with the exact pool-row
// locks used by reservation allocation. If a reservation wins first it remains
// visible as an existing commitment; if the blackout wins first, the waiting
// reservation rechecks and fails against the committed blackout.
func lockBlackoutPools(ctx *gin.Context, tx pgx.Tx, businessID, branchID string, capability, poolID, lineID *string) error {
	ids := []string{}
	if poolID != nil && *poolID != "" && (lineID == nil || *lineID == "") {
		ids = append(ids, *poolID)
	} else {
		rows, err := tx.Query(ctx, `
			SELECT id FROM production_capacity_pools
			WHERE business_id=$1 AND branch_id=$2 AND status='ACTIVE'
			  AND ($3::text IS NULL OR capability=$3)
			ORDER BY id`, businessID, branchID, capability)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	_, err := lockPools(ctx, tx, businessID, ids)
	return err
}
func (h *Handler) cancelBlackout(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("An expected version is required.", err))
		return
	}
	var branch, status string
	var version int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,status,version FROM production_blackouts WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("blackoutId")).Scan(&branch, &status, &version); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Blackout", CodeBlackoutNotFound))
		return
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version || status != "ACTIVE" {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	err := h.mutate(c, actor, branch, "production.blackout.cancelled", "production_blackout", c.Param("blackoutId"), version+1, map[string]any{"status": status, "version": version}, map[string]any{"status": "CANCELLED", "version": version + 1}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_blackouts SET status='CANCELLED',updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3 AND status='ACTIVE'`, actor.VendorID, c.Param("blackoutId"), version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"cancelled": true, "version": version + 1})
}

func (h *Handler) listRules(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,capability,capacity_pool_id,production_line_id,item_family,max_quantity,days_of_week,start_local_time::text,end_local_time::text,effective_start_date::text,effective_end_date::text,specificity,status,created_at,updated_at,archived_at,version FROM production_capacity_rules WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY branch_id,specificity DESC,id`, actor.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, cap, pool string
		var line, family, startLocal, endLocal, effectiveStart, effectiveEnd *string
		var max, spec int
		var days []int16
		var status string
		var created, updated time.Time
		var archived *time.Time
		var ver int64
		if err = rows.Scan(&id, &branch, &cap, &pool, &line, &family, &max, &days, &startLocal, &endLocal, &effectiveStart, &effectiveEnd, &spec, &status, &created, &updated, &archived, &ver); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "branch_id": branch, "capability": cap, "capacity_pool_id": pool, "production_line_id": line, "item_family": family, "max_quantity": max, "days_of_week": days, "start_local_time": startLocal, "end_local_time": endLocal, "effective_start_date": effectiveStart, "effective_end_date": effectiveEnd, "specificity": spec, "status": status, "created_at": created, "updated_at": updated, "archived_at": archived, "version": ver})
	}
	productionOK(c, out)
}

func validateRuleRequest(in ruleRequest) error {
	violations := []apperrors.FieldViolation{}
	if in.MaxQuantity < 0 || in.MaxQuantity > MaxCapacity {
		violations = append(violations, apperrors.FieldViolation{Field: "max_quantity", Message: "Capacity is outside the supported range."})
	}
	if in.Specificity < 0 || in.Specificity > 100 {
		violations = append(violations, apperrors.FieldViolation{Field: "specificity", Message: "Specificity must be between 0 and 100."})
	}
	seenDays := map[int16]bool{}
	for _, day := range in.DaysOfWeek {
		if day < 1 || day > 7 || seenDays[day] {
			violations = append(violations, apperrors.FieldViolation{Field: "days_of_week", Message: "Use unique ISO weekdays from 1 to 7."})
			break
		}
		seenDays[day] = true
	}
	if (in.StartLocalTime == "") != (in.EndLocalTime == "") {
		violations = append(violations, apperrors.FieldViolation{Field: "start_local_time", Message: "Start and end local times must be supplied together."})
	} else if in.StartLocalTime != "" {
		start, startErr := time.Parse("15:04", in.StartLocalTime)
		if startErr != nil {
			start, startErr = time.Parse("15:04:05", in.StartLocalTime)
		}
		end, endErr := time.Parse("15:04", in.EndLocalTime)
		if endErr != nil {
			end, endErr = time.Parse("15:04:05", in.EndLocalTime)
		}
		if startErr != nil || endErr != nil || start.Equal(end) {
			violations = append(violations, apperrors.FieldViolation{Field: "start_local_time", Message: "Use distinct HH:MM local start and end times."})
		}
	}
	var effectiveStart, effectiveEnd time.Time
	var startErr, endErr error
	if in.EffectiveStartDate != "" {
		effectiveStart, startErr = time.Parse("2006-01-02", in.EffectiveStartDate)
	}
	if in.EffectiveEndDate != "" {
		effectiveEnd, endErr = time.Parse("2006-01-02", in.EffectiveEndDate)
	}
	if startErr != nil || endErr != nil || (!effectiveStart.IsZero() && !effectiveEnd.IsZero() && effectiveEnd.Before(effectiveStart)) {
		violations = append(violations, apperrors.FieldViolation{Field: "effective_dates", Message: "Use a valid YYYY-MM-DD date range."})
	}
	if len(violations) > 0 {
		return validation(violations...)
	}
	return nil
}

func (h *Handler) getRule(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var id, branch, capability, poolID, status string
	var lineID, itemFamily, startLocal, endLocal, effectiveStart, effectiveEnd *string
	var maxQuantity, specificity int
	var days []int16
	var created, updated time.Time
	var archived *time.Time
	var version int64
	err := h.service.db.QueryRow(c, `SELECT id,branch_id,capability,capacity_pool_id,production_line_id,item_family,max_quantity,days_of_week,start_local_time::text,end_local_time::text,effective_start_date::text,effective_end_date::text,specificity,status,created_at,updated_at,archived_at,version FROM production_capacity_rules WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("ruleId")).Scan(&id, &branch, &capability, &poolID, &lineID, &itemFamily, &maxQuantity, &days, &startLocal, &endLocal, &effectiveStart, &effectiveEnd, &specificity, &status, &created, &updated, &archived, &version)
	if err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Capacity rule", CodeRuleNotFound))
		return
	}
	productionOK(c, gin.H{"id": id, "branch_id": branch, "capability": capability, "capacity_pool_id": poolID, "production_line_id": lineID, "item_family": itemFamily, "max_quantity": maxQuantity, "days_of_week": days, "start_local_time": startLocal, "end_local_time": endLocal, "effective_start_date": effectiveStart, "effective_end_date": effectiveEnd, "specificity": specificity, "status": status, "created_at": created, "updated_at": updated, "archived_at": archived, "version": version})
}

func (h *Handler) createRule(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in ruleRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if !scope.Covers(in.BranchID) {
		httpx.RespondError(c, errForbidden())
		return
	}
	if err := validateRuleRequest(in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	id := newID("cr_")
	err := h.mutate(c, actor, in.BranchID, "production.capacity_rule.created", "production_capacity_rule", id, 1, nil, in, map[string]any{"id": id}, func(tx pgx.Tx) error {
		if e := h.service.ValidateCapacityPoolReference(c, tx, actor.VendorID, in.BranchID, in.Capability, &in.CapacityPoolID); e != nil {
			return e
		}
		if e := h.service.ValidateProductionLineReference(c, tx, actor.VendorID, in.BranchID, in.Capability, in.ProductionLineID); e != nil {
			return e
		}
		_, e := tx.Exec(c, `INSERT INTO production_capacity_rules(id,business_id,branch_id,capability,capacity_pool_id,production_line_id,item_family,max_quantity,days_of_week,start_local_time,end_local_time,effective_start_date,effective_end_date,specificity) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::time,NULLIF($11,'')::time,NULLIF($12,'')::date,NULLIF($13,'')::date,$14)`, id, actor.VendorID, in.BranchID, in.Capability, in.CapacityPoolID, in.ProductionLineID, in.ItemFamily, in.MaxQuantity, in.DaysOfWeek, in.StartLocalTime, in.EndLocalTime, in.EffectiveStartDate, in.EffectiveEndDate, in.Specificity)
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Capacity rule", CodeRuleNotFound))
		return
	}
	productionOK(c, gin.H{"id": id, "version": int64(1)})
}

func (h *Handler) updateRule(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in ruleRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	var branch, status string
	var version int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,status,version FROM production_capacity_rules WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("ruleId")).Scan(&branch, &status, &version); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Capacity rule", CodeRuleNotFound))
		return
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	if err := validateRuleRequest(in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	err := h.mutate(c, actor, branch, "production.capacity_rule.updated", "production_capacity_rule", c.Param("ruleId"), version+1, map[string]any{"status": status, "version": version}, in, nil, func(tx pgx.Tx) error {
		if e := h.service.ValidateCapacityPoolReference(c, tx, actor.VendorID, branch, in.Capability, &in.CapacityPoolID); e != nil {
			return e
		}
		if e := h.service.ValidateProductionLineReference(c, tx, actor.VendorID, branch, in.Capability, in.ProductionLineID); e != nil {
			return e
		}
		tag, e := tx.Exec(c, `UPDATE production_capacity_rules SET capability=$3,capacity_pool_id=$4,production_line_id=$5,item_family=$6,max_quantity=$7,days_of_week=$8,start_local_time=NULLIF($9,'')::time,end_local_time=NULLIF($10,'')::time,effective_start_date=NULLIF($11,'')::date,effective_end_date=NULLIF($12,'')::date,specificity=$13,updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$14 AND status='ACTIVE'`, actor.VendorID, c.Param("ruleId"), in.Capability, in.CapacityPoolID, in.ProductionLineID, in.ItemFamily, in.MaxQuantity, in.DaysOfWeek, in.StartLocalTime, in.EndLocalTime, in.EffectiveStartDate, in.EffectiveEndDate, in.Specificity, version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Capacity rule", CodeRuleNotFound))
		return
	}
	productionOK(c, gin.H{"id": c.Param("ruleId"), "version": version + 1})
}
func (h *Handler) archiveRule(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var branch string
	var ver int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,version FROM production_capacity_rules WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("ruleId")).Scan(&branch, &ver); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Capacity rule", CodeRuleNotFound))
		return
	}
	expectedVersion, err := bindExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if expectedVersion != ver {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	err = h.mutate(c, actor, branch, "production.capacity_rule.archived", "production_capacity_rule", c.Param("ruleId"), ver+1, map[string]any{"status": "ACTIVE", "version": ver}, map[string]any{"status": "ARCHIVED", "version": ver + 1}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_capacity_rules SET status='ARCHIVED',archived_at=now(),updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3`, actor.VendorID, c.Param("ruleId"), ver)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	productionOK(c, gin.H{"archived": true})
}

func (h *Handler) listLines(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,capability,name,code,production_class,capacity_pool_id,setup_buffer_minutes,cleanup_buffer_minutes,status,created_at,updated_at,archived_at,version FROM production_lines WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY branch_id,code`, actor.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, cap, name, code, class, status string
		var pool *string
		var setupBuffer, cleanupBuffer int
		var created, updated time.Time
		var archived *time.Time
		var ver int64
		if err = rows.Scan(&id, &branch, &cap, &name, &code, &class, &pool, &setupBuffer, &cleanupBuffer, &status, &created, &updated, &archived, &ver); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "branch_id": branch, "capability": cap, "name": name, "code": code, "production_class": class, "capacity_pool_id": pool, "setup_buffer_minutes": setupBuffer, "cleanup_buffer_minutes": cleanupBuffer, "status": status, "created_at": created, "updated_at": updated, "archived_at": archived, "version": ver})
	}
	productionOK(c, out)
}

func (h *Handler) getLine(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var id, branch, capability, name, code, productionClass, status string
	var poolID *string
	var setupBuffer, cleanupBuffer int
	var created, updated time.Time
	var archived *time.Time
	var version int64
	err := h.service.db.QueryRow(c, `SELECT id,branch_id,capability,name,code,production_class,capacity_pool_id,setup_buffer_minutes,cleanup_buffer_minutes,status,created_at,updated_at,archived_at,version FROM production_lines WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("lineId")).Scan(&id, &branch, &capability, &name, &code, &productionClass, &poolID, &setupBuffer, &cleanupBuffer, &status, &created, &updated, &archived, &version)
	if err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Production line", CodeLineNotFound))
		return
	}
	productionOK(c, gin.H{"id": id, "branch_id": branch, "capability": capability, "name": name, "code": code, "production_class": productionClass, "capacity_pool_id": poolID, "setup_buffer_minutes": setupBuffer, "cleanup_buffer_minutes": cleanupBuffer, "status": status, "created_at": created, "updated_at": updated, "archived_at": archived, "version": version})
}

func (h *Handler) createLine(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in lineRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if !scope.Covers(in.BranchID) {
		httpx.RespondError(c, errForbidden())
		return
	}
	name, nv := validateName("name", in.Name, 2, 160)
	code, cv := validateCode("code", in.Code)
	if nv != nil || cv != nil {
		v := []apperrors.FieldViolation{}
		if nv != nil {
			v = append(v, *nv)
		}
		if cv != nil {
			v = append(v, *cv)
		}
		httpx.RespondError(c, validation(v...))
		return
	}
	if in.ProductionClass == "" {
		in.ProductionClass = "GENERAL"
	}
	if in.SetupBufferMinutes < 0 || in.SetupBufferMinutes > MaxBufferMinutes || in.CleanupBufferMinutes < 0 || in.CleanupBufferMinutes > MaxBufferMinutes {
		httpx.RespondError(c, validation(apperrors.FieldViolation{Field: "buffers", Message: "Setup and cleanup buffers must be between 0 and 480 minutes."}))
		return
	}
	id := newID("pl_")
	err := h.mutate(c, actor, in.BranchID, "production.line.created", "production_line", id, 1, nil, in, map[string]any{"id": id}, func(tx pgx.Tx) error {
		if e := h.service.ValidateCapacityPoolReference(c, tx, actor.VendorID, in.BranchID, in.Capability, in.CapacityPoolID); e != nil {
			return e
		}
		_, e := tx.Exec(c, `INSERT INTO production_lines(id,business_id,branch_id,capability,name,code,production_class,capacity_pool_id,setup_buffer_minutes,cleanup_buffer_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, actor.VendorID, in.BranchID, in.Capability, name, code, in.ProductionClass, in.CapacityPoolID, in.SetupBufferMinutes, in.CleanupBufferMinutes)
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Production line", CodeLineNotFound))
		return
	}
	productionOK(c, gin.H{"id": id, "version": int64(1)})
}

func (h *Handler) updateLine(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in lineRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	var branch, currentStatus string
	var currentVersion int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,status,version FROM production_lines WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("lineId")).Scan(&branch, &currentStatus, &currentVersion); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Production line", CodeLineNotFound))
		return
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != currentVersion {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	name, nameViolation := validateName("name", in.Name, 2, 160)
	code, codeViolation := validateCode("code", in.Code)
	violations := []apperrors.FieldViolation{}
	if nameViolation != nil {
		violations = append(violations, *nameViolation)
	}
	if codeViolation != nil {
		violations = append(violations, *codeViolation)
	}
	if in.SetupBufferMinutes < 0 || in.SetupBufferMinutes > MaxBufferMinutes || in.CleanupBufferMinutes < 0 || in.CleanupBufferMinutes > MaxBufferMinutes {
		violations = append(violations, apperrors.FieldViolation{Field: "buffers", Message: "Setup and cleanup buffers must be between 0 and 480 minutes."})
	}
	if len(violations) > 0 {
		httpx.RespondError(c, validation(violations...))
		return
	}
	if in.ProductionClass == "" {
		in.ProductionClass = "GENERAL"
	}
	err := h.mutate(c, actor, branch, "production.line.updated", "production_line", c.Param("lineId"), currentVersion+1, map[string]any{"status": currentStatus, "version": currentVersion}, in, nil, func(tx pgx.Tx) error {
		if e := h.service.ValidateCapacityPoolReference(c, tx, actor.VendorID, branch, in.Capability, in.CapacityPoolID); e != nil {
			return e
		}
		tag, e := tx.Exec(c, `UPDATE production_lines SET capability=$3,name=$4,code=$5,production_class=$6,capacity_pool_id=$7,setup_buffer_minutes=$8,cleanup_buffer_minutes=$9,updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$10 AND status<>'ARCHIVED'`, actor.VendorID, c.Param("lineId"), in.Capability, name, code, in.ProductionClass, in.CapacityPoolID, in.SetupBufferMinutes, in.CleanupBufferMinutes, currentVersion)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Production line", CodeLineNotFound))
		return
	}
	productionOK(c, gin.H{"id": c.Param("lineId"), "version": currentVersion + 1})
}
func (h *Handler) archiveLine(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var branch string
	var ver int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,version FROM production_lines WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("lineId")).Scan(&branch, &ver); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Production line", CodeLineNotFound))
		return
	}
	expectedVersion, err := bindExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if expectedVersion != ver {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	err = h.mutate(c, actor, branch, "production.line.archived", "production_line", c.Param("lineId"), ver+1, map[string]any{"status": "ACTIVE", "version": ver}, map[string]any{"status": "ARCHIVED", "version": ver + 1}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_lines SET status='ARCHIVED',archived_at=now(),updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3`, actor.VendorID, c.Param("lineId"), ver)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	productionOK(c, gin.H{"archived": true})
}

func (h *Handler) listPools(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,capability,name,code,scope_type,scope_ref,unit_type,default_capacity,setup_buffer_minutes,cleanup_buffer_minutes,status,created_at,updated_at,archived_at,version FROM production_capacity_pools WHERE business_id=$1 AND ($2 OR branch_id = ANY($3)) ORDER BY branch_id,code`, actor.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	result := []CapacityPoolView{}
	for rows.Next() {
		var p CapacityPool
		if err = rows.Scan(&p.ID, &p.BranchID, &p.Capability, &p.Name, &p.Code, &p.ScopeType, &p.ScopeRef, &p.UnitType, &p.DefaultCapacity, &p.SetupBufferMinutes, &p.CleanupBufferMinutes, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt, &p.Version); err != nil {
			httpx.RespondError(c, err)
			return
		}
		result = append(result, CapacityPoolViewOf(p))
	}
	productionOK(c, result)
}

func (h *Handler) createPool(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in poolRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if !scope.Covers(in.BranchID) {
		httpx.RespondError(c, errForbidden())
		return
	}
	capability, _ := validateCode("capability", in.Capability)
	if !inVocabulary(Capabilities, capability) {
		httpx.RespondError(c, validation(apperrors.FieldViolation{Field: "capability", Message: "Unsupported capability."}))
		return
	}
	name, nv := validateName("name", in.Name, 2, 160)
	code, cv := validateCode("code", in.Code)
	if nv != nil || cv != nil {
		v := []apperrors.FieldViolation{}
		if nv != nil {
			v = append(v, *nv)
		}
		if cv != nil {
			v = append(v, *cv)
		}
		httpx.RespondError(c, validation(v...))
		return
	}
	if in.ScopeType == "" {
		in.ScopeType = "CAPABILITY"
	}
	if in.UnitType == "" {
		in.UnitType = "PRODUCTION_UNIT"
	}
	if err := validateCapacity("default_capacity", in.DefaultCapacity); err != nil {
		httpx.RespondError(c, validation(*err))
		return
	}
	if in.SetupBufferMinutes < 0 || in.SetupBufferMinutes > MaxBufferMinutes || in.CleanupBufferMinutes < 0 || in.CleanupBufferMinutes > MaxBufferMinutes {
		httpx.RespondError(c, validation(apperrors.FieldViolation{Field: "buffers", Message: "Setup and cleanup buffers must be between 0 and 480 minutes."}))
		return
	}
	id := newID("pp_")
	err := h.mutate(c, actor, in.BranchID, "production.capacity_pool.created", "production_capacity_pool", id, 1, nil, in, map[string]any{"id": id}, func(tx pgx.Tx) error {
		_, e := tx.Exec(c, `INSERT INTO production_capacity_pools(id,business_id,branch_id,capability,name,code,scope_type,scope_ref,unit_type,default_capacity,setup_buffer_minutes,cleanup_buffer_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, actor.VendorID, in.BranchID, capability, name, code, in.ScopeType, in.ScopeRef, in.UnitType, in.DefaultCapacity, in.SetupBufferMinutes, in.CleanupBufferMinutes)
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Capacity pool", CodePoolNotFound))
		return
	}
	productionOK(c, gin.H{"id": id, "version": int64(1)})
}

func (h *Handler) getPool(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var p CapacityPool
	err := h.service.db.QueryRow(c, `SELECT id,branch_id,capability,name,code,scope_type,scope_ref,unit_type,default_capacity,setup_buffer_minutes,cleanup_buffer_minutes,status,created_at,updated_at,archived_at,version FROM production_capacity_pools WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("poolId")).Scan(&p.ID, &p.BranchID, &p.Capability, &p.Name, &p.Code, &p.ScopeType, &p.ScopeRef, &p.UnitType, &p.DefaultCapacity, &p.SetupBufferMinutes, &p.CleanupBufferMinutes, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt, &p.Version)
	if err != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, errNotFound("Capacity pool", CodePoolNotFound))
		return
	}
	productionOK(c, CapacityPoolViewOf(p))
}

func (h *Handler) updatePool(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in poolRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	var branch string
	var version int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,version FROM production_capacity_pools WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("poolId")).Scan(&branch, &version); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Capacity pool", CodePoolNotFound))
		return
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	name, nv := validateName("name", in.Name, 2, 160)
	if nv != nil {
		httpx.RespondError(c, validation(*nv))
		return
	}
	if err := validateCapacity("default_capacity", in.DefaultCapacity); err != nil {
		httpx.RespondError(c, validation(*err))
		return
	}
	if in.SetupBufferMinutes < 0 || in.SetupBufferMinutes > MaxBufferMinutes || in.CleanupBufferMinutes < 0 || in.CleanupBufferMinutes > MaxBufferMinutes {
		httpx.RespondError(c, validation(apperrors.FieldViolation{Field: "buffers", Message: "Setup and cleanup buffers must be between 0 and 480 minutes."}))
		return
	}
	err := h.mutate(c, actor, branch, "production.capacity_pool.updated", "production_capacity_pool", c.Param("poolId"), version+1, nil, in, map[string]any{"id": c.Param("poolId")}, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_capacity_pools SET name=$1,default_capacity=$2,setup_buffer_minutes=$3,cleanup_buffer_minutes=$4,updated_at=now(),version=version+1 WHERE business_id=$5 AND id=$6 AND version=$7`, name, in.DefaultCapacity, in.SetupBufferMinutes, in.CleanupBufferMinutes, actor.VendorID, c.Param("poolId"), version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	productionOK(c, gin.H{"updated": true, "version": version + 1})
}

func (h *Handler) archivePool(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var branch string
	var version int64
	err := h.service.db.QueryRow(c, `SELECT branch_id,version FROM production_capacity_pools WHERE business_id=$1 AND id=$2`, actor.VendorID, c.Param("poolId")).Scan(&branch, &version)
	if err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, errNotFound("Capacity pool", CodePoolNotFound))
		return
	}
	expectedVersion, err := bindExpectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if expectedVersion != version {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	err = h.mutate(c, actor, branch, "production.capacity_pool.archived", "production_capacity_pool", c.Param("poolId"), version+1, map[string]any{"status": "ACTIVE", "version": version}, map[string]any{"status": "ARCHIVED", "version": version + 1}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_capacity_pools SET status='ARCHIVED',archived_at=now(),updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3 AND status<>'ARCHIVED'`, actor.VendorID, c.Param("poolId"), version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	productionOK(c, gin.H{"archived": true})
}

type reservationRequest struct {
	BranchID       string              `json:"branch_id"`
	Capability     string              `json:"capability"`
	WindowStart    time.Time           `json:"window_start"`
	WindowEnd      time.Time           `json:"window_end"`
	Allocations    []AllocationRequest `json:"allocations"`
	SourceType     string              `json:"source_type"`
	SourceRef      string              `json:"source_ref"`
	Note           string              `json:"note"`
	Priority       string              `json:"priority"`
	HoldForMinutes int                 `json:"hold_for_minutes"`
}

func (h *Handler) reserve(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	var in reservationRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if !scope.Covers(in.BranchID) {
		httpx.RespondError(c, errForbidden())
		return
	}
	r, a, err := h.service.Reserve(c, actor, scope, ReserveInput{BranchID: in.BranchID, Capability: in.Capability, WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, Allocations: in.Allocations, SourceType: in.SourceType, SourceRef: in.SourceRef, Note: in.Note, Priority: in.Priority, HoldFor: time.Duration(in.HoldForMinutes) * time.Minute})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, ReservationViewOf(r, a, h.service.Now()))
}
func (h *Handler) getReservation(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	r, a, err := h.service.GetReservation(c, actor, scope, c.Param("reservationId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, ReservationViewOf(r, a, h.service.Now()))
}
func (h *Handler) confirm(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	_ = json.NewDecoder(c.Request.Body).Decode(&in)
	r, a, err := h.service.Confirm(c, actor, scope, c.Param("reservationId"), in.ExpectedVersion)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, ReservationViewOf(r, a, h.service.Now()))
}
func (h *Handler) release(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	var in ReleaseInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	r, a, err := h.service.Release(c, actor, scope, c.Param("reservationId"), in)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, ReservationViewOf(r, a, h.service.Now()))
}
func (h *Handler) preview(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var in reservationRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	inAlloc := ReserveInput{BranchID: in.BranchID, Capability: in.Capability, WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, Allocations: in.Allocations, SourceType: in.SourceType, SourceRef: in.SourceRef}
	p, err := h.service.PreviewCapacity(c, actor, scope, inAlloc)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, p)
}

func (h *Handler) utilization(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		httpx.RespondError(c, validation(apperrors.FieldViolation{Field: "start", Message: "RFC3339 start is required."}))
		return
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil {
		httpx.RespondError(c, validation(apperrors.FieldViolation{Field: "end", Message: "RFC3339 end is required."}))
		return
	}
	branch := c.Query("branch_id")
	capability := c.Query("capability")
	if !scope.Covers(branch) {
		httpx.RespondError(c, errForbidden())
		return
	}
	result, err := h.service.Utilization(c, actor, scope, branch, capability, start, end)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, result)
}

func (h *Handler) reschedule(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	var in struct {
		WindowStart     time.Time           `json:"window_start"`
		WindowEnd       time.Time           `json:"window_end"`
		Allocations     []AllocationRequest `json:"allocations"`
		ExpectedVersion int64               `json:"expected_version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	r, a, err := h.service.Reschedule(c, actor, scope, c.Param("reservationId"), RescheduleInput{WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, ExpectedVersion: in.ExpectedVersion})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, ReservationViewOf(r, a, h.service.Now()))
}

func RegisterRoutes(group *gin.RouterGroup, handler *Handler, flags Flags) {
	if handler == nil || !flags.ProductionEnabled {
		return
	}
	p := group.Group("/production")
	p.Use(idempotency.CommandMiddleware(handler.idempotency))
	p.GET("/pools", handler.listPools)
	p.POST("/pools", handler.createPool)
	p.GET("/pools/:poolId", handler.getPool)
	p.GET("/lines", handler.listLines)
	p.POST("/lines", handler.createLine)
	p.GET("/lines/:lineId", handler.getLine)
	p.PATCH("/lines/:lineId", handler.updateLine)
	p.POST("/lines/:lineId/archive", handler.archiveLine)
	p.GET("/rules", handler.listRules)
	p.POST("/rules", handler.createRule)
	p.GET("/rules/:ruleId", handler.getRule)
	p.PATCH("/rules/:ruleId", handler.updateRule)
	p.POST("/rules/:ruleId/archive", handler.archiveRule)
	p.GET("/blackouts", handler.listBlackouts)
	p.POST("/blackouts", handler.createBlackout)
	p.POST("/blackouts/:blackoutId/cancel", handler.cancelBlackout)
	p.PATCH("/pools/:poolId", handler.updatePool)
	p.POST("/pools/:poolId/archive", handler.archivePool)
	p.POST("/reservations", handler.reserve)
	p.GET("/reservations/:reservationId", handler.getReservation)
	p.POST("/reservations/:reservationId/confirm", handler.confirm)
	p.POST("/reservations/:reservationId/release", handler.release)
	p.POST("/capacity/preview", handler.preview)
	p.GET("/capacity/utilization", handler.utilization)
	p.POST("/reservations/:reservationId/reschedule", handler.reschedule)
	p.GET("/slots", handler.listSlots)
	p.POST("/slots", handler.createSlot)
	p.GET("/slots/:slotId", handler.getSlot)
	p.PATCH("/slots/:slotId", handler.updateSlot)
	p.POST("/slots/:slotId/close", handler.closeSlot)
	p.POST("/slots/:slotId/open", handler.openSlot)
	p.POST("/slots/:slotId/blackout", handler.blackoutSlot)
	p.POST("/slots/:slotId/archive", handler.archiveSlot)
	p.GET("/jobs", handler.listJobs)
	p.POST("/jobs", handler.createJob)
	p.GET("/jobs/:jobId", handler.getJob)
	p.GET("/jobs/:jobId/events", handler.listJobEvents)
	p.GET("/alerts", handler.listAlerts)
	p.POST("/alerts/:alertId/acknowledge", handler.acknowledgeAlert)
	p.POST("/alerts/:alertId/resolve", handler.resolveAlert)
	p.POST("/jobs/:jobId/start", handler.startJob)
	p.POST("/jobs/:jobId/ready", handler.readyJob)
	p.POST("/jobs/:jobId/complete", handler.completeJob)
	p.POST("/jobs/:jobId/:status", handler.transitionJob)
	p.GET("/slot-templates", handler.listSlotTemplates)
	p.POST("/slot-templates", handler.createSlotTemplate)
	p.GET("/slot-templates/:templateId", handler.getSlotTemplate)
	p.PATCH("/slot-templates/:templateId", handler.updateSlotTemplate)
	p.POST("/slot-templates/:templateId/archive", handler.archiveSlotTemplate)
	p.POST("/slot-templates/:templateId/generate", handler.generateSlotTemplate)
}

type slotTemplateRequest struct {
	BranchID           string  `json:"branch_id"`
	Capability         string  `json:"capability"`
	CapacityPoolID     string  `json:"capacity_pool_id"`
	ProductionLineID   *string `json:"production_line_id"`
	Label              string  `json:"label"`
	DaysOfWeek         []int16 `json:"days_of_week"`
	StartLocalTime     string  `json:"start_local_time"`
	EndLocalTime       string  `json:"end_local_time"`
	CapacityOverride   *int    `json:"capacity_override"`
	EffectiveStartDate *string `json:"effective_start_date"`
	EffectiveEndDate   *string `json:"effective_end_date"`
	Timezone           string  `json:"timezone"`
	GenerationHorizon  int     `json:"generation_horizon_days"`
	ExpectedVersion    int64   `json:"expected_version"`
}

type slotTemplateView struct {
	ID, BranchID, Capability, CapacityPoolID, Label, Timezone, Status string
	ProductionLineID                                                  *string
	DaysOfWeek                                                        []int16
	StartLocalTime, EndLocalTime                                      string
	CapacityOverride                                                  *int
	EffectiveStartDate, EffectiveEndDate                              *time.Time
	GenerationHorizon                                                 int
	Version                                                           int64
}

const slotTemplateSelect = `id,branch_id,capability,capacity_pool_id,production_line_id,label,days_of_week,to_char(start_local_time,'HH24:MI'),to_char(end_local_time,'HH24:MI'),capacity_override,effective_start_date,effective_end_date,timezone,generation_horizon_days,status,version`

func slotTemplateJSON(v slotTemplateView) gin.H {
	return gin.H{"id": v.ID, "branch_id": v.BranchID, "capability": v.Capability, "capacity_pool_id": v.CapacityPoolID, "production_line_id": v.ProductionLineID, "label": v.Label, "days_of_week": v.DaysOfWeek, "start_local_time": v.StartLocalTime, "end_local_time": v.EndLocalTime, "capacity_override": v.CapacityOverride, "effective_start_date": v.EffectiveStartDate, "effective_end_date": v.EffectiveEndDate, "timezone": v.Timezone, "generation_horizon_days": v.GenerationHorizon, "status": v.Status, "version": v.Version}
}

func scanSlotTemplate(row interface{ Scan(...any) error }) (slotTemplateView, error) {
	var v slotTemplateView
	err := row.Scan(&v.ID, &v.BranchID, &v.Capability, &v.CapacityPoolID, &v.ProductionLineID, &v.Label, &v.DaysOfWeek, &v.StartLocalTime, &v.EndLocalTime, &v.CapacityOverride, &v.EffectiveStartDate, &v.EffectiveEndDate, &v.Timezone, &v.GenerationHorizon, &v.Status, &v.Version)
	return v, err
}

func (h *Handler) listSlotTemplates(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT `+slotTemplateSelect+` FROM production_slot_templates WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY branch_id,label,id`, a.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		v, e := scanSlotTemplate(rows)
		if e == nil {
			out = append(out, slotTemplateJSON(v))
		}
	}
	productionOK(c, out)
}

func (h *Handler) getSlotTemplate(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	v, e := scanSlotTemplate(h.service.db.QueryRow(c, `SELECT `+slotTemplateSelect+` FROM production_slot_templates WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("templateId")))
	if e != nil || !scope.Covers(v.BranchID) {
		httpx.RespondError(c, apperrors.NotFound("Slot template not found.", nil))
		return
	}
	productionOK(c, slotTemplateJSON(v))
}

func parseTemplateWindow(in slotTemplateRequest) (time.Time, time.Time, error) {
	start, e := time.Parse("15:04", in.StartLocalTime)
	if e != nil {
		return time.Time{}, time.Time{}, apperrors.BadRequest("Invalid local start time.", nil)
	}
	end, e := time.Parse("15:04", in.EndLocalTime)
	if e != nil {
		return time.Time{}, time.Time{}, apperrors.BadRequest("Invalid local end time.", nil)
	}
	return start, end, nil
}

func validateTemplate(in slotTemplateRequest) error {
	if len(in.DaysOfWeek) == 0 || len(in.DaysOfWeek) > 7 || in.Label == "" || in.Capability == "" || in.BranchID == "" || in.CapacityPoolID == "" {
		return apperrors.BadRequest("Check the slot template details.", nil)
	}
	seen := map[int16]bool{}
	for _, d := range in.DaysOfWeek {
		if d < 1 || d > 7 || seen[d] {
			return apperrors.BadRequest("days_of_week must contain unique values from 1 to 7.", nil)
		}
		seen[d] = true
	}
	startClock, endClock, e := parseTemplateWindow(in)
	if e != nil {
		return e
	}
	if startClock.Equal(endClock) {
		return apperrors.BadRequest("Slot template start and end times must differ.", nil)
	}
	if in.CapacityOverride != nil && (*in.CapacityOverride < 0 || *in.CapacityOverride > MaxCapacity) {
		return apperrors.BadRequest("capacity_override is outside the supported range.", nil)
	}
	if in.EffectiveStartDate != nil {
		if _, e = time.Parse("2006-01-02", *in.EffectiveStartDate); e != nil {
			return apperrors.BadRequest("effective_start_date must use YYYY-MM-DD.", nil)
		}
	}
	if in.EffectiveEndDate != nil {
		if _, e = time.Parse("2006-01-02", *in.EffectiveEndDate); e != nil {
			return apperrors.BadRequest("effective_end_date must use YYYY-MM-DD.", nil)
		}
	}
	if in.EffectiveStartDate != nil && in.EffectiveEndDate != nil && *in.EffectiveEndDate < *in.EffectiveStartDate {
		return apperrors.BadRequest("effective_end_date must not precede effective_start_date.", nil)
	}
	if in.GenerationHorizon < 1 || in.GenerationHorizon > 366 {
		return apperrors.BadRequest("generation_horizon_days must be between 1 and 366.", nil)
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return apperrors.BadRequest("timezone must be a valid IANA timezone.", nil)
	}
	return nil
}

func (h *Handler) applyBranchTimezone(c *gin.Context, businessID string, in *slotTemplateRequest) error {
	var branchTimezone string
	if err := h.service.db.QueryRow(c, `SELECT timezone FROM vendor_branches WHERE business_id=$1 AND id=$2`, businessID, in.BranchID).Scan(&branchTimezone); err != nil {
		return apperrors.NotFound("Branch not found.", nil)
	}
	if in.Timezone != "" && in.Timezone != branchTimezone {
		return apperrors.BadRequest("timezone must match the branch IANA timezone.", nil)
	}
	in.Timezone = branchTimezone
	return nil
}

func (h *Handler) createSlotTemplate(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in slotTemplateRequest
	if c.ShouldBindJSON(&in) != nil || !scope.Covers(in.BranchID) {
		httpx.RespondError(c, apperrors.BadRequest("Check the slot template details.", nil))
		return
	}
	if e := h.applyBranchTimezone(c, a.VendorID, &in); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if e := validateTemplate(in); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if in.EffectiveStartDate == nil {
		location, _ := time.LoadLocation(in.Timezone)
		today := h.service.Now().In(location).Format("2006-01-02")
		in.EffectiveStartDate = &today
	}
	id := newID("stpl_")
	tx, e := h.service.db.Begin(c)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer tx.Rollback(c)
	if e = h.service.ValidateCapacityPoolReference(c, tx, a.VendorID, in.BranchID, in.Capability, &in.CapacityPoolID); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if e = h.service.ValidateProductionLineReference(c, tx, a.VendorID, in.BranchID, in.Capability, in.ProductionLineID); e != nil {
		httpx.RespondError(c, e)
		return
	}
	_, e = tx.Exec(c, `INSERT INTO production_slot_templates(id,business_id,branch_id,capability,capacity_pool_id,production_line_id,label,days_of_week,start_local_time,end_local_time,capacity_override,effective_start_date,effective_end_date,timezone,generation_horizon_days) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::time,$10::time,$11,$12::date,$13::date,$14,$15)`, id, a.VendorID, in.BranchID, in.Capability, in.CapacityPoolID, in.ProductionLineID, in.Label, in.DaysOfWeek, in.StartLocalTime, in.EndLocalTime, in.CapacityOverride, in.EffectiveStartDate, in.EffectiveEndDate, in.Timezone, in.GenerationHorizon)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	if e = h.service.RecordMutation(c, tx, a, "production.slot_template.created", "production.slot_template.created", "production_slot_template", id, in.BranchID, 1, nil, in, map[string]any{"id": id}, ""); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if e = tx.Commit(c); e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id, "version": int64(1)}})
}

func (h *Handler) updateSlotTemplate(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in slotTemplateRequest
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check the slot template details.", nil))
		return
	}
	var branch string
	var version int64
	if e := h.service.db.QueryRow(c, `SELECT branch_id,version FROM production_slot_templates WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("templateId")).Scan(&branch, &version); e != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Slot template not found.", nil))
		return
	}
	in.BranchID = branch
	if e := h.applyBranchTimezone(c, a.VendorID, &in); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if e := validateTemplate(in); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		httpx.RespondError(c, apperrors.Conflict("The slot template changed; reload and retry.", nil))
		return
	}
	tx, e := h.service.db.Begin(c)
	if e == nil {
		defer tx.Rollback(c)
	}
	if e == nil {
		_, e = tx.Exec(c, `UPDATE production_slot_templates SET label=$1,days_of_week=$2,start_local_time=$3::time,end_local_time=$4::time,capacity_override=$5,effective_start_date=$6::date,effective_end_date=$7::date,timezone=$8,generation_horizon_days=$9,updated_at=now(),version=version+1 WHERE business_id=$10 AND id=$11 AND version=$12`, in.Label, in.DaysOfWeek, in.StartLocalTime, in.EndLocalTime, in.CapacityOverride, in.EffectiveStartDate, in.EffectiveEndDate, in.Timezone, in.GenerationHorizon, a.VendorID, c.Param("templateId"), version)
	}
	if e == nil {
		e = h.service.RecordMutation(c, tx, a, "production.slot_template.updated", "production.slot_template.updated", "production_slot_template", c.Param("templateId"), branch, version+1, nil, in, map[string]any{"id": c.Param("templateId")}, "")
	}
	if e == nil {
		e = tx.Commit(c)
	}
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"updated": true, "version": version + 1})
}

func (h *Handler) archiveSlotTemplate(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var branch string
	if h.service.db.QueryRow(c, `SELECT branch_id FROM production_slot_templates WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("templateId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Slot template not found.", nil))
		return
	}
	expectedVersion, expectedErr := bindExpectedVersion(c)
	if expectedErr != nil {
		httpx.RespondError(c, expectedErr)
		return
	}
	tx, e := h.service.db.Begin(c)
	if e == nil {
		defer tx.Rollback(c)
	}
	var version int64
	if e == nil {
		e = tx.QueryRow(c, `SELECT version FROM production_slot_templates WHERE business_id=$1 AND id=$2 FOR UPDATE`, a.VendorID, c.Param("templateId")).Scan(&version)
	}
	if e == nil && version != expectedVersion {
		e = errStaleVersion()
	}
	if e == nil {
		var tag pgconn.CommandTag
		tag, e = tx.Exec(c, `UPDATE production_slot_templates SET status='ARCHIVED',archived_at=now(),updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3 AND status='ACTIVE'`, a.VendorID, c.Param("templateId"), version)
		if e == nil && tag.RowsAffected() == 0 {
			e = errStaleVersion()
		}
	}
	if e == nil {
		e = h.service.RecordMutation(c, tx, a, "production.slot_template.archived", "production.slot_template.archived", "production_slot_template", c.Param("templateId"), branch, version+1, map[string]any{"status": "ACTIVE"}, map[string]any{"status": "ARCHIVED"}, nil, "")
	}
	if e == nil {
		e = tx.Commit(c)
	}
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"archived": true})
}

func (h *Handler) generateSlotTemplate(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	v, e := scanSlotTemplate(h.service.db.QueryRow(c, `SELECT `+slotTemplateSelect+` FROM production_slot_templates WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("templateId")))
	if e != nil || !scope.Covers(v.BranchID) || v.Status != "ACTIVE" {
		httpx.RespondError(c, apperrors.NotFound("Active slot template not found.", nil))
		return
	}
	loc, e := time.LoadLocation(v.Timezone)
	if e != nil {
		httpx.RespondError(c, apperrors.BadRequest("Template timezone is invalid.", nil))
		return
	}
	startClock, e := time.Parse("15:04", v.StartLocalTime)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	endClock, e := time.Parse("15:04", v.EndLocalTime)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	nowLocal := h.service.Now().In(loc)
	startDate := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	if v.EffectiveStartDate != nil {
		startDate = time.Date(v.EffectiveStartDate.Year(), v.EffectiveStartDate.Month(), v.EffectiveStartDate.Day(), 0, 0, 0, 0, loc)
	}
	horizon := v.GenerationHorizon
	if horizon <= 0 {
		horizon = 90
	}
	if v.EffectiveEndDate != nil {
		days := int(v.EffectiveEndDate.Sub(startDate).Hours()/24) + 1
		if days < horizon {
			horizon = days
		}
	}
	created := 0
	tx, e := h.service.db.Begin(c)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer tx.Rollback(c)
	for i := 0; i < horizon; i++ {
		d := startDate.AddDate(0, 0, i)
		weekday := int16(d.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		found := false
		for _, want := range v.DaysOfWeek {
			if want == weekday {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		localStart := time.Date(d.Year(), d.Month(), d.Day(), startClock.Hour(), startClock.Minute(), 0, 0, loc)
		localEnd := time.Date(d.Year(), d.Month(), d.Day(), endClock.Hour(), endClock.Minute(), 0, 0, loc)
		if !localEnd.After(localStart) {
			localEnd = localEnd.Add(24 * time.Hour)
		}
		var tag pgconn.CommandTag
		tag, e = tx.Exec(c, `INSERT INTO production_slots(id,business_id,branch_id,capability,capacity_pool_id,production_line_id,template_id,start_at,end_at,capacity_override,label) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(template_id,start_at) DO NOTHING`, newID("slot_"), a.VendorID, v.BranchID, v.Capability, v.CapacityPoolID, v.ProductionLineID, v.ID, localStart, localEnd, v.CapacityOverride, v.Label)
		if e != nil {
			httpx.RespondError(c, e)
			return
		}
		if tag.RowsAffected() > 0 {
			created++
		}
	}
	if e = h.service.RecordMutation(c, tx, a, "production.slot_template.generated", "production.slot_template.generated", "production_slot_template", v.ID, v.BranchID, v.Version, nil, map[string]any{"created": created, "horizon_days": horizon}, map[string]any{"created": created}, ""); e != nil {
		httpx.RespondError(c, e)
		return
	}
	if e = tx.Commit(c); e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"created": created, "horizon_days": horizon, "template_id": v.ID})
}

func (h *Handler) listSlots(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,capability,capacity_pool_id,production_line_id,start_at,end_at,capacity_override,label,status,version FROM production_slots WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) AND status<>'ARCHIVED' ORDER BY start_at`, a.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, cap, pool string
		var line *string
		var start, end time.Time
		var override *int
		var label, status string
		var version int64
		if rows.Scan(&id, &branch, &cap, &pool, &line, &start, &end, &override, &label, &status, &version) == nil {
			out = append(out, gin.H{"id": id, "branch_id": branch, "capability": cap, "capacity_pool_id": pool, "production_line_id": line, "start_at": start, "end_at": end, "capacity_override": override, "label": label, "status": status, "version": version})
		}
	}
	productionOK(c, out)
}
func (h *Handler) createSlot(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in slotRequest
	if c.ShouldBindJSON(&in) != nil || !scope.Covers(in.BranchID) || !in.EndAt.After(in.StartAt) || (in.CapacityOverride != nil && (*in.CapacityOverride < 0 || *in.CapacityOverride > MaxCapacity)) {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	id := newID("slot_")
	err := h.mutate(c, a, in.BranchID, "production.slot.created", "production_slot", id, 1, nil, in, map[string]any{"id": id}, func(tx pgx.Tx) error {
		if e := h.service.ValidateCapacityPoolReference(c, tx, a.VendorID, in.BranchID, in.Capability, &in.CapacityPoolID); e != nil {
			return e
		}
		if e := h.service.ValidateProductionLineReference(c, tx, a.VendorID, in.BranchID, in.Capability, in.ProductionLineID); e != nil {
			return e
		}
		_, e := tx.Exec(c, `INSERT INTO production_slots(id,business_id,branch_id,capability,capacity_pool_id,production_line_id,start_at,end_at,capacity_override,label) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, a.VendorID, in.BranchID, in.Capability, in.CapacityPoolID, in.ProductionLineID, in.StartAt, in.EndAt, in.CapacityOverride, in.Label)
		return e
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}

func (h *Handler) getSlot(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var id, branch, capability, poolID, label, status string
	var lineID, templateID *string
	var startAt, endAt, createdAt, updatedAt time.Time
	var archivedAt *time.Time
	var capacityOverride *int
	var version int64
	err := h.service.db.QueryRow(c, `SELECT id,branch_id,capability,capacity_pool_id,production_line_id,template_id,start_at,end_at,capacity_override,label,status,created_at,updated_at,archived_at,version FROM production_slots WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("slotId")).Scan(&id, &branch, &capability, &poolID, &lineID, &templateID, &startAt, &endAt, &capacityOverride, &label, &status, &createdAt, &updatedAt, &archivedAt, &version)
	if err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Slot not found.", nil))
		return
	}
	productionOK(c, gin.H{"id": id, "branch_id": branch, "capability": capability, "capacity_pool_id": poolID, "production_line_id": lineID, "template_id": templateID, "start_at": startAt, "end_at": endAt, "capacity_override": capacityOverride, "label": label, "status": status, "created_at": createdAt, "updated_at": updatedAt, "archived_at": archivedAt, "version": version})
}

func (h *Handler) updateSlot(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var in slotRequest
	if err := c.ShouldBindJSON(&in); err != nil || !in.EndAt.After(in.StartAt) || (in.CapacityOverride != nil && (*in.CapacityOverride < 0 || *in.CapacityOverride > MaxCapacity)) {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	var branch, status string
	var version int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,status,version FROM production_slots WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("slotId")).Scan(&branch, &status, &version); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Slot not found.", nil))
		return
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	err := h.mutate(c, a, branch, "production.slot.updated", "production_slot", c.Param("slotId"), version+1, map[string]any{"status": status, "version": version}, in, nil, func(tx pgx.Tx) error {
		if e := h.service.ValidateCapacityPoolReference(c, tx, a.VendorID, branch, in.Capability, &in.CapacityPoolID); e != nil {
			return e
		}
		if e := h.service.ValidateProductionLineReference(c, tx, a.VendorID, branch, in.Capability, in.ProductionLineID); e != nil {
			return e
		}
		tag, e := tx.Exec(c, `UPDATE production_slots SET capability=$3,capacity_pool_id=$4,production_line_id=$5,start_at=$6,end_at=$7,capacity_override=$8,label=$9,updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$10 AND status<>'ARCHIVED'`, a.VendorID, c.Param("slotId"), in.Capability, in.CapacityPoolID, in.ProductionLineID, in.StartAt, in.EndAt, in.CapacityOverride, in.Label, version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, mapDB(err, "Slot", CodeSlotClosed))
		return
	}
	productionOK(c, gin.H{"id": c.Param("slotId"), "version": version + 1})
}

func (h *Handler) transitionSlot(c *gin.Context, target string) {
	a, scope, ok := h.authorized(c, PermProductionManage)
	if !ok {
		return
	}
	var branch, current string
	var version int64
	if h.service.db.QueryRow(c, `SELECT branch_id,status,version FROM production_slots WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("slotId")).Scan(&branch, &current, &version) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Slot not found.", nil))
		return
	}
	if target != "OPEN" && target != "CLOSED" && target != "BLACKOUT" && target != "ARCHIVED" {
		httpx.RespondError(c, apperrors.BadRequest("Invalid slot state.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	if current == "ARCHIVED" || current == target {
		httpx.RespondError(c, coded(apperrors.Conflict("Invalid slot transition.", nil), CodeInvalidTransition))
		return
	}
	err := h.mutate(c, a, branch, "production.slot."+strings.ToLower(target), "production_slot", c.Param("slotId"), version+1, map[string]any{"status": current}, map[string]any{"status": target}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_slots SET status=$3,updated_at=now(),version=version+1,archived_at=CASE WHEN $3='ARCHIVED' THEN now() ELSE NULL END WHERE business_id=$1 AND id=$2 AND version=$4`, a.VendorID, c.Param("slotId"), target, version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"status": target, "version": version + 1})
}
func (h *Handler) closeSlot(c *gin.Context)    { h.transitionSlot(c, "CLOSED") }
func (h *Handler) openSlot(c *gin.Context)     { h.transitionSlot(c, "OPEN") }
func (h *Handler) blackoutSlot(c *gin.Context) { h.transitionSlot(c, "BLACKOUT") }
func (h *Handler) archiveSlot(c *gin.Context)  { h.transitionSlot(c, "ARCHIVED") }
func (h *Handler) listJobs(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,capability,reservation_id,production_line_id,source_type,source_ref,source_version,title,status,priority,planned_quantity,produced_quantity,waste_quantity,scheduled_start,scheduled_end,actual_start,ready_at,completed_at,expected_ready_window_start,expected_ready_window_end,blocked_reason,risk_flags,created_at,updated_at,version FROM production_jobs WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY scheduled_start NULLS LAST,created_at DESC`, a.VendorID, scope.Global, scope.BranchIDs)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, cap, source, title, status, priority, blockedReason string
		var reservation, line, ref *string
		var sourceVersion *int64
		var planned, produced, waste int
		var start, end, actualStart, readyAt, completedAt, readyWindowStart, readyWindowEnd *time.Time
		var riskFlags json.RawMessage
		var created, updated time.Time
		var ver int64
		if rows.Scan(&id, &branch, &cap, &reservation, &line, &source, &ref, &sourceVersion, &title, &status, &priority, &planned, &produced, &waste, &start, &end, &actualStart, &readyAt, &completedAt, &readyWindowStart, &readyWindowEnd, &blockedReason, &riskFlags, &created, &updated, &ver) == nil {
			out = append(out, gin.H{"id": id, "branch_id": branch, "capability": cap, "reservation_id": reservation, "production_line_id": line, "source_type": source, "source_ref": ref, "source_version": sourceVersion, "title": title, "status": status, "priority": priority, "planned_quantity": planned, "produced_quantity": produced, "waste_quantity": waste, "scheduled_start": start, "scheduled_end": end, "actual_start": actualStart, "ready_at": readyAt, "completed_at": completedAt, "expected_ready_window_start": readyWindowStart, "expected_ready_window_end": readyWindowEnd, "blocked_reason": blockedReason, "risk_flags": riskFlags, "created_at": created, "updated_at": updated, "version": ver})
		}
	}
	productionOK(c, out)
}
func (h *Handler) createJob(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	var in struct {
		BranchID                 string     `json:"branch_id"`
		Capability               string     `json:"capability"`
		SourceType               string     `json:"source_type"`
		Title                    string     `json:"title"`
		ReservationID            *string    `json:"reservation_id"`
		ProductionLineID         *string    `json:"production_line_id"`
		SourceRef                *string    `json:"source_ref"`
		SourceVersion            *int64     `json:"source_version"`
		PlannedQuantity          int        `json:"planned_quantity"`
		ScheduledStart           *time.Time `json:"scheduled_start"`
		ScheduledEnd             *time.Time `json:"scheduled_end"`
		Priority                 string     `json:"priority"`
		ExpectedReadyWindowStart *time.Time `json:"expected_ready_window_start"`
		ExpectedReadyWindowEnd   *time.Time `json:"expected_ready_window_end"`
		RiskFlags                []string   `json:"risk_flags"`
	}
	if c.ShouldBindJSON(&in) != nil || !scope.Covers(in.BranchID) || len(strings.TrimSpace(in.Title)) < 2 || in.PlannedQuantity < 0 || in.Capability == "" || in.SourceType == "" {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	if in.SourceVersion != nil && *in.SourceVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("source_version must be positive.", nil))
		return
	}
	if in.ScheduledStart != nil && in.ScheduledEnd != nil && !in.ScheduledEnd.After(*in.ScheduledStart) {
		httpx.RespondError(c, apperrors.BadRequest("Scheduled end must be after scheduled start.", nil))
		return
	}
	if in.ExpectedReadyWindowStart != nil && in.ExpectedReadyWindowEnd != nil && in.ExpectedReadyWindowEnd.Before(*in.ExpectedReadyWindowStart) {
		httpx.RespondError(c, apperrors.BadRequest("Expected ready window end must not be before its start.", nil))
		return
	}
	if in.Priority == "" {
		in.Priority = "NORMAL"
	}
	if !inVocabulary(Priorities, in.Priority) {
		httpx.RespondError(c, apperrors.BadRequest("Priority must be NORMAL, HIGH or URGENT.", nil))
		return
	}
	riskFlags, _ := json.Marshal(in.RiskFlags)
	id := newID("job_")
	err := h.mutate(c, a, in.BranchID, "production.job.created", "production_job", id, 1, nil, in, map[string]any{"id": id}, func(tx pgx.Tx) error {
		_, e := tx.Exec(c, `INSERT INTO production_jobs(id,business_id,branch_id,capability,reservation_id,production_line_id,source_type,source_ref,source_version,title,priority,planned_quantity,scheduled_start,scheduled_end,expected_ready_window_start,expected_ready_window_end,risk_flags) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, id, a.VendorID, in.BranchID, in.Capability, in.ReservationID, in.ProductionLineID, in.SourceType, in.SourceRef, in.SourceVersion, in.Title, in.Priority, in.PlannedQuantity, in.ScheduledStart, in.ScheduledEnd, in.ExpectedReadyWindowStart, in.ExpectedReadyWindowEnd, riskFlags)
		if e != nil {
			return e
		}
		_, e = tx.Exec(c, `INSERT INTO production_job_events(id,business_id,job_id,event_type,to_status,note,actor_id,correlation_id) VALUES($1,$2,$3,'CREATED','PLANNED','',$4,NULLIF($5,''))`, newID("pje_"), a.VendorID, id, a.StaffID, a.CorrelationID)
		return e
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}
func (h *Handler) transitionJob(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	status := strings.ToUpper(c.Param("status"))
	allowed := map[string]bool{"QUEUED": true, "IN_PROGRESS": true, "BLOCKED": true, "READY": true, "COMPLETED": true, "CANCELLED": true}
	if !allowed[status] {
		httpx.RespondError(c, apperrors.BadRequest("Invalid production transition.", nil))
		return
	}
	var branch, current, sourceType string
	var sourceRef *string
	var plannedQuantity int
	var version int64
	if h.service.db.QueryRow(c, `SELECT branch_id,status,source_type,source_ref,planned_quantity,version FROM production_jobs WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("jobId")).Scan(&branch, &current, &sourceType, &sourceRef, &plannedQuantity, &version) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Job not found.", nil))
		return
	}
	valid := map[string]map[string]bool{"PLANNED": {"QUEUED": true, "IN_PROGRESS": true, "CANCELLED": true}, "QUEUED": {"IN_PROGRESS": true, "BLOCKED": true, "CANCELLED": true}, "IN_PROGRESS": {"BLOCKED": true, "READY": true, "CANCELLED": true}, "BLOCKED": {"IN_PROGRESS": true, "CANCELLED": true}, "READY": {"COMPLETED": true}, "COMPLETED": {}, "CANCELLED": {}}
	if !valid[current][status] {
		httpx.RespondError(c, coded(apperrors.Conflict("Invalid production job transition.", nil), CodeInvalidTransition))
		return
	}
	var in struct {
		Note             string `json:"note"`
		BlockedReason    string `json:"blocked_reason"`
		ProducedQuantity *int   `json:"produced_quantity"`
		WasteQuantity    *int   `json:"waste_quantity"`
	}
	_ = c.ShouldBindJSON(&in)
	if (in.ProducedQuantity != nil && (*in.ProducedQuantity < 0 || *in.ProducedQuantity > plannedQuantity)) || (in.WasteQuantity != nil && (*in.WasteQuantity < 0 || *in.WasteQuantity > plannedQuantity)) {
		httpx.RespondError(c, apperrors.BadRequest("Produced and waste quantities must be between zero and the planned quantity.", nil))
		return
	}
	if in.ProducedQuantity != nil && in.WasteQuantity != nil && *in.ProducedQuantity+*in.WasteQuantity > plannedQuantity {
		httpx.RespondError(c, apperrors.BadRequest("Produced and waste quantities cannot exceed the planned quantity.", nil))
		return
	}
	if status == "BLOCKED" && strings.TrimSpace(in.BlockedReason) == "" {
		httpx.RespondError(c, apperrors.BadRequest("A blocked reason is required.", nil))
		return
	}
	var expected int64
	if raw := strings.TrimSpace(c.GetHeader("If-Match")); raw == "" {
		httpx.RespondError(c, apperrors.BadRequest("A positive If-Match job version is required.", nil))
		return
	} else if _, scanErr := fmt.Sscan(strings.Trim(raw, `"`), &expected); scanErr != nil || expected <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("If-Match must contain a positive job version.", scanErr))
		return
	}
	if expected != version {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	tx, err := h.service.db.Begin(c)
	if err == nil {
		defer tx.Rollback(c)
	}
	if err == nil {
		if err == nil {
			var tag pgconn.CommandTag
			tag, err = tx.Exec(c, `UPDATE production_jobs SET status=$1,blocked_reason=CASE WHEN $1='BLOCKED' THEN $5 WHEN $1='IN_PROGRESS' THEN '' ELSE blocked_reason END,produced_quantity=COALESCE($6,produced_quantity),waste_quantity=COALESCE($7,waste_quantity),updated_at=now(),version=version+1,actual_start=CASE WHEN $1='IN_PROGRESS' THEN COALESCE(actual_start,now()) ELSE actual_start END,ready_at=CASE WHEN $1='READY' THEN now() ELSE ready_at END,completed_at=CASE WHEN $1='COMPLETED' THEN now() ELSE completed_at END WHERE business_id=$2 AND id=$3 AND version=$4`, status, a.VendorID, c.Param("jobId"), version, in.BlockedReason, in.ProducedQuantity, in.WasteQuantity)
			if err == nil && tag.RowsAffected() == 0 {
				err = errStaleVersion()
			}
		}
		if err == nil {
			eventType := map[string]string{"QUEUED": "QUEUED", "IN_PROGRESS": "STARTED", "BLOCKED": "BLOCKED", "READY": "READY", "COMPLETED": "COMPLETED", "CANCELLED": "CANCELLED"}[status]
			_, err = tx.Exec(c, `INSERT INTO production_job_events(id,business_id,job_id,event_type,from_status,to_status,note,actor_id,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''))`, newID("pje_"), a.VendorID, c.Param("jobId"), eventType, current, status, in.Note, a.StaffID, a.CorrelationID)
		}
		if err == nil && sourceType == "CORPORATE_OCCURRENCE" && sourceRef != nil && (status == "COMPLETED" || status == "CANCELLED") {
			occurrenceState := status
			var tag pgconn.CommandTag
			tag, err = tx.Exec(c, `UPDATE cloud_kitchen_occurrences o SET state=$1,version=o.version+1 FROM cloud_kitchen_corporate_plans p WHERE o.plan_id=p.id AND p.business_id=$2 AND o.id=$3 AND o.state IN ('RESERVED','PLANNED','CAPACITY_CONFLICT')`, occurrenceState, a.VendorID, *sourceRef)
			if err == nil && tag.RowsAffected() > 0 {
				err = h.service.RecordMutation(c, tx, a, "cloud_kitchen.occurrence."+strings.ToLower(occurrenceState), "cloud_kitchen.occurrence."+strings.ToLower(occurrenceState), "cloud_kitchen_occurrence", *sourceRef, branch, 1, nil, map[string]any{"state": occurrenceState, "production_job_id": c.Param("jobId")}, map[string]any{"occurrence_id": *sourceRef, "production_job_id": c.Param("jobId")}, "")
			}
		}
		if err == nil {
			err = h.service.RecordMutation(c, tx, a, "production.job."+strings.ToLower(status), "production.job."+strings.ToLower(status), "production_job", c.Param("jobId"), branch, version+1, map[string]any{"status": current}, map[string]any{"status": status}, map[string]any{"status": status}, "")
		}
		if err == nil {
			err = tx.Commit(c)
		}
	}
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"status": status})
}

func (h *Handler) getJob(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var id, branch, cap, source, title, status, priority, blockedReason string
	var reservation, line, ref *string
	var sourceVersion *int64
	var planned, produced, waste int
	var start, end, actualStart, readyAt, completedAt, readyWindowStart, readyWindowEnd *time.Time
	var riskFlags json.RawMessage
	var created, updated time.Time
	var version int64
	err := h.service.db.QueryRow(c, `SELECT id,branch_id,capability,reservation_id,production_line_id,source_type,source_ref,source_version,title,status,priority,planned_quantity,produced_quantity,waste_quantity,scheduled_start,scheduled_end,actual_start,ready_at,completed_at,expected_ready_window_start,expected_ready_window_end,blocked_reason,risk_flags,created_at,updated_at,version FROM production_jobs WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("jobId")).Scan(&id, &branch, &cap, &reservation, &line, &source, &ref, &sourceVersion, &title, &status, &priority, &planned, &produced, &waste, &start, &end, &actualStart, &readyAt, &completedAt, &readyWindowStart, &readyWindowEnd, &blockedReason, &riskFlags, &created, &updated, &version)
	if err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Job not found.", nil))
		return
	}
	productionOK(c, gin.H{"id": id, "branch_id": branch, "capability": cap, "reservation_id": reservation, "production_line_id": line, "source_type": source, "source_ref": ref, "source_version": sourceVersion, "title": title, "status": status, "priority": priority, "planned_quantity": planned, "produced_quantity": produced, "waste_quantity": waste, "scheduled_start": start, "scheduled_end": end, "actual_start": actualStart, "ready_at": readyAt, "completed_at": completedAt, "expected_ready_window_start": readyWindowStart, "expected_ready_window_end": readyWindowEnd, "blocked_reason": blockedReason, "risk_flags": riskFlags, "created_at": created, "updated_at": updated, "version": version})
}

func (h *Handler) listJobEvents(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	var branch string
	if err := h.service.db.QueryRow(c, `SELECT branch_id FROM production_jobs WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("jobId")).Scan(&branch); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Job not found.", nil))
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,event_type,from_status,to_status,note,actor_type,actor_id,correlation_id,recorded_at FROM production_job_events WHERE business_id=$1 AND job_id=$2 ORDER BY recorded_at,id`, a.VendorID, c.Param("jobId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, eventType, note, actorType string
		var fromStatus, toStatus, actorID, correlationID *string
		var recordedAt time.Time
		if err = rows.Scan(&id, &eventType, &fromStatus, &toStatus, &note, &actorType, &actorID, &correlationID, &recordedAt); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "event_type": eventType, "from_status": fromStatus, "to_status": toStatus, "note": note, "actor_type": actorType, "actor_id": actorID, "correlation_id": correlationID, "recorded_at": recordedAt})
	}
	productionOK(c, out)
}

func (h *Handler) listAlerts(c *gin.Context) {
	a, scope, ok := h.authorized(c, PermProductionRead)
	if !ok {
		return
	}
	status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
	if status != "" && status != "OPEN" && status != "ACKNOWLEDGED" && status != "RESOLVED" {
		httpx.RespondError(c, apperrors.BadRequest("Invalid alert status.", nil))
		return
	}
	rows, err := h.service.db.Query(c, `SELECT id,branch_id,alert_type,source_type,source_ref,status,payload,detected_at,acknowledged_at,acknowledged_by,resolved_at,version FROM production_operational_alerts WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) AND ($4='' OR status=$4) ORDER BY detected_at DESC,id LIMIT 500`, a.VendorID, scope.Global, scope.BranchIDs, status)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, alertType, sourceType, sourceRef, alertStatus string
		var payload json.RawMessage
		var detectedAt time.Time
		var acknowledgedAt, resolvedAt *time.Time
		var acknowledgedBy *string
		var version int64
		if err = rows.Scan(&id, &branch, &alertType, &sourceType, &sourceRef, &alertStatus, &payload, &detectedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &version); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "branch_id": branch, "alert_type": alertType, "source_type": sourceType, "source_ref": sourceRef, "status": alertStatus, "payload": payload, "detected_at": detectedAt, "acknowledged_at": acknowledgedAt, "acknowledged_by": acknowledgedBy, "resolved_at": resolvedAt, "version": version})
	}
	productionOK(c, out)
}

func (h *Handler) transitionAlert(c *gin.Context, target string) {
	a, scope, ok := h.authorized(c, PermProductionOperate)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64  `json:"expected_version"`
		Note            string `json:"note"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 || len(in.Note) > 280 {
		httpx.RespondError(c, apperrors.BadRequest("Check the alert transition details.", nil))
		return
	}
	var branch, current string
	var version int64
	if err := h.service.db.QueryRow(c, `SELECT branch_id,status,version FROM production_operational_alerts WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("alertId")).Scan(&branch, &current, &version); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Alert not found.", nil))
		return
	}
	if version != in.ExpectedVersion || current == "RESOLVED" || (target == "ACKNOWLEDGED" && current != "OPEN") {
		httpx.RespondError(c, errStaleVersion())
		return
	}
	err := h.mutate(c, a, branch, "production.alert."+strings.ToLower(target), "production_operational_alert", c.Param("alertId"), version+1, map[string]any{"status": current, "version": version}, map[string]any{"status": target, "version": version + 1}, map[string]any{"note": in.Note}, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE production_operational_alerts SET status=$3,acknowledged_at=CASE WHEN $3='ACKNOWLEDGED' THEN now() ELSE acknowledged_at END,acknowledged_by=CASE WHEN $3='ACKNOWLEDGED' THEN NULLIF($4,'') ELSE acknowledged_by END,resolved_at=CASE WHEN $3='RESOLVED' THEN now() ELSE resolved_at END,version=version+1 WHERE business_id=$1 AND id=$2 AND version=$5`, a.VendorID, c.Param("alertId"), target, a.StaffID, version)
		if e == nil && tag.RowsAffected() == 0 {
			return errStaleVersion()
		}
		return e
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"status": target, "version": version + 1})
}

func (h *Handler) acknowledgeAlert(c *gin.Context) { h.transitionAlert(c, "ACKNOWLEDGED") }
func (h *Handler) resolveAlert(c *gin.Context)     { h.transitionAlert(c, "RESOLVED") }
func (h *Handler) transitionJobStatus(c *gin.Context, status string) {
	c.Params = append(c.Params, gin.Param{Key: "status", Value: status})
	h.transitionJob(c)
}
func (h *Handler) startJob(c *gin.Context)    { h.transitionJobStatus(c, "IN_PROGRESS") }
func (h *Handler) readyJob(c *gin.Context)    { h.transitionJobStatus(c, "READY") }
func (h *Handler) completeJob(c *gin.Context) { h.transitionJobStatus(c, "COMPLETED") }
