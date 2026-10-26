package vendors

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct {
	db      *pgxpool.Pool
	enabled bool
	allowed map[string]bool
}

func New(db *pgxpool.Pool, enabled bool, allowed []string) *Handler {
	set := map[string]bool{}
	for _, v := range allowed {
		set[strings.TrimSpace(v)] = true
	}
	return &Handler{db: db, enabled: enabled, allowed: set}
}

func Register(g *gin.RouterGroup, h *Handler, v *serviceauth.Verifier) {
	r := g.Group("/internal/vendor-cases")
	r.Use(serviceauth.Middleware(v), h.allow, h.nonce)
	r.POST("/cases", h.create)
	r.GET("/cases/by-client-reference/:clientReference", h.readByClient)
	r.GET("/cases/:caseRef", h.read)
	r.POST("/cases/:caseRef/decisions", h.decide)
}

func caller(c *gin.Context) string { v, _ := c.Get("service_name"); s, _ := v.(string); return s }
func (h *Handler) allow(c *gin.Context) {
	if !h.allowed[caller(c)] {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}
func (h *Handler) nonce(c *gin.Context) {
	n := c.GetHeader(serviceauth.HeaderNonce)
	if n == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tag, e := h.db.Exec(c, `INSERT INTO verification_service_nonces(caller,nonce,expires_at) VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, caller(c), n)
	if e != nil || tag.RowsAffected() != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

type createBody struct {
	ClientReference string   `json:"client_reference"`
	BusinessID      string   `json:"business_id"`
	Dimension       string   `json:"dimension"`
	SubjectType     string   `json:"subject_type"`
	SubjectRef      string   `json:"subject_ref"`
	DocumentRefs    []string `json:"document_refs"`
}
type decisionBody struct {
	Status                string     `json:"status"`
	ReasonCode            string     `json:"reason_code"`
	Remediation           string     `json:"remediation"`
	ExpiresAt             *time.Time `json:"expires_at"`
	ExpectedSourceVersion int64      `json:"expected_source_version"`
	// AppealRef is required (and only allowed) when reopening a REJECTED,
	// EXPIRED or SUSPENDED case back to PENDING_REVIEW.
	AppealRef string `json:"appeal_ref,omitempty"`
	// DecidedByStaffRef names the human reviewer, when the caller knows one.
	DecidedByStaffRef string `json:"decided_by_staff_ref,omitempty"`
}
type view struct {
	SourceRef       string     `json:"source_ref"`
	ClientReference string     `json:"client_reference"`
	BusinessID      string     `json:"business_id"`
	Dimension       string     `json:"dimension"`
	SubjectType     string     `json:"subject_type"`
	SubjectRef      string     `json:"subject_ref"`
	Status          string     `json:"status"`
	SourceVersion   int64      `json:"source_version"`
	ObservedAt      time.Time  `json:"observed_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	ReasonCode      string     `json:"reason_code,omitempty"`
	Remediation     string     `json:"remediation,omitempty"`
}

func scan(row pgx.Row) (view, error) {
	var v view
	err := row.Scan(&v.SourceRef, &v.ClientReference, &v.BusinessID, &v.Dimension, &v.SubjectType, &v.SubjectRef, &v.Status, &v.SourceVersion, &v.ObservedAt, &v.ExpiresAt, &v.ReasonCode, &v.Remediation)
	return v, err
}

const cols = `id::text,client_reference,business_id,dimension,subject_type,subject_ref,status,source_version,observed_at,expires_at,COALESCE(reason_code,''),COALESCE(remediation,'')`

func respond(c *gin.Context, status int, v any) { c.JSON(status, gin.H{"success": true, "data": v}) }

func (h *Handler) create(c *gin.Context) {
	if !h.enabled {
		c.JSON(503, gin.H{"error": gin.H{"code": "VERIFICATION_DISABLED"}})
		return
	}
	if caller(c) != "vendor-service" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var b createBody
	if c.ShouldBindJSON(&b) != nil || b.ClientReference == "" || b.BusinessID == "" || !dimension(b.Dimension) || !subject(b.SubjectType) || b.SubjectRef == "" || len(b.DocumentRefs) == 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	fp := hex.EncodeToString(sum[:])
	tx, e := h.db.Begin(c)
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	defer tx.Rollback(c)
	v, e := scan(tx.QueryRow(c, `INSERT INTO vendor_verification_cases(client_reference,business_id,dimension,subject_type,subject_ref,document_refs,request_fingerprint,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE vendor_verification_cases.request_fingerprint=EXCLUDED.request_fingerprint RETURNING `+cols, b.ClientReference, b.BusinessID, b.Dimension, b.SubjectType, b.SubjectRef, rawDocs(b.DocumentRefs), fp, key))
	if errors.Is(e, pgx.ErrNoRows) {
		c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
		return
	}
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	_, e = tx.Exec(c, `INSERT INTO vendor_verification_case_history(case_id,source_version,status,observed_at) VALUES($1,1,'PENDING_REVIEW',$2) ON CONFLICT DO NOTHING`, v.SourceRef, v.ObservedAt)
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	payload, _ := json.Marshal(v)
	_, e = tx.Exec(c, `INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload) VALUES($1,'vendor.verification.case_created',1,$2) ON CONFLICT DO NOTHING`, v.SourceRef, payload)
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	if tx.Commit(c) != nil {
		c.AbortWithStatus(500)
		return
	}
	respond(c, http.StatusCreated, v)
}
func rawDocs(v []string) []byte { b, _ := json.Marshal(v); return b }
func dimension(v string) bool {
	switch v {
	case "IDENTITY", "BUSINESS", "BANK", "LOCATION", "BRAND", "FOOD_KITCHEN":
		return true
	}
	return false
}
func subject(v string) bool {
	switch v {
	case "BUSINESS", "REPRESENTATIVE", "BRANCH", "BRAND", "CAPABILITY", "BRANCH_CAPABILITY":
		return true
	}
	return false
}
func decision(v string) bool {
	switch v {
	case "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED":
		return true
	}
	return false
}

func (h *Handler) read(c *gin.Context) {
	if caller(c) != "vendor-service" && caller(c) != "admin-backoffice-service" {
		c.AbortWithStatus(403)
		return
	}
	v, e := scan(h.db.QueryRow(c, `SELECT `+cols+` FROM vendor_verification_cases WHERE id::text=$1`, c.Param("caseRef")))
	if errors.Is(e, pgx.ErrNoRows) {
		c.AbortWithStatus(404)
		return
	}
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	respond(c, 200, v)
}
func (h *Handler) readByClient(c *gin.Context) {
	if caller(c) != "vendor-service" && caller(c) != "admin-backoffice-service" {
		c.AbortWithStatus(403)
		return
	}
	v, e := scan(h.db.QueryRow(c, `SELECT `+cols+` FROM vendor_verification_cases WHERE client_reference=$1`, c.Param("clientReference")))
	if errors.Is(e, pgx.ErrNoRows) {
		c.AbortWithStatus(404)
		return
	}
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	respond(c, 200, v)
}

func (h *Handler) decide(c *gin.Context) {
	if !h.enabled {
		c.AbortWithStatus(503)
		return
	}
	if caller(c) != "admin-backoffice-service" {
		c.AbortWithStatus(403)
		return
	}
	var b decisionBody
	if c.ShouldBindJSON(&b) != nil || !decision(b.Status) || b.ExpectedSourceVersion < 1 || (b.DecidedByStaffRef != "" && !staffRef.MatchString(b.DecidedByStaffRef)) {
		c.AbortWithStatus(400)
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" || len(key) > 200 {
		c.AbortWithStatus(400)
		return
	}
	// The fingerprint binds the exact case and body; reusing a key for a
	// different case or decision is an identity conflict, never a new decision.
	raw, _ := json.Marshal(struct {
		Case string       `json:"case"`
		Body decisionBody `json:"body"`
	}{c.Param("caseRef"), b})
	sum := sha256.Sum256(raw)
	fp := hex.EncodeToString(sum[:])
	tx, e := h.db.Begin(c)
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	defer tx.Rollback(c)
	// Serialize concurrent uses of one key so the second one observes the first.
	if _, e = tx.Exec(c, `SELECT pg_advisory_xact_lock(hashtextextended('vendor_case_decision:'||$1,0))`, key); e != nil {
		c.AbortWithStatus(500)
		return
	}
	var cached []byte
	var storedFP string
	e = tx.QueryRow(c, `SELECT request_fingerprint,response_body FROM verification_commands WHERE command_type='vendor_case_decision' AND idempotency_key=$1`, key).Scan(&storedFP, &cached)
	if e == nil {
		if storedFP != fp {
			c.JSON(409, gin.H{"error": gin.H{"code": "IDEMPOTENCY_CONFLICT"}})
			return
		}
		c.Data(200, "application/json", cached)
		return
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		c.AbortWithStatus(500)
		return
	}
	decidedBy, kind := caller(c), DecidedByService
	if b.DecidedByStaffRef != "" {
		decidedBy, kind = b.DecidedByStaffRef, DecidedByStaff
	}
	result, e := ApplyDecision(c, tx, DecisionInput{CaseRef: c.Param("caseRef"), ExpectedVersion: b.ExpectedSourceVersion, Status: b.Status, ReasonCode: b.ReasonCode, Remediation: b.Remediation, ExpiresAt: b.ExpiresAt, AppealRef: b.AppealRef, DecidedBy: decidedBy, DecidedByKind: kind, OperationRef: "idempotency:" + key, Now: time.Now().UTC()})
	if e != nil {
		decisionError(c, e)
		return
	}
	response, _ := json.Marshal(gin.H{"success": true, "data": result.Current})
	_, e = tx.Exec(c, `INSERT INTO verification_commands(command_type,idempotency_key,request_fingerprint,response_body) VALUES('vendor_case_decision',$1,$2,$3)`, key, fp, response)
	if e != nil {
		c.AbortWithStatus(500)
		return
	}
	if tx.Commit(c) != nil {
		c.AbortWithStatus(500)
		return
	}
	c.Data(200, "application/json", response)
}

var staffRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)

// decisionError maps the native state machine and fence onto stable codes.
func decisionError(c *gin.Context, e error) {
	code, status := "", 500
	switch {
	case errors.Is(e, ErrCaseNotFound):
		code, status = "CASE_NOT_FOUND", 404
	case errors.Is(e, ErrVersionConflict):
		code, status = "VERSION_CONFLICT", 409
	case errors.Is(e, ErrIllegalTransition):
		code, status = "ILLEGAL_TRANSITION", 409
	case errors.Is(e, ErrAppealReused):
		code, status = "APPEAL_ALREADY_DECIDED", 409
	case errors.Is(e, ErrReasonRequired):
		code, status = "REASON_REQUIRED", 400
	case errors.Is(e, ErrInvalidReason):
		code, status = "INVALID_REASON_CODE", 400
	case errors.Is(e, ErrFutureExpiryRequired):
		code, status = "FUTURE_EXPIRY_REQUIRED", 400
	case errors.Is(e, ErrAppealRequired):
		code, status = "APPEAL_REQUIRED", 400
	case errors.Is(e, ErrAppealNotApplicable):
		code, status = "APPEAL_NOT_APPLICABLE", 400
	}
	if code == "" {
		c.AbortWithStatus(status)
		return
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}
