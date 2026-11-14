package models

// Settlement-delivery vocabulary (FLEET-13.11) shared by the use cases, the
// repository and the money-authority adapter in internal/app.

// JobManagedSettlementDelivery is the durable job that sends one instruction.
const JobManagedSettlementDelivery = "SECTION12_MANAGED_SETTLEMENT_DELIVERY"

// Instruction statuses.
const (
	InstructionCreated                = "CREATED"
	InstructionPendingWallet          = "PENDING_WALLET"
	InstructionReconciliationRequired = "RECONCILIATION_REQUIRED"
	InstructionPosted                 = "POSTED"
	InstructionFailedTerminal         = "FAILED_TERMINAL"
	InstructionSuperseded             = "SUPERSEDED"
	InstructionCancelled              = "CANCELLED"
)

// Delivery dispositions reported by the money-authority port. They match the
// financeops posting dispositions one for one.
const (
	SettlementPosted         = "POSTED"
	SettlementRetry          = "RETRY"
	SettlementOutcomeUnknown = "OUTCOME_UNKNOWN"
	SettlementRefused        = "REFUSED"
	SettlementUnavailable    = "UNAVAILABLE"
)

// SettlementSubmission is one instruction as the money authority receives it.
// Every figure is copied from the committed instruction; nothing is
// recomputed at delivery time.
type SettlementSubmission struct {
	InstructionID      string
	InstructionVersion int64

	OrganizationID string
	MembershipID   string
	ProviderID     string
	ProviderType   string

	SourceService      string
	SourceJobID        string
	SourceAssignmentID string

	// PaymentSourceReference is the reference the execution service created
	// the job's payment under, when it is not SourceJobID. Empty otherwise.
	PaymentSourceReference string

	Currency               string
	GrossBasisMinor        int64
	ProviderShareMinor     int64
	OrganizationShareMinor int64

	CompensationPlanID            string
	CompensationPlanVersion       int64
	CompensationResolutionVersion int64

	CorrelationID  string
	IdempotencyKey string
}

// SettlementOutcome is what one submission established.
type SettlementOutcome struct {
	OperationID      string
	Disposition      string
	Reference        string
	ConfirmedVersion int64
	ErrorCode        string
}

// UndeliveredInstruction identifies one instruction the sweep must deliver.
type UndeliveredInstruction struct{ OrganizationID, InstructionID string }

// ProviderMembership is the live membership a provider holds, as the
// providers feature reports it.
type ProviderMembership struct {
	ID               string
	OrganizationID   string
	ProviderService  string
	ServiceCategory  string
	RelationshipType string
	Status           string
	Version          int64
}
