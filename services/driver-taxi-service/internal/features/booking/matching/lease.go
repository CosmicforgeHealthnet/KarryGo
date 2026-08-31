// Package matching holds the booking-level matcher lease: the mechanism
// that lets exactly one goroutine (in exactly one service instance) run the
// matching loop for a given booking at a time, per addendum §30's
// distributed-matching requirement.
//
// This is a DIFFERENT lock from provider_availability's AcquireMatchLock:
// that one reserves one DRIVER for one offer (so two bookings cannot offer
// the same driver simultaneously); this one reserves one BOOKING for one
// matcher goroutine (so two service instances -- or two goroutines within
// one, after a bug -- cannot both run the search loop for the same
// booking). Both exist and both matter; they answer different questions.
package matching

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const leaseKeyPrefix = "taxi:booking:matcher:"

func leaseKey(bookingID string) string { return leaseKeyPrefix + bookingID }

// MatcherLease is a Redis SETNX lease scoped to one booking.
type MatcherLease struct {
	client *redis.Client
}

func NewMatcherLease(client *redis.Client) *MatcherLease {
	return &MatcherLease{client: client}
}

// Acquire claims the lease for bookingID under ownerID (a process-unique
// instance id). Returns true only if this call claimed it -- false means
// another instance already holds it, and the caller must not run a second
// concurrent matcher for the same booking.
func (l *MatcherLease) Acquire(ctx context.Context, bookingID, ownerID string, ttl time.Duration) (bool, error) {
	return l.client.SetNX(ctx, leaseKey(bookingID), ownerID, ttl).Result()
}

// Renew extends the lease's TTL, but ONLY if ownerID still holds it -- a
// compare-value operation (get-then-conditionally-expire, atomic via a Lua
// script) so one instance can never renew a lease it does not actually
// still own, e.g. after a long pause let the TTL lapse and a recovery
// worker (phase B10) took over the booking.
func (l *MatcherLease) Renew(ctx context.Context, bookingID, ownerID string, ttl time.Duration) (bool, error) {
	const script = `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		end
		return 0
	`
	result, err := l.client.Eval(ctx, script, []string{leaseKey(bookingID)}, ownerID, ttl.Milliseconds()).Result()
	if err != nil {
		return false, err
	}
	n, _ := result.(int64)
	return n == 1, nil
}

// Release removes the lease, but ONLY if ownerID still holds it -- so a
// matcher goroutine that already lost its lease (taken over elsewhere)
// can never delete a lease it no longer owns.
func (l *MatcherLease) Release(ctx context.Context, bookingID, ownerID string) error {
	const script = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) end return 0`
	return l.client.Eval(ctx, script, []string{leaseKey(bookingID)}, ownerID).Err()
}
