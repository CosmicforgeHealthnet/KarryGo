package adminowner

import (
	"context"
	"errors"
	"strconv"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"github.com/jackc/pgx/v5"
)

// CustodyInventory contains source-owned operational metadata only. Challenge
// tokens/hashes, addresses, contact details, free text, evidence locations and
// earnings are deliberately absent. All children belong to the same actual
// mission FK; a caller cannot supply a replacement tenant or parent reference.
type CustodyInventory struct {
	ContractVersion string            `json:"contract_version"`
	Authority       string            `json:"authority"`
	Mode            string            `json:"mode"`
	Legacy          *LegacyManifest   `json:"legacy_manifest"`
	Stops           []CustodyStop     `json:"stops"`
	Manifests       []CustodyManifest `json:"manifests"`
	Packages        []CustodyPackage  `json:"packages"`
	Holdings        []CustodyHolding  `json:"holdings"`
	Transfers       []CustodyTransfer `json:"transfers"`
	Truncated       bool              `json:"truncated"`
}
type LegacyManifest struct {
	Reference        string    `json:"reference"`
	Version          int64     `json:"version"`
	PickupStop       string    `json:"pickup_stop"`
	PackageCount     int64     `json:"package_count"`
	ParcelReferences []string  `json:"parcel_references"`
	FrozenAt         time.Time `json:"frozen_at"`
}
type CustodyStop struct {
	Reference string `json:"reference"`
	Type      string `json:"type"`
	Sequence  int64  `json:"sequence"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
}
type CustodyManifest struct {
	Reference       string  `json:"reference"`
	ManifestVersion int64   `json:"manifest_version"`
	PickupStop      *string `json:"pickup_stop"`
	Authority       string  `json:"authority"`
	PackageCount    int64   `json:"package_count"`
	WeightGrams     int64   `json:"weight_grams"`
	VolumeML        int64   `json:"volume_ml"`
	Required        bool    `json:"required"`
	Status          string  `json:"status"`
	Custodian       *string `json:"custodian"`
	Version         int64   `json:"version"`
}
type CustodyPackage struct {
	Reference       string `json:"reference"`
	Manifest        string `json:"manifest"`
	ManifestVersion int64  `json:"manifest_version"`
	PackageCount    int64  `json:"package_count"`
	WeightGrams     int64  `json:"weight_grams"`
	VolumeML        int64  `json:"volume_ml"`
	Status          string `json:"status"`
}
type CustodyHolding struct {
	Reference       string     `json:"reference"`
	Manifest        string     `json:"manifest"`
	ManifestVersion int64      `json:"manifest_version"`
	Custodian       string     `json:"custodian"`
	AcquiredVia     string     `json:"acquired_via"`
	AcquiredAt      time.Time  `json:"acquired_at"`
	ReleaseReason   *string    `json:"release_reason"`
	ReleasedAt      *time.Time `json:"released_at"`
}
type CustodyTransfer struct {
	Operation       string    `json:"operation"`
	Manifest        string    `json:"manifest"`
	ManifestVersion int64     `json:"manifest_version"`
	From            string    `json:"from"`
	To              string    `json:"to"`
	PackageCount    int64     `json:"package_count"`
	At              time.Time `json:"at"`
}

func readCustodyInventory(ctx context.Context, tx pgx.Tx, ref string) (*CustodyInventory, error) {
	out := &CustodyInventory{ContractVersion: "custody.inventory.v1", Stops: []CustodyStop{}, Manifests: []CustodyManifest{}, Packages: []CustodyPackage{}, Holdings: []CustodyHolding{}, Transfers: []CustodyTransfer{}}
	var tenant string
	var manifestRef string
	var manifestVersion int64
	if err := tx.QueryRow(ctx, `SELECT m.authority,m.custody_mode,m.tenant_ref,s.manifest_ref,s.manifest_version FROM custody_missions m JOIN custody_mission_state s ON s.mission_ref=m.mission_ref AND s.tenant_ref=m.tenant_ref WHERE m.mission_ref=$1`, ref).Scan(&out.Authority, &out.Mode, &tenant, &manifestRef, &manifestVersion); err != nil {
		return nil, err
	}
	if out.Mode == "LEGACY_SINGLE" {
		legacy := &LegacyManifest{}
		if err := tx.QueryRow(ctx, `SELECT manifest_ref,manifest_version,pickup_stop_ref,package_count,parcel_refs[1:101],frozen_at FROM custody_manifests WHERE mission_ref=$1 AND tenant_ref=$2 AND manifest_ref=$3 AND manifest_version=$4`, ref, tenant, manifestRef, manifestVersion).Scan(&legacy.Reference, &legacy.Version, &legacy.PickupStop, &legacy.PackageCount, &legacy.ParcelReferences, &legacy.FrozenAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrContract
			}
			return nil, err
		}
		out.Legacy = legacy
		if len(legacy.ParcelReferences) > 100 {
			out.Truncated = true
			legacy.ParcelReferences = legacy.ParcelReferences[:100]
		}
	}
	// Every query is compiled and parameterized. No whole row/JSON payload is
	// serialized, so later private columns cannot leak through this projection.
	rows, err := tx.Query(ctx, `SELECT stop_ref,stop_type,sequence,status,version FROM custody_mission_stops WHERE mission_ref=$1 ORDER BY sequence,stop_ref LIMIT 101`, ref)
	if err != nil {
		return nil, err
	}
	stops, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustodyStop, error) {
		var x CustodyStop
		err := row.Scan(&x.Reference, &x.Type, &x.Sequence, &x.Status, &x.Version)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out.Stops = stops
	rows, err = tx.Query(ctx, `SELECT manifest_ref,manifest_version,pickup_stop_ref,source_authority,package_count,aggregate_weight_grams,aggregate_volume_ml,required,status,NULLIF(current_custodian_ref,''),version FROM custody_stop_manifests WHERE mission_ref=$1 ORDER BY manifest_ref,manifest_version LIMIT 101`, ref)
	if err != nil {
		return nil, err
	}
	manifests, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustodyManifest, error) {
		var x CustodyManifest
		err := row.Scan(&x.Reference, &x.ManifestVersion, &x.PickupStop, &x.Authority, &x.PackageCount, &x.WeightGrams, &x.VolumeML, &x.Required, &x.Status, &x.Custodian, &x.Version)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out.Manifests = manifests
	rows, err = tx.Query(ctx, `SELECT package_ref,manifest_ref,manifest_version,package_count,weight_grams,volume_ml,status FROM custody_manifest_packages WHERE mission_ref=$1 ORDER BY package_ref LIMIT 101`, ref)
	if err != nil {
		return nil, err
	}
	packages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustodyPackage, error) {
		var x CustodyPackage
		err := row.Scan(&x.Reference, &x.Manifest, &x.ManifestVersion, &x.PackageCount, &x.WeightGrams, &x.VolumeML, &x.Status)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out.Packages = packages
	rows, err = tx.Query(ctx, `SELECT id,manifest_ref,manifest_version,custodian_ref,acquired_via,acquired_at,release_reason,released_at FROM custody_manifest_holdings WHERE mission_ref=$1 ORDER BY (released_at IS NULL) DESC,acquired_at DESC,id LIMIT 101`, ref)
	if err != nil {
		return nil, err
	}
	holdings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustodyHolding, error) {
		var x CustodyHolding
		err := row.Scan(&x.Reference, &x.Manifest, &x.ManifestVersion, &x.Custodian, &x.AcquiredVia, &x.AcquiredAt, &x.ReleaseReason, &x.ReleasedAt)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out.Holdings = holdings
	rows, err = tx.Query(ctx, `SELECT transfer_operation_ref,manifest_ref,manifest_version,from_custodian_ref,to_custodian_ref,package_count,created_at FROM custody_transfer_items WHERE mission_ref=$1 ORDER BY created_at DESC,transfer_operation_ref,manifest_ref,manifest_version LIMIT 101`, ref)
	if err != nil {
		return nil, err
	}
	transfers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustodyTransfer, error) {
		var x CustodyTransfer
		err := row.Scan(&x.Operation, &x.Manifest, &x.ManifestVersion, &x.From, &x.To, &x.PackageCount, &x.At)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out.Transfers = transfers
	if len(out.Stops) > 100 {
		out.Truncated = true
		out.Stops = out.Stops[:100]
	}
	if len(out.Manifests) > 100 {
		out.Truncated = true
		out.Manifests = out.Manifests[:100]
	}
	if len(out.Packages) > 100 {
		out.Truncated = true
		out.Packages = out.Packages[:100]
	}
	if len(out.Holdings) > 100 {
		out.Truncated = true
		out.Holdings = out.Holdings[:100]
	}
	if len(out.Transfers) > 100 {
		out.Truncated = true
		out.Transfers = out.Transfers[:100]
	}
	if err := validateCustodyInventory(out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateCustodyInventory(c *CustodyInventory) error {
	if c == nil || c.ContractVersion != "custody.inventory.v1" || !adminread.Opaque(c.Authority) || c.Mode != "LEGACY_SINGLE" && c.Mode != "MULTI_MANIFEST" || (c.Mode == "LEGACY_SINGLE") != (c.Legacy != nil) || c.Stops == nil || c.Manifests == nil || c.Packages == nil || c.Holdings == nil || c.Transfers == nil || len(c.Stops) > 100 || len(c.Manifests) > 100 || len(c.Packages) > 100 || len(c.Holdings) > 100 || len(c.Transfers) > 100 {
		return ErrContract
	}
	valid := func(values ...string) bool {
		for _, value := range values {
			if !adminread.Opaque(value) {
				return false
			}
		}
		return true
	}
	optional := func(value *string) bool { return value == nil || adminread.Opaque(*value) }
	seen := map[string]bool{}
	unique := func(kind, ref string, version int64) bool {
		key := kind + "\x00" + ref + "\x00" + strconv.FormatInt(version, 10)
		if seen[key] {
			return false
		}
		seen[key] = true
		return true
	}
	if x := c.Legacy; x != nil && (!valid(x.Reference, x.PickupStop) || x.Version < 1 || x.PackageCount < 1 || x.FrozenAt.IsZero()) {
		return ErrContract
	}
	if x := c.Legacy; x != nil {
		if len(x.ParcelReferences) < 1 || len(x.ParcelReferences) > 100 {
			return ErrContract
		}
		for _, ref := range x.ParcelReferences {
			if !unique("parcel", ref, 0) || !valid(ref) {
				return ErrContract
			}
		}
	}
	for _, x := range c.Stops {
		if !unique("stop", x.Reference, 0) || !unique("sequence", "", x.Sequence) || !valid(x.Reference, x.Type, x.Status) || x.Sequence < 1 || x.Version < 1 {
			return ErrContract
		}
	}
	for _, x := range c.Manifests {
		if !unique("manifest", x.Reference, x.ManifestVersion) || !valid(x.Reference, x.Authority, x.Status) || !optional(x.PickupStop) || !optional(x.Custodian) || x.ManifestVersion < 1 || x.PackageCount < 1 || x.WeightGrams < 0 || x.VolumeML < 0 || x.Version < 1 {
			return ErrContract
		}
	}
	for _, x := range c.Packages {
		if !unique("package", x.Reference, 0) || !valid(x.Reference, x.Manifest, x.Status) || x.ManifestVersion < 1 || x.PackageCount < 1 || x.WeightGrams < 0 || x.VolumeML < 0 {
			return ErrContract
		}
	}
	for _, x := range c.Holdings {
		if !unique("holding", x.Reference, 0) || !valid(x.Reference, x.Manifest, x.Custodian, x.AcquiredVia) || !optional(x.ReleaseReason) || x.ManifestVersion < 1 || x.AcquiredAt.IsZero() || (x.ReleaseReason == nil) != (x.ReleasedAt == nil) {
			return ErrContract
		}
	}
	for _, x := range c.Transfers {
		if !unique("transfer", x.Operation+"\x00"+x.Manifest, x.ManifestVersion) || !valid(x.Operation, x.Manifest, x.From, x.To) || x.From == x.To || x.ManifestVersion < 1 || x.PackageCount < 1 || x.At.IsZero() {
			return ErrContract
		}
	}
	return nil
}
