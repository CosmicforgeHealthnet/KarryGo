package models

import (
	"sync"

	"cosmicforge/logistics/shared/go/admincase"
)

// The source case registry (shared/go/admincase.Specs) is compiled code, not
// configuration: a case kind an owner enrolls there brings its binding type
// and evidence purposes with it. Nothing a request sends can extend this set.
var caseVocabulary = sync.OnceValues(func() (map[string]bool, map[string]bool) {
	bindings, purposes := map[string]bool{}, map[string]bool{}
	for _, spec := range admincase.Specs() {
		bindings[spec.BindingType] = true
		for _, purpose := range spec.Purposes {
			purposes[purpose] = true
		}
	}
	return bindings, purposes
})

// CaseBindingType reports whether a compiled source case binds evidence with
// this type.
func CaseBindingType(value string) bool {
	bindings, _ := caseVocabulary()
	return value != "" && bindings[value]
}

// CaseOnlyBindingType is a case binding type Media did not already know. Such
// bindings only ever describe administrative case evidence and must be private.
func CaseOnlyBindingType(value string) bool {
	return CaseBindingType(value) && !baseBindingType(value)
}

// CaseOnlyPurpose is an evidence purpose declared by a compiled source case
// that is not part of Media's own vocabulary.
func CaseOnlyPurpose(value string) bool {
	_, purposes := caseVocabulary()
	return value != "" && purposes[value] && !basePurpose(value)
}
