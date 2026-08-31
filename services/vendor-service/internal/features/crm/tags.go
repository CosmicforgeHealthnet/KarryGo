package crm

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Tag lifecycle.
const (
	TagActive   = "ACTIVE"
	TagArchived = "ARCHIVED"
)

// Tag membership sources. MANUAL is the only value a vendor request may
// produce; SYSTEM is reserved for future platform-managed tags and is never
// settable from the wire, which is what keeps a vendor from forging a
// system-authored classification.
const (
	TagSourceManual = "MANUAL"
	TagSourceSystem = "SYSTEM"
)

// Bulk operation bounds. Bulk work is all-or-nothing inside one transaction,
// so the ceiling is also the transaction size: a request above it is refused
// rather than silently truncated, because a partially applied bulk tag is
// indistinguishable from a successful one to the caller.
const (
	DefaultBulkBatch = 100
	MaxBulkBatch     = 500
)

// reservedTagPrefixes are namespaces a vendor may not claim. A vendor-created
// tag called "sys:churn-risk" would later be indistinguishable from a genuine
// platform-computed trait, so the namespace is closed from the start.
var reservedTagPrefixes = []string{"sys:", "system:", "karrygo:"}

// Tag is a vendor-defined customer classification label.
type Tag struct {
	ID         string
	BusinessID string

	ScopeType string
	BranchID  *string

	Name           string
	NormalizedName string
	Description    *string
	Status         string

	CreatedByStaffRef *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Version           int64
}

// IsArchived reports whether the tag may no longer receive new memberships.
func (t Tag) IsArchived() bool { return t.Status == TagArchived }

