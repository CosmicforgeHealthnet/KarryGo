package taxonomy

// Section 13I platform taxonomy administration API.
//
// This is the surface that decides how EVERY vendor on the platform is
// classified and discovered. Creating a facet, editing a node, publishing a
// version or governing the vocabulary is a platform act, not a vendor act, so
// none of it is reachable with a vendor bearer token at any permission level:
// it is mounted at the engine root under /internal/vendor/v1/taxonomy and
// authenticated with the certified serviceauth HMAC-v2 contract plus an
// explicit caller allowlist and durable nonce replay protection — the same
// arrangement Sections 8I, 9C and 12B already use for internal authority.
//
// The separation is structural rather than conventional. There is no vendor
// role that grants vendor.taxonomy.platform.admin (migration 000059 grants it
// to nobody), and there is no bearer route here to grant it on. A vendor Owner
// is the highest authority inside their own business and no authority at all
// over the platform vocabulary.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// NonceStore records a caller's nonce exactly once, so a captured signed
// request cannot be replayed inside its timestamp window. It is a port over
// the existing vendor_service_nonces table rather than a new mechanism.
type NonceStore interface {
	Consume(ctx context.Context, caller string, nonce string) error
}

// InternalHandler serves the platform taxonomy administration surface.
type InternalHandler struct {
	service *Service
	allowed map[string]bool
	nonces  NonceStore
	flags   Flags
}

func NewInternalHandler(
	service *Service,
	callers []string,
	nonces NonceStore,
	flags Flags,
) *InternalHandler {
	allowed := map[string]bool{}
	for _, caller := range callers {
		if caller = strings.TrimSpace(caller); caller != "" {
			allowed[caller] = true
		}
	}
	return &InternalHandler{service: service, allowed: allowed, nonces: nonces, flags: flags}
}

// allowCaller is the second gate after HMAC verification. Holding a valid
// configured secret proves identity; it does not confer platform taxonomy
// authority. Only the callers named in the allowlist may administer the
// global vocabulary.
func (h *InternalHandler) allowCaller(c *gin.Context) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	if !h.allowed[caller] {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}

// requireNonce consumes the signed nonce exactly once.
func (h *InternalHandler) requireNonce(c *gin.Context) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if nonce == "" || h.nonces == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.nonces.Consume(c, caller, nonce); err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

// requireWrites gates every mutation on the rollout flag, after
// authentication, so a disabled rollout never reads as an auth failure.
func (h *InternalHandler) requireWrites(c *gin.Context) bool {
	if !h.flags.WritesEnabled {
		httpx.RespondError(c, apperrors.Unavailable("Taxonomy administration is temporarily unavailable.", nil))
		return false
	}
	return true
}

// platformActor identifies the calling service, and the caller's own command
// reference for idempotent replay. The actor is the verified service name;
// nothing here trusts an actor supplied in a body.
func platformActor(c *gin.Context) (Actor, error) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	if caller == "" {
		return Actor{}, ErrForbidden
	}
	commandRef := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if commandRef == "" {
		return Actor{}, ErrInvalidArgument
	}
	return Actor{
		ActorRef:      "service:" + caller,
		CommandRef:    commandRef,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
	}, nil
}

// internalNamespace resolves and validates the addressed namespace.
func internalNamespace(c *gin.Context) (NamespaceCode, bool) {
	code, ok := normalizeNamespaceCode(c.Param("namespaceCode"))
	if !ok {
		respondError(c, ErrNotFound)
		return "", false
	}
	return code, true
}

// commandContext resolves namespace and actor for a mutation.
func (h *InternalHandler) commandContext(c *gin.Context) (NamespaceCode, Actor, bool) {
	if !h.requireWrites(c) {
		return "", Actor{}, false
	}
	namespace, ok := internalNamespace(c)
	if !ok {
		return "", Actor{}, false
	}
	actor, err := platformActor(c)
	if err != nil {
		respondError(c, err)
		return "", Actor{}, false
	}
	return namespace, actor, true
}

// requireVersionInNamespace is the cross-namespace gate for every
// version-addressed administration route. A SHOP version id supplied on the
// FOOD path is refused before any mutation is attempted.
func (h *InternalHandler) requireVersionInNamespace(
	c *gin.Context,
	namespace NamespaceCode,
	versionID string,
) bool {
	resolved, err := h.service.Namespace(c, namespace)
	if err != nil {
		respondError(c, err)
		return false
	}
	version, err := h.service.GetVersion(c, versionID)
	if err != nil {
		respondError(c, err)
		return false
	}
	if version.NamespaceID != resolved.ID {
		respondError(c, ErrNamespaceMismatch)
		return false
	}
	return true
}

