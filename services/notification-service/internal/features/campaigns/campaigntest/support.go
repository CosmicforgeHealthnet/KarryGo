// Package campaigntest is test support for campaign fulfilment: an isolated
// PostgreSQL schema carrying the real migrations, an in-memory consent
// authority, and an intent builder. It is imported only by tests.
package campaigntest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignclients "cosmicforge/logistics/services/notification-service/internal/features/campaigns/clients"
	campaignmodels "cosmicforge/logistics/services/notification-service/internal/features/campaigns/models"
	"cosmicforge/logistics/shared/go/admincampaign"
)

// Pool applies the service's actual migrations into a fresh schema of the
// disposable NOTIFICATION_TEST_DATABASE_URL database.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NOTIFICATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: NOTIFICATION_TEST_DATABASE_URL (disposable PostgreSQL) is required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "campaign_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		root.Close()
	})
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

// Template inserts an active marketing template and returns its key,
// revision and content hash.
func Template(t *testing.T, db *pgxpool.Pool, title, body string) (string, int64, string) {
	t.Helper()
	key := "marketing.test." + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	var rev int64
	if err := db.QueryRow(context.Background(), `INSERT INTO notification_templates(key,locale,title,body,default_channels,active) VALUES($1,'en-NG',$2,$3,'["push"]',true) RETURNING revision`, key, title, body).Scan(&rev); err != nil {
		t.Fatal(err)
	}
	return key, rev, campaignmodels.ContentSHA256(title, body)
}

// Intent returns a valid intent bound to a template.
func Intent(key string, rev int64, hash string, markets, channels []string) admincampaign.Intent {
	now := time.Now().UTC()
	return admincampaign.Intent{
		ContractVersion: admincampaign.Contract, CampaignID: uuid.NewString(), Revision: 1, IdempotencyKey: "idem-" + uuid.NewString(),
		Product: admincampaign.Product, RequestingDepartment: "growth", RequestedBy: "staff-requester",
		Approvals: []admincampaign.Approval{{ApprovalID: uuid.NewString(), ApproverRef: "staff-approver", ApprovedAt: now.Add(-time.Minute)}},
		Category:  "MARKETING", Markets: markets, Audience: "CUSTOMERS_OPTED_IN", AudienceDefinitionSHA256: strings.Repeat("a", 64),
		LegalBasis: "CONSENT", Channels: channels, TemplateKey: key, TemplateLocale: "en-NG", TemplateRevision: rev, ContentSHA256: hash,
		StartAt: now.Add(-time.Minute), EndAt: now.Add(24 * time.Hour), MaxPerRecipientPerDay: 3, MaxMessages: 1000, Priority: "normal",
	}
}

// Raw marshals an intent body.
func Raw(t *testing.T, i admincampaign.Intent) []byte {
	t.Helper()
	raw, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Customer is one consent record in the fake authority.
type Customer struct {
	Market   *string
	Channels map[string]bool
	Active   bool
	Email    string
	Version  int64
}

// Consent is an in-memory customer-service consent authority.
type Consent struct {
	mu          sync.Mutex
	Customers   map[string]*Customer
	Unavailable bool
	// Duplicate repeats every audience member (dedupe test).
	Duplicate bool
	Checks    int
}

func NewConsent() *Consent { return &Consent{Customers: map[string]*Customer{}} }

// Add registers a customer opted into channels in market ("" = unknown).
func (c *Consent) Add(market string, channels ...string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := uuid.NewString()
	cust := &Customer{Channels: map[string]bool{}, Active: true, Email: "c-" + id[:8] + "@example.invalid", Version: 1}
	if market != "" {
		m := market
		cust.Market = &m
	}
	for _, ch := range channels {
		cust.Channels[ch] = true
	}
	c.Customers[id] = cust
	return id
}

// Withdraw withdraws one channel.
func (c *Consent) Withdraw(id, channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Customers[id].Channels[channel] = false
	c.Customers[id].Version++
}

func (c *Consent) Audience(_ context.Context, channel, market, cursor string, limit int) (campaignclients.AudiencePage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Unavailable {
		return campaignclients.AudiencePage{}, campaignclients.ErrUnavailable
	}
	var ids []string
	for id, cust := range c.Customers {
		if cust.Active && cust.Channels[channel] && cust.Market != nil && *cust.Market == market && id > cursor {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	page := campaignclients.AudiencePage{Channel: channel, Market: market, AsOf: time.Now().UTC(), Members: []campaignclients.AudienceMember{}}
	if len(ids) > limit {
		ids = ids[:limit]
		page.NextCursor = ids[limit-1]
	}
	for _, id := range ids {
		page.Members = append(page.Members, campaignclients.AudienceMember{CustomerID: id, ConsentVersion: c.Customers[id].Version})
		if c.Duplicate {
			page.Members = append(page.Members, campaignclients.AudienceMember{CustomerID: id, ConsentVersion: c.Customers[id].Version})
		}
	}
	return page, nil
}

func (c *Consent) Check(_ context.Context, channel string, ids []string) (campaignclients.CheckResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Checks++
	if c.Unavailable {
		return campaignclients.CheckResult{}, campaignclients.ErrUnavailable
	}
	res := campaignclients.CheckResult{Channel: channel, AsOf: time.Now().UTC()}
	for _, id := range ids {
		cust, ok := c.Customers[id]
		e := campaignclients.Eligibility{CustomerID: id}
		switch {
		case !ok:
			e.Reason = "ACCOUNT_NOT_FOUND"
		case !cust.Active:
			e.Reason, e.Market = "ACCOUNT_INACTIVE", cust.Market
		case !cust.Channels[channel]:
			e.Reason, e.Market, e.ConsentVersion = "NOT_OPTED_IN", cust.Market, cust.Version
		default:
			e.Eligible, e.Reason, e.Market, e.ConsentVersion = true, "OPTED_IN", cust.Market, cust.Version
			if channel == "email" {
				email := cust.Email
				e.Email = &email
			}
		}
		res.Results = append(res.Results, e)
	}
	return res, nil
}
