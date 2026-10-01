package journeyshare

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// Concurrent authority reads must reuse pooled connections. With Go's default
// transport (two idle connections per host) every burst opened new sockets,
// which exhausted ephemeral ports under sustained local load.
func TestClientReusesConnectionsUnderConcurrency(t *testing.T) {
	var opened int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": Projection{Authority: "TAXI", SourceRef: "ref", ProductCode: "RIDE", CustomerID: "owner", SourceVersion: 1}})
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt64(&opened, 1)
		}
	}
	server.Start()
	defer server.Close()
	client := &Client{Endpoints: map[string]Endpoint{"TAXI": {BaseURL: server.URL, Secret: []byte("secret")}}}
	const workers, reads = 16, 50
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < reads; i++ {
				if _, err := client.Read(context.Background(), "TAXI", "ref", "owner"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt64(&opened); got > 2*workers {
		t.Fatalf("opened %d connections for %d reads; pooled reads should stay near %d", got, workers*reads, workers)
	}
}
