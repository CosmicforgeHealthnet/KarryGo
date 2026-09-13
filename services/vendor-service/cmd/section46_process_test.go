package main

import (
	"context"
	"errors"
	"testing"
)

type processProbeDB struct{ err error }

func (p processProbeDB) Ping(context.Context) error { return p.err }

func TestSection46APIAndKillSwitchSuppressAllWorkerClaims(t *testing.T) {
	workers := []func(context.Context) error{func(context.Context) error { return nil }}
	if got := workersForProcess("api", false, workers); len(got) != 0 {
		t.Fatal("api mode must not receive workers")
	}
	if got := workersForProcess("worker", true, workers); len(got) != 0 {
		t.Fatal("global kill switch must suppress new worker claims")
	}
	if got := workersForProcess("worker", false, workers); len(got) != 1 {
		t.Fatal("worker mode must retain configured workers when kill switch is off")
	}
}

func TestSection47WorkerStartupRequiresDatabase(t *testing.T) {
	dbErr := errors.New("database unavailable")
	if err := requireWorkerDatabase(context.Background(), "worker", processProbeDB{err: dbErr}); !errors.Is(err, dbErr) {
		t.Fatalf("worker database check = %v; want %v", err, dbErr)
	}
	if err := requireWorkerDatabase(context.Background(), "api", processProbeDB{err: dbErr}); err != nil {
		t.Fatalf("API readiness is served through /ready, startup check = %v", err)
	}
}
