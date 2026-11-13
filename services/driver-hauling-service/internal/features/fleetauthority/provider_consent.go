package fleetauthority

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/orgprovider"
)

// The provider's own consent to an organization association (FLEET-10.2).
//
// Fleet requests an association (REQUESTED) through the private surface, but
// only the truck provider can accept or decline it, from their own app and
// with their own session. Fleet reads the decision back and only then moves
// the organization membership out of PENDING_PROVIDER_ACCEPTANCE. The
// organization-facing transition route refuses REQUESTED -> ACTIVE/REJECTED
// for the same reason: a company cannot consent on a provider's behalf.

// RegisterProviderConsent mounts the provider routes behind auth.
func RegisterProviderConsent(group *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	if group == nil || h == nil || len(auth) == 0 {
		return
	}
	group.Use(auth...)
	group.GET("", h.listProviderAssociations)
	group.POST("/:linkId/accept", func(c *gin.Context) { h.decideAssociation(c, true) })
	group.POST("/:linkId/decline", func(c *gin.Context) { h.decideAssociation(c, false) })
}

func providerSubject(c *gin.Context) string {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		return ""
	}
	return claims.Subject
}

func (h *Handler) listProviderAssociations(c *gin.Context) {
	providerID := providerSubject(c)
	if !validIDs(providerID) {
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
		return
	}
	rows, err := h.db.Query(c, `SELECT link_id::text, organization_id, state, version, created_at, decided_at
		FROM hauling_organization_provider_links WHERE provider_id=$1 ORDER BY created_at DESC LIMIT 50`, providerID)
	if err != nil {
		dbError(c, err)
		return
	}
	defer rows.Close()
	out := []orgprovider.ProviderAssociationView{}
	for rows.Next() {
		var v orgprovider.ProviderAssociationView
		if err := rows.Scan(&v.LinkID, &v.OrganizationID, &v.Status, &v.SourceVersion, &v.RequestedAt, &v.DecidedAt); err != nil {
			dbError(c, err)
			return
		}
		out = append(out, v)
	}
	if rows.Err() != nil {
		dbError(c, rows.Err())
		return
	}
	data(c, 200, gin.H{"associations": out})
}

var (
	errDecisionNotAvailable = errors.New("association is not awaiting this provider")
	errStaleDecision        = errors.New("stale association version")
)

func (h *Handler) decideAssociation(c *gin.Context, accept bool) {
	providerID := providerSubject(c)
	linkID := c.Param("linkId")
	if !validIDs(providerID) {
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
		return
	}
	if _, err := uuid.Parse(linkID); err != nil {
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
		return
	}
	var body orgprovider.ProviderDecisionRequest
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&body) != nil {
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
		return
	}
	target := orgprovider.ProviderDecision(accept)
	var out orgprovider.ProviderAssociationView
	err := pgx.BeginFunc(c, h.db, func(tx pgx.Tx) error {
		var view linkView
		var createdAt time.Time
		var decidedAt *time.Time
		err := tx.QueryRow(c, `SELECT link_id::text, organization_id, COALESCE(branch_id,''), membership_id, provider_id::text,
				state, version, created_at, decided_at
			FROM hauling_organization_provider_links WHERE link_id=$1 AND provider_id=$2 FOR UPDATE`, linkID, providerID).
			Scan(&view.LinkID, &view.OrganizationID, &view.BranchID, &view.MembershipID, &view.ProviderID,
				&view.Status, &view.SourceVersion, &createdAt, &decidedAt)
		if err != nil {
			return err
		}
		out = orgprovider.ProviderAssociationView{LinkID: view.LinkID, OrganizationID: view.OrganizationID,
			Status: view.Status, SourceVersion: view.SourceVersion, RequestedAt: createdAt, DecidedAt: decidedAt}
		if view.Status == target && decidedAt != nil {
			return nil // the same decision again is answered, not repeated
		}
		if view.Status != orgprovider.StateRequested {
			return errDecisionNotAvailable
		}
		if body.ExpectedVersion > 0 && body.ExpectedVersion != view.SourceVersion {
			return errStaleDecision
		}
		prior := view.Status
		if err := tx.QueryRow(c, `UPDATE hauling_organization_provider_links
			SET state=$2, decided_at=now(), version=version+1, updated_at=now()
			WHERE link_id=$1 RETURNING state, version, decided_at`, linkID, target).
			Scan(&view.Status, &view.SourceVersion, &out.DecidedAt); err != nil {
			return err
		}
		out.Status, out.SourceVersion = view.Status, view.SourceVersion
		reason := "provider_accepted"
		if !accept {
			reason = "provider_declined"
		}
		if _, err := tx.Exec(c, `INSERT INTO hauling_organization_link_history
			(link_type, link_id, organization_id, branch_id, prior_state, new_state, source_version,
			 actor_service, actor_type, actor_id, reason_code, correlation_id, causation_id)
			VALUES ('PROVIDER', $1, $2, NULLIF($3,''), $4, $5, $6, 'driver-hauling-service', 'PROVIDER', $7, $8, '', '')`,
			view.LinkID, view.OrganizationID, view.BranchID, prior, view.Status, view.SourceVersion, providerID, reason); err != nil {
			return err
		}
		return h.event(c, tx, "PROVIDER_LINK", view.LinkID, "hauling.provider_link."+map[bool]string{true: "active", false: "rejected"}[accept]+".v1",
			view.SourceVersion, map[string]any{"link": view, "reasonCode": reason, "decidedBy": "PROVIDER"}, c.GetHeader("X-Correlation-ID"))
	})
	switch {
	case err == nil:
		data(c, 200, out)
	case errors.Is(err, pgx.ErrNoRows):
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
	case errors.Is(err, errDecisionNotAvailable), errors.Is(err, errStaleDecision):
		c.JSON(409, gin.H{"error": gin.H{"code": orgprovider.CodeInvalidStateVersion}})
	default:
		dbError(c, err)
	}
}