// requireFacetInNamespace and requireNodeInNamespace do the same for the
// records addressed by id rather than by version.
func (h *InternalHandler) requireFacetInNamespace(
	c *gin.Context,
	namespace NamespaceCode,
	facetID string,
) bool {
	resolved, err := h.service.Namespace(c, namespace)
	if err != nil {
		respondError(c, err)
		return false
	}
	facet, err := h.service.GetFacet(c, facetID)
	if err != nil {
		respondError(c, err)
		return false
	}
	if facet.NamespaceID != resolved.ID {
		respondError(c, ErrNamespaceMismatch)
		return false
	}
	return true
}

func (h *InternalHandler) requireNodeInNamespace(
	c *gin.Context,
	namespace NamespaceCode,
	nodeID string,
) bool {
	resolved, err := h.service.Namespace(c, namespace)
	if err != nil {
		respondError(c, err)
		return false
	}
	node, err := h.service.GetNode(c, nodeID)
	if err != nil {
		respondError(c, err)
		return false
	}
	if node.NamespaceID != resolved.ID {
		respondError(c, ErrNamespaceMismatch)
		return false
	}
	return true
}

// isNotFound reports the "no published version yet" case, which is a normal
// state for a namespace that has been drafted but never published rather than
// an error to propagate.
func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func expectedRevision(c *gin.Context) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(c.GetHeader("If-Match")), 10, 64)
	return value
}

// ---------------------------------------------------------------------------
// Versions
// ---------------------------------------------------------------------------

func (h *InternalHandler) listVersions(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	versions, err := h.service.ListVersions(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"namespace_code": namespace, "versions": versions})
}

func (h *InternalHandler) getVersion(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	version, err := h.service.GetVersion(c, c.Param("versionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"version": version})
}

func (h *InternalHandler) inspectDraft(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	inspection, err := h.service.InspectDraft(c, c.Param("versionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, inspection)
}

func (h *InternalHandler) createDraft(c *gin.Context) {
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	version, err := h.service.CreateDraft(c, CreateDraftInput{NamespaceCode: namespace, Actor: actor})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"version": version})
}

func (h *InternalHandler) cloneActiveVersion(c *gin.Context) {
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	version, err := h.service.CloneActiveVersion(c, CloneActiveVersionInput{NamespaceCode: namespace, Actor: actor})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"version": version})
}

func (h *InternalHandler) compareVersions(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	target := strings.TrimSpace(c.Query("target_version_id"))
	if target == "" {
		respondError(c, ErrInvalidArgument)
		return
	}
	// Both sides of a diff must belong to the addressed namespace, otherwise
	// a caller could diff FOOD against SHOP and read the other namespace's
	// structure out of the result.
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) ||
		!h.requireVersionInNamespace(c, namespace, target) {
		return
	}
	comparison, err := h.service.CompareVersions(c, c.Param("versionId"), target)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, comparison)
}

// ---------------------------------------------------------------------------
// Facets
// ---------------------------------------------------------------------------

