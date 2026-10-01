package custodyops

import (
	"errors"
	"sort"
	"strings"
)

// Multi-manifest custody. One mission has ordered stops. Each pickup stop
// owns one or more immutable manifest snapshots, and each manifest owns one
// or more packages. Custody is tracked per manifest, so the ledger can always
// answer what a provider holds, what is still pending, what was transferred
// and what was delivered.

const (
	ModeLegacySingle  = "LEGACY_SINGLE"
	ModeMultiManifest = "MULTI_MANIFEST"

	StopPickup  = "PICKUP"
	StopDropoff = "DROPOFF"

	ManifestRegistered = "REGISTERED"
	ManifestCollected  = "COLLECTED"
	ManifestDelivered  = "DELIVERED"
	ManifestFailed     = "FAILED"
	ManifestMissing    = "MISSING"
	ManifestDamaged    = "DAMAGED"
	ManifestReturned   = "RETURNED"
	ManifestReplanned  = "CANCELLED_BY_REPLAN"
)

var (
	// ErrStaleSourceVersion means a command carried an older source version
	// than the custody ledger has already applied.
	ErrStaleSourceVersion = errors.New("custody source version is stale")
	// ErrManifestsUnaccounted blocks completion while required manifests are open.
	ErrManifestsUnaccounted = errors.New("required manifests are not accounted for")
)

