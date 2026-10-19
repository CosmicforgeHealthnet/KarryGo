package adminowner

// NewSQLSource binds each public resource definition to the source owner's
// fixed SQL dictionary. An omitted or unknown dictionary entry is a coding
// error, not a request-time fallback to arbitrary tables or another owner.
func NewSQLSource(owner string, specs map[string]SQLSpec) *SQLSource {
	profile, ok := ForOwner(owner)
	if !ok {
		panic("unknown administrative source owner")
	}
	if len(specs) != len(profile.Definitions) {
		panic("incomplete administrative source dictionary")
	}
	for kind, spec := range specs {
		definition, ok := profile.Definitions[kind]
		if !ok || spec.Table == "" {
			panic("invalid administrative source dictionary")
		}
		spec.Definition = definition
		specs[kind] = spec
	}
	return &SQLSource{Specs: specs}
}
