package adminowner

import "strings"

// MatchBridge resolves only the compiled finite resource dictionary. It never
// accepts a destination, source table, SQL predicate or owner from a caller.
func MatchBridge(path string) (Profile, Definition, string, bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 3 && len(parts) != 4 || len(parts) > 1 && parts[1] != "resources" {
		return Profile{}, Definition{}, "", false
	}
	profile, ok := ForDomain(parts[0])
	if !ok {
		return Profile{}, Definition{}, "", false
	}
	definition, ok := profile.Definitions[parts[2]]
	if !ok {
		return Profile{}, Definition{}, "", false
	}
	ref := ""
	if len(parts) == 4 {
		ref = parts[3]
		if ref == "" {
			return Profile{}, Definition{}, "", false
		}
	}
	return profile, definition, ref, true
}
