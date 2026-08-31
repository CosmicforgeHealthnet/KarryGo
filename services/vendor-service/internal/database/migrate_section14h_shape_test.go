package database

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSection14HMigrationShape(
	t *testing.T,
) {
	_,
		current,
		_,
		ok :=
		runtime.Caller(
			0,
		)

	if !ok {
		t.Fatal(
			"cannot resolve path",
		)
	}

	path :=
		filepath.Join(
			filepath.Dir(
				current,
			),
			"..",
			"..",
			"migrations",
			"000066_vendor_inventory_operational_hardening.up.sql",
		)

	raw,
		err :=
		os.ReadFile(
			path,
		)

	if err != nil {
		t.Fatal(err)
	}

	text :=
		string(
			raw,
		)

	required :=
		[]string{
			"CREATE TABLE vendor_job_controls",
			"lease_owner",
			"lease_expires_at",
			"heartbeat_at",
			"attempt_count",
			"max_attempts",
			"replay_count",
			"DEAD_LETTER",
			"inventory_alert_event_emissions",
			"vendor.inventory.adjust",
			"vendor.inventory.reserve",
			"vendor.inventory.quarantine",
			"vendor.inventory.policy",
			"role_branch_manager",
			"role_shop_inventory",
			"role_kitchen_pastry",
			"role_procurement",
			"role_analyst_auditor",
		}

	for _, value := range required {

		if !strings.Contains(
			text,
			value,
		) {
			t.Errorf(
				"migration missing %s",
				value,
			)
		}
	}
}
