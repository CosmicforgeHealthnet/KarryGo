package organizations

import (
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	db      *pgxpool.Pool
	enabled bool
}

func New(db *pgxpool.Pool, enabled bool) *Handler { return &Handler{db: db, enabled: enabled} }
func Register(g *gin.RouterGroup, h *Handler, v *serviceauth.Verifier, allowed []string) {
	r := g.Group("/internal/organizations")
	r.Use(serviceauth.Middleware(v), h.allow(allowed), h.nonce())
	r.POST("/submissions", h.submit)
	r.GET("/:organizationId/submissions/:submissionId", h.read)
	r.POST("/:organizationId/submissions/:submissionId/decisions", h.decide)
}
func (h *Handler) allow(names []string) gin.HandlerFunc {
	set := map[string]bool{}
	for _, n := range names {
		set[strings.TrimSpace(n)] = true
	}
	return func(c *gin.Context) {
		raw, _ := c.Get("service_name")
		name, _ := raw.(string)
		if !set[name] {
			c.AbortWithStatus(403)
			return
		}
		c.Next()
	}
}
func (h *Handler) nonce() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, _ := c.Get("service_name")
		n := c.GetHeader(serviceauth.HeaderNonce)
		if n == "" {
			c.AbortWithStatus(401)
			return
		}
		tag, err := h.db.Exec(c, `INSERT INTO verification_service_nonces(caller,nonce,expires_at)VALUES($1,$2,now()+interval '10 minutes')ON CONFLICT DO NOTHING`, raw, n)
		if err != nil || tag.RowsAffected() != 1 {
			c.AbortWithStatus(401)
			return
		}
		c.Next()
	}
}

type submitBody struct {
	OrganizationID  string         `json:"organizationId"`
	SubmissionID    string         `json:"submissionId"`
	Snapshot        map[string]any `json:"snapshot"`
	DocumentFileIDs []string       `json:"documentFileIds"`
}
type view struct {
	ExternalReference string           `json:"externalReference"`
	Status            string           `json:"status"`
	RestrictionStatus string           `json:"restrictionStatus"`
	SourceVersion     int64            `json:"sourceVersion"`
	Issues            []map[string]any `json:"issues"`
}

