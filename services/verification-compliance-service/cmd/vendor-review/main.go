// Command vendor-review lets the operations team review new vendor businesses
// by hand until an admin screen exists.
//
//	vendor-review list                       businesses waiting for review
//	vendor-review show    <business_id>      every check for one business
//	vendor-review approve <business_id>      approve all of its waiting checks
//	vendor-review reject  <business_id> "<what the vendor must fix>"
//
// Reads the case list from the verification database, but records every
// decision through the signed decision API as admin-backoffice-service, so the
// history, idempotency and the callback to vendor-service all behave exactly as
// they would for an admin screen.
//
// Environment:
//
//	VERIFICATION_COMPLIANCE_DATABASE_URL  verification database (read only use)
//	VENDOR_REVIEW_SECRET                  admin-backoffice-service HMAC secret
//	VENDOR_REVIEW_URL                     service origin (default http://karrygo-verification-staging:8111)
//	VENDOR_REVIEW_OPERATOR                name recorded in the reason code (optional)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const caller = "admin-backoffice-service"

type reviewCase struct {
	ID         string
	BusinessID string
	Dimension  string
	Subject    string
	Status     string
	Version    int64
	Updated    time.Time
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := pgxpool.New(ctx, mustEnv("VERIFICATION_COMPLIANCE_DATABASE_URL"))
	if err != nil {
		fail("could not connect to the verification database: %v", err)
	}
	defer db.Close()

	switch os.Args[1] {
	case "list":
		list(ctx, db)
	case "show":
		need(3)
		printCases(load(ctx, db, os.Args[2], false))
	case "approve":
		need(3)
		decide(ctx, db, os.Args[2], "VERIFIED", "")
	case "reject":
		need(4)
		decide(ctx, db, os.Args[2], "REJECTED", strings.TrimSpace(strings.Join(os.Args[3:], " ")))
	default:
		usage()
	}
}

func list(ctx context.Context, db *pgxpool.Pool) {
	rows, err := db.Query(ctx, `SELECT business_id, string_agg(dimension||':'||status, ', ' ORDER BY dimension), min(created_at)
		FROM vendor_verification_cases
		WHERE status IN ('PENDING_REVIEW','IN_PROGRESS')
		GROUP BY business_id ORDER BY min(created_at)`)
	if err != nil {
		fail("could not read cases: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var business, checks string
		var since time.Time
		if err := rows.Scan(&business, &checks, &since); err != nil {
			fail("could not read cases: %v", err)
		}
		n++
		fmt.Printf("%s  waiting since %s\n    %s\n", business, since.Format("2006-01-02 15:04"), checks)
	}
	if n == 0 {
		fmt.Println("No businesses are waiting for review.")
	}
}

func load(ctx context.Context, db *pgxpool.Pool, business string, waitingOnly bool) []reviewCase {
	q := `SELECT id::text, business_id, dimension, subject_type||' '||subject_ref, status, source_version, updated_at
		FROM vendor_verification_cases WHERE business_id=$1`
	if waitingOnly {
		q += ` AND status IN ('PENDING_REVIEW','IN_PROGRESS')`
	}
	rows, err := db.Query(ctx, q+` ORDER BY dimension, updated_at`, business)
	if err != nil {
		fail("could not read cases: %v", err)
	}
	defer rows.Close()
	var out []reviewCase
	for rows.Next() {
		var c reviewCase
		if err := rows.Scan(&c.ID, &c.BusinessID, &c.Dimension, &c.Subject, &c.Status, &c.Version, &c.Updated); err != nil {
			fail("could not read cases: %v", err)
		}
		out = append(out, c)
	}
	return out
}

func printCases(cases []reviewCase) {
	if len(cases) == 0 {
		fmt.Println("No checks found for that business.")
		return
	}
	for _, c := range cases {
		fmt.Printf("%-13s %-15s %s  (case %s, v%d)\n", c.Dimension, c.Status, c.Subject, c.ID, c.Version)
	}
}

func decide(ctx context.Context, db *pgxpool.Pool, business, status, remediation string) {
	if status == "REJECTED" && remediation == "" {
		fail("say what the vendor must fix, e.g. reject vb_123 \"Upload a clear photo of your CAC certificate\"")
	}
	cases := load(ctx, db, business, true)
	if len(cases) == 0 {
		fmt.Println("Nothing to decide: this business has no checks waiting for review.")
		return
	}
	secret := []byte(mustEnv("VENDOR_REVIEW_SECRET"))
	base := strings.TrimRight(envOr("VENDOR_REVIEW_URL", "http://karrygo-verification-staging:8111"), "/")
	reason := "MANUAL_REVIEW"
	if op := strings.TrimSpace(os.Getenv("VENDOR_REVIEW_OPERATOR")); op != "" {
		reason = "MANUAL_REVIEW_" + strings.ToUpper(strings.ReplaceAll(op, " ", "_"))
	}
	failed := 0
	for _, c := range cases {
		body, _ := json.Marshal(map[string]any{
			"status":                  status,
			"reason_code":             reason,
			"remediation":             remediation,
			"expected_source_version": c.Version,
		})
		url := base + "/api/v1/verification-compliance/internal/vendor-cases/cases/" + c.ID + "/decisions"
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("vendor-review-%s-v%d-%s", c.ID, c.Version, status))
		req.Header.Set(serviceauth.HeaderNonce, uuid.NewString())
		if err := serviceauth.SignRequest(req, caller, secret, body, time.Now()); err != nil {
			fail("could not sign the request: %v", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			failed++
			fmt.Printf("FAILED  %-13s could not reach the verification service: %v\n", c.Dimension, err)
			continue
		}
		msg, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			failed++
			fmt.Printf("FAILED  %-13s HTTP %d %s\n", c.Dimension, res.StatusCode, strings.TrimSpace(string(msg)))
			continue
		}
		fmt.Printf("%-7s %-13s %s\n", map[string]string{"VERIFIED": "APPROVED", "REJECTED": "REJECTED"}[status], c.Dimension, c.Subject)
	}
	if failed > 0 {
		fail("%d check(s) were not decided; fix the error above and run the same command again (it is safe to repeat)", failed)
	}
	fmt.Println("Done. The vendor app shows the new status within a minute.")
}

func need(n int) {
	if len(os.Args) < n {
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  vendor-review list
  vendor-review show    <business_id>
  vendor-review approve <business_id>
  vendor-review reject  <business_id> "<what the vendor must fix>"`)
	os.Exit(2)
}

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		fail("%s is not set", k)
	}
	return v
}

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
