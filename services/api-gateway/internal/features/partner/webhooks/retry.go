package webhooks

import "time"

// maxWebhookAttempts is the total number of delivery attempts before a delivery
// is marked failed_permanent.
const maxWebhookAttempts = 5

// retrySchedule[i] is the delay before attempt number (i+2), i.e. the wait after
// the i-th attempt fails. Attempt 1 is immediate.
//
//	after attempt 1 -> 30s   (attempt 2)
//	after attempt 2 -> 5m    (attempt 3)
//	after attempt 3 -> 30m   (attempt 4)
//	after attempt 4 -> 2h    (attempt 5)
var retrySchedule = []time.Duration{
	30 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// NextRetryDelay returns the delay before the next attempt given the number of
// attempts already made, and whether another retry is allowed. After
// maxWebhookAttempts failed attempts, ok is false.
func NextRetryDelay(attempts int) (time.Duration, bool) {
	if attempts < 1 || attempts >= maxWebhookAttempts {
		return 0, false
	}
	// attempts=1 -> retrySchedule[0] (30s), attempts=4 -> retrySchedule[3] (2h).
	return retrySchedule[attempts-1], true
}
