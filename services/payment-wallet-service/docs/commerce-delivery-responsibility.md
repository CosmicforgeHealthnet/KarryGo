# Commerce delivery responsibility transition

Wallet owns the funded pre-custody transition from Vendor-owned delivery to a
platform provider. The command is internal HMAC v2, idempotent and recoverable
by operation reference.

The original checkout allocation, customer contribution and subsidy fields
remain immutable evidence. Effective responsibility/payable fields determine
future settlement. The Vendor delivery payable becomes zero and the provider
payable becomes the accepted Dispatch execution cost. If that cost is higher,
Wallet posts the exact gap from the platform operational expense account into
the same commerce escrow; Vendor Service never writes a ledger entry.

The allocation row lock, unique transition constraints and post-lock read-back
make simultaneous same-key retries converge. An unheld allocation, a released
delivery, a foreign/reused key, or a second logical transition fails closed.
A cheaper execution is delegated to the existing customer-benefit/refund
authority and is not silently retained as platform income.
