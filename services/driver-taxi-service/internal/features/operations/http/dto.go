package operationshttp

type adminActionRequest struct {
	// OperatorID identifies the STAFF member who initiated this action, as
	// asserted by the calling service (this platform has no staff-identity
	// passthrough on the HMAC service-auth channel itself -- see
	// bookingusecases' own ListLiveOperations doc comment on the same
	// limitation). Falls back to the verified calling service's own name
	// when omitted, so an action is never attributed to nobody.
	OperatorID string `json:"operator_id"`
	Reason     string `json:"reason"`
}
