package taxonomy

// Section 13I service reads.
//
// These are thin: they resolve the namespace a caller named, enforce that the
// record they return actually belongs to it, and delegate to the repository.
// No classification, publication or governance rule is re-implemented here —
// Sections 13A–13C already own those, and a second copy of a taxonomy rule is
// a second thing to get wrong.

import (
	"context"
)

// Namespace resolves a namespace by its code. An unknown code is
// ErrInvalidArgument rather than a default, so a typo never silently
// addresses FOOD.
func (s *Service) Namespace(
	ctx context.Context,
	code NamespaceCode,
) (Namespace, error) {
	if s == nil || s.repo == nil {
		return Namespace{}, ErrInvalidArgument
	}
	resolved, ok := normalizeNamespaceCode(string(code))
	if !ok {
		return Namespace{}, ErrInvalidArgument
	}
	return s.repo.GetNamespaceByCode(ctx, resolved)
}

// Repository exposes the repository to composition-root wiring that needs
// bounded Section 13I reads (the HTTP handlers in this same package use it
// directly). It is not a general escape hatch: nothing outside this package
// can reach the repository's unexported pool.
func (s *Service) Repository() *Repository {
	if s == nil {
		return nil
	}
	return s.repo
}

// ValidateDraft runs the publication validation rules against a draft without
// publishing it.
//
// It reuses validateDraftForPublication — the exact predicate PublishVersion
// enforces — inside a transaction that is always rolled back. A separate
// "validate" implementation would be free to drift from the one that actually
// gates publication, which would make a green validation meaningless.
func (s *Service) ValidateDraft(
	ctx context.Context,
	namespaceCode NamespaceCode,
	versionID string,
) (Version, error) {
	if s == nil || s.repo == nil || versionID == "" {
		return Version{}, ErrInvalidArgument
	}

	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return Version{}, err
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	version, err := s.repo.GetVersionByIDTx(ctx, tx, versionID, false)
	if err != nil {
		return Version{}, err
	}

	if version.NamespaceID != namespace.ID {
		return Version{}, ErrNamespaceMismatch
	}

	if err = s.validateDraftForPublication(ctx, tx, version); err != nil {
		return Version{}, err
	}

	return version, nil
}

// versionInNamespace loads a version and refuses it if it belongs to a
// different namespace. Every version-addressed read below goes through this,
// which is what stops a SHOP version id being read through the FOOD path.
func (s *Service) versionInNamespace(
	ctx context.Context,
	namespaceCode NamespaceCode,
	versionID string,
) (Namespace, Version, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return Namespace{}, Version{}, err
	}
	if versionID == "" {
		return Namespace{}, Version{}, ErrInvalidArgument
	}
	version, err := s.repo.GetVersionByID(ctx, versionID)
	if err != nil {
		return Namespace{}, Version{}, err
	}
	if version.NamespaceID != namespace.ID {
		return Namespace{}, Version{}, ErrNamespaceMismatch
	}
	return namespace, version, nil
}

// ListNodeTerms returns the vocabulary attached to one node, refusing a node
// that belongs to a different version or namespace than the one addressed.
func (s *Service) ListNodeTerms(
	ctx context.Context,
	namespaceCode NamespaceCode,
	versionID string,
	nodeID string,
) ([]TaxonomyNodeTerm, error) {
	namespace, version, err := s.versionInNamespace(ctx, namespaceCode, versionID)
	if err != nil {
		return nil, err
	}

	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node.NamespaceID != namespace.ID || node.VersionID != version.ID {
		return nil, ErrNamespaceMismatch
	}

	return s.repo.ListNodeTerms(ctx, version.ID, node.ID)
}

// ListAuthorityVocabulary returns the governed authority vocabulary for one
// version of one namespace.
func (s *Service) ListAuthorityVocabulary(
	ctx context.Context,
	namespaceCode NamespaceCode,
	versionID string,
) ([]AuthorityVocabularyRecord, error) {
	namespace, version, err := s.versionInNamespace(ctx, namespaceCode, versionID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListAuthorityVocabulary(ctx, namespace.ID, version.ID)
}

// ListSeedRuns returns append-only seed evidence for a namespace.
func (s *Service) ListSeedRuns(
	ctx context.Context,
	namespaceCode NamespaceCode,
) ([]SeedRunRecord, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return nil, err
	}
	return s.repo.ListSeedRuns(ctx, namespace.ID)
}

