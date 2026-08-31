package taxonomy

import "errors"

var (
	ErrNotFound        = errors.New("taxonomy: not found")
	ErrConflict        = errors.New("taxonomy: conflict")
	ErrInvalidArgument = errors.New("taxonomy: invalid argument")
	ErrNotDraft        = errors.New("taxonomy: version is not draft")
	ErrVersionMismatch = errors.New("taxonomy: revision mismatch")
	ErrIdempotency     = errors.New("taxonomy: idempotency conflict")

	// Section 13I API-surface errors. ErrForbidden is a denied permission;
	// ErrNamespaceMismatch is an identifier addressed through the wrong
	// canonical namespace (a Food ref against SHOP, or the reverse) and is
	// kept distinct from a plain conflict so the FOOD/SHOP isolation gates can
	// assert on it rather than on a message.
	ErrForbidden         = errors.New("taxonomy: forbidden")
	ErrNamespaceMismatch = errors.New("taxonomy: namespace mismatch")
)