func (h *Handler) submit(c *gin.Context) {
	if !h.enabled {
		c.JSON(503, gin.H{"error": gin.H{"code": "VERIFICATION_DISABLED"}})
		return
	}
	var b submitBody
	if c.ShouldBindJSON(&b) != nil || b.OrganizationID == "" || b.SubmissionID == "" {
		c.AbortWithStatus(400)
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		c.AbortWithStatus(400)
		return
	}
	encoded, _ := json.Marshal(b)
	sum := sha256.Sum256(encoded)
	fp := hex.EncodeToString(sum[:])
	tx, err := h.db.Begin(c)
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	defer tx.Rollback(c)
	var v view
	var oldfp string
	err = tx.QueryRow(c, `INSERT INTO organization_verification_submissions(organization_id,submission_id,idempotency_key,request_fingerprint,snapshot,document_file_ids)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(idempotency_key)DO UPDATE SET idempotency_key=excluded.idempotency_key RETURNING id::text,status,restriction_status,source_version,issues,request_fingerprint`, b.OrganizationID, b.SubmissionID, key, fp, b.Snapshot, b.DocumentFileIDs).Scan(&v.ExternalReference, &v.Status, &v.RestrictionStatus, &v.SourceVersion, &v.Issues, &oldfp)
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	if oldfp != fp {
		c.AbortWithStatus(409)
		return
	}
	payload, _ := json.Marshal(v)
	_, err = tx.Exec(c, `INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload)VALUES($1,'organization.verification.submitted.v1',$2,$3)ON CONFLICT DO NOTHING`, b.OrganizationID, v.SourceVersion, payload)
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	if tx.Commit(c) != nil {
		c.AbortWithStatus(500)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": v})
}
func (h *Handler) read(c *gin.Context) {
	var v view
	err := h.db.QueryRow(c, `SELECT id::text,status,restriction_status,source_version,issues FROM organization_verification_submissions WHERE organization_id=$1 AND submission_id=$2`, c.Param("organizationId"), c.Param("submissionId")).Scan(&v.ExternalReference, &v.Status, &v.RestrictionStatus, &v.SourceVersion, &v.Issues)
	if err == pgx.ErrNoRows {
		c.AbortWithStatus(404)
		return
	}
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	c.JSON(200, gin.H{"data": v, "observedAt": time.Now().UTC()})
}

type decisionBody struct {
	Status            string           `json:"status"`
	RestrictionStatus string           `json:"restrictionStatus"`
	ExpectedVersion   int64            `json:"expectedVersion"`
	Issues            []map[string]any `json:"issues"`
}

func (h *Handler) decide(c *gin.Context) {
	if !h.enabled {
		c.JSON(503, gin.H{"error": gin.H{"code": "VERIFICATION_DISABLED"}})
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	var body decisionBody
	if key == "" || c.ShouldBindJSON(&body) != nil || body.ExpectedVersion < 1 {
		c.AbortWithStatus(400)
		return
	}
	body.Status = strings.ToUpper(strings.TrimSpace(body.Status))
	body.RestrictionStatus = strings.ToUpper(strings.TrimSpace(body.RestrictionStatus))
	if !map[string]bool{"PENDING": true, "VERIFIED": true, "REJECTED": true, "SUSPENDED": true}[body.Status] ||
		!map[string]bool{"NONE": true, "RESTRICTED": true, "SUSPENDED": true}[body.RestrictionStatus] {
		c.AbortWithStatus(422)
		return
	}
	encoded, _ := json.Marshal(map[string]any{
		"organizationId": c.Param("organizationId"),
		"submissionId":   c.Param("submissionId"),
		"decision":       body,
	})
	sum := sha256.Sum256(encoded)
	fp := hex.EncodeToString(sum[:])
	var priorRaw []byte
	var priorFingerprint string
	err := h.db.QueryRow(c, `SELECT response_body,request_fingerprint FROM verification_commands WHERE command_type='DECIDE_ORGANIZATION' AND idempotency_key=$1`, key).Scan(&priorRaw, &priorFingerprint)
	if err == nil {
		if priorFingerprint != fp {
			c.AbortWithStatus(409)
			return
		}
		var prior view
		if json.Unmarshal(priorRaw, &prior) != nil {
			c.AbortWithStatus(500)
			return
		}
		c.JSON(200, gin.H{"data": prior})
		return
	}
	tx, err := h.db.Begin(c)
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	defer tx.Rollback(c)
	var result view
	err = tx.QueryRow(c, `UPDATE organization_verification_submissions SET status=$3,restriction_status=$4,issues=$5,source_version=source_version+1,decided_at=now(),updated_at=now() WHERE organization_id=$1 AND submission_id=$2 AND source_version=$6 RETURNING id::text,status,restriction_status,source_version,issues`, c.Param("organizationId"), c.Param("submissionId"), body.Status, body.RestrictionStatus, body.Issues, body.ExpectedVersion).Scan(&result.ExternalReference, &result.Status, &result.RestrictionStatus, &result.SourceVersion, &result.Issues)
	if errors.Is(err, pgx.ErrNoRows) {
		c.AbortWithStatus(409)
		return
	}
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	response, _ := json.Marshal(result)
	_, err = tx.Exec(c, `INSERT INTO verification_commands(command_type,idempotency_key,request_fingerprint,response_body) VALUES('DECIDE_ORGANIZATION',$1,$2,$3)`, key, fp, response)
	if err == nil {
		_, err = tx.Exec(c, `INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload) VALUES($1,'organization.verification.status_changed.v1',$2,$3)`, c.Param("organizationId"), result.SourceVersion, response)
	}
	if err == nil {
		err = tx.Commit(c)
	}
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	c.JSON(200, gin.H{"data": result})
}