type ManifestKey struct {
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`
}

type StopInput struct {
	StopRef  string `json:"stop_ref"`
	StopType string `json:"stop_type"`
	Sequence int    `json:"sequence"`
}

type PackageInput struct {
	PackageRef   string `json:"package_ref"`
	PackageCount int64  `json:"package_count"`
	WeightGrams  int64  `json:"weight_grams"`
	VolumeML     int64  `json:"volume_ml"`
}

type ManifestInput struct {
	ManifestRef            string         `json:"manifest_ref"`
	ManifestVersion        int64          `json:"manifest_version"`
	PickupStopRef          string         `json:"pickup_stop_ref"`
	SourceAuthority        string         `json:"source_authority"`
	SourceManifestChecksum string         `json:"source_manifest_checksum"`
	VendorRef              string         `json:"vendor_ref,omitempty"`
	VendorBranchRef        string         `json:"vendor_branch_ref,omitempty"`
	Handling               []string       `json:"handling,omitempty"`
	Optional               bool           `json:"optional,omitempty"`
	Packages               []PackageInput `json:"packages"`
}

func (m ManifestInput) Key() ManifestKey {
	return ManifestKey{ManifestRef: m.ManifestRef, ManifestVersion: m.ManifestVersion}
}

// PackageCount and weights are derived from the package snapshot so the
// aggregate can never disagree with its items.
func (m ManifestInput) Totals() (count, weight, volume int64) {
	for _, p := range m.Packages {
		count += p.PackageCount
		weight += p.WeightGrams
		volume += p.VolumeML
	}
	return
}

type RegisterMissionCommand struct {
	OperationRef         string          `json:"operation_ref"`
	CorrelationID        string          `json:"correlation_id,omitempty"`
	MissionRef           string          `json:"mission_ref"`
	TenantRef            string          `json:"tenant_ref"`
	ReceiverChallengeRef string          `json:"receiver_challenge_ref"`
	SourceVersion        int64           `json:"source_version"`
	Stops                []StopInput     `json:"stops"`
	Manifests            []ManifestInput `json:"manifests"`
}

// Normalize validates the mission topology. Exactly one dropoff is the final
// stop, every manifest belongs to a pickup stop, and package refs are unique
// across the mission. More than one manifest per stop is allowed.
func (c *RegisterMissionCommand) Normalize() error {
	c.OperationRef, c.MissionRef, c.TenantRef = trim(c.OperationRef), trim(c.MissionRef), trim(c.TenantRef)
	c.ReceiverChallengeRef, c.CorrelationID = trim(c.ReceiverChallengeRef), trim(c.CorrelationID)
	if c.OperationRef == "" || c.MissionRef == "" || c.TenantRef == "" || c.ReceiverChallengeRef == "" || c.SourceVersion < 1 || len(c.Stops) < 2 || len(c.Manifests) == 0 {
		return ErrInvalid
	}
	pickups := map[string]bool{}
	sequences := map[int]bool{}
	var dropoffs, lastSequence int
	var lastType string
	for i := range c.Stops {
		stop := &c.Stops[i]
		stop.StopRef, stop.StopType = trim(stop.StopRef), strings.ToUpper(trim(stop.StopType))
		if stop.StopRef == "" || stop.Sequence < 1 || sequences[stop.Sequence] || pickups[stop.StopRef] {
			return ErrInvalid
		}
		sequences[stop.Sequence] = true
		switch stop.StopType {
		case StopPickup:
			pickups[stop.StopRef] = true
		case StopDropoff:
			dropoffs++
		default:
			return ErrInvalid
		}
		if stop.Sequence > lastSequence {
			lastSequence, lastType = stop.Sequence, stop.StopType
		}
	}
	if dropoffs != 1 || lastType != StopDropoff || len(pickups) == 0 {
		return ErrInvalid
	}
	seenManifest := map[ManifestKey]bool{}
	seenPackage := map[string]bool{}
	for i := range c.Manifests {
		m := &c.Manifests[i]
		m.ManifestRef, m.PickupStopRef = trim(m.ManifestRef), trim(m.PickupStopRef)
		m.SourceAuthority, m.SourceManifestChecksum = trim(m.SourceAuthority), trim(m.SourceManifestChecksum)
		m.VendorRef, m.VendorBranchRef = trim(m.VendorRef), trim(m.VendorBranchRef)
		if m.ManifestRef == "" || m.ManifestVersion < 1 || !pickups[m.PickupStopRef] || m.SourceAuthority == "" ||
			m.SourceManifestChecksum == "" || len(m.SourceManifestChecksum) > 128 || len(m.Packages) == 0 || seenManifest[m.Key()] {
			return ErrInvalid
		}
		seenManifest[m.Key()] = true
		for j := range m.Packages {
			p := &m.Packages[j]
			p.PackageRef = trim(p.PackageRef)
			if p.PackageRef == "" || p.PackageCount < 1 || p.WeightGrams < 0 || p.VolumeML < 0 || seenPackage[p.PackageRef] {
				return ErrInvalid
			}
			seenPackage[p.PackageRef] = true
		}
		for j := range m.Handling {
			m.Handling[j] = strings.ToUpper(trim(m.Handling[j]))
		}
	}
	for stop := range pickups {
		found := false
		for _, m := range c.Manifests {
			if m.PickupStopRef == stop {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}

type CollectManifestsCommand struct {
	OperationRef          string        `json:"operation_ref"`
	CorrelationID         string        `json:"correlation_id,omitempty"`
	MissionRef            string        `json:"mission_ref"`
	TenantRef             string        `json:"tenant_ref"`
	StopRef               string        `json:"stop_ref"`
	ProviderRef           string        `json:"provider_ref"`
	HandoverChallengeRef  string        `json:"handover_challenge_ref"`
	HandoverVerifiedByRef string        `json:"handover_verified_by_ref"`
	SourceVersion         int64         `json:"source_version"`
	Manifests             []ManifestKey `json:"manifests"`
}

type TransferManifestsCommand struct {
	OperationRef            string        `json:"operation_ref"`
	CorrelationID           string        `json:"correlation_id,omitempty"`
	MissionRef              string        `json:"mission_ref"`
	TenantRef               string        `json:"tenant_ref"`
	TransferRef             string        `json:"transfer_ref"`
	OldProviderRef          string        `json:"old_provider_ref"`
	NewProviderRef          string        `json:"new_provider_ref"`
	EvidenceRef             string        `json:"evidence_ref"`
	OldProviderConfirm      bool          `json:"old_provider_confirmed"`
	NewProviderConfirm      bool          `json:"new_provider_confirmed"`
	NewReceiverChallengeRef string        `json:"new_receiver_challenge_ref"`
	SourceVersion           int64         `json:"source_version"`
	Manifests               []ManifestKey `json:"manifests"`
}

type DeliverManifestsCommand struct {
	OperationRef         string        `json:"operation_ref"`
	CorrelationID        string        `json:"correlation_id,omitempty"`
	MissionRef           string        `json:"mission_ref"`
	TenantRef            string        `json:"tenant_ref"`
	DropoffStopRef       string        `json:"dropoff_stop_ref"`
	ProviderRef          string        `json:"provider_ref"`
	DeliveryChallengeRef string        `json:"delivery_challenge_ref"`
	SourceVersion        int64         `json:"source_version"`
	Manifests            []ManifestKey `json:"manifests"`
}

type ManifestExceptionCommand struct {
	OperationRef  string      `json:"operation_ref"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	MissionRef    string      `json:"mission_ref"`
	TenantRef     string      `json:"tenant_ref"`
	ActorRef      string      `json:"actor_ref"`
	Manifest      ManifestKey `json:"manifest"`
	Status        string      `json:"status"`
	PackageRefs   []string    `json:"package_refs,omitempty"`
	SourceVersion int64       `json:"source_version"`
}

