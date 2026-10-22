package adminprivacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runtime keeps every other Analytics report/service operational when this
// optional scoped transport is off. No key, policy or schema is read when off.
// An explicit unit selector is configuration, never governance approval.
func Runtime(ctx context.Context, db *pgxpool.Pool, legacy serviceauth.Secrets) (*Handler, error) {
	h := &Handler{Service: &Service{DB: db}}
	if os.Getenv("ANALYTICS_ADMIN_SCOPED_NETWORK_READ_ENABLED") != "true" {
		return h, nil
	}
	if db == nil {
		return nil, errors.New("scoped Analytics database required")
	}
	raw, err := os.ReadFile(os.Getenv("ANALYTICS_ADMIN_SCOPED_SECRET_FILE"))
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("scoped Analytics credential file unavailable")
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(secret) < 32 {
		return nil, errors.New("invalid scoped Analytics credential")
	}
	for _, old := range legacy {
		if bytes.Equal(secret, old) {
			return nil, errors.New("scoped Analytics requires a distinct credential audience")
		}
	}
	policy, err := adminpolicy.LoadFiles(os.Getenv("ANALYTICS_ADMIN_SCOPED_POLICY_FILE"), os.Getenv("ANALYTICS_ADMIN_SCOPED_POLICY_KEYS_FILE"), adminpolicyenv.Environment, time.Now())
	if err != nil && !errors.Is(err, adminpolicy.ErrBlocked) {
		return nil, errors.New("invalid scoped Analytics policy")
	}
	field := os.Getenv("ANALYTICS_ADMIN_MISSION_SUCCESS_UNIT_FIELD")
	if field != "" {
		if _, err := LineageKind("mission-success", field); err != nil {
			return nil, errors.New("invalid native Analytics unit selector")
		}
	}
	authorities, err := sourceAuthorities(os.Getenv("ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE"))
	if err != nil {
		return nil, err
	}
	source := &MissionSuccessSource{Authorities: authorities, UnitField: field}
	if policy != nil {
		rule, e := policy.Rule(analyticsadmin.MissionSuccessPermission, time.Now())
		if e != nil || rule.Owner != "analytics-service" || rule.SourceContract != source.SourceContract() {
			return nil, errors.New("scoped Analytics source approval mismatch")
		}
	}
	ready, done := context.WithTimeout(ctx, 3*time.Second)
	defer done()
	var ok bool
	if err := db.QueryRow(ready, `SELECT to_regclass('analytics_admin_http_nonces') IS NOT NULL AND to_regclass('analytics_admin_disclosure_authorizations') IS NOT NULL AND to_regclass('admin_owner_context_heads') IS NOT NULL`).Scan(&ok); err != nil || !ok {
		return nil, errors.New("scoped Analytics schema unavailable; apply normal migrations")
	}
	h.Reports = map[string]*Service{}
	for _, report := range analyticsadmin.Reports() {
		if report.Name == "mission-success" {
			continue
		}
		field := os.Getenv("ANALYTICS_ADMIN_SCOPED_" + strings.ToUpper(strings.ReplaceAll(report.Name, "-", "_")) + "_UNIT_FIELD")
		if field == "" {
			continue
		}
		if _, err := LineageKind(report.Name, field); err != nil {
			return nil, errors.New("invalid report unit selector")
		}
		var adapter Source = &ReportSource{Report: report.Name, Authorities: authorities, UnitField: field}
		if report.Name == "readiness" {
			adapter = &ReadinessSource{Authorities: authorities, UnitField: field}
		}
		if policy != nil {
			rule, e := policy.Rule(report.Permission, time.Now())
			if e != nil || rule.Owner != "analytics-service" || rule.SourceContract != report.Contract+":"+field {
				return nil, errors.New("report source approval mismatch")
			}
		}
		h.Reports[report.Name] = &Service{DB: db, Policy: policy, Source: adapter}
	}
	h.Enabled = true
	h.Secret = secret
	h.Service.Policy = policy
	h.Service.Source = source
	return h, nil
}

func uniqueKeyMap(d *json.Decoder, depth int) error {
	if depth > 3 {
		return errors.New("authority key nesting")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token == json.Delim('{') {
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate authority key")
			}
			seen[name] = true
			if err := uniqueKeyMap(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid authority object")
		}
	} else if _, ok := token.(string); !ok {
		return errors.New("invalid authority key value")
	}
	return nil
}

func sourceAuthorities(path string) (map[string]admincontext.Authority, error) {
	authorities := map[string]admincontext.Authority{}
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 65536 {
			return nil, errors.New("Analytics source authority keys unavailable")
		}
		var keys map[string]map[string]string
		d := json.NewDecoder(bytes.NewReader(raw))
		if uniqueKeyMap(d, 0) != nil {
			return nil, errors.New("duplicate or malformed Analytics source authorities")
		}
		if _, err := d.Token(); err != io.EOF {
			return nil, errors.New("multiple Analytics source authority documents")
		}
		if json.Unmarshal(raw, &keys) != nil || keys == nil || len(keys) > 4 {
			return nil, errors.New("invalid Analytics source authorities")
		}
		for owner, values := range keys {
			canonical, _ := canonicalLineageOwner(owner)
			if canonical == "" || canonical != owner || len(values) == 0 || len(values) > 32 {
				return nil, errors.New("unmapped Analytics source authority")
			}
			authority := admincontext.Authority{Owner: owner, Environment: adminpolicyenv.Environment, Keys: map[string]ed25519.PublicKey{}}
			for id, encoded := range values {
				key, err := base64.RawURLEncoding.DecodeString(encoded)
				if err != nil || len(key) != ed25519.PublicKeySize || !adminread.Opaque(id) {
					return nil, errors.New("invalid Analytics source authority key")
				}
				authority.Keys[id] = ed25519.PublicKey(key)
			}
			authorities[owner] = authority
		}
	}
	return authorities, nil
}