func (h *InternalHandler) listFacets(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	facets, err := h.service.ListFacets(c, c.Param("versionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"facets": facets})
}

type createFacetRequest struct {
	Code          string             `json:"code"`
	DisplayName   string             `json:"display_name"`
	Description   string             `json:"description"`
	StructureType FacetStructureType `json:"structure_type"`
	Position      int                `json:"position"`
	Metadata      json.RawMessage    `json:"metadata"`
}

func (h *InternalHandler) createFacet(c *gin.Context) {
	var request createFacetRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	facet, err := h.service.CreateFacet(c, CreateFacetInput{
		VersionID:     c.Param("versionId"),
		Code:          request.Code,
		DisplayName:   request.DisplayName,
		Description:   request.Description,
		StructureType: request.StructureType,
		Position:      request.Position,
		Metadata:      request.Metadata,
		Actor:         actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"facet": facet})
}

type updateFacetRequest struct {
	Code          string             `json:"code"`
	DisplayName   string             `json:"display_name"`
	Description   string             `json:"description"`
	StructureType FacetStructureType `json:"structure_type"`
	Position      int                `json:"position"`
	Metadata      json.RawMessage    `json:"metadata"`
}

func (h *InternalHandler) updateFacet(c *gin.Context) {
	var request updateFacetRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireFacetInNamespace(c, namespace, c.Param("facetId")) {
		return
	}
	facet, err := h.service.UpdateFacet(c, UpdateFacetInput{
		FacetID:          c.Param("facetId"),
		Code:             request.Code,
		DisplayName:      request.DisplayName,
		Description:      request.Description,
		StructureType:    request.StructureType,
		Position:         request.Position,
		Metadata:         request.Metadata,
		ExpectedRevision: expectedRevision(c),
		Actor:            actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"facet": facet})
}

func (h *InternalHandler) facetStatus(restore bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, actor, ok := h.commandContext(c)
		if !ok {
			return
		}
		if !h.requireFacetInNamespace(c, namespace, c.Param("facetId")) {
			return
		}
		input := FacetStatusInput{
			FacetID:          c.Param("facetId"),
			ExpectedRevision: expectedRevision(c),
			Actor:            actor,
		}
		var (
			facet Facet
			err   error
		)
		if restore {
			facet, err = h.service.RestoreFacet(c, input)
		} else {
			facet, err = h.service.ArchiveFacet(c, input)
		}
		if err != nil {
			respondError(c, err)
			return
		}
		respond(c, http.StatusOK, gin.H{"facet": facet})
	}
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

func (h *InternalHandler) listNodes(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	var (
		nodes []Node
		err   error
	)
	if facetID := strings.TrimSpace(c.Query("facet_id")); facetID != "" {
		nodes, err = h.service.ListNodesByFacet(c, c.Param("versionId"), facetID)
	} else {
		nodes, err = h.service.ListNodes(c, c.Param("versionId"))
	}
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"nodes": nodes})
}

func (h *InternalHandler) listNodeTerms(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	terms, err := h.service.ListNodeTerms(c, namespace, c.Param("versionId"), c.Param("nodeId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"terms": terms})
}

type createNodeRequest struct {
	FacetID       string          `json:"facet_id"`
	CanonicalCode string          `json:"canonical_code"`
	DisplayName   string          `json:"display_name"`
	Description   string          `json:"description"`
	ParentID      *string         `json:"parent_id"`
	Position      int             `json:"position"`
	Metadata      json.RawMessage `json:"metadata"`
}

func (h *InternalHandler) createNode(c *gin.Context) {
	var request createNodeRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	node, err := h.service.CreateNode(c, CreateNodeInput{
		VersionID:     c.Param("versionId"),
		FacetID:       request.FacetID,
		CanonicalCode: request.CanonicalCode,
		DisplayName:   request.DisplayName,
		Description:   request.Description,
		ParentID:      request.ParentID,
		Position:      request.Position,
		Metadata:      request.Metadata,
		Actor:         actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"node": node})
}

type updateNodeRequest struct {
	CanonicalCode string          `json:"canonical_code"`
	DisplayName   string          `json:"display_name"`
	Description   string          `json:"description"`
	ParentID      *string         `json:"parent_id"`
	Position      int             `json:"position"`
	Metadata      json.RawMessage `json:"metadata"`
}

func (h *InternalHandler) updateNode(c *gin.Context) {
	var request updateNodeRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireNodeInNamespace(c, namespace, c.Param("nodeId")) {
		return
	}
	node, err := h.service.UpdateNode(c, UpdateNodeInput{
		NodeID:           c.Param("nodeId"),
		CanonicalCode:    request.CanonicalCode,
		DisplayName:      request.DisplayName,
		Description:      request.Description,
		ParentID:         request.ParentID,
		Position:         request.Position,
		Metadata:         request.Metadata,
		ExpectedRevision: expectedRevision(c),
		Actor:            actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"node": node})
}

func (h *InternalHandler) nodeStatus(restore bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, actor, ok := h.commandContext(c)
		if !ok {
			return
		}
		if !h.requireNodeInNamespace(c, namespace, c.Param("nodeId")) {
			return
		}
		input := NodeStatusInput{
			NodeID:           c.Param("nodeId"),
			ExpectedRevision: expectedRevision(c),
			Actor:            actor,
		}
		var (
			node Node
			err  error
		)
		if restore {
			node, err = h.service.RestoreNode(c, input)
		} else {
			node, err = h.service.ArchiveNode(c, input)
		}
		if err != nil {
			respondError(c, err)
			return
		}
		respond(c, http.StatusOK, gin.H{"node": node})
	}
}

// ---------------------------------------------------------------------------
// Vocabulary
// ---------------------------------------------------------------------------

func (h *InternalHandler) applySemanticSeed(c *gin.Context) {
	var bundle SemanticSeedBundle
	if err := decodeStrict(c, &bundle); err != nil {
		httpx.RespondError(c, err)
		return
	}
	_, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if err := h.service.ApplySemanticSeed(c, bundle, actor); err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"applied": true})
}

