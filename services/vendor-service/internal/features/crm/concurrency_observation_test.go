package crm

// Deterministic observation of PostgreSQL lock waits.
//
// Every concurrency test in this package needs the same thing before it
// releases its blocker: proof that the other transaction is genuinely blocked
// on the intended lock. That proof has to come from PostgreSQL's own lock
// state, never from a sleep, or the test passes because one goroutine happened
// to be slower and proves nothing about production.
//
// Five hand-rolled pollers used to do this, and three of them were wrong in
// ways that only surfaced under load:
//
//   - waitForBlockedTransaction asked `wait_event_type='Lock' AND
//     state='active'` with NO database filter, so any backend anywhere on the
//     server waiting on any lock satisfied it. It also polled in a tight loop
//     with no sleep, issuing thousands of round trips a second through the very
//     pool whose connections the blocked transactions were holding. When that
//     pool ran out of connections, Acquire waited on context.Background() with
//     no deadline, so the 20-second budget became an indefinite hang - observed
//     as a single test sitting for 8m42s until the package timeout killed it.
//   - waitForActivityFenceLock and waitForGenerationFenceLock matched on query
//     text only, again server-wide.
//   - waitForLoyaltyDBWaiters matched "%SELECT id FROM vendor_customer_profiles
//     WHERE business_id%", a prefix shared by four different statements in this
//     package, so a backend running an unrelated query could satisfy it.
//
// All five now run through one observer, which:
//
//   - polls on its OWN connection, opened directly from the DSN rather than
//     taken from env.pool. These tests deliberately saturate that pool (a
//     barrier connection, a blocker transaction, one or more blocked waiters),
//     and an observer competing for the last free slot is exactly how the
//     indefinite hang happened.
//   - reads pg_locks for genuinely ungranted locks instead of inferring a wait
//     from wait_event alone.
//   - restricts to the current database AND to connections tagged with
//     crmTestAppName, so only this test binary's own backends can ever satisfy
//     it.
//   - additionally matches the production fence marker carried in the
//     statement text when the test cares which statement is blocked.
//   - sleeps between polls, and fails with the lock state it actually observed
//     rather than a bare "never happened".

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// crmTestAppName tags every connection newTestEnv opens. Lock observation is
// scoped to it, so this test binary can never be satisfied by an unrelated
// backend - another package's database, autovacuum, or a stray psql session.
const crmTestAppName = "vendor-crm-integration-test"

const (
	// lockWaitBudget is how long a waiter is given to reach its lock. It is a
	// ceiling on a condition, not a delay: every observer returns the instant
	// the lock state it is waiting for is real.
	lockWaitBudget = 20 * time.Second
	// lockPollInterval keeps the observer from flooding the server it is
	// measuring. The previous busy-loop made the database slower for the very
	// transaction it was waiting on.
	lockPollInterval = 10 * time.Millisecond
)

// lockObserver polls PostgreSQL lock state on a connection of its own.
type lockObserver struct {
	conn *pgx.Conn
}

// newLockObserver opens a dedicated observation connection. It is deliberately
// NOT taken from env.pool; see the file comment.
func newLockObserver(t *testing.T) *lockObserver {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres CRM test")
	}

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse observation DSN: %v", err)
	}
	// The observer is tagged separately so it can never match its own query
	// while looking for blocked backends.
	config.RuntimeParams["application_name"] = crmTestAppName + "-observer"

	conn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("open observation connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	return &lockObserver{conn: conn}
}

// blockedQuery counts distinct backends of this test binary that hold at least
// one ungranted lock, optionally narrowed to a statement marker and to advisory
// locks.
//
// The pg_locks join is what makes this a fact rather than an inference:
// wait_event can lag, but a row in pg_locks with granted=false is the server
// stating that the backend is queued behind someone else's lock.
const blockedQuery = `
	SELECT count(DISTINCT activity.pid)
	  FROM pg_locks locks
	  JOIN pg_stat_activity activity ON activity.pid = locks.pid
	 WHERE NOT locks.granted
	   AND activity.datname = current_database()
	   AND activity.application_name = $1
	   AND ($2 = '' OR activity.query LIKE $2)
	   AND ($3 = false OR locks.locktype = 'advisory')`

// describeBlocked renders whatever this test binary's backends are actually
// waiting on, so a failure says what the database was doing instead of only
// what it was not doing.
const describeBlocked = `
	SELECT coalesce(string_agg(
	           format('pid=%s locktype=%s wait=%s query=%s',
	                  activity.pid, locks.locktype,
	                  coalesce(activity.wait_event, '-'),
	                  left(regexp_replace(activity.query, '\s+', ' ', 'g'), 120)),
	           E'\n'), '(no backend of this test is waiting on any lock)')
	  FROM pg_locks locks
	  JOIN pg_stat_activity activity ON activity.pid = locks.pid
	 WHERE NOT locks.granted
	   AND activity.datname = current_database()
	   AND activity.application_name = $1`

// await blocks until at least want backends are blocked as described, or fails
// the test with the observed lock state.
func (o *lockObserver) await(t *testing.T, want int, marker string, advisoryOnly bool, what string) {
	t.Helper()

	deadline := time.Now().Add(lockWaitBudget)
	for {
		var count int
		if err := o.conn.QueryRow(context.Background(), blockedQuery,
			crmTestAppName, marker, advisoryOnly).Scan(&count); err != nil {
			t.Fatalf("observe lock state: %v", err)
		}
		if count >= want {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("%s: expected %d blocked transaction(s)%s within %s, saw %d.\nobserved lock waits:\n%s",
				what, want, markerSuffix(marker), lockWaitBudget, count, o.describe(t))
		}
		time.Sleep(lockPollInterval)
	}
}

// awaitBackend blocks until one specific backend is waiting on a lock. This is
// the strongest form available and is used wherever the test owns the waiting
// connection and can read its pg_backend_pid().
func (o *lockObserver) awaitBackend(t *testing.T, backendPID int, what string) {
	t.Helper()

	deadline := time.Now().Add(lockWaitBudget)
	for {
		var waiting bool
		if err := o.conn.QueryRow(context.Background(), `
			SELECT EXISTS(
				SELECT 1 FROM pg_locks
				 WHERE pid = $1 AND NOT granted)`, backendPID).Scan(&waiting); err != nil {
			t.Fatalf("observe lock state for backend %d: %v", backendPID, err)
		}
		if waiting {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("%s: backend %d never reached the expected lock within %s.\nobserved lock waits:\n%s",
				what, backendPID, lockWaitBudget, o.describe(t))
		}
		time.Sleep(lockPollInterval)
	}
}

func (o *lockObserver) describe(t *testing.T) string {
	t.Helper()
	var description string
	if err := o.conn.QueryRow(context.Background(), describeBlocked, crmTestAppName).Scan(&description); err != nil {
		return fmt.Sprintf("(could not read lock state: %v)", err)
	}
	return description
}

func markerSuffix(marker string) string {
	if marker == "" {
		return ""
	}
	return " matching " + strings.Trim(marker, "%")
}