// TagView is the safe wire projection of a tag.
type TagView struct {
	TagRef      string  `json:"tag_ref"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ScopeType   string  `json:"scope_type"`
	BranchRef   *string `json:"branch_ref,omitempty"`
	Status      string  `json:"status"`
	MemberCount *int    `json:"member_count,omitempty"`
	CreatedAt   string  `json:"created_at"`
	Version     int64   `json:"version"`
}

// TagViewOf renders a tag for the wire.
func TagViewOf(tag Tag) TagView {
	return TagView{
		TagRef: tag.ID, Name: tag.Name, Description: tag.Description,
		ScopeType: tag.ScopeType, BranchRef: tag.BranchID, Status: tag.Status,
		CreatedAt: tag.CreatedAt.UTC().Format(time.RFC3339), Version: tag.Version,
	}
}

// ---------------------------------------------------------------------------
// Name rules
// ---------------------------------------------------------------------------

// NormalizeTagName folds a display name to its comparison key.
//
// Case and internal whitespace are collapsed so "VIP", "vip" and "V I P" do not
// become three tags that a human would read as one — duplicate tags are the
// single most common way a CRM taxonomy rots.
func NormalizeTagName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

// ValidateTagName enforces the tag naming rules.
//
// The name is stored and returned as plain data and is never interpreted as
// markup anywhere, so the checks here are about legibility and namespace
// integrity rather than about escaping: control characters, format characters
// and invisible-only strings are refused because they produce tags a human
// cannot tell apart, which is a correctness problem before it is a display one.
func ValidateTagName(name string) (display, normalized string, err error) {
	display = strings.TrimSpace(name)
	if display == "" {
		return "", "", tagValidation("name", "A tag name is required.")
	}
	if len([]rune(display)) < 2 || len([]rune(display)) > 80 {
		return "", "", tagValidation("name", "A tag name must be between 2 and 80 characters.")
	}
	for _, r := range display {
		// Cf covers zero-width joiners, bidi overrides and the byte-order mark:
		// characters that let two visually identical tag names be different
		// strings, which is a correctness problem before it is a display one.
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", "", tagValidation("name", "A tag name cannot contain control or invisible characters.")
		}
	}
	if strings.ContainsAny(display, "<>") {
		return "", "", tagValidation("name", "A tag name cannot contain markup characters.")
	}
	normalized = NormalizeTagName(display)
	if normalized == "" {
		return "", "", tagValidation("name", "A tag name must contain visible characters.")
	}
	for _, prefix := range reservedTagPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return "", "", tagValidation("name", "That tag name uses a reserved prefix.")
		}
	}
	return display, normalized, nil
}

func validateTagDescription(description string) (*string, error) {
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > 500 {
		return nil, tagValidation("description", "A tag description must be 500 characters or fewer.")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) && r != '\n' {
			return nil, tagValidation("description", "A tag description cannot contain control characters.")
		}
	}
	if strings.ContainsAny(trimmed, "<>") {
		return nil, tagValidation("description", "A tag description cannot contain markup characters.")
	}
	return &trimmed, nil
}

func tagValidation(field, message string) error {
	return apperrors.Validation(message, []apperrors.FieldViolation{{Field: field, Message: message}})
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

func errTagNotFound() error {
	return crmError(apperrors.NotFound("Tag not found.", nil), CodeTagNotFound)
}
func errTagExists() error {
	return crmError(apperrors.Conflict("A tag with this name already exists.", nil), CodeTagAlreadyExists)
}
func errTagArchived() error {
	return crmError(apperrors.Conflict("This tag is archived and cannot be assigned.", nil), CodeTagArchived)
}
func errScopeDenied(code string) error {
	return crmError(apperrors.Forbidden("You do not have access at this scope.", nil), code)
}
func errBulkTooLarge(limit int) error {
	err := apperrors.Validation("Too many customers in one request.", []apperrors.FieldViolation{
		{Field: "profile_refs", Message: "Send at most " + itoa(limit) + " customers per request."},
	})
	err.Details = map[string]any{"code": CodeBulkTooLarge, "max_batch": limit}
	return err
}
func errSegmentationDisabled() error {
	return crmError(apperrors.Unavailable("Customer segmentation is temporarily unavailable.", nil), CodeFeatureDisabled)
}

// ---------------------------------------------------------------------------
// Scope enforcement
// ---------------------------------------------------------------------------

// authorizeObjectScope decides whether an actor may act on an object at a given
// scope.
//
// A BUSINESS-scope object reasons over the whole business, so it needs a
// business-wide grant; a branch manager cannot create one, because doing so
// would hand them a lawful route to business-wide customer behaviour they
// cannot otherwise see. A BRANCH-scope object needs either a business-wide
// grant or that specific branch.
func authorizeObjectScope(scope Scope, scopeType string, branchID *string, deniedCode string) error {
	switch scopeType {
	case ScopeBusiness:
		if !scope.Global {
			return errScopeDenied(deniedCode)
		}
		return nil
	case ScopeBranch:
		if branchID == nil || *branchID == "" {
			return tagValidation("scope.branch_ref", "A branch is required for a branch-scoped object.")
		}
		if scope.Global || containsString(scope.BranchIDs, *branchID) {
			return nil
		}
		return errScopeDenied(deniedCode)
	}
	return tagValidation("scope.type", "Scope must be BUSINESS or BRANCH.")
}

// ---------------------------------------------------------------------------
// Tag reads
// ---------------------------------------------------------------------------

const tagCols = `id,business_id,scope_type,branch_id,name,normalized_name,description,status,
created_by_staff_ref,created_at,updated_at,version`

// qualifiedTagCols is the same list aliased to crm_tags, for queries that join
// another table.
const qualifiedTagCols = `t.id,t.business_id,t.scope_type,t.branch_id,t.name,t.normalized_name,t.description,
t.status,t.created_by_staff_ref,t.created_at,t.updated_at,t.version`

func scanTag(row rowScanner) (Tag, error) {
	var t Tag
	err := row.Scan(&t.ID, &t.BusinessID, &t.ScopeType, &t.BranchID, &t.Name, &t.NormalizedName,
		&t.Description, &t.Status, &t.CreatedByStaffRef, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	return t, err
}

// TagListOptions are the safe filters over the tag list.
type TagListOptions struct {
	Limit     int
	Offset    int
	ScopeType string
	BranchRef string
	Status    string
	Search    string
}

func (o TagListOptions) bounded() TagListOptions {
	if o.Limit <= 0 {
		o.Limit = DefaultPageSize
	}
	if o.Limit > MaxPageSize {
		o.Limit = MaxPageSize
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// Pagination bounds shared by every Section 8B list. There is no unbounded
// listing anywhere in the section.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// ListTags returns tags visible to the actor's scope, with member counts.
//
// A branch-scoped actor sees business tags (which apply to the whole business
// by definition) plus the branch tags of branches they hold, and never another
// branch's private taxonomy.
func (s *Service) ListTags(ctx context.Context, actor Actor, scope Scope, opts TagListOptions) ([]Tag, map[string]int, bool, error) {
	opts = opts.bounded()
	where := []string{"t.business_id = $1"}
	args := []any{actor.VendorID}
	bind := func(value any) string {
		args = append(args, value)
		return "$" + itoa(len(args))
	}

	if !scope.Global {
		where = append(where, "(t.scope_type = 'BUSINESS' OR t.branch_id = ANY("+bind(scope.BranchIDs)+"))")
	}
	if opts.ScopeType != "" {
		where = append(where, "t.scope_type = "+bind(opts.ScopeType))
	}
	if opts.Status != "" {
		where = append(where, "t.status = "+bind(opts.Status))
	} else {
		where = append(where, "t.status = 'ACTIVE'")
	}
	if opts.BranchRef != "" {
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, nil, false, errScopeDenied(CodeTagScopeDenied)
		}
		where = append(where, "t.branch_id = "+bind(opts.BranchRef))
	}
	if opts.Search != "" {
		// Parameterised prefix/substring match on the normalised name. The
		// pattern is a bind parameter, never concatenated SQL, and the caller
		// cannot choose the column or the operator.
		where = append(where, "t.normalized_name LIKE "+bind("%"+escapeLike(NormalizeTagName(opts.Search))+"%")+" ESCAPE '\\'")
	}

	limit := bind(opts.Limit + 1)
	offset := bind(opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+tagCols+` FROM crm_tags t
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY t.normalized_name ASC, t.id ASC LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, nil, false, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		tag, scanErr := scanTag(rows)
		if scanErr != nil {
			return nil, nil, false, scanErr
		}
		tags = append(tags, tag)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, false, err
	}
	hasMore := len(tags) > opts.Limit
	if hasMore {
		tags = tags[:opts.Limit]
	}
	counts, err := s.tagMemberCounts(ctx, actor.VendorID, scope, tags)
	if err != nil {
		return nil, nil, false, err
	}
	return tags, counts, hasMore, nil
}

