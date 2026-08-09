// Command section31-testgate rejects unexpected skips in a go test -json stream.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type values []string

func (v *values) String() string { return strings.Join(*v, ",") }
func (v *values) Set(value string) error {
	*v = append(*v, value)
	return nil
}

type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

func main() {
	var allowed values
	flag.Var(&allowed, "allow", "exact test name allowed to skip; may be repeated")
	flag.Parse()
	if err := enforce(os.Stdin, allowed); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func enforce(reader io.Reader, allowed []string) error {
	allow := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allow[name] = true
	}
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	seen := 0
	var skipped []string
	var failed []string
	for scanner.Scan() {
		var item event
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return fmt.Errorf("section31-testgate: invalid go test JSON: %w", err)
		}
		if item.Test != "" && (item.Action == "run" || item.Action == "pass" || item.Action == "fail" || item.Action == "skip") {
			seen++
		}
		if item.Action == "skip" && item.Test != "" && !allow[item.Test] {
			skipped = append(skipped, item.Package+"/"+item.Test)
		}
		if item.Action == "fail" && item.Test != "" {
			failed = append(failed, item.Package+"/"+item.Test)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("section31-testgate: read stream: %w", err)
	}
	if seen == 0 {
		return fmt.Errorf("section31-testgate: stream contained no test events")
	}
	if len(skipped) > 0 {
		return fmt.Errorf("section31-testgate: %d unexpected mandatory skip(s): %s", len(skipped), strings.Join(skipped, ", "))
	}
	if len(failed) > 0 {
		return fmt.Errorf("section31-testgate: %d failed test(s): %s", len(failed), strings.Join(failed, ", "))
	}
	fmt.Fprintf(os.Stderr, "section31-testgate: PASS; mandatory skips=0; test events=%d\n", seen)
	return nil
}
