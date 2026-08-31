package crm

import (
	"context"
	"testing"
)

func TestCRMHarnessPreservesPlatformShopMeasurementSeeds(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	var platformDefinitions int
	if err := env.pool.QueryRow(
		context.Background(),
		`
            SELECT count(*)
            FROM shop_measure_definitions
            WHERE business_id IS NULL
        `,
	).Scan(&platformDefinitions); err != nil {
		t.Fatalf("count platform measurement definitions: %v", err)
	}

	if platformDefinitions < 40 {
		t.Fatalf(
			"CRM test harness destroyed platform measurement seeds: count=%d",
			platformDefinitions,
		)
	}

	var localCanaries int
	if err := env.pool.QueryRow(
		context.Background(),
		`
            SELECT count(*)
            FROM shop_measure_definitions
            WHERE business_id IS NULL
              AND code IN ('PAINT','DERICA','MUDU','CONGO','TIYA')
        `,
	).Scan(&localCanaries); err != nil {
		t.Fatalf("count local measurement canaries: %v", err)
	}

	if localCanaries != 5 {
		t.Fatalf(
			"CRM test harness damaged Nigerian/local measurement seeds: count=%d want=5",
			localCanaries,
		)
	}
}
