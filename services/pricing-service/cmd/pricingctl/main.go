// Command pricingctl is the operations tool for pricing configuration.
//
// It exists because the admin screens do not yet: pricing's administrative API is
// signed service-to-service, so there has to be SOME way for an operator to load
// a rate book, submit it and approve it. This is that way, and it deliberately
// goes through the same HTTP API and the same four-eye checks an admin UI will —
// no back door into the database.
//
// Usage:
//
//	pricingctl import-draft --file seeds/v1/taxi_tariff_ng.json --by ada@karrygo --reason "V1 baseline"
//	pricingctl submit   --version pv_... --by ada@karrygo  --reason "ready for review"
//	pricingctl approve  --version pv_... --by bola@karrygo --reason "checked against golden corpus"
//	pricingctl activate --version pv_... --by bola@karrygo --reason "go live" [--from 2026-10-01T00:00:00Z]
//	pricingctl resolve  --kind taxi.tariff --country NG
//	pricingctl simulate --version pv_... --scope TAXI --class economy --metres 8000 --seconds 1230
//	pricingctl rollout  --scope TAXI --capability QUOTE --market NG-LAG --mode SHADOW --by bola@karrygo --reason "start shadow"
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const defaultBaseURL = "http://localhost:8115"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var err error
	switch command {
	case "import-draft":
		err = importDraft(args)
	case "submit":
		err = lifecycle(args, "submit")
	case "approve":
		err = lifecycle(args, "approve")
	case "reject":
		err = lifecycle(args, "reject")
	case "activate":
		err = activate(args)
	case "resolve":
		err = resolve(args)
	case "simulate":
		err = simulate(args)
	case "rollout":
		err = rollout(args)
	case "approvals":
		err = approvals(args)
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "pricingctl: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pricingctl manages pricing configuration through the signed admin API.

Commands:
  import-draft  --file <seed.json> --by <operator> --reason <why>
  submit        --version <id> --by <operator> --reason <why>
  approve       --version <id> --by <operator> --reason <why>   (must differ from the submitter)
  reject        --version <id> --by <operator> --reason <why>
  activate      --version <id> --by <operator> --reason <why> [--from <RFC3339>] [--rollback]
  resolve       --kind <kind> [--country|--state|--market|--zone <ref>] [--as-of <RFC3339>]
  simulate      --version <id> --scope <TAXI|DISPATCH|HAULAGE> --class <class> [--metres N --seconds N ...]
  rollout       --scope <s> --capability <QUOTE|BIND|RECONCILE> --market <code> --mode <LEGACY|SHADOW|CENTRAL> --by <op> --reason <why>
  approvals

Environment:
  PRICING_ADMIN_URL     default `+defaultBaseURL+`
  PRICING_OPS_SECRET    the HMAC secret for the pricing-ops-cli caller (required)
  PRICING_OPS_CALLER    default pricing-ops-cli
`)
}

// seedFile is the on-disk shape of a policy seed: the body plus where it applies.
type seedFile struct {
	Kind           string          `json:"kind"`
	GeographyLevel string          `json:"geography_level"`
	GeographyRef   string          `json:"geography_ref"`
	SchemaVersion  int             `json:"schema_version"`
	ChangeReason   string          `json:"change_reason"`
	Body           json.RawMessage `json:"body"`
}

func importDraft(args []string) error {
	flags := flag.NewFlagSet("import-draft", flag.ExitOnError)
	file := flags.String("file", "", "seed file to load")
	by := flags.String("by", "", "operator creating the draft")
	reason := flags.String("reason", "", "why this change is being made")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *file == "" || *by == "" {
		return fmt.Errorf("--file and --by are required")
	}

	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read %s: %w", *file, err)
	}
	var seed seedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		return fmt.Errorf("parse %s: %w", *file, err)
	}

	changeReason := *reason
	if changeReason == "" {
		changeReason = seed.ChangeReason
	}
	if changeReason == "" {
		return fmt.Errorf("a reason is required: pass --reason or set change_reason in the seed file")
	}

	return call("POST", "/policies/drafts", map[string]any{
		"kind":            seed.Kind,
		"geography_level": seed.GeographyLevel,
		"geography_ref":   seed.GeographyRef,
		"schema_version":  seed.SchemaVersion,
		"body":            seed.Body,
		"performed_by":    *by,
		"reason":          changeReason,
	})
}

func lifecycle(args []string, action string) error {
	flags := flag.NewFlagSet(action, flag.ExitOnError)
	version := flags.String("version", "", "policy version id")
	by := flags.String("by", "", "operator performing the action")
	reason := flags.String("reason", "", "why")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version == "" || *by == "" || *reason == "" {
		return fmt.Errorf("--version, --by and --reason are required")
	}
	return call("POST", "/policy-versions/"+*version+"/"+action, map[string]any{
		"performed_by": *by, "reason": *reason,
	})
}

func activate(args []string) error {
	flags := flag.NewFlagSet("activate", flag.ExitOnError)
	version := flags.String("version", "", "policy version id")
	by := flags.String("by", "", "operator activating")
	reason := flags.String("reason", "", "why")
	from := flags.String("from", "", "RFC3339 moment to go live (default now)")
	rollback := flags.Bool("rollback", false, "mark this as a return to an earlier version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version == "" || *by == "" || *reason == "" {
		return fmt.Errorf("--version, --by and --reason are required")
	}
	body := map[string]any{"performed_by": *by, "reason": *reason, "rollback": *rollback}
	if *from != "" {
		body["effective_from"] = *from
	}
	return call("POST", "/policy-versions/"+*version+"/activate", body)
}

func resolve(args []string) error {
	flags := flag.NewFlagSet("resolve", flag.ExitOnError)
	kind := flags.String("kind", "", "policy kind")
	country := flags.String("country", "", "country code")
	state := flags.String("state", "", "state id")
	market := flags.String("market", "", "market id")
	zone := flags.String("zone", "", "zone id")
	asOf := flags.String("as-of", "", "RFC3339 moment (default now)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *kind == "" {
		return fmt.Errorf("--kind is required")
	}
	query := url.Values{}
	query.Set("kind", *kind)
	for key, value := range map[string]string{
		"country_code": *country, "state_id": *state, "market_id": *market,
		"zone_id": *zone, "as_of": *asOf,
	} {
		if value != "" {
			query.Set(key, value)
		}
	}
	return call("GET", "/resolve?"+query.Encode(), nil)
}

func simulate(args []string) error {
	flags := flag.NewFlagSet("simulate", flag.ExitOnError)
	version := flags.String("version", "", "policy version id to price against")
	commission := flags.String("commission", "", "commission version id (optional)")
	scope := flags.String("scope", "", "TAXI, DISPATCH or HAULAGE")
	channel := flags.String("channel", "CUSTOMER_APP", "channel")
	class := flags.String("class", "", "vehicle or service class")
	metres := flags.Int64("metres", 0, "distance in whole metres")
	seconds := flags.Int64("seconds", 0, "duration in whole seconds")
	size := flags.String("size", "", "package size (dispatch)")
	tier := flags.String("tier", "", "service tier (dispatch)")
	grams := flags.Int64("grams", 0, "cargo weight in grams (haulage)")
	helpers := flags.Int64("helpers", 0, "helper count (haulage)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version == "" || *scope == "" || *class == "" {
		return fmt.Errorf("--version, --scope and --class are required")
	}
	return call("POST", "/simulate", map[string]any{
		"version_id": *version, "commission_version_id": *commission,
		"scope": *scope, "channel": *channel, "class": *class,
		"distance_meters": *metres, "duration_seconds": *seconds,
		"package_size": *size, "service_tier": *tier,
		"weight_grams": *grams, "helpers": *helpers,
	})
}

func rollout(args []string) error {
	flags := flag.NewFlagSet("rollout", flag.ExitOnError)
	scope := flags.String("scope", "", "TAXI, DISPATCH or HAULAGE")
	capability := flags.String("capability", "", "QUOTE, BIND or RECONCILE")
	market := flags.String("market", "", "market code")
	mode := flags.String("mode", "", "LEGACY, SHADOW or CENTRAL")
	by := flags.String("by", "", "operator")
	reason := flags.String("reason", "", "why")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *scope == "" || *capability == "" || *market == "" || *mode == "" || *by == "" || *reason == "" {
		return fmt.Errorf("all of --scope --capability --market --mode --by --reason are required")
	}
	return call("POST", "/rollouts", map[string]any{
		"scope": *scope, "capability": *capability, "market_code": *market,
		"mode": *mode, "performed_by": *by, "reason": *reason,
	})
}

func approvals(args []string) error {
	flags := flag.NewFlagSet("approvals", flag.ExitOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	return call("GET", "/approvals", nil)
}

// call signs and sends a request to the admin API, then prints the response.
func call(method, path string, payload any) error {
	baseURL := os.Getenv("PRICING_ADMIN_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	secret := os.Getenv("PRICING_OPS_SECRET")
	if secret == "" {
		return fmt.Errorf("PRICING_OPS_SECRET is required: pricingctl signs every request like any other caller")
	}
	caller := os.Getenv("PRICING_OPS_CALLER")
	if caller == "" {
		caller = "pricing-ops-cli"
	}

	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}

	request, err := http.NewRequest(method,
		strings.TrimRight(baseURL, "/")+"/internal/pricing/v1/admin"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(request, caller, []byte(secret), body, time.Now()); err != nil {
		return err
	}

	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var pretty bytes.Buffer
	if json.Indent(&pretty, raw, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(raw))
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("the admin API returned %s", response.Status)
	}
	return nil
}
