package adminfeed_test

import (
	"context"
	"cosmicforge/logistics/services/notification-service/internal/features/adminfeed"
	"cosmicforge/logistics/services/notification-service/internal/features/campaigns/campaigntest"
	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	campaignusecases "cosmicforge/logistics/services/notification-service/internal/features/campaigns/usecases"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/outbox"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func TestFeedDefaultsOffAndRejectsMissingAuthority(t *testing.T) {
	for _, suffix := range []string{"RESOURCE_READ_ENABLED", "CONTEXT_PUBLISH_ENABLED", "EVENTS_ENABLED", "EVENTS_SNAPSHOT_ENABLED"} {
		t.Setenv("NOTIFICATION_ADMIN_"+suffix, "")
	}
	h, guard, err := adminfeed.Runtime(context.Background(), nil)
	if err != nil || h.Enabled || h.PublishEnabled || guard.Configured() {
		t.Fatal("disabled feed touched runtime authority", err)
	}
	if !errors.Is(guard.Export(context.Background(), outbox.Record{}), adminfeed.ErrScope) {
		t.Fatal("unconfigured relay accepted")
	}
	t.Setenv("NOTIFICATION_ADMIN_EVENTS_ENABLED", "true")
	if _, _, err := adminfeed.Runtime(context.Background(), nil); err == nil {
		t.Fatal("feed enabled without signed authority")
	}
}
func TestSignedCampaignFeedScopeRevocationAndNoStaffGrant(t *testing.T) {
	ctx := context.Background()
	db := campaigntest.Pool(t)
	repo := campaignrepositories.New(db)
	service := &campaignusecases.Service{Repo: repo}
	intent := campaigntest.Intent("test.feed", 1, strings.Repeat("a", 64), []string{"NG-LAG"}, []string{"push"})
	if _, _, err := service.Accept(ctx, campaigntest.Raw(t, intent)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Snapshot(ctx, "", 100); err == nil {
		t.Fatal("snapshot without export guard accepted")
	}
	authority := campaigntest.NewFeedAuthority(t, db)
	repo.CheckEvent = authority.Guard.Check
	repo.ExportReady = authority.Guard.Configured
	if _, err := service.Snapshot(ctx, "", 100); err == nil {
		t.Fatal("unallocated snapshot accepted")
	}
	ref := intent.CampaignID + ":1"
	authority.Approve(ref)
	wrongPolicy := testfixture.New(t, authority.Scope, adminfeed.Source, "hq.campaigns.readback", "karryam.campaign-intent.v1")
	current := authority.Guard.Owner.Policy
	authority.Guard.Owner.Policy = wrongPolicy.Engine
	if _, err := service.Snapshot(ctx, "", 100); err == nil {
		t.Fatal("snapshot accepted without signed feed purpose")
	}
	authority.Guard.Owner.Policy = current

	page, err := service.Snapshot(ctx, "", 100)
	if err != nil || !page.Complete || len(page.Events) != 1 {
		t.Fatalf("authorized snapshot %+v %v", page, err)
	}
	ev := page.Events[0]
	if ev.Scope.Tenant != authority.Scope.Tenant || ev.Scope.Organization != authority.Scope.Organization {
		t.Fatal("signed scope missing")
	}
	wire, _ := json.Marshal(ev)
	if strings.Contains(string(wire), "requested_by") || strings.Contains(string(wire), "content_body") {
		t.Fatal("private campaign data exported")
	}
	record := outbox.Record{ID: ev.EventID, EventType: ev.EventType, Payload: wire}
	if err := authority.Guard.Export(ctx, record); err != nil {
		t.Fatal(err)
	}
	ev.Scope.Tenant = "another-tenant"
	wire, _ = json.Marshal(ev)
	record.Payload = wire
	if err := authority.Guard.Export(ctx, record); err == nil {
		t.Fatal("cross-tenant campaign exported")
	}
	authority.Revoke(ref)
	if _, err := service.Snapshot(ctx, "", 100); err == nil {
		t.Fatal("revoked allocation exported")
	}
	record.Payload, _ = json.Marshal(page.Events[0])
	if err := authority.Guard.Export(ctx, record); err == nil {
		t.Fatal("revoked relay event exported")
	}
	// An ordinary business update still succeeds with a revoked allocation.
	if _, err := db.Exec(ctx, `UPDATE campaigns SET audience_count=audience_count+1 WHERE campaign_id=$1::uuid`, intent.CampaignID); err != nil {
		t.Fatal("authorization metadata broke ordinary business update", err)
	}
}
func TestMultiMarketCampaignCannotBeAssignedToOneMarket(t *testing.T) {
	ctx := context.Background()
	db := campaigntest.Pool(t)
	repo := campaignrepositories.New(db)
	service := &campaignusecases.Service{Repo: repo}
	intent := campaigntest.Intent("test.multi", 1, strings.Repeat("a", 64), []string{"NG-LAG", "NG-ABJ"}, []string{"push"})
	if _, _, err := service.Accept(ctx, campaigntest.Raw(t, intent)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := (adminfeed.SourceReader{}).Binding(ctx, tx, "campaign-revisions", intent.CampaignID+":1"); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("multi-market counts could be relabelled as one market", err)
	}
}

func TestOrdinaryCampaignIntakeBeforeOptionalFeedMigration(t *testing.T) {
	ctx := context.Background()
	db := campaigntest.Pool(t)
	if _, err := db.Exec(ctx, `DROP FUNCTION notification_admin_event_scope(text)`); err != nil {
		t.Fatal(err)
	}
	repo := campaignrepositories.New(db)
	service := &campaignusecases.Service{Repo: repo}
	intent := campaigntest.Intent("test.legacy", 1, strings.Repeat("a", 64), []string{"NG-LAG"}, []string{"push"})
	if _, _, err := service.Accept(ctx, campaigntest.Raw(t, intent)); err != nil {
		t.Fatal("optional feed migration blocked ordinary intake", err)
	}
	var market string
	if err := db.QueryRow(ctx, `SELECT payload->'scope'->>'market' FROM admin_event_outbox LIMIT 1`).Scan(&market); err != nil || market != "NG-LAG" {
		t.Fatalf("legacy owner scope changed: %q %v", market, err)
	}
	if _, err := service.Snapshot(ctx, "", 100); err == nil {
		t.Fatal("unconfigured snapshot accepted")
	}
}
