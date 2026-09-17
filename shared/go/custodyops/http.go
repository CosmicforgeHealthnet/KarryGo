package custodyops

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct {
	repo    *Repository
	media   MediaAuthorizer
	policy  Policy
	allowed map[string]struct{}
}

func RegisterProvider(group *gin.RouterGroup, repo *Repository, signer *sharedauth.TokenSigner, role, service string) {
	if group == nil || repo == nil || signer == nil {
		return
	}
	h := &Handler{repo: repo}
	g := group.Group("/provider/custody")
	g.Use(sharedauth.BearerMiddleware(signer, role, service))
	g.POST("/missions/:missionRef/incidents", h.providerIncident)
}

func (h *Handler) providerIncident(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		failHTTP(c, ErrUnauthorized)
		return
	}
	var command IncidentCommand
	if !bindJSON(c, &command) {
		return
	}
	command.MissionRef = strings.TrimSpace(c.Param("missionRef"))
	command.ProviderRef = claims.Subject
	result, err := h.repo.ReportIncident(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}

func RegisterInternal(group *gin.RouterGroup, repo *Repository, verifier *serviceauth.Verifier, media MediaAuthorizer, policy Policy, allowedCallers []string) {
	if group == nil || repo == nil || verifier == nil {
		return
	}
	h := &Handler{repo: repo, media: media, policy: policy, allowed: map[string]struct{}{}}
	for _, caller := range allowedCallers {
		if caller = strings.TrimSpace(caller); caller != "" {
			h.allowed[caller] = struct{}{}
		}
	}
	g := group.Group("/internal/custody/v1")
	g.Use(serviceauth.Middleware(verifier), h.authorize())
	g.POST("/manifests/freeze", h.freezeManifest)
	g.POST("/manifests/confirm", h.confirmParcels)
	g.POST("/evidence", h.recordEvidence)
	g.POST("/exceptions", h.reportException)
	g.POST("/returns", h.startReturn)
	g.POST("/returns/complete", h.completeReturn)
	g.POST("/incidents", h.reportIncident)
	g.POST("/replacements", h.assignReplacement)
	g.POST("/transfers", h.completeTransfer)
	g.POST("/earning-allocations", h.finalizeAllocation)
	g.POST("/missions/register", h.registerMission)
	g.GET("/missions/:missionRef", h.readMissionCustody)
	g.POST("/manifests/collect", h.collectManifests)
	g.POST("/manifests/replacements", h.assignManifestReplacement)
	g.POST("/manifests/transfers", h.transferManifests)
	g.POST("/manifests/deliver", h.deliverManifests)
	g.POST("/manifests/exceptions", h.reportManifestException)
}

func (h *Handler) authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
		caller := c.GetString("service_name")
		if _, ok := h.allowed[caller]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "CUSTODY_CALLER_FORBIDDEN"}})
			return
		}
		nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
		if nonce == "" || h.repo.claimNonce(c, caller, nonce, c.Request.URL.Path, time.Now().Add(10*time.Minute)) != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "CUSTODY_REQUEST_REPLAYED"}})
			return
		}
		c.Next()
	}
}

func bindJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	if c.ShouldBindJSON(target) != nil {
		failHTTP(c, ErrInvalid)
		return false
	}
	return true
}
func (h *Handler) freezeManifest(c *gin.Context) {
	var command FreezeManifestCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.FreezeManifest(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) confirmParcels(c *gin.Context) {
	var command ConfirmParcelsCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.ConfirmParcels(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) recordEvidence(c *gin.Context) {
	var command EvidenceCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.RecordEvidence(c, command, h.media)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) reportException(c *gin.Context) {
	var command ExceptionCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.ReportException(c, command, h.policy)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) startReturn(c *gin.Context) {
	var command ReturnCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.StartReturn(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) completeReturn(c *gin.Context) {
	var command ReturnCompletionCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.CompleteReturn(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) reportIncident(c *gin.Context) {
	var command IncidentCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.ReportIncident(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) assignReplacement(c *gin.Context) {
	var command AssignReplacementCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.AssignReplacement(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) completeTransfer(c *gin.Context) {
	var command TransferCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.CompleteTransfer(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) finalizeAllocation(c *gin.Context) {
	var command AllocationCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.FinalizeAllocation(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) registerMission(c *gin.Context) {
	var command RegisterMissionCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.RegisterMission(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) readMissionCustody(c *gin.Context) {
	result, err := h.repo.ReadMissionCustody(c, c.Param("missionRef"), c.Query("tenant_ref"))
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) collectManifests(c *gin.Context) {
	var command CollectManifestsCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.CollectManifests(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) assignManifestReplacement(c *gin.Context) {
	var command AssignReplacementCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.AssignManifestReplacement(c, command)
	respondHTTP(c, http.StatusCreated, result, err)
}
func (h *Handler) transferManifests(c *gin.Context) {
	var command TransferManifestsCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.TransferManifests(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) deliverManifests(c *gin.Context) {
	var command DeliverManifestsCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.DeliverManifests(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func (h *Handler) reportManifestException(c *gin.Context) {
	var command ManifestExceptionCommand
	if !bindJSON(c, &command) {
		return
	}
	result, err := h.repo.ReportManifestException(c, command)
	respondHTTP(c, http.StatusOK, result, err)
}
func respondHTTP(c *gin.Context, status int, result any, err error) {
	if err != nil {
		failHTTP(c, err)
		return
	}
	c.JSON(status, gin.H{"success": true, "data": result})
}
func failHTTP(c *gin.Context, err error) {
	status, code := http.StatusConflict, "CUSTODY_STATE_CONFLICT"
	switch {
	case errors.Is(err, ErrInvalid):
		status, code = http.StatusBadRequest, "CUSTODY_REQUEST_INVALID"
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, "CUSTODY_NOT_FOUND"
	case errors.Is(err, ErrUnauthorized):
		status, code = http.StatusForbidden, "CUSTODY_SCOPE_FORBIDDEN"
	case errors.Is(err, ErrStaleSourceVersion):
		status, code = http.StatusConflict, "CUSTODY_SOURCE_VERSION_STALE"
	case errors.Is(err, ErrManifestsUnaccounted):
		status, code = http.StatusConflict, "CUSTODY_MANIFESTS_UNACCOUNTED"
	case errors.Is(err, ErrPolicy):
		status, code = http.StatusUnprocessableEntity, "CUSTODY_RECOVERY_NOT_ALLOWED"
	case errors.Is(err, ErrMediaRejected):
		status, code = http.StatusUnprocessableEntity, "CUSTODY_MEDIA_REJECTED"
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}
