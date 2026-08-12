package platformclients

// HaulingExecution is Fleet's live adapter to the authoritative truck-hauling
// marketplace. It deliberately shares the mature request/response mapping
// with Dispatch: both authorities implement the same provider-neutral Fleet
// execution contract, while retaining separate URLs, credentials and data.
type HaulingExecution struct {
	*DispatchExecution
}

const haulingInternalBase = "/api/v1/hauling/internal/marketplace"

// NewHaulingExecution builds a signed client for Hauling's private surface.
func NewHaulingExecution(client *Client) *HaulingExecution {
	return &HaulingExecution{DispatchExecution: &DispatchExecution{
		client: client, apiBase: haulingInternalBase, category: CategoryHauling,
	}}
}

var _ ExecutionContract = (*HaulingExecution)(nil)
