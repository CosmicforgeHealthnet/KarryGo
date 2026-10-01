# Settlement lag

Trigger: vendor settlement age/backlog exceeds SLO. Inspect Vendor allocation projection, Wallet operation IDs/status, durable retries, and relevant outbox age. Pause new settlement claims if errors are systemic, query Payment Wallet, and reconcile with the same deterministic operation ID. Never credit a vendor by editing Vendor data or issue a duplicate settlement. Escalate to Wallet when status is UNKNOWN beyond SLO or allocations disagree. Verify Wallet terminal state, Vendor projection agreement, one ledger operation per allocation, and drained backlog.
