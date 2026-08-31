package searchrebuild

import (
	"context"
	"encoding/json"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
)

// classification is everything Section 13A-13C says about one source resource:
// which canonical nodes it maps to, the vocabulary those nodes carry, and the
// semantic tags and dietary claims assigned to it.
type classification struct {
	// nodesByFacet maps a facet code to the canonical node codes the resource
	// is classified under, e.g. CUISINE -> [YORUBA].
	nodesByFacet map[string][]string

	// displayByFacet carries the human-readable node names, which are what a
	// customer actually types.
	displayByFacet map[string][]string

	aliases  []string
	synonyms []string

	tagCodes       []string
	preferenceTags []string
	dietaryClaims  []string
}

func newClassification() *classification {
	return &classification{
		nodesByFacet:   map[string][]string{},
		displayByFacet: map[string][]string{},
	}
}

// facetValues returns the searchable values for one facet: canonical codes plus
// the display names, deduplicated.
//
// Both are included because a customer searching "yoruba" and a partner service
// filtering on the canonical code YORUBA are asking the same question through
// different vocabularies, and a projection that carried only one of them would
// answer only one of those callers.
func (c classification) facetValues(facetCode string) []string {
	if c.nodesByFacet == nil {
		return nil
	}
	return dedupeStrings(append(
		append([]string{}, c.nodesByFacet[facetCode]...),
		c.displayByFacet[facetCode]...,
	))
}

