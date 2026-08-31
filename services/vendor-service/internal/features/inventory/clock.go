package inventory

import "time"

// Clock provides deterministic time for future Inventory expiry and worker
// operations.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

// FixedClock provides deterministic test time.
type FixedClock struct {
	Time time.Time
}

// Now returns the fixed instant in UTC.
func (c FixedClock) Now() time.Time {
	return c.Time.UTC()
}
