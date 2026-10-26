// Package adminresources exposes Support's fixed, privacy-minimized Secure
// Admin owner projections (Section 16).
//
// Every projection is built from explicitly listed columns. Chat bodies,
// complaint descriptions, resolution notes, identity snapshots (names and
// phones), incident coordinates and media URLs are never selected. The only
// free text that leaves is a bounded, credential-redacted label: a case
// subject or a message preview. Market and zone are not recorded natively on
// complaints; they come only from an independently signed owner context.
package adminresources

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/supportcase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "support-dispute-service"

// MaxLabelRunes is the Secure Admin label bound.
const MaxLabelRunes = 240

// caseCounts are the per-case counters shared by cases and sos-cases.
// unread_count is what staff have not read: complainant messages still unread.
const caseCounts = `(SELECT count(*) FROM support_chat_messages m WHERE m.complaint_id = c.id AND NOT m.is_read AND m.sender_type <> 'admin') AS unread_count,
 (SELECT count(*) FROM support_chat_messages m WHERE m.complaint_id = c.id) AS message_count,
 (SELECT count(*) FROM complaint_evidence e WHERE e.complaint_id = c.id) AS evidence_count`

// caseTable selects only minimized columns. organization_reference stays raw
// because it is the canonical tenant binding; its exposed reference is the
// separately filtered organization_ref.
const caseTable = `(SELECT c.id, c.status::text AS status, c.admin_version, c.created_at, c.updated_at,
 c.organization_reference, support_admin_opaque(c.organization_reference) AS organization_ref,
 c.service_type::text AS service_type, support_admin_opaque(c.category) AS category, lower(c.priority) AS priority,
 c.complainant_type::text AS complainant_type, support_admin_opaque(c.context_type) AS context_type,
 NULL::text AS locale, 'POLICY_UNAVAILABLE'::text AS sla_state,
 CASE WHEN lower(c.priority) = 'emergency' THEN 'SAFETY_EVIDENCE' ELSE 'OPERATIONS_MINIMIZED' END AS consent_classification,
 support_admin_opaque(c.escalation_level) AS escalation_level,
 support_admin_opaque(c.mission_ref) AS mission_ref, support_admin_opaque(c.order_ref) AS order_ref,
 support_admin_opaque(c.assigned_to) AS assignee,
 (SELECT d.id::text FROM disputes d WHERE d.complaint_id = c.id ORDER BY d.created_at DESC, d.id DESC LIMIT 1) AS dispute_ref,
 ` + caseCounts + `
 FROM complaints c)`

// caseTimeline exposes only the audit event type and time, never its payload.
const caseTimeline = `(SELECT x.id, x.complaint_id, x.created_at,
 CASE WHEN x.event_type ~ '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$' THEN x.event_type ELSE 'other' END AS event_type
 FROM complaint_events x)`

var caseDimensions = map[string]string{"service_type": "service_type", "category": "category", "priority": "priority", "complainant_type": "complainant_type", "context_type": "context_type", "locale": "locale", "sla_state": "sla_state", "consent_classification": "consent_classification"}

func specs() map[string]adminowner.SQLSpec {
	p := adminowner.SupportProfile()
	caseSpec := func(kind string, dims map[string]string, counters, refs map[string]string, predicate string) adminowner.SQLSpec {
		return adminowner.SQLSpec{Definition: p.Definitions[kind], Table: caseTable, StateColumn: "status", VersionColumn: "admin_version", OrganizationColumn: "organization_reference",
			BindingColumns: []string{"complainant_type", "service_type"}, Dimensions: dims, Counters: counters, References: refs, Predicate: predicate,
			TimelineTable: caseTimeline, TimelineParentColumn: "complaint_id", TimelineTypeColumn: "event_type", TimelineTimeColumn: "created_at"}
	}
	return map[string]adminowner.SQLSpec{
		"cases": caseSpec("cases", caseDimensions,
			map[string]string{"unread_count": "unread_count", "message_count": "message_count", "evidence_count": "evidence_count"},
			map[string]string{"organization": "organization_ref", "mission": "mission_ref", "order": "order_ref", "assignee": "assignee", "dispute": "dispute_ref"}, ""),
		"sos-cases": caseSpec("sos-cases",
			map[string]string{"service_type": "service_type", "complainant_type": "complainant_type", "escalation_level": "escalation_level", "sla_state": "sla_state"},
			map[string]string{"unread_count": "unread_count", "message_count": "message_count"},
			map[string]string{"organization": "organization_ref", "assignee": "assignee"}, "t.priority = 'emergency'"),
		"disputes": {Definition: p.Definitions["disputes"], Table: `(SELECT d.id, d.complaint_id, d.outcome::text AS outcome, d.admin_version, d.created_at, d.updated_at,
 c.organization_reference, d.service_type::text AS service_type, d.respondent_type::text AS respondent_type, d.remedy_state,
 d.complaint_id::text AS case_ref, d.remedy_wallet_operation_id::text AS remedy_operation
 FROM disputes d JOIN complaints c ON c.id = d.complaint_id)`, StateColumn: "outcome", VersionColumn: "admin_version", OrganizationColumn: "organization_reference",
			BindingColumns: []string{"complaint_id", "respondent_type"},
			Dimensions:     map[string]string{"service_type": "service_type", "respondent_type": "respondent_type", "remedy_state": "remedy_state"},
			References:     map[string]string{"case": "case_ref", "remedy_operation": "remedy_operation"}},
		"case-messages": {Definition: p.Definitions["case-messages"], Table: `(SELECT m.id, m.complaint_id, m.created_at, m.created_at AS updated_at, c.organization_reference,
 m.sender_type, m.is_read, m.sequence, m.complaint_id::text AS case_ref
 FROM support_chat_messages m JOIN complaints c ON c.id = m.complaint_id)`, OrganizationColumn: "organization_reference",
			BindingColumns: []string{"complaint_id", "sender_type"},
			Dimensions:     map[string]string{"sender_type": "sender_type", "read": "is_read"},
			Counters:       map[string]string{"sequence": "sequence"},
			References:     map[string]string{"case": "case_ref"}},
		"case-evidence": {Definition: p.Definitions["case-evidence"], Table: `(SELECT e.id, e.complaint_id, e.created_at, e.created_at AS updated_at, c.organization_reference,
 e.uploader_type::text AS uploader_type, e.complaint_id::text AS case_ref, support_admin_opaque(e.media_asset_id) AS media_object
 FROM complaint_evidence e JOIN complaints c ON c.id = e.complaint_id)`, OrganizationColumn: "organization_reference",
			BindingColumns: []string{"complaint_id", "uploader_type"},
			Dimensions:     map[string]string{"uploader_type": "uploader_type"},
			References:     map[string]string{"case": "case_ref", "media_object": "media_object"}},
	}
}