// GetGovernanceCase reads one governance case inside one namespace.
func (s *Service) GetGovernanceCase(
	ctx context.Context,
	namespaceCode NamespaceCode,
	caseID string,
) (GovernanceCase, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return GovernanceCase{}, err
	}
	return s.repo.GetGovernanceCase(ctx, namespace.ID, caseID)
}

// GetMapping reads one mapping inside one namespace.
func (s *Service) GetMapping(
	ctx context.Context,
	namespaceCode NamespaceCode,
	mappingID string,
) (TaxonomyMapping, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return TaxonomyMapping{}, err
	}
	mapping, err := s.repo.GetMappingByID(ctx, mappingID)
	if err != nil {
		return TaxonomyMapping{}, err
	}
	if mapping.NamespaceID != namespace.ID {
		return TaxonomyMapping{}, ErrNamespaceMismatch
	}
	return mapping, nil
}

// ListDietaryClaims returns the namespace's dietary claim catalogue.
func (s *Service) ListDietaryClaims(
	ctx context.Context,
	namespaceCode NamespaceCode,
) ([]DietaryClaim, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return nil, err
	}
	return s.repo.ListDietaryClaims(ctx, namespace.ID)
}

// ---------------------------------------------------------------------------
// Vendor-scoped reads
// ---------------------------------------------------------------------------

// VendorSourceClassification is everything a vendor may see about how one of
// their own catalogue resources is classified.
type VendorSourceClassification struct {
	NamespaceCode NamespaceCode `json:"namespace_code"`

	SourceType  string `json:"source_type"`
	SourceRef   string `json:"source_ref"`
	BusinessRef string `json:"business_ref"`
	BranchRef   string `json:"branch_ref,omitempty"`

	Mappings        []TaxonomyMapping            `json:"mappings"`
	TagAssignments  []TagAssignmentView          `json:"tag_assignments"`
	DietaryTagsView []DietaryClaimAssignmentView `json:"dietary_claim_assignments"`
}

// ListMappingsForOwnedSource returns the mappings for a source the caller has
// already been proven to own.
func (s *Service) ListMappingsForOwnedSource(
	ctx context.Context,
	namespaceCode NamespaceCode,
	ownership SourceOwnership,
) ([]TaxonomyMapping, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return nil, err
	}
	return s.repo.ListMappingsForSourceScoped(
		ctx, namespace.ID, ownership.SourceType, ownership.SourceRef, ownership.BusinessRef,
	)
}

// ListTagAssignmentsForOwnedSource returns the semantic tag assignments for a
// source the caller has already been proven to own.
func (s *Service) ListTagAssignmentsForOwnedSource(
	ctx context.Context,
	namespaceCode NamespaceCode,
	ownership SourceOwnership,
) ([]TagAssignmentView, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return nil, err
	}
	return s.repo.ListTagAssignmentsForSourceScoped(
		ctx, namespace.ID, ownership.SourceType, ownership.SourceRef, ownership.BusinessRef,
	)
}

// ListDietaryClaimAssignmentsForOwnedSource returns the dietary claim
// assignments, with verification state, for a source the caller owns.
func (s *Service) ListDietaryClaimAssignmentsForOwnedSource(
	ctx context.Context,
	namespaceCode NamespaceCode,
	ownership SourceOwnership,
) ([]DietaryClaimAssignmentView, error) {
	namespace, err := s.Namespace(ctx, namespaceCode)
	if err != nil {
		return nil, err
	}
	return s.repo.ListDietaryClaimAssignmentsForSourceScoped(
		ctx, namespace.ID, ownership.SourceType, ownership.SourceRef, ownership.BusinessRef,
	)
}
