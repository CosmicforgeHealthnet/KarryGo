package platformclients

// TaxiExecution is Fleet's live adapter to the authoritative taxi execution
// service (driver-taxi-service).
//
// Like Hauling, it shares the mature request/response mapping with Dispatch:
// all three authorities implement the same provider-neutral Fleet execution
// contract, while keeping separate URLs, credentials and data. Taxi exposes
// that contract under its own internal marketplace base.
//
// This replaces UnimplementedExecution for TAXI, but only where the
// deployment has explicitly enabled the Taxi source AND supplied Taxi's URL
// and secret. Section 8 TAX-028 requires the Taxi source to stay dark until
// the whole corporate chain is certified, so BuildExecutionSet keeps handing
// back UnimplementedExecution unless both conditions hold.
type TaxiExecution struct {
	*DispatchExecution
}

// taxiInternalBase is the base path of driver-taxi-service's internal
// marketplace surface. It mirrors dispatch's shape under taxi's own API root.
const taxiInternalBase = "/api/v1/taxi/internal/marketplace"

// NewTaxiExecution builds a signed client for Taxi's private surface.
func NewTaxiExecution(client *Client) *TaxiExecution {
	return &TaxiExecution{DispatchExecution: &DispatchExecution{
		client: client, apiBase: taxiInternalBase, category: CategoryTaxi,
	}}
}

var _ ExecutionContract = (*TaxiExecution)(nil)