// ManifestState is the ledger view of one manifest inside a snapshot.
type ManifestState struct {
	ManifestKey
	PickupStopRef string `json:"pickup_stop_ref"`
	Status        string `json:"status"`
	Custodian     string `json:"current_custodian_ref"`
	PackageCount  int64  `json:"package_count"`
	Required      bool   `json:"required"`
}

type StopState struct {
	StopRef  string `json:"stop_ref"`
	StopType string `json:"stop_type"`
	Sequence int    `json:"sequence"`
	Status   string `json:"status"`
}

// MissionSnapshot is the locked, in-transaction view that every decision
// function evaluates.
type MissionSnapshot struct {
	MissionRef          string                        `json:"mission_ref"`
	Mode                string                        `json:"custody_mode"`
	TenantRef           string                        `json:"tenant_ref"`
	Status              string                        `json:"status"`
	CurrentProvider     string                        `json:"current_provider_ref"`
	ReplacementProvider string                        `json:"replacement_provider_ref"`
	SourceVersion       int64                         `json:"source_version"`
	Stops               map[string]StopState          `json:"-"`
	Manifests           map[ManifestKey]ManifestState `json:"-"`
}

func canonicalKeys(keys []ManifestKey) ([]ManifestKey, error) {
	if len(keys) == 0 {
		return nil, ErrInvalid
	}
	seen := map[ManifestKey]bool{}
	out := make([]ManifestKey, 0, len(keys))
	for _, key := range keys {
		key.ManifestRef = trim(key.ManifestRef)
		if key.ManifestRef == "" || key.ManifestVersion < 1 || seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ManifestRef == out[j].ManifestRef {
			return out[i].ManifestVersion < out[j].ManifestVersion
		}
		return out[i].ManifestRef < out[j].ManifestRef
	})
	return out, nil
}

func (s MissionSnapshot) checkCommon(tenant string, sourceVersion int64) error {
	if s.Mode != ModeMultiManifest {
		return ErrConflict
	}
	if s.TenantRef != tenant {
		return ErrUnauthorized
	}
	if sourceVersion < 1 {
		return ErrInvalid
	}
	if sourceVersion < s.SourceVersion {
		return ErrStaleSourceVersion
	}
	if s.Status == "COMPLETED" || s.Status == "CANCELLED" {
		return ErrConflict
	}
	return nil
}

// HeldBy lists the manifests actively held by one custodian, sorted.
func (s MissionSnapshot) HeldBy(custodian string) []ManifestKey {
	var out []ManifestKey
	for key, m := range s.Manifests {
		if m.Status == ManifestCollected && m.Custodian == custodian {
			out = append(out, key)
		}
	}
	sorted, _ := canonicalKeys(out)
	return sorted
}

