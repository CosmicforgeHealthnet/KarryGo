package adminowner

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/jackc/pgx/v5"
)

// SQLSpec is supplied only by the owning service's compiled source dictionary.
// Values are source column names, not paths or names taken from HTTP input.
// The projection is built from explicit fields, never an entire row serialized
// and then stripped. Coordinates, credentials and free-text columns are absent.
type SQLSpec struct {
	JoinClause, CreatedAtExpression, UpdatedAtExpression string
	TenantColumn                                         string
	TimelineRefExpression, TimelineTimeColumn            string
	ReferenceExpression, VersionColumn                   string
	CanonicalQuery                                       string
	BindingExpression                                    string
	// DimensionQuery is compiled owner SQL returning only allowed dimension
	// labels. It can read canonical related records; absent rows remain unknown.
	DimensionQuery                                          string
	BindingColumns                                          []string
	Definition                                              Definition
	Table, StateColumn                                      string
	Dimensions, Counters, References                        map[string]string
	OrganizationColumn                                      string
	CanonicalTenantIsOrganization                           bool
	TimelineTable, TimelineParentColumn, TimelineTypeColumn string
	Predicate                                               string
}
type SQLSource struct{ Specs map[string]SQLSpec }

func fields(data map[string]json.RawMessage, key string) *string {
	var value string
	if raw := data[key]; len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == "" {
		return nil
	}
	return &value
}
func (s *SQLSource) row(ctx context.Context, tx pgx.Tx, kind, ref string, lock bool) (map[string]json.RawMessage, SQLSpec, error) {
	spec, ok := s.Specs[kind]
	if !ok {
		return nil, spec, admincontext.ErrUnmapped
	}
	// Identifiers and predicates are fixed source code; values are parameters.
	refExpr := spec.ReferenceExpression
	if refExpr == "" {
		refExpr = "t.id::text"
	}
	versionColumn := spec.VersionColumn
	created, updated := spec.CreatedAtExpression, spec.UpdatedAtExpression
	if created == "" {
		created = "t.created_at"
	}
	if updated == "" {
		updated = "t.updated_at"
	}
	bindings := spec.BindingExpression
	if bindings == "" {
		bindings = `(SELECT jsonb_object_agg(k,to_jsonb(t)->k) FROM unnest($7::text[]) k)`
	} else {
		bindings = `(` + bindings + ` || jsonb_build_object('_columns',$7::text[]))`
	}
	projection := `SELECT jsonb_build_object('id',` + refExpr + `,'created_at',` + created + `,'updated_at',` + updated + `,'status',to_jsonb(t)->$2::text,'source_version',to_jsonb(t)->$8::text,'organization',to_jsonb(t)->$3::text,'tenant',to_jsonb(t)->$9::text,'dimensions',(SELECT jsonb_object_agg(k,to_jsonb(t)->k) FROM unnest($4::text[]) k),'counters',(SELECT jsonb_object_agg(k,to_jsonb(t)->k) FROM unnest($5::text[]) k),'references',(SELECT jsonb_object_agg(k,to_jsonb(t)->k) FROM unnest($6::text[]) k),'bindings',` + bindings + `) FROM ` + spec.Table + ` t ` + spec.JoinClause + ` WHERE ` + refExpr + `=$1`
	if spec.Predicate != "" {
		projection += " AND (" + spec.Predicate + ")"
	}
	if lock {
		projection += " FOR UPDATE OF t"
	}
	columns := func(mapping map[string]string) []string {
		values := []string{}
		for _, value := range mapping {
			values = append(values, value)
		}
		sort.Strings(values)
		return values
	}
	var raw []byte
	err := tx.QueryRow(ctx, projection, ref, spec.StateColumn, spec.OrganizationColumn, columns(spec.Dimensions), columns(spec.Counters), columns(spec.References), spec.BindingColumns, versionColumn, spec.TenantColumn).Scan(&raw)
	if err != nil {
		return nil, spec, err
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil {
		return nil, spec, errors.New("invalid owner projection")
	}
	return data, spec, nil
}
func (s *SQLSource) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (admincontext.RecordBinding, error) {
	data, spec, err := s.row(ctx, tx, kind, ref, false)
	if err != nil {
		return admincontext.RecordBinding{}, err
	}
	canonical := adminpolicy.Scope{Product: "karryam"}
	if spec.CanonicalQuery != "" {
		if err := tx.QueryRow(ctx, spec.CanonicalQuery, ref).Scan(&canonical.Market, &canonical.Zone, &canonical.Organization, &canonical.Tenant); err != nil {
			return admincontext.RecordBinding{}, err
		}
	}
	if org := fields(data, "organization"); org != nil {
		if canonical.Organization != "" && canonical.Organization != *org {
			return admincontext.RecordBinding{}, admincontext.ErrUnmapped
		}
		canonical.Organization = *org
		if spec.CanonicalTenantIsOrganization {
			canonical.Tenant = *org
		}
	}
	if tenant := fields(data, "tenant"); tenant != nil {
		if canonical.Tenant != "" && canonical.Tenant != *tenant {
			return admincontext.RecordBinding{}, admincontext.ErrUnmapped
		}
		canonical.Tenant = *tenant
	}
	// Record identity + actual creation epoch and authorization-relevant links
	// prevent an attestation being carried onto a different/recreated record.
	fingerprint := admincontext.Fingerprint(struct {
		OwnerContract, Kind, Reference string
		Created                        json.RawMessage
		Bindings                       json.RawMessage
		Canonical                      adminpolicy.Scope
	}{spec.Definition.SourceContract, kind, ref, data["created_at"], data["bindings"], canonical})
	return admincontext.RecordBinding{Fingerprint: fingerprint, SourceContract: spec.Definition.SourceContract, Canonical: canonical}, nil
}
func (s *SQLSource) Read(ctx context.Context, tx pgx.Tx, kind, ref string, details bool) (Resource, error) {
	data, spec, err := s.row(ctx, tx, kind, ref, false)
	if err != nil {
		return Resource{}, err
	}
	out := Resource{Reference: ref, Kind: kind, Status: fields(data, "status"), Dimensions: []Dimension{}, Counters: []Counter{}, References: []Reference{}, Timeline: []TimelineEvent{}}
	if json.Unmarshal(data["created_at"], &out.CreatedAt) != nil || json.Unmarshal(data["updated_at"], &out.UpdatedAt) != nil {
		return out, errors.New("owner timestamp unavailable")
	}
	var version *int64
	if json.Unmarshal(data["source_version"], &version) == nil {
		out.SourceVersion = version
	}
	var dimensions, counters, references map[string]json.RawMessage
	json.Unmarshal(data["dimensions"], &dimensions)
	json.Unmarshal(data["counters"], &counters)
	json.Unmarshal(data["references"], &references)
	var related map[string]json.RawMessage
	if spec.DimensionQuery != "" {
		var raw []byte
		if err := tx.QueryRow(ctx, spec.DimensionQuery, ref).Scan(&raw); err != nil {
			return out, err
		}
		if err := json.Unmarshal(raw, &related); err != nil {
			return out, err
		}
		for name := range related {
			found := false
			for _, allowed := range spec.Definition.Dimensions {
				found = found || name == allowed
			}
			if !found {
				return out, errors.New("unregistered owner dimension")
			}
		}
	}
	for _, name := range spec.Definition.Dimensions {
		dimension := Dimension{Name: name, Value: fields(dimensions, spec.Dimensions[name])}
		raw := dimensions[spec.Dimensions[name]]
		if value, exists := related[name]; exists {
			raw = value
			dimension.Value = fields(related, name)
		}
		var flag *bool
		if json.Unmarshal(raw, &flag) == nil {
			dimension.Flag = flag
		}
		out.Dimensions = append(out.Dimensions, dimension)
	}
	for _, name := range spec.Definition.Counters {
		var value *int64
		json.Unmarshal(counters[spec.Counters[name]], &value)
		out.Counters = append(out.Counters, Counter{Name: name, Value: value})
	}
	for _, name := range spec.Definition.References {
		if value := fields(references, spec.References[name]); value != nil {
			out.References = append(out.References, Reference{Kind: name, Reference: *value})
		}
	}
	if spec.TimelineTable != "" && details {
		eventRef := spec.TimelineRefExpression
		if eventRef == "" {
			eventRef = "e.id::text"
		}
		eventTime := `COALESCE((to_jsonb(e)->>'occurred_at')::timestamptz,(to_jsonb(e)->>'created_at')::timestamptz)`
		if spec.TimelineTimeColumn != "" {
			eventTime = `(to_jsonb(e)->>'` + spec.TimelineTimeColumn + `')::timestamptz`
		}
		rows, err := tx.Query(ctx, `SELECT `+eventRef+`,to_jsonb(e)->>$2::text,`+eventTime+` FROM `+spec.TimelineTable+` e WHERE e.`+spec.TimelineParentColumn+`::text=$1 ORDER BY `+eventTime+` DESC NULLS LAST,`+eventRef+` DESC LIMIT 201`, ref, spec.TimelineTypeColumn)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var event TimelineEvent
			var at *time.Time
			if err := rows.Scan(&event.Reference, &event.Type, &at); err != nil {
				rows.Close()
				return out, err
			}
			event.At = at
			out.Timeline = append(out.Timeline, event)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if len(out.Timeline) > 200 {
			out.TimelineTruncated = true
			out.Timeline = out.Timeline[:200]
		}
	}
	// This is a content ETag for this minimized snapshot, not a fabricated
	// monotonic owner lifecycle version or permission to mutate a trip.
	if spec.Definition.Custody && details {
		out.Custody, err = readCustodyInventory(ctx, tx, ref)
		if err != nil {
			return out, err
		}
	}
	out.SnapshotETag = admincontext.Fingerprint(out)
	return out, nil
}
