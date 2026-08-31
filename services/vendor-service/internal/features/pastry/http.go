package pastry

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type StepUpConsumer interface {
	ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error
}

type Handler struct {
	s           *Service
	stepUp      StepUpConsumer
	idempotency *idempotency.Store
}

func (h *Handler) UseIdempotency(store *idempotency.Store) *Handler {
	h.idempotency = store
	return h
}

func NewHandler(s *Service, stepUp ...StepUpConsumer) *Handler {
	h := &Handler{s: s}
	if len(stepUp) > 0 {
		h.stepUp = stepUp[0]
	}
	return h
}

func (h *Handler) mutate(c *gin.Context, a production.Actor, branch, action, aggregateType, aggregateID string, version int64, before, after, payload any, fn func(pgx.Tx) error) error {
	tx, err := h.s.DB().Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	if err = fn(tx); err != nil {
		return err
	}
	if err = h.s.Production().RecordMutation(c, tx, a, action, action, aggregateType, aggregateID, branch, version, before, after, payload, ""); err != nil {
		return err
	}
	return tx.Commit(c)
}
func (h *Handler) auth(c *gin.Context, perm string) (production.Actor, production.Scope, bool) {
	st, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return production.Actor{}, production.Scope{}, false
	}
	a := production.Actor{StaffID: st.StaffID, VendorID: st.VendorID, SessionID: st.SessionID, CorrelationID: httpx.GetRequestID(c), CausationID: c.GetHeader("X-Causation-ID"), CommandRef: c.GetHeader("Idempotency-Key")}
	scope, e := h.s.Production().Authorize(c, a, perm)
	if e != nil {
		httpx.RespondError(c, e)
		return a, scope, false
	}
	return a, scope, true
}

