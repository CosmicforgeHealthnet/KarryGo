package crm

import (
	"context"
	"testing"
)

// This reuses Section 8E's 1,000-profile fixture. Exactly 500 have a current
// EMAIL ALLOW, so run materialization crosses realistic batch/page sizes while
// still proving that ineligible snapshot channels never become deliveries.
func TestCampaignExecutionScale_ThousandTargetsFiveHundredDeliveries(t *testing.T) {
	e := newTestEnv(t, allFlagsOn())
	notifier := &recordingNotifier{}
	e.service.notifier = notifier
	source, _, _ := seedScaleFixture(t, e)
	campaign := e.createCampaign(t, businessA, "vs_a", campaignInput("Execution scale", source.ID, string(ChannelEmail)))
	snapshot := e.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	run := createExecutionRun(t, e, campaign, snapshot)
	if run.TotalDeliveries != 500 {
		t.Fatalf("deliveries=%d want=500 eligible of 1,000 targets", run.TotalDeliveries)
	}
	if got, err := e.service.RunCampaignSchedulerPass(context.Background(), 10); err != nil || got != 1 {
		t.Fatalf("scheduler=%d err=%v", got, err)
	}
	if got, err := e.service.RunCampaignDeliveryPass(context.Background(), 100); err != nil || got != 100 {
		t.Fatalf("dispatcher=%d err=%v", got, err)
	}
	if notifier.count() != 100 {
		t.Fatalf("handoffs=%d want=100", notifier.count())
	}
	var contacts int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM crm_campaign_contact_history WHERE business_id=$1 AND run_id=$2`, businessA, run.ID).Scan(&contacts); err != nil || contacts != 100 {
		t.Fatalf("contact history=%d err=%v", contacts, err)
	}
}