// SourceReader wraps the compiled SQL dictionary and adds bounded labels.
type SourceReader struct{ *adminowner.SQLSource }

func Source() *SourceReader {
	return &SourceReader{adminowner.NewSQLSource(Owner, specs())}
}

// labelQueries is a fixed allowlist; nothing else may become a label.
var labelQueries = map[string]string{
	"cases":         `SELECT subject FROM complaints WHERE id::text = $1`,
	"case-messages": `SELECT content FROM support_chat_messages WHERE id::text = $1`,
}

func (s *SourceReader) Read(ctx context.Context, tx pgx.Tx, kind, id string, details bool) (adminowner.Resource, error) {
	item, err := s.SQLSource.Read(ctx, tx, kind, id, details)
	if err != nil {
		return item, err
	}
	if query, ok := labelQueries[kind]; ok {
		var raw *string
		if err := tx.QueryRow(ctx, query, id).Scan(&raw); err != nil {
			return adminowner.Resource{}, err
		}
		if raw != nil {
			item.Label = Label(*raw)
		}
		item.SnapshotETag = ""
		item.SnapshotETag = admincontext.Fingerprint(item)
	}
	return item, nil
}

var (
	emailLike = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// Phone-length digit runs (with separators) and anything 9+ digits long.
	longNumber = regexp.MustCompile(`\+?[0-9][0-9 ()./-]{7,}[0-9]`)
	spaces     = regexp.MustCompile(`\s+`)
)

// Label is the credential-redacted, single-line preview bounded to 240 runes.
// One-time codes (shared supportcase redaction), phone-like numbers and email
// addresses never survive; an empty result is no label at all.
func Label(raw string) *string {
	// Contacts first: the shared one-time-code redaction would otherwise split
	// a phone number into fragments the long-number pattern no longer sees.
	value := emailLike.ReplaceAllString(raw, "[REDACTED]")
	value = longNumber.ReplaceAllString(value, "[REDACTED]")
	value = supportcase.SanitizeDescription(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(spaces.ReplaceAllString(value, " "))
	if utf8.RuneCountInString(value) > MaxLabelRunes {
		runes := []rune(value)
		value = strings.TrimSpace(string(runes[:MaxLabelRunes]))
	}
	if value == "" {
		return nil
	}
	return &value
}

// ReadPermission is the native grant each projection requires. It is the
// profile capability itself, so the delegated and native names cannot drift.
func ReadPermission(kind string) string {
	if d, ok := adminowner.SupportProfile().Definitions[kind]; ok {
		return d.Capability
	}
	return ""
}

// Native mutation grants (Section 16).
const (
	PermissionCaseManage     = "support.case.manage"
	PermissionSOSManage      = "support.sos.manage"
	PermissionDisputeDecide  = "support.dispute.decide"
	PermissionEvidenceAttach = "support.evidence.attach"
)

// ErrNoGrant is a missing, revoked or expired native grant.
var ErrNoGrant = errors.New("support staff grant absent")

func requireGrant(ctx context.Context, tx pgx.Tx, staff, permission string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM support_admin_staff_grants WHERE staff_ref = $1 AND permission = $2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now()) ORDER BY id LIMIT 1 FOR SHARE`, staff, permission).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	return err
}

// AuthorizeStaff is the native read check: an absent grant is pgx.ErrNoRows,
// which the shared handler treats as "not visible", never as an outage.
func (s *SourceReader) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, _ string) error {
	permission := ReadPermission(kind)
	if permission == "" || staff == "" {
		return pgx.ErrNoRows
	}
	return requireGrant(ctx, tx, staff, permission)
}

// RequireMutationGrant holds the grant row (FOR SHARE) until the command
// commits, so a concurrent revocation cannot race past the decision.
func RequireMutationGrant(ctx context.Context, tx pgx.Tx, staff, permission string) error {
	if staff == "" {
		return ErrNoGrant
	}
	if err := requireGrant(ctx, tx, staff, permission); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoGrant
		}
		return err
	}
	return nil
}

// Runtime composes the owner read boundary. Every switch defaults off.
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	p := adminowner.SupportProfile()
	return adminowner.FromEnvironment(ctx, db, Source(), p.Definitions, Owner, p.Prefix, p.Path)
}

var _ adminowner.StaffAuthorizer = (*SourceReader)(nil)
var _ adminowner.Source = (*SourceReader)(nil)