// taxonomyJSON is the canonical classification as stored on the document.
func (c classification) taxonomyJSON() json.RawMessage {
	if len(c.nodesByFacet) == 0 {
		return json.RawMessage(`{}`)
	}
	payload := make(map[string]any, len(c.nodesByFacet))
	for facet, nodes := range c.nodesByFacet {
		payload[strings.ToLower(facet)] = nodes
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

// Source types the canonical taxonomy uses for each namespace's resources.
const (
	sourceTypeFoodItem    = "FOOD_ITEM"
	sourceTypeShopProduct = "SHOP_PRODUCT"
	sourceTypeShopVariant = "SHOP_VARIANT"
)

// loadClassification bulk-loads mappings, vocabulary, tags and dietary claims
// for a whole batch of source refs in four queries, regardless of batch size.
//
// It reads only ACTIVE mappings against the namespace's PUBLISHED version:
// classifying a catalogue item against a draft or a retired vocabulary would
// make search disagree with the taxonomy the platform is actually publishing.
func (s *Source) loadClassification(
	ctx context.Context,
	sourceType string,
	sourceRefs []string,
	namespaceCode string,
) (map[string]classification, error) {
	out := map[string]classification{}
	if len(sourceRefs) == 0 {
		return out, nil
	}

	buckets := map[string]*classification{}
	bucket := func(ref string) *classification {
		existing, ok := buckets[ref]
		if !ok {
			existing = newClassification()
			buckets[ref] = existing
		}
		return existing
	}

	// 1. Mappings joined to their node and facet, restricted to the published
	//    version of the namespace.
	mappingRows, err := s.pool.Query(
		ctx,
		`
        SELECT
            mapping.source_ref,
            facet.code,
            node.canonical_code,
            node.display_name
          FROM canonical_taxonomy_mappings mapping
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id = mapping.namespace_id
           AND ns.code = $3
          JOIN canonical_taxonomy_versions version
            ON version.id = mapping.version_id
           AND version.status = 'PUBLISHED'
          JOIN canonical_taxonomy_nodes node
            ON node.id = mapping.node_id
           AND node.status = 'ACTIVE'
          JOIN canonical_taxonomy_facets facet
            ON facet.id = node.facet_id
           AND facet.status = 'ACTIVE'
         WHERE mapping.source_type = $1
           AND mapping.source_ref = ANY($2)
           AND mapping.status = 'ACTIVE'
        `,
		sourceType, sourceRefs, namespaceCode,
	)
	if err != nil {
		return nil, err
	}
	nodeCodesByRef := map[string][]string{}
	for mappingRows.Next() {
		var ref, facetCode, nodeCode, displayName string
		if err = mappingRows.Scan(&ref, &facetCode, &nodeCode, &displayName); err != nil {
			mappingRows.Close()
			return nil, err
		}
		entry := bucket(ref)
		entry.nodesByFacet[facetCode] = append(entry.nodesByFacet[facetCode], nodeCode)
		entry.displayByFacet[facetCode] = append(entry.displayByFacet[facetCode], displayName)
		nodeCodesByRef[ref] = append(nodeCodesByRef[ref], nodeCode)
	}
	mappingRows.Close()
	if err = mappingRows.Err(); err != nil {
		return nil, err
	}

	// 2. Vocabulary for every mapped node, in one query over the batch's nodes.
	//    Aliases and synonyms are what make Nigerian search work - "jolof",
	//    "moin moin", "paint" - so a rebuild that dropped them would quietly
	//    degrade every query that relies on them.
	if len(nodeCodesByRef) > 0 {
		allNodes := []string{}
		for _, codes := range nodeCodesByRef {
			allNodes = append(allNodes, codes...)
		}
		allNodes = dedupeStrings(allNodes)

		termRows, err := s.pool.Query(
			ctx,
			`
            SELECT
                node.canonical_code,
                term.term,
                term.relationship_type
              FROM canonical_taxonomy_node_terms term
              JOIN canonical_taxonomy_nodes node
                ON node.id = term.node_id
              JOIN canonical_taxonomy_versions version
                ON version.id = term.version_id
               AND version.status = 'PUBLISHED'
              JOIN canonical_taxonomy_namespaces ns
                ON ns.id = term.namespace_id
               AND ns.code = $2
             WHERE node.canonical_code = ANY($1)
               AND term.status = 'ACTIVE'
            `,
			allNodes, namespaceCode,
		)
		if err != nil {
			return nil, err
		}
		aliasesByNode := map[string][]string{}
		synonymsByNode := map[string][]string{}
		for termRows.Next() {
			var nodeCode, term, relationship string
			if err = termRows.Scan(&nodeCode, &term, &relationship); err != nil {
				termRows.Close()
				return nil, err
			}
			switch strings.ToUpper(relationship) {
			case "ALIAS":
				aliasesByNode[nodeCode] = append(aliasesByNode[nodeCode], term)
			default:
				// SYNONYM and RELATED_TERM both widen recall; they are kept
				// apart from aliases because Section 13E weights them
				// differently.
				synonymsByNode[nodeCode] = append(synonymsByNode[nodeCode], term)
			}
		}
		termRows.Close()
		if err = termRows.Err(); err != nil {
			return nil, err
		}

		for ref, codes := range nodeCodesByRef {
			entry := bucket(ref)
			for _, code := range codes {
				entry.aliases = append(entry.aliases, aliasesByNode[code]...)
				entry.synonyms = append(entry.synonyms, synonymsByNode[code]...)
			}
		}
	}

	// 3. Semantic tag assignments.
	tagRows, err := s.pool.Query(
		ctx,
		`
        SELECT
            assignment.source_ref,
            tag.code,
            tag.tag_type
          FROM canonical_taxonomy_tag_assignments assignment
          JOIN canonical_taxonomy_tags tag
            ON tag.id = assignment.tag_id
           AND tag.status = 'ACTIVE'
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id = assignment.namespace_id
           AND ns.code = $3
         WHERE assignment.source_type = $1
           AND assignment.source_ref = ANY($2)
           AND assignment.status = 'ACTIVE'
        `,
		sourceType, sourceRefs, namespaceCode,
	)
	if err != nil {
		return nil, err
	}
	for tagRows.Next() {
		var ref, code, tagType string
		if err = tagRows.Scan(&ref, &code, &tagType); err != nil {
			tagRows.Close()
			return nil, err
		}
		entry := bucket(ref)
		entry.tagCodes = append(entry.tagCodes, code)
		if strings.EqualFold(tagType, "ATTRIBUTE") {
			entry.preferenceTags = append(entry.preferenceTags, code)
		}
	}
	tagRows.Close()
	if err = tagRows.Err(); err != nil {
		return nil, err
	}

	// 4. Dietary claim assignments.
	//
	//    Every assigned claim is projected regardless of verification state,
	//    because a customer filtering for HALAL must see declared claims too -
	//    but the claim's verification status stays in the authoritative
	//    assignment record and is never rewritten to "verified" here.
	claimRows, err := s.pool.Query(
		ctx,
		`
        SELECT
            assignment.source_ref,
            claim.code
          FROM canonical_taxonomy_dietary_claim_assignments assignment
          JOIN canonical_taxonomy_dietary_claims claim
            ON claim.id = assignment.claim_id
           AND claim.status = 'ACTIVE'
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id = assignment.namespace_id
           AND ns.code = $3
         WHERE assignment.source_type = $1
           AND assignment.source_ref = ANY($2)
           AND assignment.status <> 'REJECTED'
        `,
		sourceType, sourceRefs, namespaceCode,
	)
	if err != nil {
		return nil, err
	}
	for claimRows.Next() {
		var ref, code string
		if err = claimRows.Scan(&ref, &code); err != nil {
			claimRows.Close()
			return nil, err
		}
		bucket(ref).dietaryClaims = append(bucket(ref).dietaryClaims, code)
	}
	claimRows.Close()
	if err = claimRows.Err(); err != nil {
		return nil, err
	}

	for ref, entry := range buckets {
		entry.aliases = dedupeStrings(entry.aliases)
		entry.synonyms = dedupeStrings(entry.synonyms)
		entry.tagCodes = dedupeStrings(entry.tagCodes)
		entry.preferenceTags = dedupeStrings(entry.preferenceTags)
		entry.dietaryClaims = dedupeStrings(entry.dietaryClaims)
		for facet := range entry.nodesByFacet {
			entry.nodesByFacet[facet] = dedupeStrings(entry.nodesByFacet[facet])
			entry.displayByFacet[facet] = dedupeStrings(entry.displayByFacet[facet])
		}
		out[ref] = *entry
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Trust and serviceability
// ---------------------------------------------------------------------------

type trustSignal struct {
	status   searchprojection.SearchTrustStatus
	source   string
	revision int64
}

type serviceabilitySignal struct {
	status   searchprojection.ServiceabilityStatus
	source   string
	revision int64
}

// loadTrust reads Section 7's business verification projection.
//
// Trust is the BUSINESS dimension only. A business whose verification has never
// been recorded is UNKNOWN, not "not verified": those are different claims, and
// only one of them is something the platform actually established.
func (s *Source) loadTrust(ctx context.Context, businessIDs []string) (map[string]trustSignal, error) {
	out := map[string]trustSignal{}
	if len(businessIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT business_id, status, COALESCE(source_version, 0)
          FROM vendor_verification_projections
         WHERE business_id = ANY($1)
           AND dimension = 'BUSINESS'
        `,
		businessIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var businessID, status string
		var version int64
		if err = rows.Scan(&businessID, &status, &version); err != nil {
			return nil, err
		}
		out[businessID] = trustSignal{
			status:   trustFromVerification(status),
			source:   "vendor_verification_projections",
			revision: version,
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for _, businessID := range businessIDs {
		if _, ok := out[businessID]; !ok {
			out[businessID] = trustSignal{
				status: searchprojection.SearchTrustUnknown,
				source: "unavailable",
			}
		}
	}
	return out, nil
}

func trustFromVerification(status string) searchprojection.SearchTrustStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "VERIFIED":
		return searchprojection.SearchTrustVerified
	case "REJECTED", "EXPIRED":
		return searchprojection.SearchTrustNotVerified
	case "SUSPENDED":
		return searchprojection.SearchTrustSuspended
	case "NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW":
		// Under review is not yet verified, and is not a failure either.
		return searchprojection.SearchTrustNotVerified
	default:
		return searchprojection.SearchTrustUnknown
	}
}

// loadServiceability reads branch status and whether the branch has a declared
// service area.
//
// A branch that is ACTIVE and has at least one service area is serviceable. An
// active branch with no declared area is UNKNOWN rather than serviceable: the
// platform genuinely does not know where it delivers, and guessing "everywhere"
// would surface it to customers it cannot reach.
func (s *Source) loadServiceability(
	ctx context.Context,
	branchIDs []string,
) (map[string]serviceabilitySignal, error) {
	out := map[string]serviceabilitySignal{}
	if len(branchIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT
            branch.id,
            branch.status,
            branch.version,
            count(area.branch_id)
          FROM vendor_branches branch
          LEFT JOIN vendor_branch_service_areas area
            ON area.branch_id = branch.id
         WHERE branch.id = ANY($1)
         GROUP BY branch.id, branch.status, branch.version
        `,
		branchIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var branchID, status string
		var version, areas int64
		if err = rows.Scan(&branchID, &status, &version, &areas); err != nil {
			return nil, err
		}

		signal := serviceabilitySignal{
			source:   "vendor_branches",
			revision: version,
		}
		switch {
		case !strings.EqualFold(status, "ACTIVE"):
			signal.status = searchprojection.ServiceabilityNotServiceable
		case areas > 0:
			signal.status = searchprojection.ServiceabilityServiceable
			signal.source = "vendor_branch_service_areas"
		default:
			signal.status = searchprojection.ServiceabilityUnknown
		}
		out[branchID] = signal
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for _, branchID := range branchIDs {
		if _, ok := out[branchID]; !ok {
			out[branchID] = serviceabilitySignal{
				status: searchprojection.ServiceabilityUnknown,
				source: "unavailable",
			}
		}
	}
	return out, nil
}