func (h *Handler) requestBranchInScope(c *gin.Context, actor production.Actor, scope production.Scope, requestID string) (string, bool) {
	var branch string
	if err := h.s.DB().QueryRow(c, `SELECT branch_id FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, actor.VendorID, requestID).Scan(&branch); err != nil || !scope.Covers(branch) {
		return "", false
	}
	return branch, true
}

func (h *Handler) quoteBranchInScope(c *gin.Context, actor production.Actor, scope production.Scope, quoteID string) (string, bool) {
	var branch string
	if err := h.s.DB().QueryRow(c, `SELECT r.branch_id FROM pastry_quotes q JOIN pastry_custom_requests r ON r.id=q.request_id WHERE q.id=$1 AND r.business_id=$2`, quoteID, actor.VendorID).Scan(&branch); err != nil || !scope.Covers(branch) {
		return "", false
	}
	return branch, true
}

func (h *Handler) changeBranchInScope(c *gin.Context, actor production.Actor, scope production.Scope, changeID string) (string, bool) {
	var branch string
	if err := h.s.DB().QueryRow(c, `SELECT r.branch_id FROM pastry_change_requests ch JOIN pastry_custom_requests r ON r.id=ch.request_id WHERE ch.id=$1 AND r.business_id=$2`, changeID, actor.VendorID).Scan(&branch); err != nil || !scope.Covers(branch) {
		return "", false
	}
	return branch, true
}
func (h *Handler) products(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	v, e := h.s.ListProducts(c, a.VendorID, scope)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}
func (h *Handler) createProduct(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var p Product
	if c.ShouldBindJSON(&p) != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	id, e := h.s.CreateProduct(c, a.VendorID, p)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}

type standardBatchRequest struct {
	Quantity         int       `json:"quantity"`
	WindowStart      time.Time `json:"window_start"`
	WindowEnd        time.Time `json:"window_end"`
	ProductionLineID string    `json:"production_line_id"`
}

func (h *Handler) previewStandardBatch(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	var in standardBatchRequest
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check the batch details.", nil))
		return
	}
	preview, err := h.s.PreviewStandardBatch(c, a, scope, StandardBatchInput{ProductID: c.Param("productId"), Quantity: in.Quantity, WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, ProductionLineID: in.ProductionLineID})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

func (h *Handler) createStandardBatch(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in standardBatchRequest
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check the batch details.", nil))
		return
	}
	jobID, reservationID, err := h.s.CreateStandardBatch(c, a, scope, StandardBatchInput{ProductID: c.Param("productId"), Quantity: in.Quantity, WindowStart: in.WindowStart, WindowEnd: in.WindowEnd, ProductionLineID: in.ProductionLineID})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"production_job_id": jobID, "reservation_id": reservationID}})
}
func (h *Handler) groups(c *gin.Context) {
	a, _, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	rows, e := h.s.DB().Query(c, `SELECT id,name,group_type,min_select,max_select,status,version FROM pastry_option_groups WHERE business_id=$1 ORDER BY name`, a.VendorID)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, name, typ, status string
		var min, max int
		var ver int64
		if rows.Scan(&id, &name, &typ, &min, &max, &status, &ver) == nil {
			out = append(out, gin.H{"id": id, "name": name, "group_type": typ, "min_select": min, "max_select": max, "status": status, "version": ver})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) createGroup(c *gin.Context) {
	a, _, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		Name      string `json:"name"`
		GroupType string `json:"group_type"`
		Min       int    `json:"min_select"`
		Max       int    `json:"max_select"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	id, e := h.s.CreateGroup(c, a.VendorID, in.Name, in.GroupType, in.Min, in.Max)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}
func (h *Handler) addOption(c *gin.Context) {
	a, _, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		Name            string  `json:"name"`
		Code            string  `json:"code"`
		PriceDeltaMinor int64   `json:"price_delta_minor"`
		WorkloadWeight  int     `json:"workload_weight"`
		Position        int     `json:"position"`
		CapacityClass   *string `json:"capacity_class"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	var owner string
	if h.s.DB().QueryRow(c, `SELECT business_id FROM pastry_option_groups WHERE id=$1`, c.Param("groupId")).Scan(&owner) != nil || owner != a.VendorID {
		httpx.RespondError(c, apperrors.NotFound("Option group not found.", nil))
		return
	}
	id, e := h.s.AddOption(c, c.Param("groupId"), in.Name, in.Code, in.PriceDeltaMinor, in.WorkloadWeight, in.Position, in.CapacityClass)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}

func (h *Handler) options(c *gin.Context) {
	a, _, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	var owner string
	if h.s.DB().QueryRow(c, `SELECT business_id FROM pastry_option_groups WHERE id=$1`, c.Param("groupId")).Scan(&owner) != nil || owner != a.VendorID {
		httpx.RespondError(c, apperrors.NotFound("Option group not found.", nil))
		return
	}
	rows, err := h.s.DB().Query(c, `SELECT id,name,code,price_delta_minor,workload_weight,position,capacity_class,status,version FROM pastry_options WHERE group_id=$1 ORDER BY position,id`, c.Param("groupId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, name, code, status string
		var capacityClass *string
		var price int64
		var weight, position int
		var version int64
		if err = rows.Scan(&id, &name, &code, &price, &weight, &position, &capacityClass, &status, &version); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "name": name, "code": code, "price_delta_minor": price, "workload_weight": weight, "position": position, "capacity_class": capacityClass, "status": status, "version": version})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) attach(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		GroupID         string `json:"group_id"`
		Position        int    `json:"position"`
		Required        bool   `json:"required"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	var productBranch, groupOwner string
	if h.s.DB().QueryRow(c, `SELECT branch_id FROM pastry_products WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("productId")).Scan(&productBranch) != nil || !scope.Covers(productBranch) || h.s.DB().QueryRow(c, `SELECT business_id FROM pastry_option_groups WHERE id=$1`, in.GroupID).Scan(&groupOwner) != nil || groupOwner != a.VendorID {
		httpx.RespondError(c, apperrors.NotFound("Product or option group not found.", nil))
		return
	}
	if in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if e := h.s.AttachGroup(c, c.Param("productId"), in.GroupID, in.Position, in.Required, in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) request(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		BranchID            string         `json:"branch_id"`
		Title               string         `json:"title"`
		Notes               string         `json:"notes"`
		CustomerRef         string         `json:"customer_ref"`
		EventAt             *time.Time     `json:"event_at"`
		Logistics           map[string]any `json:"logistics"`
		RiskFlags           []string       `json:"risk_flags"`
		CorporateProfileRef string         `json:"corporate_profile_ref"`
		ChangeCutoffAt      *time.Time     `json:"change_cutoff_at"`
		ChangePolicy        map[string]any `json:"change_policy"`
		CancellationPolicy  map[string]any `json:"cancellation_policy"`
	}
	if c.ShouldBindJSON(&in) != nil || !scope.Covers(in.BranchID) {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	id, e := h.s.CreateRequest(c, a.VendorID, CustomRequestInput{BranchID: in.BranchID, CustomerRef: in.CustomerRef, CorporateProfileRef: in.CorporateProfileRef, Title: in.Title, Notes: in.Notes, RequiredAt: in.EventAt, ChangeCutoffAt: in.ChangeCutoffAt, ChangePolicy: in.ChangePolicy, CancellationPolicy: in.CancellationPolicy, Logistics: in.Logistics, RiskFlags: in.RiskFlags})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id, "version": 1}})
}
func (h *Handler) updateRequest(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		BranchID        string     `json:"branch_id"`
		Title           string     `json:"title"`
		Notes           string     `json:"notes"`
		EventAt         *time.Time `json:"event_at"`
		ExpectedVersion int64      `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check request details.", nil))
		return
	}
	var branch string
	if h.s.DB().QueryRow(c, `SELECT branch_id FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("requestId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	if e := h.s.UpdateRequest(c, a.VendorID, c.Param("requestId"), in.Title, in.Notes, in.EventAt, in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": c.Param("requestId")}})
}
func (h *Handler) spec(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64          `json:"expected_version"`
		Specification   map[string]any `json:"specification"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	var status string
	var cutoff *time.Time
	if h.s.DB().QueryRow(c, `SELECT status,change_cutoff_at FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("requestId")).Scan(&status, &cutoff) != nil {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	if status == "ACCEPTED" || status == "ACCEPTED_PENDING_ORDER" || status == "CONVERTED" || status == "CANCELLED" || (cutoff != nil && h.s.Production().Now().After(*cutoff)) {
		httpx.RespondError(c, apperrors.Conflict("This request is outside its change window.", nil))
		return
	}
	id, rev, e := h.s.AddSpec(c, c.Param("requestId"), in.Specification, in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id, "revision": rev}})
}
func (h *Handler) quote(c *gin.Context) {
	a, scope, ok := h.auth(c, PermQuote)
	if !ok {
		return
	}
	var in struct {
		SpecificationVersionID string                         `json:"specification_version_id"`
		AmountMinor            int64                          `json:"amount_minor"`
		ExpiresAt              time.Time                      `json:"expires_at"`
		CapacityPoolID         *string                        `json:"capacity_pool_id"`
		ProductionStart        time.Time                      `json:"production_start"`
		ProductionEnd          time.Time                      `json:"production_end"`
		CapacityAllocations    []production.AllocationRequest `json:"capacity_allocations"`
		HoldForMinutes         int                            `json:"hold_for_minutes"`
		ApprovalRequired       *bool                          `json:"approval_required"`
		VendorTerms            string                         `json:"vendor_terms"`
		NegotiationReason      string                         `json:"negotiation_reason"`
		NegotiationNote        string                         `json:"negotiation_note"`
		ExpectedRequestVersion int64                          `json:"expected_request_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedRequestVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	var specOwner string
	if h.s.DB().QueryRow(c, `SELECT r.business_id FROM pastry_specification_versions s JOIN pastry_custom_requests r ON r.id=s.request_id WHERE s.id=$1 AND s.request_id=$2`, in.SpecificationVersionID, c.Param("requestId")).Scan(&specOwner) != nil || specOwner != a.VendorID {
		httpx.RespondError(c, apperrors.NotFound("Specification not found.", nil))
		return
	}
	if len(in.CapacityAllocations) == 0 && in.CapacityPoolID != nil && *in.CapacityPoolID != "" {
		in.CapacityAllocations = []production.AllocationRequest{{CapacityPoolID: *in.CapacityPoolID, Quantity: 1}}
	}
	id, reservationID, e := h.s.CreateQuote(c, a, scope, c.Param("requestId"), in.SpecificationVersionID, in.AmountMinor, in.ExpiresAt, QuoteCapacityInput{WindowStart: in.ProductionStart, WindowEnd: in.ProductionEnd, Allocations: in.CapacityAllocations, HoldFor: time.Duration(in.HoldForMinutes) * time.Minute, ApprovalRequired: in.ApprovalRequired, VendorTerms: in.VendorTerms, NegotiationReason: in.NegotiationReason, NegotiationNote: in.NegotiationNote}, in.ExpectedRequestVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id, "reservation_id": reservationID}})
}
func (h *Handler) approve(c *gin.Context) {
	a, scope, ok := h.auth(c, PermApprove)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	grantID := c.GetHeader("X-Step-Up-Grant")
	if h.stepUp == nil || h.stepUp.ConsumeGrant(c, grantID, a.SessionID, a.StaffID, a.VendorID, "pastry.high_value_quote_approval") != nil {
		httpx.RespondError(c, apperrors.Forbidden("Step-up authentication is required for quote approval.", nil))
		return
	}
	if e := h.s.ApproveQuote(c, a, scope, c.Param("quoteId"), in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) accept(c *gin.Context) {
	a, scope, ok := h.auth(c, PermQuote)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	intent, res, e := h.s.AcceptQuote(c, a, scope, c.Param("quoteId"), in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"conversion_intent_id": intent, "reservation_id": res, "order_authority": "NOT_PRESENT"}})
}
func (h *Handler) milestones(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	rows, e := h.s.DB().Query(c, `SELECT id,milestone_type,state,position,version FROM pastry_milestones WHERE request_id=$1 ORDER BY position`, c.Param("requestId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, typ, state string
		var pos int
		var ver int64
		if rows.Scan(&id, &typ, &state, &pos, &ver) == nil {
			out = append(out, gin.H{"id": id, "type": typ, "state": state, "position": pos, "version": ver})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) getProduct(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	var p Product
	var raw []byte
	e := h.s.DB().QueryRow(c, `SELECT id,branch_id,name,description,product_type,base_price_minor,currency,customizable,production_class,capacity_pool_id,setup_buffer_minutes,cleanup_buffer_minutes,workload_units_per_item,logistics,status,version,position FROM pastry_products WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("productId")).Scan(&p.ID, &p.BranchID, &p.Name, &p.Description, &p.ProductType, &p.BasePriceMinor, &p.Currency, &p.Customizable, &p.ProductionClass, &p.CapacityPoolID, &p.SetupBufferMinutes, &p.CleanupBufferMinutes, &p.WorkloadUnitsPerItem, &raw, &p.Status, &p.Version, &p.Position)
	if e != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, apperrors.NotFound("Product not found.", nil))
		return
	}
	_ = json.Unmarshal(raw, &p.Logistics)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}
func (h *Handler) updateProduct(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var p Product
	if c.ShouldBindJSON(&p) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check product details.", nil))
		return
	}
	if err := validateLogisticsMetadata(p.Logistics); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check product logistics metadata.", err))
		return
	}
	var branch string
	var ver int64
	if h.s.DB().QueryRow(c, `SELECT branch_id,version FROM pastry_products WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("productId")).Scan(&branch, &ver) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Product not found.", nil))
		return
	}
	if p.Version <= 0 || p.Version != ver {
		httpx.RespondError(c, apperrors.Conflict("Product changed; reload and retry.", nil))
		return
	}
	raw, _ := json.Marshal(p.Logistics)
	e := h.mutate(c, a, branch, "pastry.product.updated", "pastry_product", c.Param("productId"), ver+1, nil, p, map[string]any{"id": c.Param("productId")}, func(tx pgx.Tx) error {
		if p.SetupBufferMinutes < 0 || p.SetupBufferMinutes > production.MaxBufferMinutes || p.CleanupBufferMinutes < 0 || p.CleanupBufferMinutes > production.MaxBufferMinutes {
			return apperrors.BadRequest("Production buffers must be between 0 and 480 minutes.", nil)
		}
		if p.WorkloadUnitsPerItem == 0 {
			p.WorkloadUnitsPerItem = 1
		}
		if e := h.s.Production().ValidateCapacityPoolReference(c, tx, a.VendorID, branch, "PASTRY_BAKERY", p.CapacityPoolID); e != nil {
			return e
		}
		tag, e := tx.Exec(c, `UPDATE pastry_products SET name=$1,description=$2,base_price_minor=$3,customizable=$4,production_class=$5,capacity_pool_id=$6,setup_buffer_minutes=$7,cleanup_buffer_minutes=$8,workload_units_per_item=$9,logistics=$10,position=$11,updated_at=now(),version=version+1 WHERE business_id=$12 AND id=$13 AND version=$14`, p.Name, p.Description, p.BasePriceMinor, p.Customizable, p.ProductionClass, p.CapacityPoolID, p.SetupBufferMinutes, p.CleanupBufferMinutes, p.WorkloadUnitsPerItem, raw, p.Position, a.VendorID, c.Param("productId"), ver)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return apperrors.Conflict("Product changed; reload and retry.", nil)
		}
		if _, e = tx.Exec(c, `INSERT INTO pastry_product_price_history(id,product_id,product_version,amount_minor,currency) SELECT $1,id,version,base_price_minor,currency FROM pastry_products WHERE business_id=$2 AND id=$3`, "pph_"+uuid.NewString(), a.VendorID, c.Param("productId")); e != nil {
			return e
		}
		return snapshotProduct(c, tx, c.Param("productId"), "CONFIG_UPDATED")
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"version": ver + 1}})
}
func (h *Handler) archiveProduct(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var branch string
	if h.s.DB().QueryRow(c, `SELECT branch_id FROM pastry_products WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("productId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Product not found.", nil))
		return
	}
	var ver int64
	_ = h.s.DB().QueryRow(c, `SELECT version FROM pastry_products WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("productId")).Scan(&ver)
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 || in.ExpectedVersion != ver {
		httpx.RespondError(c, apperrors.Conflict("Product changed; reload and retry.", nil))
		return
	}
	e := h.mutate(c, a, branch, "pastry.product.archived", "pastry_product", c.Param("productId"), ver+1, map[string]any{"status": "ACTIVE"}, map[string]any{"status": "ARCHIVED"}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE pastry_products SET status='ARCHIVED',updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3 AND status<>'ARCHIVED'`, a.VendorID, c.Param("productId"), ver)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return apperrors.Conflict("Product changed; reload and retry.", nil)
		}
		if e = recordProductPriceVersion(c, tx, c.Param("productId")); e != nil {
			return e
		}
		return snapshotProduct(c, tx, c.Param("productId"), "ARCHIVED")
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) getGroup(c *gin.Context) {
	a, _, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	var id, name, typ, status string
	var min, max int
	var ver int64
	if e := h.s.DB().QueryRow(c, `SELECT id,name,group_type,min_select,max_select,status,version FROM pastry_option_groups WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("groupId")).Scan(&id, &name, &typ, &min, &max, &status, &ver); e != nil {
		httpx.RespondError(c, apperrors.NotFound("Option group not found.", nil))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": id, "name": name, "group_type": typ, "min_select": min, "max_select": max, "status": status, "version": ver}})
}
func (h *Handler) updateGroup(c *gin.Context) {
	a, _, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		Name    string `json:"name"`
		Min     int    `json:"min_select"`
		Max     int    `json:"max_select"`
		Version int64  `json:"version"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check option group details.", nil))
		return
	}
	var ver int64
	_ = h.s.DB().QueryRow(c, `SELECT version FROM pastry_option_groups WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("groupId")).Scan(&ver)
	if in.Version <= 0 || in.Version != ver {
		httpx.RespondError(c, apperrors.Conflict("The option group changed; reload and retry.", nil))
		return
	}
	e := h.mutate(c, a, "", "pastry.option_group.updated", "pastry_option_group", c.Param("groupId"), ver+1, nil, in, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE pastry_option_groups SET name=$1,min_select=$2,max_select=$3,version=version+1 WHERE business_id=$4 AND id=$5 AND version=$6`, in.Name, in.Min, in.Max, a.VendorID, c.Param("groupId"), ver)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return apperrors.Conflict("The option group changed; reload and retry.", nil)
		}
		return snapshotProductsAttachedToGroup(c, tx, c.Param("groupId"))
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) archiveGroup(c *gin.Context) {
	a, _, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var ver int64
	_ = h.s.DB().QueryRow(c, `SELECT version FROM pastry_option_groups WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("groupId")).Scan(&ver)
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 || in.ExpectedVersion != ver {
		httpx.RespondError(c, apperrors.Conflict("The option group changed; reload and retry.", nil))
		return
	}
	e := h.mutate(c, a, "", "pastry.option_group.archived", "pastry_option_group", c.Param("groupId"), ver+1, map[string]any{"status": "ACTIVE"}, map[string]any{"status": "ARCHIVED"}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE pastry_option_groups SET status='ARCHIVED',version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3 AND status='ACTIVE'`, a.VendorID, c.Param("groupId"), ver)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return apperrors.Conflict("The option group changed; reload and retry.", nil)
		}
		return snapshotProductsAttachedToGroup(c, tx, c.Param("groupId"))
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) updateOption(c *gin.Context) {
	a, _, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		Name          string  `json:"name"`
		Code          string  `json:"code"`
		Delta         int64   `json:"price_delta_minor"`
		Weight        int     `json:"workload_weight"`
		Position      int     `json:"position"`
		CapacityClass *string `json:"capacity_class"`
		Version       int64   `json:"version"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check option details.", nil))
		return
	}
	if in.Version <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive version is required.", nil))
		return
	}
	if in.Delta < 0 || in.Weight <= 0 || in.Position < 0 || in.Position > 999 {
		httpx.RespondError(c, apperrors.BadRequest("Check option price, workload, and position.", nil))
		return
	}
	if in.CapacityClass != nil {
		value := strings.ToUpper(strings.TrimSpace(*in.CapacityClass))
		allowed := map[string]bool{"GENERAL": true, "HOT": true, "COLD": true, "BAKERY": true, "DECORATION": true, "PACKING": true, "CHILLING": true}
		if !allowed[value] {
			httpx.RespondError(c, apperrors.BadRequest("Invalid option capacity_class.", nil))
			return
		}
		in.CapacityClass = &value
	}
	e := h.mutate(c, a, "", "pastry.option.updated", "pastry_option", c.Param("optionId"), in.Version+1, nil, in, nil, func(tx pgx.Tx) error {
		var groupID string
		if e := tx.QueryRow(c, `SELECT o.group_id FROM pastry_options o JOIN pastry_option_groups g ON g.id=o.group_id WHERE o.id=$1 AND g.business_id=$2 AND o.version=$3 FOR UPDATE OF o`, c.Param("optionId"), a.VendorID, in.Version).Scan(&groupID); e != nil {
			return apperrors.Conflict("The option changed; reload and retry.", nil)
		}
		tag, e := tx.Exec(c, `UPDATE pastry_options SET name=$1,code=$2,price_delta_minor=$3,workload_weight=$4,position=$5,capacity_class=$6,version=version+1 WHERE id=$7 AND version=$8`, in.Name, in.Code, in.Delta, in.Weight, in.Position, in.CapacityClass, c.Param("optionId"), in.Version)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return apperrors.Conflict("The option changed; reload and retry.", nil)
		}
		return snapshotProductsAttachedToGroup(c, tx, groupID)
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) archiveOption(c *gin.Context) {
	a, _, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	e := h.mutate(c, a, "", "pastry.option.archived", "pastry_option", c.Param("optionId"), in.ExpectedVersion+1, map[string]any{"status": "ACTIVE"}, map[string]any{"status": "ARCHIVED"}, nil, func(tx pgx.Tx) error {
		var groupID string
		if e := tx.QueryRow(c, `SELECT o.group_id FROM pastry_options o JOIN pastry_option_groups g ON g.id=o.group_id WHERE o.id=$1 AND g.business_id=$2 AND o.version=$3 AND o.status='ACTIVE' FOR UPDATE OF o`, c.Param("optionId"), a.VendorID, in.ExpectedVersion).Scan(&groupID); e != nil {
			return apperrors.Conflict("The option changed; reload and retry.", nil)
		}
		if _, e := tx.Exec(c, `UPDATE pastry_options SET status='ARCHIVED',version=version+1 WHERE id=$1 AND version=$2`, c.Param("optionId"), in.ExpectedVersion); e != nil {
			return e
		}
		return snapshotProductsAttachedToGroup(c, tx, groupID)
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) detach(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var branch string
	var version int64
	if e := h.s.DB().QueryRow(c, `SELECT branch_id,version FROM pastry_products WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("productId")).Scan(&branch, &version); e != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Product not found.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		httpx.RespondError(c, apperrors.Conflict("Product changed; reload and retry.", nil))
		return
	}
	e := h.mutate(c, a, branch, "pastry.product_option_group.detached", "pastry_product_option_group", c.Param("productId")+":"+c.Param("groupId"), version+1, nil, map[string]any{"detached": true}, nil, func(tx pgx.Tx) error {
		tag, err := tx.Exec(c, `DELETE FROM pastry_product_option_groups p USING pastry_products pr,pastry_option_groups g WHERE p.product_id=pr.id AND p.group_id=g.id AND pr.id=$1 AND p.group_id=$2 AND pr.business_id=$3 AND g.business_id=$3`, c.Param("productId"), c.Param("groupId"), a.VendorID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperrors.NotFound("Product option group attachment not found.", nil)
		}
		productTag, err := tx.Exec(c, `UPDATE pastry_products SET version=version+1,updated_at=now() WHERE business_id=$1 AND id=$2 AND version=$3`, a.VendorID, c.Param("productId"), version)
		if err != nil {
			return err
		}
		if productTag.RowsAffected() == 0 {
			return apperrors.Conflict("Product changed; reload and retry.", nil)
		}
		if err := recordProductPriceVersion(c, tx, c.Param("productId")); err != nil {
			return err
		}
		return snapshotProduct(c, tx, c.Param("productId"), "OPTIONS_UPDATED")
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) requests(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	rows, e := h.s.DB().Query(c, `SELECT id,branch_id,customer_ref,corporate_profile_ref,title,notes,status,event_at,change_cutoff_at,logistics,risk_flags,change_policy,cancellation_policy,version,created_at,updated_at FROM pastry_custom_requests WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY created_at DESC`, a.VendorID, scope.Global, scope.BranchIDs)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, branch, title, notes, status string
		var customerRef, corporateProfileRef *string
		var logistics, riskFlags, changePolicy, cancellationPolicy json.RawMessage
		var event, cutoff *time.Time
		var created, updated time.Time
		var ver int64
		if rows.Scan(&id, &branch, &customerRef, &corporateProfileRef, &title, &notes, &status, &event, &cutoff, &logistics, &riskFlags, &changePolicy, &cancellationPolicy, &ver, &created, &updated) == nil {
			out = append(out, gin.H{"id": id, "branch_id": branch, "customer_ref": customerRef, "corporate_profile_ref": corporateProfileRef, "title": title, "notes": notes, "status": status, "required_at": event, "change_cutoff_at": cutoff, "logistics": logistics, "risk_flags": riskFlags, "change_policy": changePolicy, "cancellation_policy": cancellationPolicy, "version": ver, "created_at": created, "updated_at": updated})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) getRequest(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	var branch, title, notes, status string
	var customerRef, corporateProfileRef *string
	var logistics, riskFlags, changePolicy, cancellationPolicy json.RawMessage
	var event, cutoff *time.Time
	var ver int64
	if e := h.s.DB().QueryRow(c, `SELECT branch_id,customer_ref,corporate_profile_ref,title,notes,status,event_at,change_cutoff_at,logistics,risk_flags,change_policy,cancellation_policy,version FROM pastry_custom_requests WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("requestId")).Scan(&branch, &customerRef, &corporateProfileRef, &title, &notes, &status, &event, &cutoff, &logistics, &riskFlags, &changePolicy, &cancellationPolicy, &ver); e != nil {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": c.Param("requestId"), "branch_id": branch, "customer_ref": customerRef, "corporate_profile_ref": corporateProfileRef, "title": title, "notes": notes, "status": status, "required_at": event, "change_cutoff_at": cutoff, "logistics": logistics, "risk_flags": riskFlags, "change_policy": changePolicy, "cancellation_policy": cancellationPolicy, "version": ver}})
}
func (h *Handler) submitRequest(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	branch, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId"))
	if !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	e := h.mutate(c, a, branch, "pastry.custom_request.submitted", "pastry_custom_request", c.Param("requestId"), in.ExpectedVersion+1, map[string]any{"status": "DRAFT", "version": in.ExpectedVersion}, map[string]any{"status": "SUBMITTED", "version": in.ExpectedVersion + 1}, nil, func(tx pgx.Tx) error {
		tag, e := tx.Exec(c, `UPDATE pastry_custom_requests SET status='SUBMITTED',version=version+1,updated_at=now() WHERE business_id=$1 AND id=$2 AND status='DRAFT' AND version=$3`, a.VendorID, c.Param("requestId"), in.ExpectedVersion)
		if e == nil && tag.RowsAffected() == 0 {
			return apperrors.Conflict("Request changed; reload and retry.", nil)
		}
		return e
	})
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) cancelRequest(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	var in struct {
		Reason          string `json:"reason"`
		ReviewApproved  bool   `json:"review_approved"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	eligibility, e := h.s.CancelRequest(c, a, scope, c.Param("requestId"), in.Reason, in.ReviewApproved, in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "CANCELLED", "eligibility_at_decision": eligibility}})
}
func (h *Handler) specifications(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	rows, e := h.s.DB().Query(c, `SELECT id,revision,specification,created_at FROM pastry_specification_versions WHERE request_id=$1 ORDER BY revision`, c.Param("requestId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id string
		var rev int
		var raw []byte
		var created time.Time
		if rows.Scan(&id, &rev, &raw, &created) == nil {
			var spec map[string]any
			_ = json.Unmarshal(raw, &spec)
			out = append(out, gin.H{"id": id, "revision": rev, "specification": spec, "created_at": created})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) change(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		Reason          string         `json:"reason"`
		Specification   map[string]any `json:"specification"`
		ExpectedVersion int64          `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check change details.", nil))
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	eligibility, eligibilityErr := h.s.ChangeEligibility(c, a.VendorID, c.Param("requestId"))
	if eligibilityErr != nil {
		httpx.RespondError(c, eligibilityErr)
		return
	}
	if !eligibility.Allowed {
		httpx.RespondError(c, apperrors.Conflict("This request is outside its change window: "+eligibility.ReasonCode, nil))
		return
	}
	id, spec, rev, e := h.s.RequestChange(c, c.Param("requestId"), in.Reason, in.Specification, in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"change_request_id": id, "specification_id": spec, "revision": rev}})
}
func (h *Handler) changes(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	rows, e := h.s.DB().Query(c, `SELECT id,from_revision,to_revision,reason,status,decided_at,decided_by,decision_note,version,created_at,updated_at FROM pastry_change_requests WHERE request_id=$1 ORDER BY created_at`, c.Param("requestId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, reason, status, decisionNote string
		var from, to int
		var decidedAt *time.Time
		var decidedBy *string
		var version int64
		var created, updated time.Time
		if rows.Scan(&id, &from, &to, &reason, &status, &decidedAt, &decidedBy, &decisionNote, &version, &created, &updated) == nil {
			out = append(out, gin.H{"id": id, "from_revision": from, "to_revision": to, "reason": reason, "status": status, "decided_at": decidedAt, "decided_by": decidedBy, "decision_note": decisionNote, "version": version, "created_at": created, "updated_at": updated})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) decideChange(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	var in struct {
		Decision        string `json:"decision"`
		Note            string `json:"note"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check change decision.", nil))
		return
	}
	if _, inScope := h.changeBranchInScope(c, a, scope, c.Param("changeId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Change request not found.", nil))
		return
	}
	if in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if e := h.s.DecideChange(c, a.VendorID, c.Param("changeId"), in.Decision, a.StaffID, in.Note, in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": c.Param("changeId"), "status": in.Decision}})
}
func (h *Handler) changeHistory(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.changeBranchInScope(c, a, scope, c.Param("changeId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Change request not found.", nil))
		return
	}
	v, e := h.s.ListChangeHistory(c, a.VendorID, c.Param("changeId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}
func (h *Handler) quotes(c *gin.Context) {
	a, scope, ok := h.auth(c, PermQuote)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	rows, e := h.s.DB().Query(c, `SELECT id,revision,specification_version_id,amount_minor,currency,expires_at,status,reservation_id,production_start,production_end,approval_required,approved_by,approved_at,vendor_terms,negotiation_reason,negotiation_note,accepted_at,version,created_at FROM pastry_quotes WHERE request_id=$1 ORDER BY revision`, c.Param("requestId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, spec, currency, status, vendorTerms, negotiationReason, negotiationNote string
		var rev int
		var amount int64
		var expires, created time.Time
		var reservation, approvedBy *string
		var productionStart, productionEnd, approvedAt, acceptedAt *time.Time
		var approvalRequired bool
		var version int64
		if rows.Scan(&id, &rev, &spec, &amount, &currency, &expires, &status, &reservation, &productionStart, &productionEnd, &approvalRequired, &approvedBy, &approvedAt, &vendorTerms, &negotiationReason, &negotiationNote, &acceptedAt, &version, &created) == nil {
			out = append(out, gin.H{"id": id, "revision": rev, "specification_version_id": spec, "amount_minor": amount, "currency": currency, "expires_at": expires, "status": status, "reservation_id": reservation, "production_start": productionStart, "production_end": productionEnd, "approval_required": approvalRequired, "approved_by": approvedBy, "approved_at": approvedAt, "vendor_terms": vendorTerms, "negotiation_reason": negotiationReason, "negotiation_note": negotiationNote, "accepted_at": acceptedAt, "version": version, "created_at": created})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) getQuote(c *gin.Context) {
	a, scope, ok := h.auth(c, PermQuote)
	if !ok {
		return
	}
	if _, inScope := h.quoteBranchInScope(c, a, scope, c.Param("quoteId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Quote not found.", nil))
		return
	}
	var rev int
	var req, spec, currency, status, vendorTerms, negotiationReason, negotiationNote string
	var amount int64
	var expires time.Time
	var reservation, approvedBy *string
	var productionStart, productionEnd, approvedAt, acceptedAt *time.Time
	var approvalRequired bool
	var version int64
	if e := h.s.DB().QueryRow(c, `SELECT request_id,revision,specification_version_id,amount_minor,currency,expires_at,status,reservation_id,production_start,production_end,approval_required,approved_by,approved_at,vendor_terms,negotiation_reason,negotiation_note,accepted_at,version FROM pastry_quotes WHERE id=$1`, c.Param("quoteId")).Scan(&req, &rev, &spec, &amount, &currency, &expires, &status, &reservation, &productionStart, &productionEnd, &approvalRequired, &approvedBy, &approvedAt, &vendorTerms, &negotiationReason, &negotiationNote, &acceptedAt, &version); e != nil {
		httpx.RespondError(c, e)
		return
	}
	requirementRows, e := h.s.DB().Query(c, `SELECT capacity_pool_id,production_line_id,quantity FROM pastry_quote_capacity_requirements WHERE quote_id=$1 ORDER BY capacity_pool_id`, c.Param("quoteId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer requirementRows.Close()
	requirements := []gin.H{}
	for requirementRows.Next() {
		var poolID string
		var lineID *string
		var quantity int
		if e = requirementRows.Scan(&poolID, &lineID, &quantity); e != nil {
			httpx.RespondError(c, e)
			return
		}
		requirements = append(requirements, gin.H{"capacity_pool_id": poolID, "production_line_id": lineID, "quantity": quantity})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": c.Param("quoteId"), "request_id": req, "revision": rev, "specification_version_id": spec, "amount_minor": amount, "currency": currency, "expires_at": expires, "status": status, "reservation_id": reservation, "production_start": productionStart, "production_end": productionEnd, "capacity_requirements": requirements, "approval_required": approvalRequired, "approved_by": approvedBy, "approved_at": approvedAt, "vendor_terms": vendorTerms, "negotiation_reason": negotiationReason, "negotiation_note": negotiationNote, "accepted_at": acceptedAt, "version": version}})
}
func (h *Handler) reviseQuote(c *gin.Context) {
	a, scope, ok := h.auth(c, PermQuote)
	if !ok {
		return
	}
	var in struct {
		SpecificationVersionID string    `json:"specification_version_id"`
		AmountMinor            int64     `json:"amount_minor"`
		ExpiresAt              time.Time `json:"expires_at"`
		ReasonCode             string    `json:"reason_code"`
		Note                   string    `json:"note"`
		VendorTerms            string    `json:"vendor_terms"`
		ExpectedVersion        int64     `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check quote details.", nil))
		return
	}
	if _, inScope := h.quoteBranchInScope(c, a, scope, c.Param("quoteId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Quote not found.", nil))
		return
	}
	if in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	id, e := h.s.ReviseQuote(c, a, scope, c.Param("quoteId"), in.SpecificationVersionID, in.AmountMinor, in.ExpiresAt, in.ReasonCode, in.Note, in.VendorTerms, in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}
func (h *Handler) decline(c *gin.Context) {
	a, scope, ok := h.auth(c, PermQuote)
	if !ok {
		return
	}
	if _, inScope := h.quoteBranchInScope(c, a, scope, c.Param("quoteId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Quote not found.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if e := h.s.DeclineQuote(c, a, scope, c.Param("quoteId"), in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) diff(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	var branch string
	if h.s.DB().QueryRow(c, `SELECT r.branch_id FROM pastry_specification_versions s JOIN pastry_custom_requests r ON r.id=s.request_id WHERE s.id=$1 AND r.id=$2 AND r.business_id=$3`, c.Query("from"), c.Param("requestId"), a.VendorID).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Specification not found.", nil))
		return
	}
	var toRequest string
	if h.s.DB().QueryRow(c, `SELECT request_id FROM pastry_specification_versions WHERE id=$1`, c.Query("to")).Scan(&toRequest) != nil || toRequest != c.Param("requestId") {
		httpx.RespondError(c, apperrors.NotFound("Specification not found.", nil))
		return
	}
	out, e := h.s.SpecificationDiff(c, c.Query("from"), c.Query("to"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) transitionMilestone(c *gin.Context) {
	a, scope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	a.CorrelationID = c.GetHeader("X-Correlation-ID")
	var in struct {
		Note            string `json:"note"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	version, e := h.s.TransitionMilestone(c, a, c.Param("requestId"), c.Param("milestoneType"), c.Param("state"), in.Note, in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"version": version}})
}

func (h *Handler) milestoneHistory(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	rows, err := h.s.DB().Query(c, `SELECT h.id,h.from_state,h.to_state,h.actor_id,h.correlation_id,h.note,h.changed_at FROM pastry_milestone_history h JOIN pastry_milestones m ON m.id=h.milestone_id WHERE m.request_id=$1 AND m.milestone_type=$2 ORDER BY h.changed_at,h.id`, c.Param("requestId"), c.Param("milestoneType"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, toState, note string
		var fromState, actorID, correlationID *string
		var changedAt time.Time
		if err = rows.Scan(&id, &fromState, &toState, &actorID, &correlationID, &note, &changedAt); err != nil {
			httpx.RespondError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "from_state": fromState, "to_state": toState, "actor_id": actorID, "correlation_id": correlationID, "note": note, "changed_at": changedAt})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) changeEligibility(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	eligibility, err := h.s.ChangeEligibility(c, a.VendorID, c.Param("requestId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": eligibility})
}
func (h *Handler) cancellationEligibility(c *gin.Context) {
	a, scope, ok := h.auth(c, PermRead)
	if !ok {
		return
	}
	if _, inScope := h.requestBranchInScope(c, a, scope, c.Param("requestId")); !inScope {
		httpx.RespondError(c, apperrors.NotFound("Request not found.", nil))
		return
	}
	eligibility, err := h.s.CancellationEligibility(c, a.VendorID, c.Param("requestId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": eligibility})
}

func RegisterRoutes(group *gin.RouterGroup, h *Handler, flags production.Flags) {
	if !flags.ProductionEnabled {
		return
	}
	g := group.Group("/pastry")
	g.Use(idempotency.CommandMiddleware(h.idempotency))
	g.GET("/products", h.products)
	g.POST("/products", h.createProduct)
	g.GET("/products/:productId", h.getProduct)
	g.PATCH("/products/:productId", h.updateProduct)
	g.POST("/products/:productId/archive", h.archiveProduct)
	g.POST("/products/:productId/capacity-preview", h.previewStandardBatch)
	g.POST("/products/:productId/production-batches", h.createStandardBatch)
	g.GET("/option-groups", h.groups)
	g.POST("/option-groups", h.createGroup)
	g.GET("/option-groups/:groupId", h.getGroup)
	g.PATCH("/option-groups/:groupId", h.updateGroup)
	g.POST("/option-groups/:groupId/archive", h.archiveGroup)
	g.GET("/option-groups/:groupId/options", h.options)
	g.POST("/option-groups/:groupId/options", h.addOption)
	g.PATCH("/options/:optionId", h.updateOption)
	g.POST("/options/:optionId/archive", h.archiveOption)
	g.POST("/products/:productId/option-groups", h.attach)
	g.DELETE("/products/:productId/option-groups/:groupId", h.detach)
	g.POST("/custom-requests", h.request)
	g.GET("/custom-requests", h.requests)
	g.GET("/custom-requests/:requestId", h.getRequest)
	g.PATCH("/custom-requests/:requestId", h.updateRequest)
	g.POST("/custom-requests/:requestId/submit", h.submitRequest)
	g.POST("/custom-requests/:requestId/cancel", h.cancelRequest)
	g.POST("/custom-requests/:requestId/specifications", h.spec)
	g.GET("/custom-requests/:requestId/specifications", h.specifications)
	g.POST("/custom-requests/:requestId/changes", h.change)
	g.GET("/custom-requests/:requestId/changes", h.changes)
	g.POST("/changes/:changeId/decision", h.decideChange)
	g.GET("/changes/:changeId/history", h.changeHistory)
	g.POST("/custom-requests/:requestId/quotes", h.quote)
	g.GET("/custom-requests/:requestId/quotes", h.quotes)
	g.GET("/quotes/:quoteId", h.getQuote)
	g.POST("/quotes/:quoteId/revise", h.reviseQuote)
	g.POST("/quotes/:quoteId/approve", h.approve)
	g.POST("/quotes/:quoteId/accept", h.accept)
	g.POST("/quotes/:quoteId/decline", h.decline)
	g.GET("/custom-requests/:requestId/milestones", h.milestones)
	g.GET("/custom-requests/:requestId/milestones/:milestoneType/history", h.milestoneHistory)
	g.POST("/custom-requests/:requestId/milestones/:milestoneType/:state", h.transitionMilestone)
	g.GET("/custom-requests/:requestId/specifications/diff", h.diff)
	g.GET("/custom-requests/:requestId/change-eligibility", h.changeEligibility)
	g.GET("/custom-requests/:requestId/cancellation-eligibility", h.cancellationEligibility)
}
