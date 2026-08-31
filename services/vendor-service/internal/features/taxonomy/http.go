package taxonomy

// Section 13I vendor-facing taxonomy API.
//
// This surface answers one question for a vendor: "how is MY catalogue
// classified, and how do I change that classification". It is deliberately
// small. Everything global — creating facets, editing nodes, publishing a
// version, governing the vocabulary — lives on the root-mounted internal
// surface in internal_http.go, because those actions decide how every vendor
// on the platform is discovered.
//
// The isolation chain every mutating and every source-addressed read walks is:
//
//	session -> staff -> vendor/business -> permission scope -> branch -> source resource
//
// The last hop is the one that matters and the one a handler is tempted to
// skip: business_ref and branch_ref are never taken from the request. They are
// read back from the owning catalogue table by VendorAccess.ResolveSource, so
// guessing another vendor's source_ref returns "not found" rather than their
// classification.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Flags gate the Section 13 rollout. Reads and writes are separable so the
// classification surface can go out read-only first.
type Flags struct {
	TaxonomyEnabled bool
	WritesEnabled   bool
}

// Handler serves the authenticated vendor taxonomy surface.
type Handler struct {
	service *Service
	access  *VendorAccess
	flags   Flags
}

func NewHandler(service *Service, access *VendorAccess, flags Flags) *Handler {
	return &Handler{service: service, access: access, flags: flags}
}

// respondError maps this package's sentinel errors onto the platform error
// envelope.
//
// ErrNamespaceMismatch deliberately becomes 404, not 422: a caller who
// addresses a SHOP identifier through the FOOD path learns only that FOOD has
// no such record. Telling them "that exists, but in the other namespace" is a
// cross-namespace enumeration oracle.
func respondError(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrForbidden):
		httpx.RespondError(c, apperrors.Forbidden("You do not have permission to do this.", nil))
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNamespaceMismatch):
		httpx.RespondError(c, apperrors.NotFound("Taxonomy record not found.", nil))
	case errors.Is(err, ErrInvalidArgument):
		httpx.RespondError(c, apperrors.BadRequest("Request is invalid.", err))
	case errors.Is(err, ErrNotDraft), errors.Is(err, ErrVersionMismatch),
		errors.Is(err, ErrConflict), errors.Is(err, ErrIdempotency):
		httpx.RespondError(c, apperrors.Conflict("This taxonomy record changed or is not in a state that allows this.", err))
	default:
		httpx.RespondError(c, apperrors.Internal("Taxonomy request failed.", err))
	}
}

func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

// decodeStrict reads a bounded body and refuses unknown fields.
//
// Rejecting unknown fields is a security property here rather than a
// tidiness one: it is what turns an attempt to smuggle business_ref or
// branch_ref into a classification command into a 400, instead of a field
// that is quietly ignored and leaves the caller believing it took effect.
func decodeStrict(c *gin.Context, target any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return apperrors.BadRequest("Could not read request body.", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	return nil
}

// staffActor builds the taxonomy Actor from the resolved session. ActorRef is
// the staff id, never anything the caller supplied.
func (h *Handler) staffActor(c *gin.Context) (authctx.StaffContext, Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok || staff.StaffID == "" || staff.VendorID == "" {
		return authctx.StaffContext{}, Actor{}, ErrForbidden
	}
	commandRef := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	return staff, Actor{
		ActorRef:      "vendor_staff:" + staff.StaffID,
		CommandRef:    commandRef,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
	}, nil
}

// namespaceParam reads and validates the namespace from the path.
func namespaceParam(c *gin.Context) (NamespaceCode, error) {
	code, ok := normalizeNamespaceCode(c.Param("namespaceCode"))
	if !ok {
		return "", ErrNotFound
	}
	return code, nil
}

// authorizedSource is the whole isolation chain in one call: permission,
// namespace, and authoritative ownership of the addressed source resource.
func (h *Handler) authorizedSource(
	c *gin.Context,
	permission string,
	sourceType string,
	sourceRef string,
) (NamespaceCode, Actor, SourceOwnership, bool) {
	namespace, err := namespaceParam(c)
	if err != nil {
		respondError(c, err)
		return "", Actor{}, SourceOwnership{}, false
	}

	staff, actor, err := h.staffActor(c)
	if err != nil {
		respondError(c, err)
		return "", Actor{}, SourceOwnership{}, false
	}

	scope, err := h.access.Authorize(c, staff.StaffID, permission)
	if err != nil {
		respondError(c, err)
		return "", Actor{}, SourceOwnership{}, false
	}

	ownership, err := h.access.ResolveSource(
		c, namespace, sourceType, sourceRef, staff.VendorID, scope,
	)
	if err != nil {
		respondError(c, err)
		return "", Actor{}, SourceOwnership{}, false
	}

	return namespace, actor, ownership, true
}

// authorizedRead covers the reads that address no source resource: the
// published version, the facet/node browse and the claim catalogue. They are
// global-but-published information a vendor needs in order to classify at all.
func (h *Handler) authorizedRead(c *gin.Context, permission string) (NamespaceCode, bool) {
	namespace, err := namespaceParam(c)
	if err != nil {
		respondError(c, err)
		return "", false
	}
	staff, _, err := h.staffActor(c)
	if err != nil {
		respondError(c, err)
		return "", false
	}
	if _, err = h.access.Authorize(c, staff.StaffID, permission); err != nil {
		respondError(c, err)
		return "", false
	}
	return namespace, true
}

// sourceQuery reads the source coordinates a classification read addresses.
func sourceQuery(c *gin.Context) (string, string) {
	return strings.TrimSpace(c.Query("source_type")), strings.TrimSpace(c.Query("source_ref"))
}

// ---------------------------------------------------------------------------
// Published taxonomy reads
// ---------------------------------------------------------------------------

func (h *Handler) publishedVersion(c *gin.Context) {
	namespace, ok := h.authorizedRead(c, PermissionTaxonomyRead)
	if !ok {
		return
	}
	version, err := h.service.GetPublishedVersion(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"namespace_code": namespace, "version": version})
}