// Unaccounted lists required manifests that are neither delivered, returned
// nor removed by replanning.
func (s MissionSnapshot) Unaccounted() []ManifestKey {
	var out []ManifestKey
	for key, m := range s.Manifests {
		if m.Required && m.Status != ManifestDelivered && m.Status != ManifestReturned && m.Status != ManifestReplanned {
			out = append(out, key)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sorted, _ := canonicalKeys(out)
	return sorted
}

// CollectionDecision is the outcome of a pickup handover evaluation.
type CollectionDecision struct {
	Manifests    []ManifestKey
	StopStatus   string
	AlreadyHeld  bool
	PackageCount int64
}

// DecideCollection applies pickup invariants. Only manifests registered at
// this stop may be collected, and only by the mission's current provider
// after a verified handover. A repeat of an already applied collection is
// reported as AlreadyHeld and never duplicates custody.
func DecideCollection(s MissionSnapshot, c CollectManifestsCommand) (CollectionDecision, error) {
	if trim(c.OperationRef) == "" || trim(c.StopRef) == "" || trim(c.ProviderRef) == "" || trim(c.HandoverChallengeRef) == "" || trim(c.HandoverVerifiedByRef) == "" {
		return CollectionDecision{}, ErrInvalid
	}
	if err := s.checkCommon(trim(c.TenantRef), c.SourceVersion); err != nil {
		return CollectionDecision{}, err
	}
	keys, err := canonicalKeys(c.Manifests)
	if err != nil {
		return CollectionDecision{}, err
	}
	stop, ok := s.Stops[trim(c.StopRef)]
	if !ok || stop.StopType != StopPickup {
		return CollectionDecision{}, ErrConflict
	}
	switch s.Status {
	case "REPLACEMENT_REQUIRED", "TRANSFER_REQUIRED", "RECOVERY_REQUIRED", "RETURN_REQUIRED":
		return CollectionDecision{}, ErrConflict
	}
	provider := trim(c.ProviderRef)
	if s.CurrentProvider != "" && s.CurrentProvider != provider {
		return CollectionDecision{}, ErrUnauthorized
	}
	held := 0
	var packages int64
	for _, key := range keys {
		m, ok := s.Manifests[key]
		if !ok || m.PickupStopRef != stop.StopRef {
			return CollectionDecision{}, ErrConflict
		}
		switch {
		case m.Status == ManifestCollected && m.Custodian == provider:
			held++
		case m.Status == ManifestRegistered:
			packages += m.PackageCount
		default:
			return CollectionDecision{}, ErrConflict
		}
	}
	if held == len(keys) {
		return CollectionDecision{Manifests: keys, StopStatus: stop.Status, AlreadyHeld: true}, nil
	}
	if held > 0 {
		return CollectionDecision{}, ErrConflict
	}
	collectedAtStop := map[ManifestKey]bool{}
	for _, key := range keys {
		collectedAtStop[key] = true
	}
	status := "COLLECTED"
	for key, m := range s.Manifests {
		if m.PickupStopRef == stop.StopRef && m.Required && m.Status == ManifestRegistered && !collectedAtStop[key] {
			status = "PARTIALLY_COLLECTED"
		}
	}
	return CollectionDecision{Manifests: keys, StopStatus: status, PackageCount: packages}, nil
}

type TransferItem struct {
	ManifestKey
	From         string `json:"from_custodian_ref"`
	To           string `json:"to_custodian_ref"`
	PackageCount int64  `json:"package_count"`
}

// DecideTransfer requires the transfer to move exactly the manifests the old
// provider actively holds, to the one assigned replacement, with both
// confirmations. Pending future-stop manifests stay pending.
func DecideTransfer(s MissionSnapshot, c TransferManifestsCommand) ([]TransferItem, error) {
	if trim(c.OperationRef) == "" || trim(c.TransferRef) == "" || trim(c.EvidenceRef) == "" || trim(c.NewReceiverChallengeRef) == "" ||
		!c.OldProviderConfirm || !c.NewProviderConfirm {
		return nil, ErrInvalid
	}
	oldProvider, newProvider := trim(c.OldProviderRef), trim(c.NewProviderRef)
	if oldProvider == "" || newProvider == "" || oldProvider == newProvider {
		return nil, ErrInvalid
	}
	if err := s.checkCommon(trim(c.TenantRef), c.SourceVersion); err != nil {
		return nil, err
	}
	if s.Status != "TRANSFER_REQUIRED" || s.CurrentProvider != oldProvider || s.ReplacementProvider != newProvider {
		return nil, ErrConflict
	}
	keys, err := canonicalKeys(c.Manifests)
	if err != nil {
		return nil, err
	}
	held := s.HeldBy(oldProvider)
	if len(held) != len(keys) {
		return nil, ErrConflict
	}
	items := make([]TransferItem, 0, len(keys))
	for i, key := range keys {
		if held[i] != key {
			return nil, ErrConflict
		}
		items = append(items, TransferItem{ManifestKey: key, From: oldProvider, To: newProvider, PackageCount: s.Manifests[key].PackageCount})
	}
	return items, nil
}

// DeliveryDecision reports which manifests the dropoff hands over and
// whether that accounts for every required manifest.
type DeliveryDecision struct {
	Manifests       []ManifestKey
	MissionComplete bool
}

// DecideDelivery lets only the active custodian deliver manifests they hold.
// A future-stop manifest that was never collected cannot be delivered.
func DecideDelivery(s MissionSnapshot, c DeliverManifestsCommand) (DeliveryDecision, error) {
	if trim(c.OperationRef) == "" || trim(c.DropoffStopRef) == "" || trim(c.ProviderRef) == "" || trim(c.DeliveryChallengeRef) == "" {
		return DeliveryDecision{}, ErrInvalid
	}
	if err := s.checkCommon(trim(c.TenantRef), c.SourceVersion); err != nil {
		return DeliveryDecision{}, err
	}
	stop, ok := s.Stops[trim(c.DropoffStopRef)]
	if !ok || stop.StopType != StopDropoff {
		return DeliveryDecision{}, ErrConflict
	}
	if s.Status == "REPLACEMENT_REQUIRED" || s.Status == "TRANSFER_REQUIRED" {
		return DeliveryDecision{}, ErrConflict
	}
	provider := trim(c.ProviderRef)
	if s.CurrentProvider != provider {
		return DeliveryDecision{}, ErrUnauthorized
	}
	keys, err := canonicalKeys(c.Manifests)
	if err != nil {
		return DeliveryDecision{}, err
	}
	delivering := map[ManifestKey]bool{}
	for _, key := range keys {
		m, ok := s.Manifests[key]
		if !ok || m.Status != ManifestCollected || m.Custodian != provider {
			return DeliveryDecision{}, ErrConflict
		}
		delivering[key] = true
	}
	complete := true
	for _, key := range s.Unaccounted() {
		if !delivering[key] {
			complete = false
			break
		}
	}
	return DeliveryDecision{Manifests: keys, MissionComplete: complete}, nil
}

// ExceptionDecision is the ledger effect of a manifest exception.
type ExceptionDecision struct {
	Manifest       ManifestState
	PackageStatus  string
	ReleaseCustody bool
}

// DecideException records FAILED, MISSING or DAMAGED. Damage to goods still
// being carried keeps the manifest in the provider's custody and marks
// packages damaged. Failure or loss ends custody. A delivered manifest can
// never move backwards.
func DecideException(s MissionSnapshot, c ManifestExceptionCommand) (ExceptionDecision, error) {
	if trim(c.OperationRef) == "" || trim(c.ActorRef) == "" {
		return ExceptionDecision{}, ErrInvalid
	}
	status := strings.ToUpper(trim(c.Status))
	if status != ManifestFailed && status != ManifestMissing && status != ManifestDamaged {
		return ExceptionDecision{}, ErrInvalid
	}
	if err := s.checkCommon(trim(c.TenantRef), c.SourceVersion); err != nil {
		return ExceptionDecision{}, err
	}
	key := ManifestKey{ManifestRef: trim(c.Manifest.ManifestRef), ManifestVersion: c.Manifest.ManifestVersion}
	m, ok := s.Manifests[key]
	if !ok {
		return ExceptionDecision{}, ErrNotFound
	}
	if m.Status == ManifestDelivered || m.Status == ManifestReturned || m.Status == ManifestReplanned {
		return ExceptionDecision{}, ErrConflict
	}
	packageStatus := ManifestMissing
	if status == ManifestDamaged {
		packageStatus = ManifestDamaged
		if m.Status == ManifestCollected {
			return ExceptionDecision{Manifest: m, PackageStatus: packageStatus}, nil
		}
	}
	release := m.Status == ManifestCollected
	m.Status, m.Custodian = status, ""
	return ExceptionDecision{Manifest: m, PackageStatus: packageStatus, ReleaseCustody: release}, nil
}
