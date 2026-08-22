package taxonomy

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"

	"github.com/jackc/pgx/v5"
)

// InspectDraft returns a consistent snapshot of a DRAFT taxonomy version.
// The version row is locked while its facet/node content is read because all
// taxonomy mutation paths lock that same version before editing.
func (s *Service) InspectDraft(
	ctx context.Context,
	versionID string,
) (DraftInspection, error) {
	if s == nil || s.repo == nil || versionID == "" {
		return DraftInspection{}, ErrInvalidArgument
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return DraftInspection{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		versionID,
		true,
	)
	if err != nil {
		return DraftInspection{}, err
	}

	if version.Status != VersionStatusDraft {
		return DraftInspection{}, ErrNotDraft
	}

	facets, err := s.repo.ListFacetsTx(ctx, tx, version.ID)
	if err != nil {
		return DraftInspection{}, err
	}

	nodes, err := s.repo.ListNodesTx(ctx, tx, version.ID)
	if err != nil {
		return DraftInspection{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return DraftInspection{}, err
	}

	return DraftInspection{
		Version: version,
		Facets:  facets,
		Nodes:   nodes,
	}, nil
}

// CompareVersions compares canonical meaning rather than database row IDs.
//
// CloneActiveVersion deliberately gives cloned facets/nodes new identifiers,
// so comparison identity is:
//
//	facet: facet code
//	node:  facet code + canonical node code
//
// Parent relationships are compared by the parent's canonical identity.
func (s *Service) CompareVersions(
	ctx context.Context,
	fromVersionID string,
	toVersionID string,
) (VersionComparison, error) {
	if s == nil ||
		s.repo == nil ||
		fromVersionID == "" ||
		toVersionID == "" {
		return VersionComparison{}, ErrInvalidArgument
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return VersionComparison{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	fromVersion, toVersion, err := s.lockComparisonVersions(
		ctx,
		tx,
		fromVersionID,
		toVersionID,
	)
	if err != nil {
		return VersionComparison{}, err
	}

	if fromVersion.NamespaceID != toVersion.NamespaceID {
		return VersionComparison{}, ErrConflict
	}

	fromFacets, err := s.repo.ListFacetsTx(ctx, tx, fromVersion.ID)
	if err != nil {
		return VersionComparison{}, err
	}

	toFacets, err := s.repo.ListFacetsTx(ctx, tx, toVersion.ID)
	if err != nil {
		return VersionComparison{}, err
	}

	fromNodes, err := s.repo.ListNodesTx(ctx, tx, fromVersion.ID)
	if err != nil {
		return VersionComparison{}, err
	}

	toNodes, err := s.repo.ListNodesTx(ctx, tx, toVersion.ID)
	if err != nil {
		return VersionComparison{}, err
	}

	namespace, err := s.repo.GetNamespaceByID(ctx, fromVersion.NamespaceID)
	if err != nil {
		return VersionComparison{}, err
	}

	comparison := VersionComparison{
		NamespaceCode: namespace.Code,
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
	}

	fromFacetByCode := make(map[string]Facet, len(fromFacets))
	toFacetByCode := make(map[string]Facet, len(toFacets))

	for _, facet := range fromFacets {
		fromFacetByCode[facet.Code] = facet
	}

	for _, facet := range toFacets {
		toFacetByCode[facet.Code] = facet
	}

	// Preserve deterministic repository ordering.
	for _, facet := range toFacets {
		before, exists := fromFacetByCode[facet.Code]
		if !exists {
			comparison.AddedFacets = append(comparison.AddedFacets, facet)
			continue
		}

		if !facetContentEqual(before, facet) {
			comparison.ModifiedFacets = append(
				comparison.ModifiedFacets,
				FacetChange{
					Before: before,
					After:  facet,
				},
			)
		}
	}

	for _, facet := range fromFacets {
		if _, exists := toFacetByCode[facet.Code]; !exists {
			comparison.RemovedFacets = append(
				comparison.RemovedFacets,
				facet,
			)
		}
	}

	fromNodeIndex, err := buildNodeComparisonIndex(
		fromFacets,
		fromNodes,
	)
	if err != nil {
		return VersionComparison{}, err
	}

	toNodeIndex, err := buildNodeComparisonIndex(
		toFacets,
		toNodes,
	)
	if err != nil {
		return VersionComparison{}, err
	}

	for _, item := range toNodeIndex.Ordered {
		before, exists := fromNodeIndex.ByKey[item.Key]
		if !exists {
			comparison.AddedNodes = append(
				comparison.AddedNodes,
				item.Node,
			)
			continue
		}

		if !nodeContentEqual(before, item) {
			comparison.ModifiedNodes = append(
				comparison.ModifiedNodes,
				NodeChange{
					Before: before.Node,
					After:  item.Node,
				},
			)
		}
	}

	for _, item := range fromNodeIndex.Ordered {
		if _, exists := toNodeIndex.ByKey[item.Key]; !exists {
			comparison.RemovedNodes = append(
				comparison.RemovedNodes,
				item.Node,
			)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return VersionComparison{}, err
	}

	return comparison, nil
}

func (s *Service) lockComparisonVersions(
	ctx context.Context,
	tx pgx.Tx,
	fromID string,
	toID string,
) (Version, Version, error) {
	ids := []string{fromID, toID}

	if fromID != toID {
		sort.Strings(ids)
	}

	versions := make(map[string]Version, len(ids))

	for _, id := range ids {
		if _, exists := versions[id]; exists {
			continue
		}

		version, err := s.repo.GetVersionByIDTx(
			ctx,
			tx,
			id,
			true,
		)
		if err != nil {
			return Version{}, Version{}, err
		}

		versions[id] = version
	}

	return versions[fromID], versions[toID], nil
}

type nodeComparisonItem struct {
	Key       string
	ParentKey string
	Node      Node
}

type nodeComparisonIndex struct {
	ByKey   map[string]nodeComparisonItem
	Ordered []nodeComparisonItem
}

func buildNodeComparisonIndex(
	facets []Facet,
	nodes []Node,
) (nodeComparisonIndex, error) {
	facetCodeByID := make(map[string]string, len(facets))

	for _, facet := range facets {
		facetCodeByID[facet.ID] = facet.Code
	}

	nodeKeyByID := make(map[string]string, len(nodes))

	for _, node := range nodes {
		facetCode, exists := facetCodeByID[node.FacetID]
		if !exists {
			return nodeComparisonIndex{}, ErrConflict
		}

		nodeKeyByID[node.ID] = taxonomyNodeKey(
			facetCode,
			node.CanonicalCode,
		)
	}

	result := nodeComparisonIndex{
		ByKey:   make(map[string]nodeComparisonItem, len(nodes)),
		Ordered: make([]nodeComparisonItem, 0, len(nodes)),
	}

	for _, node := range nodes {
		key := nodeKeyByID[node.ID]

		item := nodeComparisonItem{
			Key:  key,
			Node: node,
		}

		if node.ParentID != nil {
			parentKey, exists := nodeKeyByID[*node.ParentID]
			if !exists {
				return nodeComparisonIndex{}, ErrConflict
			}

			item.ParentKey = parentKey
		}

		result.ByKey[key] = item
		result.Ordered = append(result.Ordered, item)
	}

	return result, nil
}

func taxonomyNodeKey(
	facetCode string,
	nodeCode string,
) string {
	return facetCode + "\x00" + nodeCode
}

func facetContentEqual(
	left Facet,
	right Facet,
) bool {
	return left.Code == right.Code &&
		left.DisplayName == right.DisplayName &&
		left.Description == right.Description &&
		left.StructureType == right.StructureType &&
		left.Position == right.Position &&
		left.Status == right.Status &&
		jsonEquivalent(left.Metadata, right.Metadata)
}

func nodeContentEqual(
	left nodeComparisonItem,
	right nodeComparisonItem,
) bool {
	return left.Node.CanonicalCode == right.Node.CanonicalCode &&
		left.Node.DisplayName == right.Node.DisplayName &&
		left.Node.Description == right.Node.Description &&
		left.ParentKey == right.ParentKey &&
		left.Node.Position == right.Node.Position &&
		left.Node.Status == right.Node.Status &&
		jsonEquivalent(left.Node.Metadata, right.Node.Metadata)
}

func jsonEquivalent(
	left json.RawMessage,
	right json.RawMessage,
) bool {
	var leftValue any
	var rightValue any

	if len(left) == 0 {
		left = json.RawMessage(`{}`)
	}

	if len(right) == 0 {
		right = json.RawMessage(`{}`)
	}

	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}

	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}

	return reflect.DeepEqual(leftValue, rightValue)
}
