package filters

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

var strictSchema = Schema{Fields: map[string]Field{
	"status":          {Kind: KindString},
	"includeArchived": {Kind: KindBool},
	"limit":           {Kind: KindLimit},
	"offset":          {Kind: KindOffset},
	"cursor":          {Kind: KindCursor},
	"tier":            {Kind: KindEnum, Values: []string{"GOLD"}},
}}

func violationFields(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("expected a validation error")
	}
	appErr := apperrors.From(err)
	if appErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", appErr.Status)
	}
	var out []string
	for _, f := range appErr.Fields {
		out = append(out, f.Field)
	}
	return out
}

func TestValidateAcceptsDeclaredParameters(t *testing.T) {
	query := url.Values{"status": {"ACTIVE"}, "includeArchived": {"true"}, "limit": {"200"},
		"offset": {"10000"}, "cursor": {"opaque"}, "tier": {"GOLD"}}
	if err := Validate(query, strictSchema); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	// Empty values mean "not supplied", as every handler already treats them.
	if err := Validate(url.Values{"status": {""}, "limit": {""}}, strictSchema); err != nil {
		t.Fatalf("empty values = %v", err)
	}
	if err := Validate(url.Values{}, Schema{}); err != nil {
		t.Fatalf("no query = %v", err)
	}
}

func TestValidateRejectsUnknownRepeatedAndMalformed(t *testing.T) {
	cases := map[string]url.Values{
		"bogusFilter":     {"bogusFilter": {"1"}},
		"limit":           {"limit": {"100000"}},
		"offset":          {"offset": {"-1"}},
		"includeArchived": {"includeArchived": {"yes"}},
		"tier":            {"tier": {"SILVER"}},
		"cursor":          {"cursor": {strings.Repeat("x", 2000)}},
		"status":          {"status": {"A", "B"}},
	}
	for want, query := range cases {
		fields := violationFields(t, Validate(query, strictSchema))
		if len(fields) != 1 || fields[0] != want {
			t.Fatalf("%v -> %v, want [%s]", query, fields, want)
		}
	}
	// No implicit reserved parameters: a route without offset paging refuses it.
	fields := violationFields(t, Validate(url.Values{"offset": {"0"}, "page": {"1"}, "sort": {"x"}}, Schema{}))
	if strings.Join(fields, ",") != "offset,page,sort" {
		t.Fatalf("undeclared paging params = %v", fields)
	}
}

func TestValidateSortAgainstDeclaredFields(t *testing.T) {
	schema := Schema{Sorts: []string{"createdAt"}}
	if err := Validate(url.Values{"sort": {"-createdAt"}}, schema); err != nil {
		t.Fatalf("declared sort = %v", err)
	}
	if fields := violationFields(t, Validate(url.Values{"sort": {"internal_column"}}, schema)); fields[0] != "sort" {
		t.Fatalf("unknown sort = %v", fields)
	}
}