func (h *Handler) listFacets(c *gin.Context) {
	namespace, ok := h.authorizedRead(c, PermissionTaxonomyRead)
	if !ok {
		return
	}
	version, err := h.service.GetPublishedVersion(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	facets, err := h.service.ListFacets(c, version.ID)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code": namespace,
		"version_id":     version.ID,
		"facets":         facets,
	})
}

func (h *Handler) listNodes(c *gin.Context) {
	namespace, ok := h.authorizedRead(c, PermissionTaxonomyRead)
	if !ok {
		return
	}
	version, err := h.service.GetPublishedVersion(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}

	var (
		nodes []Node
	)
	if facetID := strings.TrimSpace(c.Query("facet_id")); facetID != "" {
		nodes, err = h.service.ListNodesByFacet(c, version.ID, facetID)
	} else {
		nodes, err = h.service.ListNodes(c, version.ID)
	}
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code": namespace,
		"version_id":     version.ID,
		"nodes":          nodes,
	})
}

func (h *Handler) getNode(c *gin.Context) {
	namespace, ok := h.authorizedRead(c, PermissionTaxonomyRead)
	if !ok {
		return
	}
	version, err := h.service.GetPublishedVersion(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	node, err := h.service.GetNode(c, c.Param("nodeId"))
	if err != nil {
		respondError(c, err)
		return
	}
	// A node id from another namespace or another version is not visible
	// here, whatever the caller guessed.
	if node.VersionID != version.ID {
		respondError(c, ErrNotFound)
		return
	}
	terms, err := h.service.ListNodeTerms(c, namespace, version.ID, node.ID)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"node": node, "terms": terms})
}

func (h *Handler) listDietaryClaims(c *gin.Context) {
	namespace, ok := h.authorizedRead(c, PermissionTaxonomyRead)
	if !ok {
		return
	}
	claims, err := h.service.ListDietaryClaims(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"namespace_code": namespace, "dietary_claims": claims})
}

// ---------------------------------------------------------------------------
// Vendor classification reads
// ---------------------------------------------------------------------------

func (h *Handler) listMappings(c *gin.Context) {
	sourceType, sourceRef := sourceQuery(c)
	namespace, _, ownership, ok := h.authorizedSource(c, PermissionTaxonomyRead, sourceType, sourceRef)
	if !ok {
		return
	}
	mappings, err := h.service.ListMappingsForOwnedSource(c, namespace, ownership)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code": namespace,
		"source_type":    ownership.SourceType,
		"source_ref":     ownership.SourceRef,
		"mappings":       mappings,
	})
}

func (h *Handler) listTagAssignments(c *gin.Context) {
	sourceType, sourceRef := sourceQuery(c)
	namespace, _, ownership, ok := h.authorizedSource(c, PermissionTaxonomyRead, sourceType, sourceRef)
	if !ok {
		return
	}
	assignments, err := h.service.ListTagAssignmentsForOwnedSource(c, namespace, ownership)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code":  namespace,
		"source_type":     ownership.SourceType,
		"source_ref":      ownership.SourceRef,
		"tag_assignments": assignments,
	})
}

