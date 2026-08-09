// Command migration-inventory emits the drift-proof Section 30 migration
// inventory as JSON. Run it from the fleet-partner-service directory:
//
//	go run ./cmd/migration-inventory -dir ./migrations
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/migrationlint"
)

func main() {
	dir := flag.String("dir", "./migrations", "directory containing paired Fleet migration files")
	flag.Parse()
	migrations, err := migrationlint.LoadMigrations(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	issues := migrationlint.CheckPairingAndNumbering(migrations)
	if len(issues) != 0 {
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "%s: %s\n", issue.Version, issue.Problem)
		}
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(migrationlint.BuildInventory(migrations)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
