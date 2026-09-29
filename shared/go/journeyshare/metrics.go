package journeyshare

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"sort"
	"strings"
	"sync"
	"time"
)

var metrics = struct {
	sync.Mutex
	Counts      map[string]int64
	Nanoseconds map[string]int64
}{Counts: map[string]int64{}, Nanoseconds: map[string]int64{}}

// Names are code-owned categories. Never pass tokens, identities or source references.
func Observe(name string, elapsed time.Duration) {
	metrics.Lock()
	defer metrics.Unlock()
	metrics.Counts[name]++
	metrics.Nanoseconds[name] += elapsed.Nanoseconds()
}

// Render adds bounded counters to an owning service's existing private scrape.
func Render() string {
	metrics.Lock()
	defer metrics.Unlock()
	names := make([]string, 0, len(metrics.Counts))
	for name := range metrics.Counts {
		names = append(names, name)
	}
	sort.Strings(names)
	var output strings.Builder
	for _, name := range names {
		fmt.Fprintf(&output, "karry_journeyshare_%s_total %d\nkarry_journeyshare_%s_duration_seconds_sum %.9f\n", name, metrics.Counts[name], name, float64(metrics.Nanoseconds[name])/1e9)
	}
	return output.String()
}
func Metrics(c *gin.Context) {
	metrics.Lock()
	defer metrics.Unlock()
	c.JSON(200, gin.H{"success": true, "data": gin.H{"counts": metrics.Counts, "duration_nanoseconds": metrics.Nanoseconds}})
}