func (h *Handler) listDietaryClaimAssignments(c *gin.Context) {
	sourceType, sourceRef := sourceQuery(c)
	namespace, _, ownership, ok := h.authorizedSource(c, PermissionTaxonomyRead, sourceType, sourceRef)
	if !ok {
		return
	}
	assignments, err := h.service.ListDietaryClaimAssignmentsForOwnedSource(c, namespace, ownership)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code":            namespace,
		"source_type":               ownership.SourceType,
		"source_ref":                ownership.SourceRef,
		"dietary_claim_assignments": assignments,
	})
}

func (h *Handler) getClassification(c *gin.Context) {
	sourceType, sourceRef := sourceQuery(c)
	namespace, _, ownership, ok := h.authorizedSource(c, PermissionTaxonomyRead, sourceType, sourceRef)
	if !ok {
		return
	}

	mappings, err := h.service.ListMappingsForOwnedSource(c, namespace, ownership)
	if err != nil {
		respondError(c, err)
		return
	}
	tags, err := h.service.ListTagAssignmentsForOwnedSource(c, namespace, ownership)
	if err != nil {
		respondError(c, err)
		return
	}
	claims, err := h.service.ListDietaryClaimAssignmentsForOwnedSource(c, namespace, ownership)
	if err != nil {
		respondError(c, err)
		return
	}

	respond(c, http.StatusOK, VendorSourceClassification{
		NamespaceCode:   namespace,
		SourceType:      ownership.SourceType,
		SourceRef:       ownership.SourceRef,
		BusinessRef:     ownership.BusinessRef,
		BranchRef:       ownership.BranchRef,
		Mappings:        mappings,
		TagAssignments:  tags,
		DietaryTagsView: claims,
	})
}

// ---------------------------------------------------------------------------
// Vendor classification command
// ---------------------------------------------------------------------------

// replaceMappingsRequest is the vendor-facing shape of the Section 13C
// facet-mapping replacement command.
//
// It has no business_ref or branch_ref field on purpose. Those are the two
// fields a caller would use to write into another tenant's scope, and the
// server fills them from the resolved ownership of the source resource
// instead. DisallowUnknownFields turns an attempt to send them into a 400
// rather than a silently ignored field.
type replaceMappingsRequest struct {
	FacetCode  string             `json:"facet_code"`
	SourceType string             `json:"source_type"`
	SourceRef  string             `json:"source_ref"`
	Mappings   []MappingCandidate `json:"mappings"`
}

func (h *Handler) replaceMappings(c *gin.Context) {
	if !h.flags.WritesEnabled {
		httpx.RespondError(c, apperrors.Unavailable("Taxonomy classification changes are temporarily unavailable.", nil))
		return
	}

	var request replaceMappingsRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}

	namespace, actor, ownership, ok := h.authorizedSource(
		c, PermissionTaxonomyMappingManage, request.SourceType, request.SourceRef,
	)
	if !ok {
		return
	}

	if actor.CommandRef == "" {
		httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", nil))
		return
	}

	businessRef := ownership.BusinessRef
	input := ReplaceFacetMappingsInput{
		NamespaceCode: namespace,
		FacetCode:     request.FacetCode,
		SourceType:    ownership.SourceType,
		SourceRef:     ownership.SourceRef,
		BusinessRef:   &businessRef,
		Mappings:      request.Mappings,
		Actor:         actor,
	}
	if ownership.BranchRef != "" {
		branchRef := ownership.BranchRef
		input.BranchRef = &branchRef
	}

	result, err := h.service.ReplaceFacetMappings(c, input)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterRoutes mounts the vendor taxonomy surface under the authenticated
// vendor base. The group passed in already carries RequireAuth.
func RegisterRoutes(group *gin.RouterGroup, h *Handler, flags Flags) {
	if h == nil || h.service == nil || h.access == nil || !flags.TaxonomyEnabled {
		return
	}

	root := group.Group("/taxonomy/namespaces/:namespaceCode")

	root.GET("/published-version", h.publishedVersion)
	root.GET("/facets", h.listFacets)
	root.GET("/nodes", h.listNodes)
	root.GET("/nodes/:nodeId", h.getNode)
	root.GET("/dietary-claims", h.listDietaryClaims)

	root.GET("/classifications", h.getClassification)
	root.GET("/mappings", h.listMappings)
	root.GET("/tag-assignments", h.listTagAssignments)
	root.GET("/dietary-claim-assignments", h.listDietaryClaimAssignments)

	// The route is always mounted; the writes flag is enforced inside the
	// handler, after authorization, so a disabled rollout reads as
	// "temporarily unavailable" and never as "you lack permission".
	root.PUT("/mappings", h.replaceMappings)
}
