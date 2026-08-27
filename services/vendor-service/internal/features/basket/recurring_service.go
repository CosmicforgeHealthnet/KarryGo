package basket

// Template management and the recurrence sweep.
//
// The sweep is the only thing in this package that runs without a customer
// asking, so it is the only thing that could do something a customer did not
// request. What it may do is bounded to exactly one act: computing a proposal
// and recording it.
//
// It never creates an order, never charges anything, never touches a cart and
// never bypasses an approval policy. The most it can produce is a suggestion
// marked AUTO_APPROVED, which means "this occurrence is inside the bounded
// preauthorization the customer themselves configured" - not "this has been
// bought".

import (
	"context"
	"strings"
	"time"
)

// AddressResolver turns a customer's own saved-location reference into
// planning coordinates.
//
// A separate port from the cart authority because a recurring template ships
// to a saved address without a cart existing. Resolution is by REFERENCE and
// scoped to the customer, so a template can never be made to quote a delivery
// to coordinates its owner does not have.
type AddressResolver interface {
	ResolveSavedAddress(
		ctx context.Context,
		customerID, addressRef string,
	) (Destination, error)
}

// WithAddressResolver attaches the saved-address authority.
func (s *Service) WithAddressResolver(resolver AddressResolver) *Service {
	s.addresses = resolver

	return s
}

// resolveCustomerAddress turns a customer saved-location reference into
// planning coordinates.
//
// Deliberately the only way a destination enters this feature. Accepting raw
// coordinates from a request body would let anyone price a delivery to an
// address they have no connection to, and would leak serviceability
// information about arbitrary points on the map.
func (s *Service) resolveCustomerAddress(
	ctx context.Context,
	customerID, addressRef string,
) (Destination, error) {
	if s.addresses == nil {
		return Destination{}, ErrDependencyMissing
	}

	addressRef = strings.TrimSpace(addressRef)

	if addressRef == "" {
		return Destination{}, ErrNoDestination
	}

	destination, err := s.addresses.ResolveSavedAddress(
		ctx, customerID, addressRef,
	)
	if err != nil {
		return Destination{}, err
	}

	if !destination.Resolved() {
		return Destination{}, ErrNoDestination
	}

	return destination, nil
}

// CreateTemplate stores a new recurring basket.
func (s *Service) CreateTemplate(
	ctx context.Context,
	template Template,
) (Template, error) {
	if s.store == nil {
		return Template{}, ErrDependencyMissing
	}

	normalized, err := template.Normalize()
	if err != nil {
		return Template{}, err
	}

	return s.store.CreateTemplate(ctx, normalized)
}

// UpdateTemplate applies an edit under an expected version.
func (s *Service) UpdateTemplate(
	ctx context.Context,
	template Template,
	expectedVersion int64,
) (Template, error) {
	if s.store == nil {
		return Template{}, ErrDependencyMissing
	}

	normalized, err := template.Normalize()
	if err != nil {
		return Template{}, err
	}

	// The stored template is read first so an edit can never move a template
	// between customers, whatever the request body says.
	existing, err := s.store.GetTemplate(
		ctx, template.CustomerID, template.ID,
	)
	if err != nil {
		return Template{}, err
	}

	normalized.ID = existing.ID
	normalized.CustomerID = existing.CustomerID

	return s.store.UpdateTemplate(ctx, normalized, expectedVersion)
}

// GetTemplate reads one template the customer owns.
func (s *Service) GetTemplate(
	ctx context.Context,
	customerID, templateID string,
) (Template, error) {
	if s.store == nil {
		return Template{}, ErrDependencyMissing
	}

	return s.store.GetTemplate(ctx, customerID, templateID)
}

// ListTemplates reads a customer's recurring baskets.
func (s *Service) ListTemplates(
	ctx context.Context,
	customerID string,
	limit int,
) ([]Template, error) {
	if s.store == nil {
		return nil, ErrDependencyMissing
	}

	return s.store.ListTemplates(ctx, customerID, limit)
}

// SetTemplateEnabled pauses or resumes a recurring basket.
func (s *Service) SetTemplateEnabled(
	ctx context.Context,
	customerID, templateID string,
	enabled bool,
	expectedVersion int64,
) (Template, error) {
	if s.store == nil {
		return Template{}, ErrDependencyMissing
	}

	return s.store.SetTemplateEnabled(
		ctx, customerID, templateID, enabled, expectedVersion,
	)
}

