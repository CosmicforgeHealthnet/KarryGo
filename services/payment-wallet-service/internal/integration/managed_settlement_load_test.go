package integration_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

// LOAD-005: bounded load on managed settlement posting.
//
// This is the single hottest financial write in the platform: every completed
// corporate job becomes one balanced journal across escrow, provider,
// organization and platform. It is also the one where a latency problem is
// most dangerous, because the retry path is a worker that will happily
// resubmit under load and the protection against that is a unique index —
// which is exactly the kind of protection that starts costing time when
// contention rises.
//
// Deliberately BOUNDED. This measures a stated concurrency against a stated
// request count and reports what it measured; it is not a stress test and it
// does not search for a breaking point. An uncontrolled run against a shared
// certification database tells you nothing except that you saturated it.
//
// Gated on WALLET_LOAD_TEST so an ordinary `go test ./...` does not pay for it.
func TestLoadManagedSettlementPosting(t *testing.T) {
	if os.Getenv("WALLET_LOAD_TEST") == "" {
		t.Skip("WALLET_LOAD_TEST not set; bounded load is run deliberately, not on every build")
	}
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)

	concurrency := envInt("WALLET_LOAD_CONCURRENCY", 16)
	requests := envInt("WALLET_LOAD_REQUESTS", 400)

	// Each request is its own job, because that is the real shape: one
	// settlement per completion. Reusing one job would measure the unique
	// index rejecting duplicates, which is a different question.
	instructions := make([]walletmodels.ManagedSettlementInstruction, requests)
	for i := range instructions {
		jobID := fmt.Sprintf("load_job_%d_%d", time.Now().UnixNano(), i)
		heldJob(t, pool, "taxi", jobID, 10000, 1000)
		instructions[i] = commissionInstruction(jobID)
	}

	latencies := make([]time.Duration, requests)
	failures := make([]error, requests)
	work := make(chan int)
	var wait sync.WaitGroup

	start := time.Now()
	for worker := 0; worker < concurrency; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range work {
				began := time.Now()
				_, err := repo.PostManagedSettlement(context.Background(), instructions[index])
				latencies[index] = time.Since(began)
				failures[index] = err
			}
		}()
	}
	for i := 0; i < requests; i++ {
		work <- i
	}
	close(work)
	wait.Wait()
	elapsed := time.Since(start)

	var errored int
	for _, err := range failures {
		if err != nil {
			errored++
		}
	}

	report := summarize(latencies)
	t.Logf("LOAD-005 managed settlement posting: concurrency=%d requests=%d duration=%s "+
		"throughput=%.1f/s p50=%s p95=%s p99=%s max=%s errors=%d (%.2f%%)",
		concurrency, requests, elapsed.Round(time.Millisecond),
		float64(requests)/elapsed.Seconds(),
		report.p50.Round(time.Microsecond), report.p95.Round(time.Microsecond),
		report.p99.Round(time.Microsecond), report.max.Round(time.Microsecond),
		errored, float64(errored)*100/float64(requests))

	stats := pool.Stat()
	t.Logf("LOAD-005 pool: max=%d acquired=%d idle=%d constructing=%d "+
		"empty_acquire_waits=%d canceled_acquires=%d",
		stats.MaxConns(), stats.AcquiredConns(), stats.IdleConns(),
		stats.ConstructingConns(), stats.EmptyAcquireCount(), stats.CanceledAcquireCount())

	// The measurement is the deliverable; the assertions are the floor
	// underneath it. Errors under bounded load mean the path is not ready,
	// whatever the percentiles say.
	if errored > 0 {
		t.Fatalf("%d of %d settlements failed under bounded load: first error %v",
			errored, requests, firstError(failures))
	}

	// Every request was a distinct job, so every one must have produced its
	// own journal. A lower count would mean load caused a settlement to be
	// silently dropped; a higher one would mean a duplicate posting.
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlement); got != requests {
		t.Fatalf("%d settlement journals for %d requests", got, requests)
	}
	assertEveryJournalBalances(t, pool)
}

// LOAD-005b: the same volume, but every request is the SAME instruction.
//
// This is the retry storm: a worker whose acknowledgement was lost resubmits
// while the synchronous path is still trying. Exactly one journal must exist
// afterwards, and the duplicates must be cheap — a duplicate that costs as
// much as a real posting turns a lost acknowledgement into an outage.
func TestLoadDuplicateSettlementStorm(t *testing.T) {
	if os.Getenv("WALLET_LOAD_TEST") == "" {
		t.Skip("WALLET_LOAD_TEST not set; bounded load is run deliberately, not on every build")
	}
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)

	concurrency := envInt("WALLET_LOAD_CONCURRENCY", 16)
	requests := envInt("WALLET_LOAD_REQUESTS", 400)

	jobID := fmt.Sprintf("storm_job_%d", time.Now().UnixNano())
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)

	latencies := make([]time.Duration, requests)
	failures := make([]error, requests)
	work := make(chan int)
	var wait sync.WaitGroup

	start := time.Now()
	for worker := 0; worker < concurrency; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range work {
				began := time.Now()
				_, err := repo.PostManagedSettlement(context.Background(), instruction)
				latencies[index] = time.Since(began)
				failures[index] = err
			}
		}()
	}
	for i := 0; i < requests; i++ {
		work <- i
	}
	close(work)
	wait.Wait()
	elapsed := time.Since(start)

	var errored int
	for _, err := range failures {
		if err != nil {
			errored++
		}
	}
	report := summarize(latencies)
	t.Logf("LOAD-005b duplicate settlement storm: concurrency=%d requests=%d duration=%s "+
		"p50=%s p95=%s p99=%s errors=%d",
		concurrency, requests, elapsed.Round(time.Millisecond),
		report.p50.Round(time.Microsecond), report.p95.Round(time.Microsecond),
		report.p99.Round(time.Microsecond), errored)

	if errored > 0 {
		t.Fatalf("a replayed settlement was reported as an error %d times: %v",
			errored, firstError(failures))
	}
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlement); got != 1 {
		t.Fatalf("%d journals from %d replays of one instruction; exactly one is the whole point",
			got, requests)
	}
	assertEveryJournalBalances(t, pool)
}

type latencyReport struct{ p50, p95, p99, max time.Duration }

func summarize(latencies []time.Duration) latencyReport {
	if len(latencies) == 0 {
		return latencyReport{}
	}
	sorted := append([]time.Duration(nil), latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	at := func(q float64) time.Duration {
		index := int(q * float64(len(sorted)-1))
		return sorted[index]
	}
	return latencyReport{p50: at(0.50), p95: at(0.95), p99: at(0.99), max: sorted[len(sorted)-1]}
}

func firstError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func envInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}
