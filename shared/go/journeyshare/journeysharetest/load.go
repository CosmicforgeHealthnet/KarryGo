// Package journeysharetest contains bounded integration-test load measurement.
package journeysharetest

import (
	"sort"
	"sync"
	"testing"
	"time"
)

func Exercise(t testing.TB, name string, requests int, read func() error) {
	t.Helper()
	samples := make([]time.Duration, requests)
	jobs := make(chan int)
	errs := make(chan error, requests)
	var wg sync.WaitGroup
	started := time.Now()
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				start := time.Now()
				err := read()
				samples[index] = time.Since(start)
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	for index := 0; index < requests; index++ {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("%s load error: %v", name, err)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	t.Logf("LOAD %s requests=%d concurrency=8 duration=%s p50=%s p95=%s max=%s", name, requests, time.Since(started), samples[requests/2], samples[requests*95/100], samples[requests-1])
}
