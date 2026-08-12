package filters

import (
	"net/url"
	"testing"
)

var schema = Schema{
	Fields: map[string]Field{
		"status": {Kind: KindEnum, Values: []string{"ACTIVE", "ARCHIVED"}},
		"code":   {Kind: KindString},
	},
	Sorts: []string{"createdAt", "name"},
}

func TestParseAllowlisted(t *testing.T) {
	query := url.Values{"status": {"ACTIVE"}, "code": {"LAG-01"}, "sort": {"-createdAt"}, "limit": {"10"}}
	parsed, err := Parse(query, schema)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Values["status"] != "ACTIVE" || parsed.Values["code"] != "LAG-01" {
		t.Fatalf("values = %+v", parsed.Values)
	}
	if parsed.SortField != "createdAt" || !parsed.SortDesc {
		t.Fatalf("sort = %s desc=%v", parsed.SortField, parsed.SortDesc)
	}
}

func TestRejectUnknownFilter(t *testing.T) {
	if _, err := Parse(url.Values{"organization_id": {"org_x"}}, schema); err == nil {
		t.Fatal("unknown filter should be rejected")
	}
}

func TestRejectUnknownSort(t *testing.T) {
	if _, err := Parse(url.Values{"sort": {"secret_column"}}, schema); err == nil {
		t.Fatal("unknown sort should be rejected")
	}
}

func TestRejectMalformedValues(t *testing.T) {
	if _, err := Parse(url.Values{"status": {"DROPPED"}}, schema); err == nil {
		t.Fatal("out-of-enum value should be rejected")
	}
	if _, err := Parse(url.Values{"code": {""}}, schema); err == nil {
		t.Fatal("empty value should be rejected")
	}
}
