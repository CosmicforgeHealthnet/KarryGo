package taxonomy

// DraftInspection is the complete canonical content of one DRAFT version.
// It is platform taxonomy state, not a Vendor-owned catalogue.
type DraftInspection struct {
	Version Version
	Facets  []Facet
	Nodes   []Node
}
