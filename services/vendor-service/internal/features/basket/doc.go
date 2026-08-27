// Package basket is customer shopping optimization: what a basket would cost
// to fulfil, and whether a better one exists.
//
// Three capabilities, one engine:
//
//	OPTIMIZE MY CART   given the customer's current cart, is there an
//	                   equivalent basket from fewer or cheaper vendors that
//	                   costs less delivered?
//
//	BUDGET SHOPPING    given a budget and a list of things the customer needs,
//	                   what is the best basket that fits - and if none fits,
//	                   exactly how far short is it and why?
//
//	RECURRING BASKET   given a stored set of REQUIREMENTS (not a stored set of
//	                   old SKUs), what should this week's delivery be, priced
//	                   against today's catalogue?
//
// # What this package is not allowed to do
//
// The constraints are the design, not a caveat on it:
//
//   - It never mutates the customer's cart. Every result is a PROPOSAL.
//     Acceptance is a separate, explicit act that creates a NEW cart version.
//
//   - It never invents a product, a price, a stock level, a vendor or an
//     equivalence. Every candidate is a real active listing with an
//     authoritative price and authoritative sellable stock behind it.
//
//   - It never estimates delivery. Every basket it compares is planned by the
//     quote-only fulfilment planner against real logistics quotes, because a
//     comparison between two baskets is mostly a comparison between their
//     delivery costs and a made-up per-vendor fee would decide it wrongly.
//
//   - It creates no missions, no payment intents, no reservations and no
//     orders. Evaluating a hundred candidate baskets creates exactly zero of
//     each, and the tests assert that by counting.
//
//   - It is deterministic. No randomness, no learned model, no weighted score
//     nobody can explain. Same catalogue, same stock, same prices, same
//     policy version, same answer.
//
// # Optimality
//
// The search is BOUNDED. A catalogue-wide exhaustive search over every
// combination of every listing is exponential and would be a denial-of-service
// vector wearing an optimizer's name, so the bounds are configured and
// enforced.
//
// The consequence is stated rather than hidden: this package finds the best
// valid candidate under the configured search, and says so in exactly those
// words. When a bound actually truncated the search it says that too. It
// never claims to have found the provably cheapest basket, because for most
// inputs it has not looked at every basket.
package basket
