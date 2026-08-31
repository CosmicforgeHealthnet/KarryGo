package inventory

// Flags are generic Inventory feature controls supplied by application
// composition.
type Flags struct {
	Enabled       bool
	WritesEnabled bool
}

// DefaultFlags fail closed.
func DefaultFlags() Flags {
	return Flags{}
}
