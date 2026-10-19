package adminowner

import (
	"cosmicforge/logistics/shared/go/adminread"
	"errors"
)

var ErrContract = errors.New("invalid minimized owner resource")

func ValidateResource(resource Resource, definition Definition) error {
	if resource.Kind != definition.Kind || !adminread.Opaque(resource.Reference) || len(resource.SnapshotETag) != 64 || resource.CreatedAt.IsZero() || resource.UpdatedAt.IsZero() || resource.SourceVersion != nil && *resource.SourceVersion < 1 || len(resource.Timeline) > 200 || len(resource.Dimensions) != len(definition.Dimensions) || len(resource.Counters) != len(definition.Counters) || len(resource.References) > len(definition.References) {
		return ErrContract
	}
	if resource.Status != nil && !adminread.Opaque(*resource.Status) {
		return ErrContract
	}
	for index, dimension := range resource.Dimensions {
		if dimension.Name != definition.Dimensions[index] || dimension.Value != nil && dimension.Flag != nil || dimension.Value != nil && !adminread.Opaque(*dimension.Value) {
			return ErrContract
		}
	}
	for index, counter := range resource.Counters {
		if counter.Name != definition.Counters[index] || counter.Value != nil && *counter.Value < 0 {
			return ErrContract
		}
	}
	seen := map[string]bool{}
	for _, ref := range resource.References {
		allowed := false
		for _, name := range definition.References {
			allowed = allowed || ref.Kind == name
		}
		if !allowed || seen[ref.Kind] || !adminread.Opaque(ref.Reference) {
			return ErrContract
		}
		seen[ref.Kind] = true
	}
	for _, event := range resource.Timeline {
		if !adminread.Opaque(event.Reference) || !adminread.Opaque(event.Type) {
			return ErrContract
		}
	}
	return nil
}
