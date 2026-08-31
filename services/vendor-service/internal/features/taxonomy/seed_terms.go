package taxonomy

import (
	"fmt"
	"strings"
)

func (b SeedBundle) validateSeedTerms() error {
	nodeByCode := make(map[string]struct{}, len(b.Nodes))

	for _, node := range b.Nodes {
		nodeByCode[node.CanonicalCode] = struct{}{}
	}

	seen := make(map[string]struct{}, len(b.Terms))

	for _, term := range b.Terms {
		if err := term.Validate(); err != nil {
			return fmt.Errorf("taxonomy seed term: %w", err)
		}

		if term.NamespaceCode != b.NamespaceCode {
			return fmt.Errorf(
				"taxonomy seed term %q is outside namespace %s",
				term.Term,
				b.NamespaceCode,
			)
		}

		if strings.TrimSpace(term.Locale) == "" {
			return fmt.Errorf(
				"taxonomy seed term %q requires locale",
				term.Term,
			)
		}

		if _, exists := nodeByCode[term.NodeCode]; !exists {
			return fmt.Errorf(
				"taxonomy seed term %q references missing node %s",
				term.Term,
				term.NodeCode,
			)
		}

		key := strings.Join([]string{
			string(term.RelationshipType),
			NormalizeVocabularyTerm(term.Term),
			strings.ToLower(strings.TrimSpace(term.Locale)),
			strings.ToLower(strings.TrimSpace(term.Region)),
		}, "|")

		if _, exists := seen[key]; exists {
			return fmt.Errorf(
				"duplicate taxonomy seed term %s",
				key,
			)
		}

		seen[key] = struct{}{}
	}

	return nil
}
