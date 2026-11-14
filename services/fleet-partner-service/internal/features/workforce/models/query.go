package models

import "time"

// ListQuery selects one page of a workforce list. Offset pages keep their
// historical shape; keyset pages carry the last row of the previous page in
// After/AfterID (nil After starts at the first row) and confine the rows to
// BranchScope in SQL, so a page is never short because rows were filtered
// after the LIMIT.
type ListQuery struct {
	BranchID, Status string
	MembershipID     string
	Limit, Offset    int
	Keyset           bool
	After            *time.Time
	AfterID          string
	BranchScope      []string
}

// VersionedID is one row a maintenance transition moved.
type VersionedID struct {
	ID      string
	Version int64
}
