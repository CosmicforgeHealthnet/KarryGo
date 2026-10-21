package delivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// requestHashHex returns a stable SHA-256 hex of the delivery request content.
// The idempotency_key itself is excluded so the hash reflects only the request
// body: reusing the same key with the same body matches (cached replay), while
// reusing it with a different body is detected as a conflict.
func requestHashHex(req DeliveryRequest) string {
	canonical := req
	canonical.IdempotencyKey = ""
	// Struct field order is stable, so json.Marshal is deterministic here.
	b, _ := json.Marshal(canonical)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