// tagMemberCounts counts memberships per tag, filtered to what the caller may
// see. A branch-scoped caller's count covers their own branches only, so a
// business-wide total can never be inferred from a branch view.
func (s *Service) tagMemberCounts(ctx context.Context, businessID string, scope Scope, tags []Tag) (map[string]int, error) {
	counts := map[string]int{}
	if len(tags) == 0 {
		return counts, nil
	}
	ids := make([]string, 0, len(tags))
	for _, tag := range tags {
		ids = append(ids, tag.ID)
	}
	query := `SELECT m.tag_id, count(*) FROM crm_customer_tag_memberships m
		JOIN vendor_customer_profiles p ON p.id = m.profile_id AND p.business_id = m.business_id
		WHERE m.business_id = $1 AND m.tag_id = ANY($2)
		  AND p.privacy_state <> 'REDACTED' AND p.merge_state <> 'MERGED_SOURCE'`
	args := []any{businessID, ids}
	if !scope.Global {
		query += ` AND EXISTS (SELECT 1 FROM crm_customer_branch_relations br
			WHERE br.business_id = p.business_id AND br.profile_id = p.id AND br.branch_id = ANY($3))`
		args = append(args, scope.BranchIDs)
	}
	query += ` GROUP BY m.tag_id`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

// GetTag loads one tag, enforcing tenancy and scope.
func (s *Service) GetTag(ctx context.Context, actor Actor, scope Scope, tagID string) (Tag, error) {
	tag, err := s.loadTag(ctx, s.db, actor.VendorID, tagID)
	if err != nil {
		return Tag{}, err
	}
	if err = s.assertTagVisible(scope, tag); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func (s *Service) loadTag(ctx context.Context, q querier, businessID, tagID string) (Tag, error) {
	tag, err := scanTag(q.QueryRow(ctx, `SELECT `+tagCols+` FROM crm_tags WHERE business_id=$1 AND id=$2`,
		businessID, tagID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, errTagNotFound()
	}
	return tag, err
}

// assertTagVisible reports not-found rather than forbidden for a tag outside
// the caller's scope, so probing ids cannot confirm another branch's taxonomy.
func (s *Service) assertTagVisible(scope Scope, tag Tag) error {
	if scope.Global || tag.ScopeType == ScopeBusiness {
		return nil
	}
	if tag.BranchID != nil && containsString(scope.BranchIDs, *tag.BranchID) {
		return nil
	}
	return errTagNotFound()
}

// ---------------------------------------------------------------------------
// Tag commands
// ---------------------------------------------------------------------------

// CreateTagInput is a vendor request to define a tag. There is deliberately no
// business_id field: tenancy comes from the authenticated session.
type CreateTagInput struct {
	Name        string
	Description string
	ScopeType   string
	BranchRef   string
}

// CreateTag defines a new tag.
func (s *Service) CreateTag(ctx context.Context, actor Actor, scope Scope, in CreateTagInput) (Tag, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Tag{}, err
	}
	name, normalized, err := ValidateTagName(in.Name)
	if err != nil {
		return Tag{}, err
	}
	description, err := validateTagDescription(in.Description)
	if err != nil {
		return Tag{}, err
	}
	scopeType := strings.ToUpper(strings.TrimSpace(in.ScopeType))
	if scopeType == "" {
		scopeType = ScopeBusiness
	}
	var branchID *string
	if branch := strings.TrimSpace(in.BranchRef); branch != "" {
		branchID = &branch
	}
	if err = authorizeObjectScope(scope, scopeType, branchID, CodeTagScopeDenied); err != nil {
		return Tag{}, err
	}
	if scopeType == ScopeBusiness {
		branchID = nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Tag{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if branchID != nil {
		owned, ownErr := s.branchBelongsToBusiness(ctx, tx, actor.VendorID, *branchID)
		if ownErr != nil {
			return Tag{}, ownErr
		}
		if !owned {
			return Tag{}, crmError(apperrors.Forbidden("That branch does not belong to this business.", nil), CodeCrossTenant)
		}
	}

	tag, err := scanTag(tx.QueryRow(ctx, `INSERT INTO crm_tags
		(id,business_id,scope_type,branch_id,name,normalized_name,description,status,created_by_staff_ref)
		VALUES($1,$2,$3,$4,$5,$6,$7,'ACTIVE',NULLIF($8,'')) RETURNING `+tagCols,
		newID("crmtag_"), actor.VendorID, scopeType, branchID, name, normalized, description, actor.StaffID))
	if err != nil {
		if isUniqueViolation(err) {
			return Tag{}, errTagExists()
		}
		return Tag{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.tag.created", "crm_tag", tag.ID, derefOr(branchID),
		nil, map[string]any{"name": tag.Name, "scope_type": tag.ScopeType}, ""); err != nil {
		return Tag{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.tag.created", "crm_tag", tag.ID, tag.Version,
		map[string]any{"tag_ref": tag.ID, "scope_type": tag.ScopeType, "branch_ref": branchID}); err != nil {
		return Tag{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Tag{}, err
	}
	s.metrics.Inc("crm_tags_total", "scope", scopeType)
	return tag, nil
}

// UpdateTagInput renames or re-describes a tag. Scope is immutable: moving a
// tag between branches would silently reinterpret every existing membership.
type UpdateTagInput struct {
	Name            *string
	Description     *string
	ExpectedVersion int64
}

// UpdateTag renames or re-describes a tag.
func (s *Service) UpdateTag(ctx context.Context, actor Actor, scope Scope, tagID string, in UpdateTagInput) (Tag, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Tag{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Tag{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadTagForUpdate(ctx, tx, actor.VendorID, tagID)
	if err != nil {
		return Tag{}, err
	}
	if err = s.assertTagVisible(scope, current); err != nil {
		return Tag{}, err
	}
	if err = authorizeObjectScope(scope, current.ScopeType, current.BranchID, CodeTagScopeDenied); err != nil {
		return Tag{}, err
	}
	if current.IsArchived() {
		return Tag{}, errTagArchived()
	}
	if in.ExpectedVersion > 0 && current.Version != in.ExpectedVersion {
		return Tag{}, errCRMVersionConflict()
	}

	name, normalized := current.Name, current.NormalizedName
	if in.Name != nil {
		if name, normalized, err = ValidateTagName(*in.Name); err != nil {
			return Tag{}, err
		}
	}
	description := current.Description
	if in.Description != nil {
		if description, err = validateTagDescription(*in.Description); err != nil {
			return Tag{}, err
		}
	}

	updated, err := scanTag(tx.QueryRow(ctx, `UPDATE crm_tags
		SET name=$1, normalized_name=$2, description=$3, updated_at=now(), version=version+1
		WHERE business_id=$4 AND id=$5 AND version=$6 AND status='ACTIVE' RETURNING `+tagCols,
		name, normalized, description, actor.VendorID, tagID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, errCRMVersionConflict()
	}
	if err != nil {
		if isUniqueViolation(err) {
			return Tag{}, errTagExists()
		}
		return Tag{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.tag.updated", "crm_tag", tagID, derefOr(current.BranchID),
		map[string]any{"name": current.Name}, map[string]any{"name": updated.Name}, ""); err != nil {
		return Tag{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.tag.updated", "crm_tag", tagID, updated.Version,
		map[string]any{"tag_ref": tagID, "scope_type": updated.ScopeType}); err != nil {
		return Tag{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Tag{}, err
	}
	return updated, nil
}

// ArchiveTag retires a tag.
//
// Archiving deactivates current use without destroying history: the tag row and
// its audit trail survive, and existing memberships are removed so the tag
// stops selecting customers, which is the whole point of retiring it. The
// removal is recorded as a count rather than as hundreds of audit rows.
func (s *Service) ArchiveTag(ctx context.Context, actor Actor, scope Scope, tagID string, expectedVersion int64) (Tag, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Tag{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Tag{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.loadTagForUpdate(ctx, tx, actor.VendorID, tagID)
	if err != nil {
		return Tag{}, err
	}
	if err = s.assertTagVisible(scope, current); err != nil {
		return Tag{}, err
	}
	if err = authorizeObjectScope(scope, current.ScopeType, current.BranchID, CodeTagScopeDenied); err != nil {
		return Tag{}, err
	}
	if current.IsArchived() {
		return current, nil
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return Tag{}, errCRMVersionConflict()
	}

	removed, err := tx.Exec(ctx, `DELETE FROM crm_customer_tag_memberships WHERE business_id=$1 AND tag_id=$2`,
		actor.VendorID, tagID)
	if err != nil {
		return Tag{}, err
	}
	archived, err := scanTag(tx.QueryRow(ctx, `UPDATE crm_tags SET status='ARCHIVED', updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2 AND version=$3 RETURNING `+tagCols,
		actor.VendorID, tagID, current.Version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, errCRMVersionConflict()
	}
	if err != nil {
		return Tag{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.crm.tag.archived", "crm_tag", tagID, derefOr(current.BranchID),
		map[string]any{"status": current.Status},
		map[string]any{"status": TagArchived, "memberships_removed": removed.RowsAffected()}, ""); err != nil {
		return Tag{}, err
	}
	if err = s.invalidateSegmentsForDependency(ctx, tx, actor.VendorID, "crm.tags"); err != nil {
		return Tag{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.tag.archived", "crm_tag", tagID, archived.Version,
		map[string]any{"tag_ref": tagID, "memberships_removed": removed.RowsAffected()}); err != nil {
		return Tag{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Tag{}, err
	}
	return archived, nil
}

func (s *Service) loadTagForUpdate(ctx context.Context, tx pgx.Tx, businessID, tagID string) (Tag, error) {
	tag, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagCols+` FROM crm_tags
		WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessID, tagID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, errTagNotFound()
	}
	return tag, err
}

// ---------------------------------------------------------------------------
// Bulk membership
// ---------------------------------------------------------------------------

// BulkTagResult reports what a bulk operation did.
type BulkTagResult struct {
	TagRef    string `json:"tag_ref"`
	Requested int    `json:"requested"`
	Applied   int    `json:"applied"`
	Skipped   int    `json:"skipped"`
}

// BulkAssignTag adds one tag to many relationships, all or nothing.
//
// Every profile is validated before anything is written, and the profile rows
// are locked for the duration, so a redaction committing concurrently either
// happens before this transaction (and the assignment is refused) or after it
// (and the redaction handler removes the membership it finds). There is no
// interleaving in which a redacted customer keeps a tag.
func (s *Service) BulkAssignTag(ctx context.Context, actor Actor, scope Scope, tagID string, profileIDs []string) (BulkTagResult, error) {
	return s.bulkTag(ctx, actor, scope, tagID, profileIDs, true)
}

// BulkRemoveTag removes one tag from many relationships.
func (s *Service) BulkRemoveTag(ctx context.Context, actor Actor, scope Scope, tagID string, profileIDs []string) (BulkTagResult, error) {
	return s.bulkTag(ctx, actor, scope, tagID, profileIDs, false)
}

func (s *Service) bulkTag(ctx context.Context, actor Actor, scope Scope, tagID string, profileIDs []string, assign bool) (BulkTagResult, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return BulkTagResult{}, err
	}
	ids, err := normalizeProfileRefs(profileIDs)
	if err != nil {
		return BulkTagResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BulkTagResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := s.loadTag(ctx, tx, actor.VendorID, tagID)
	if err != nil {
		return BulkTagResult{}, err
	}
	if err = s.assertTagVisible(scope, tag); err != nil {
		return BulkTagResult{}, err
	}
	if err = authorizeObjectScope(scope, tag.ScopeType, tag.BranchID, CodeTagScopeDenied); err != nil {
		return BulkTagResult{}, err
	}
	if assign && tag.IsArchived() {
		return BulkTagResult{}, errTagArchived()
	}

	if err = s.assertProfilesAssignable(ctx, tx, actor, scope, ids, tag.BranchID); err != nil {
		return BulkTagResult{}, err
	}

	result := BulkTagResult{TagRef: tagID, Requested: len(ids)}
	if assign {
		// Ids are minted in Go and unnested alongside the profile references, so
		// the statement needs no database uuid extension and stays one round
		// trip regardless of batch size.
		membershipIDs := make([]string, len(ids))
		for index := range ids {
			membershipIDs[index] = newID("crmtm_")
		}
		inserted, insertErr := tx.Exec(ctx, `INSERT INTO crm_customer_tag_memberships
			(id,business_id,profile_id,tag_id,branch_id,source_type,assigned_by_staff_ref)
			SELECT m.id, $1, m.profile_id, $2, $3, 'MANUAL', NULLIF($4,'')
			FROM unnest($5::text[], $6::text[]) AS m(id, profile_id)
			ON CONFLICT (business_id, tag_id, profile_id) DO NOTHING`,
			actor.VendorID, tagID, tag.BranchID, actor.StaffID, membershipIDs, ids)
		if insertErr != nil {
			return BulkTagResult{}, insertErr
		}
		result.Applied = int(inserted.RowsAffected())
	} else {
		removed, removeErr := tx.Exec(ctx, `DELETE FROM crm_customer_tag_memberships
			WHERE business_id=$1 AND tag_id=$2 AND profile_id = ANY($3)`, actor.VendorID, tagID, ids)
		if removeErr != nil {
			return BulkTagResult{}, removeErr
		}
		result.Applied = int(removed.RowsAffected())
	}
	result.Skipped = result.Requested - result.Applied

	action := "vendor.crm.tag.bulk_removed"
	if assign {
		action = "vendor.crm.tag.bulk_assigned"
	}
	// The audit entry records the batch shape, not every profile reference: the
	// membership rows themselves are the per-customer record, and a single audit
	// row carrying hundreds of customer ids would be both unreadable and a
	// needless second copy of the relationship data.
	if err = s.auditTx(ctx, tx, actor, action, "crm_tag", tagID, derefOr(tag.BranchID), nil,
		map[string]any{"requested": result.Requested, "applied": result.Applied, "skipped": result.Skipped}, ""); err != nil {
		return BulkTagResult{}, err
	}
	if err = s.invalidateSegmentsForDependency(ctx, tx, actor.VendorID, "crm.tags"); err != nil {
		return BulkTagResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BulkTagResult{}, err
	}
	s.metrics.Add("crm_tag_assignments_total", float64(result.Applied), "operation", map[bool]string{true: "assign", false: "remove"}[assign])
	return result, nil
}

// assertProfilesAssignable validates every referenced relationship before any
// mutation, locking the rows so the decision cannot be invalidated mid-request.
//
// Failures are reported as a single not-found rather than as a per-id report:
// telling a caller which of the ids they guessed actually exist would turn a
// bulk endpoint into an enumeration oracle across tenants and branches.
func (s *Service) assertProfilesAssignable(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, ids []string, branchID *string) error {
	rows, err := tx.Query(ctx, `SELECT id, privacy_state, merge_state, status FROM vendor_customer_profiles
		WHERE business_id=$1 AND id = ANY($2) ORDER BY id FOR UPDATE`, actor.VendorID, ids)
	if err != nil {
		return err
	}
	found := map[string]struct{}{}
	type state struct{ privacy, merge, status string }
	states := map[string]state{}
	for rows.Next() {
		var id string
		var st state
		if err = rows.Scan(&id, &st.privacy, &st.merge, &st.status); err != nil {
			rows.Close()
			return err
		}
		found[id] = struct{}{}
		states[id] = st
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(found) != len(ids) {
		return errProfileNotFound()
	}
	for _, id := range ids {
		switch st := states[id]; {
		case st.privacy == string(PrivacyRedacted):
			return crmError(apperrors.Conflict("One or more customers have been redacted and cannot be tagged.", nil), CodeProfileRedacted)
		case st.merge == string(MergeSource):
			return crmError(apperrors.Conflict("One or more customers have been merged and cannot be tagged independently.", nil), CodeProfileMerged)
		case st.status == string(StatusArchived):
			return crmError(apperrors.Conflict("One or more customers are archived and cannot be tagged.", nil), CodeSegmentInvalidState)
		}
	}
	return s.assertProfilesInScope(ctx, tx, actor.VendorID, scope, ids, branchID)
}

// assertProfilesInScope enforces branch visibility for every referenced
// relationship in one query.
func (s *Service) assertProfilesInScope(ctx context.Context, q querier, businessID string, scope Scope, ids []string, branchID *string) error {
	required := scope.BranchIDs
	if scope.Global {
		required = nil
	}
	if branchID != nil {
		// A branch-scoped object may only touch relationships that exist at that
		// branch, regardless of how wide the actor's own grant is.
		required = []string{*branchID}
	}
	if len(required) == 0 {
		return nil
	}
	var visible int
	if err := q.QueryRow(ctx, `SELECT count(DISTINCT p.id) FROM vendor_customer_profiles p
		WHERE p.business_id=$1 AND p.id = ANY($2)
		  AND EXISTS (SELECT 1 FROM crm_customer_branch_relations br
			WHERE br.business_id=p.business_id AND br.profile_id=p.id AND br.branch_id = ANY($3))`,
		businessID, ids, required).Scan(&visible); err != nil {
		return err
	}
	if visible != len(ids) {
		return errProfileNotFound()
	}
	return nil
}

// normalizeProfileRefs de-duplicates and bounds a bulk reference list.
func normalizeProfileRefs(refs []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		if !safeToken.MatchString(trimmed) {
			return nil, tagValidation("profile_refs", "One or more customer references are not valid.")
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil, tagValidation("profile_refs", "At least one customer is required.")
	}
	if len(out) > MaxBulkBatch {
		return nil, errBulkTooLarge(MaxBulkBatch)
	}
	return out, nil
}

// TagsForProfile lists the tags assigned to one relationship, filtered to the
// caller's scope.
func (s *Service) TagsForProfile(ctx context.Context, actor Actor, scope Scope, profileID string) ([]Tag, error) {
	// The column list is qualified here because the join brings a second table
	// with overlapping column names into scope.
	query := `SELECT ` + qualifiedTagCols + ` FROM crm_tags t
		JOIN crm_customer_tag_memberships m ON m.tag_id = t.id AND m.business_id = t.business_id
		WHERE t.business_id=$1 AND m.profile_id=$2`
	args := []any{actor.VendorID, profileID}
	if !scope.Global {
		// Customer 360 is a customer-classification disclosure, not the tag
		// management catalogue. A branch actor therefore sees only labels owned
		// by an authorized branch: business-wide and other-branch classifications
		// are both withheld even though the main tag API keeps its existing model.
		query += ` AND t.scope_type = 'BRANCH' AND t.branch_id = ANY($3)`
		args = append(args, scope.BranchIDs)
	}
	query += ` ORDER BY t.normalized_name ASC`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []Tag
	for rows.Next() {
		tag, scanErr := scanTag(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func errCRMVersionConflict() error {
	return crmError(apperrors.Conflict("This record changed since you loaded it. Reload and try again.", nil), CodeVersionConflict)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func derefOr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// escapeLike neutralises LIKE metacharacters in a search term so a search for
// "50%" means the literal text, not "anything starting with 50".
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