// ListOccurrences reads a template's generation history.
func (s *Service) ListOccurrences(
	ctx context.Context,
	customerID, templateID string,
	limit int,
) ([]Generation, error) {
	if s.store == nil {
		return nil, ErrDependencyMissing
	}

	// Ownership is re-checked rather than inferred from the template id. A
	// generation carries prices, vendors and a delivery plan, and reaching
	// one through a foreign template id must not be possible.
	if _, err := s.store.GetTemplate(ctx, customerID, templateID); err != nil {
		return nil, err
	}

	return s.store.ListOccurrences(ctx, customerID, templateID, limit)
}

// MissedOccurrencePolicy is what happens to an occurrence nobody generated in
// time.
//
// The choice made here is SKIP, and it is deliberate. A worker that was down
// for a day must not wake up and generate a backlog of yesterday's shopping,
// because a recurring basket that catches up is a recurring basket that
// delivers twice. The customer's next scheduled occurrence generates
// normally; the missed one is simply not resurrected.
//
// The window is how much lateness still counts as "this occurrence" rather
// than as a missed one. Fifteen minutes covers an ordinary restart or a slow
// tick without covering an outage.
const MissedOccurrenceWindow = 15 * time.Minute

// SweepRecurringTemplates generates any occurrence that has just fallen due.
//
// It is safe to run on any number of workers at any interval. Every
// occurrence is claimed through a unique index before any work is done, so
// concurrent sweeps produce one generation between them, and a sweep that
// crashes after claiming finds its own claim on the next pass rather than
// duplicating it.
func (s *Service) SweepRecurringTemplates(
	ctx context.Context,
	limit int,
) (int64, error) {
	if s.store == nil {
		return 0, ErrDependencyMissing
	}

	templates, err := s.store.ListDueTemplates(ctx, limit)
	if err != nil {
		return 0, err
	}

	now := s.now().UTC()

	var generated int64

	for _, template := range templates {
		occurrence, due := s.dueOccurrence(template, now)

		if !due {
			continue
		}

		destination, resolveErr := s.resolveTemplateDestination(ctx, template)

		if resolveErr != nil {
			// A template whose address no longer resolves cannot be planned.
			// It is skipped rather than failing the whole sweep: one
			// customer's deleted saved location must not stop everybody
			// else's groceries.
			continue
		}

		if _, genErr := s.GenerateOccurrence(
			ctx, template, occurrence, destination,
		); genErr != nil {
			continue
		}

		generated++
	}

	return generated, nil
}

// dueOccurrence finds the scheduled moment that has just passed, if any.
//
// It looks BACKWARD from now rather than forward, because a sweep runs after
// the moment it is generating for. The window bounds how far back it will
// look, which is what turns "the worker was down for a day" into a skipped
// occurrence rather than a backlog.
func (s *Service) dueOccurrence(
	template Template,
	now time.Time,
) (time.Time, bool) {
	// The next occurrence after (now - window). If that lands at or before
	// now, it has just fallen due and has not yet been generated.
	occurrence, err := template.NextOccurrence(now.Add(-MissedOccurrenceWindow))

	if err != nil {
		return time.Time{}, false
	}

	if occurrence.After(now) {
		return time.Time{}, false
	}

	return occurrence, true
}

// resolveTemplateDestination turns the template's saved-address reference
// into planning coordinates.
func (s *Service) resolveTemplateDestination(
	ctx context.Context,
	template Template,
) (Destination, error) {
	if s.addresses == nil {
		return Destination{}, ErrDependencyMissing
	}

	destination, err := s.addresses.ResolveSavedAddress(
		ctx, template.CustomerID, strings.TrimSpace(template.AddressRef),
	)
	if err != nil {
		return Destination{}, err
	}

	if !destination.Resolved() {
		return Destination{}, ErrNoDestination
	}

	return destination, nil
}

// GenerateNow produces the occurrence for a template on demand.
//
// It exists for the customer-facing "shop this list now" action and for
// operators re-running a template after fixing a problem. It goes through
// exactly the same claim-first path as the sweep, so a manual run and a
// scheduled run competing for one occurrence still produce one generation.
func (s *Service) GenerateNow(
	ctx context.Context,
	customerID, templateID string,
) (Generation, error) {
	if s.store == nil {
		return Generation{}, ErrDependencyMissing
	}

	template, err := s.store.GetTemplate(ctx, customerID, templateID)
	if err != nil {
		return Generation{}, err
	}

	destination, err := s.resolveTemplateDestination(ctx, template)
	if err != nil {
		return Generation{}, err
	}

	now := s.now().UTC()

	occurrence, due := s.dueOccurrence(template, now)

	if !due {
		// No scheduled occurrence has fallen due, so this is an ad-hoc run.
		// It is keyed to the current minute, which keeps two rapid taps of
		// the same button collapsing into one generation while remaining
		// distinct from the scheduled occurrences around it.
		occurrence = now.Truncate(time.Minute)
	}

	return s.GenerateOccurrence(ctx, template, occurrence, destination)
}