func (h *InternalHandler) authorityVocabulary(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	records, err := h.service.ListAuthorityVocabulary(c, namespace, c.Param("versionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"authority_vocabulary": records})
}

// ---------------------------------------------------------------------------
// Publication
// ---------------------------------------------------------------------------

func (h *InternalHandler) validateVersion(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	version, err := h.service.ValidateDraft(c, namespace, c.Param("versionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"version": version, "publishable": true})
}

func (h *InternalHandler) publishVersion(c *gin.Context) {
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	version, err := h.service.PublishVersion(c, PublishVersionInput{
		VersionID:        c.Param("versionId"),
		ExpectedRevision: expectedRevision(c),
		Actor:            actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"version": version})
}

func (h *InternalHandler) retireVersion(c *gin.Context) {
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, c.Param("versionId")) {
		return
	}
	version, err := h.service.RetireVersion(c, RetireVersionInput{
		VersionID:        c.Param("versionId"),
		ExpectedRevision: expectedRevision(c),
		Actor:            actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"version": version})
}

func (h *InternalHandler) publicationHistory(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	history, err := h.service.ListPublicationHistory(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"publication_history": history})
}

// ---------------------------------------------------------------------------
// Governance
// ---------------------------------------------------------------------------

type openGovernanceCaseRequest struct {
	SubjectType string             `json:"subject_type"`
	SubjectRef  string             `json:"subject_ref"`
	CaseType    GovernanceCaseType `json:"case_type"`
	Priority    GovernancePriority `json:"priority"`
	ReasonCode  *string            `json:"reason_code"`
	Description *string            `json:"description"`
	Metadata    json.RawMessage    `json:"metadata"`
}

func (h *InternalHandler) openGovernanceCase(c *gin.Context) {
	var request openGovernanceCaseRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	result, err := h.service.OpenGovernanceCase(c, OpenGovernanceCaseInput{
		NamespaceCode: namespace,
		SubjectType:   request.SubjectType,
		SubjectRef:    request.SubjectRef,
		CaseType:      request.CaseType,
		Priority:      request.Priority,
		ReasonCode:    request.ReasonCode,
		Description:   request.Description,
		Metadata:      request.Metadata,
		Actor:         actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, result)
}

func (h *InternalHandler) getGovernanceCase(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	governanceCase, err := h.service.GetGovernanceCase(c, namespace, c.Param("caseId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"governance_case": governanceCase})
}

type transitionGovernanceCaseRequest struct {
	Transition       GovernanceTransition `json:"transition"`
	AssignedActorRef *string              `json:"assigned_actor_ref"`
	ResolutionCode   *string              `json:"resolution_code"`
	ResolutionNotes  *string              `json:"resolution_notes"`
	Metadata         json.RawMessage      `json:"metadata"`
}

func (h *InternalHandler) transitionGovernanceCase(c *gin.Context) {
	var request transitionGovernanceCaseRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	// The case must belong to the addressed namespace before it is
	// transitioned, so a FOOD case id cannot be driven through the SHOP path.
	if _, err := h.service.GetGovernanceCase(c, namespace, c.Param("caseId")); err != nil {
		respondError(c, err)
		return
	}
	result, err := h.service.TransitionGovernanceCase(c, TransitionGovernanceCaseInput{
		CaseID:           c.Param("caseId"),
		ExpectedRevision: expectedRevision(c),
		Transition:       request.Transition,
		AssignedActorRef: request.AssignedActorRef,
		ResolutionCode:   request.ResolutionCode,
		ResolutionNotes:  request.ResolutionNotes,
		Metadata:         request.Metadata,
		Actor:            actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Mappings and semantic assignment (platform scope)
// ---------------------------------------------------------------------------

type createMappingRequest struct {
	VersionID       string                 `json:"version_id"`
	NodeID          string                 `json:"node_id"`
	SourceType      string                 `json:"source_type"`
	SourceRef       string                 `json:"source_ref"`
	MappingType     MappingType            `json:"mapping_type"`
	ConfidenceBasis MappingConfidenceBasis `json:"confidence_basis"`
	ConfidenceScore *int                   `json:"confidence_score"`
	BusinessRef     *string                `json:"business_ref"`
	BranchRef       *string                `json:"branch_ref"`
	Metadata        json.RawMessage        `json:"metadata"`
}

func (h *InternalHandler) createMapping(c *gin.Context) {
	var request createMappingRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	if !h.requireVersionInNamespace(c, namespace, request.VersionID) {
		return
	}
	mapping, err := h.service.CreateMapping(c, CreateMappingInput{
		NamespaceCode:   namespace,
		VersionID:       request.VersionID,
		NodeID:          request.NodeID,
		SourceType:      request.SourceType,
		SourceRef:       request.SourceRef,
		MappingType:     request.MappingType,
		ConfidenceBasis: request.ConfidenceBasis,
		ConfidenceScore: request.ConfidenceScore,
		BusinessRef:     request.BusinessRef,
		BranchRef:       request.BranchRef,
		Metadata:        request.Metadata,
		Actor:           actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"mapping": mapping})
}

func (h *InternalHandler) getMapping(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	mapping, err := h.service.GetMapping(c, namespace, c.Param("mappingId"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"mapping": mapping})
}

type replaceFacetMappingsPlatformRequest struct {
	FacetCode   string             `json:"facet_code"`
	SourceType  string             `json:"source_type"`
	SourceRef   string             `json:"source_ref"`
	BusinessRef *string            `json:"business_ref"`
	BranchRef   *string            `json:"branch_ref"`
	Mappings    []MappingCandidate `json:"mappings"`
}

func (h *InternalHandler) replaceFacetMappings(c *gin.Context) {
	var request replaceFacetMappingsPlatformRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	result, err := h.service.ReplaceFacetMappings(c, ReplaceFacetMappingsInput{
		NamespaceCode: namespace,
		FacetCode:     request.FacetCode,
		SourceType:    request.SourceType,
		SourceRef:     request.SourceRef,
		BusinessRef:   request.BusinessRef,
		BranchRef:     request.BranchRef,
		Mappings:      request.Mappings,
		Actor:         actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}

type assignTagRequest struct {
	TagCode     string          `json:"tag_code"`
	SourceType  string          `json:"source_type"`
	SourceRef   string          `json:"source_ref"`
	BusinessRef *string         `json:"business_ref"`
	BranchRef   *string         `json:"branch_ref"`
	Metadata    json.RawMessage `json:"metadata"`
}

func (h *InternalHandler) assignTag(c *gin.Context) {
	var request assignTagRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	id, err := h.service.AssignTag(c, AssignTagInput{
		NamespaceCode: namespace,
		TagCode:       request.TagCode,
		SourceType:    request.SourceType,
		SourceRef:     request.SourceRef,
		BusinessRef:   request.BusinessRef,
		BranchRef:     request.BranchRef,
		Metadata:      request.Metadata,
		Actor:         actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"assignment_id": id})
}

type assignDietaryClaimRequest struct {
	ClaimCode          string                       `json:"claim_code"`
	SourceType         string                       `json:"source_type"`
	SourceRef          string                       `json:"source_ref"`
	BusinessRef        *string                      `json:"business_ref"`
	BranchRef          *string                      `json:"branch_ref"`
	Status             DietaryClaimAssignmentStatus `json:"status"`
	EvidenceRef        *string                      `json:"evidence_ref"`
	VerifiedByActorRef *string                      `json:"verified_by_actor_ref"`
	Metadata           json.RawMessage              `json:"metadata"`
}

func (h *InternalHandler) assignDietaryClaim(c *gin.Context) {
	var request assignDietaryClaimRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, actor, ok := h.commandContext(c)
	if !ok {
		return
	}
	id, err := h.service.AssignDietaryClaim(c, AssignDietaryClaimInput{
		NamespaceCode:      namespace,
		ClaimCode:          request.ClaimCode,
		SourceType:         request.SourceType,
		SourceRef:          request.SourceRef,
		BusinessRef:        request.BusinessRef,
		BranchRef:          request.BranchRef,
		Status:             request.Status,
		EvidenceRef:        request.EvidenceRef,
		VerifiedByActorRef: request.VerifiedByActorRef,
		Metadata:           request.Metadata,
		Actor:              actor,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"assignment_id": id})
}

// ---------------------------------------------------------------------------
// Seed administration (read-only)
// ---------------------------------------------------------------------------

// seedStatus and seedRuns are reads. Re-running a seed is a destructive
// publication act — it rewrites the vocabulary every vendor is classified
// against — and is deliberately not exposed as an HTTP command here. Seeds are
// applied through the seed applier's own command path, which records
// fingerprinted, append-only evidence that these two routes report.
func (h *InternalHandler) seedRuns(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	runs, err := h.service.ListSeedRuns(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"seed_runs": runs})
}

func (h *InternalHandler) seedStatus(c *gin.Context) {
	namespace, ok := internalNamespace(c)
	if !ok {
		return
	}
	runs, err := h.service.ListSeedRuns(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}

	status := gin.H{
		"namespace_code": namespace,
		"seeded":         len(runs) > 0,
		"run_count":      len(runs),
	}
	if len(runs) > 0 {
		latest := runs[0]
		status["latest_run"] = latest
		status["published"] = latest.Status == "PUBLISHED"
	}

	published, err := h.service.GetPublishedVersion(c, namespace)
	switch {
	case err == nil:
		status["published_version_id"] = published.ID
		status["published_version_number"] = published.VersionNumber
	case isNotFound(err):
		status["published_version_id"] = nil
	default:
		respondError(c, err)
		return
	}

	respond(c, http.StatusOK, status)
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterInternalRoutes mounts the platform taxonomy administration surface
// at the engine root. It is never mounted under /api/v1/vendor: the gateway
// forwards that base, and platform publication authority must not be
// reachable through a public door.
func RegisterInternalRoutes(
	router *gin.Engine,
	h *InternalHandler,
	verifier *serviceauth.Verifier,
	flags Flags,
) {
	if h == nil || h.service == nil || verifier == nil || !flags.TaxonomyEnabled {
		return
	}

	group := router.Group("/internal/vendor/v1/taxonomy")
	group.Use(serviceauth.Middleware(verifier), h.allowCaller, h.requireNonce)

	namespaced := group.Group("/namespaces/:namespaceCode")

	// Versions
	namespaced.GET("/versions", h.listVersions)
	namespaced.POST("/versions", h.createDraft)
	namespaced.POST("/versions/clone-active", h.cloneActiveVersion)
	namespaced.GET("/versions/:versionId", h.getVersion)
	namespaced.GET("/versions/:versionId/draft", h.inspectDraft)
	namespaced.GET("/versions/:versionId/compare", h.compareVersions)

	// Facets
	namespaced.GET("/versions/:versionId/facets", h.listFacets)
	namespaced.POST("/versions/:versionId/facets", h.createFacet)
	namespaced.PATCH("/facets/:facetId", h.updateFacet)
	namespaced.POST("/facets/:facetId/archive", h.facetStatus(false))
	namespaced.POST("/facets/:facetId/restore", h.facetStatus(true))

	// Nodes
	namespaced.GET("/versions/:versionId/nodes", h.listNodes)
	namespaced.POST("/versions/:versionId/nodes", h.createNode)
	namespaced.GET("/versions/:versionId/nodes/:nodeId/terms", h.listNodeTerms)
	namespaced.PATCH("/nodes/:nodeId", h.updateNode)
	namespaced.POST("/nodes/:nodeId/archive", h.nodeStatus(false))
	namespaced.POST("/nodes/:nodeId/restore", h.nodeStatus(true))

	// Vocabulary
	namespaced.POST("/semantic-seed", h.applySemanticSeed)
	namespaced.GET("/versions/:versionId/authority-vocabulary", h.authorityVocabulary)

	// Publication
	namespaced.POST("/versions/:versionId/validate", h.validateVersion)
	namespaced.POST("/versions/:versionId/publish", h.publishVersion)
	namespaced.POST("/versions/:versionId/retire", h.retireVersion)
	namespaced.GET("/publication-history", h.publicationHistory)

	// Governance
	namespaced.POST("/governance-cases", h.openGovernanceCase)
	namespaced.GET("/governance-cases/:caseId", h.getGovernanceCase)
	namespaced.POST("/governance-cases/:caseId/transition", h.transitionGovernanceCase)

	// Mappings and semantic assignment
	namespaced.POST("/mappings", h.createMapping)
	namespaced.GET("/mappings/:mappingId", h.getMapping)
	namespaced.PUT("/facet-mappings", h.replaceFacetMappings)
	namespaced.POST("/tag-assignments", h.assignTag)
	namespaced.POST("/dietary-claim-assignments", h.assignDietaryClaim)

	// Seed evidence
	namespaced.GET("/seed-status", h.seedStatus)
	namespaced.GET("/seed-runs", h.seedRuns)
}
