package vendorownedhandover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sharedhandover "cosmicforge/logistics/shared/go/handover"
)

type Repository struct {
	db     *pgxpool.Pool
	secret []byte
	policy Policy
}

func NewRepository(db *pgxpool.Pool, secret []byte, policy Policy) (*Repository, error) {
	if db == nil || len(secret) < 16 {
		return nil, ErrInvalid
	}
	return &Repository{db: db, secret: append([]byte(nil), secret...), policy: policy.normalized()}, nil
}

func fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (r *Repository) Bind(ctx context.Context, b MissionBinding, now time.Time) (secretIssue, bool, error) {
	if !b.valid() {
		return secretIssue{}, false, ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return secretIssue{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "vendor-mission:"+b.MissionRef); err != nil {
		return secretIssue{}, false, err
	}
	fp := fingerprint(b)
	if _, found, opErr := operation(ctx, tx, "vendor-service", b.OperationRef, fp); opErr != nil {
		return secretIssue{}, false, opErr
	} else if found {
		o, readErr := readTx(ctx, tx, b.MissionRef, Pickup)
		return secretIssue{Observation: o}, true, commitWith(tx, readErr)
	}
	parcels, _ := json.Marshal(b.ParcelRefs)
	var currentProvider, currentReceiver, currentManifest, fulfilment, business, branch, status string
	var currentManifestVersion, sourceVersion int64
	created := false
	err = tx.QueryRow(ctx, `SELECT provider_ref,receiver_ref,manifest_ref,fulfilment_ref,business_id,branch_id,status,manifest_version,source_version FROM dispatch_vendor_delivery_missions WHERE mission_ref=$1 FOR UPDATE`, b.MissionRef).
		Scan(&currentProvider, &currentReceiver, &currentManifest, &fulfilment, &business, &branch, &status, &currentManifestVersion, &sourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_vendor_delivery_missions(mission_ref,fulfilment_ref,business_id,branch_id,provider_ref,receiver_ref,receiver_phone,manifest_ref,manifest_version,parcel_refs,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PICKUP_REQUIRED')`, b.MissionRef, b.FulfilmentRef, b.BusinessID, b.BranchID, b.ProviderRef, b.ReceiverRef, b.ReceiverPhone, b.ManifestRef, b.ManifestVersion, parcels)
		if err != nil {
			return secretIssue{}, false, err
		}
		created = true
	} else if err != nil {
		return secretIssue{}, false, err
	} else if fulfilment != b.FulfilmentRef || business != b.BusinessID || branch != b.BranchID || currentManifest != b.ManifestRef || currentManifestVersion != b.ManifestVersion {
		return secretIssue{}, false, ErrConflict
	} else if currentProvider != b.ProviderRef || currentReceiver != b.ReceiverRef {
		return secretIssue{}, false, ErrConflict
	}
	if !created {
		o, readErr := readTx(ctx, tx, b.MissionRef, Pickup)
		if readErr == nil {
			if err = insertOperation(ctx, tx, "vendor-service", b.OperationRef, fp, b.MissionRef, o.HandoverRef, o.Status); err != nil {
				return secretIssue{}, false, err
			}
			return secretIssue{Observation: o}, true, tx.Commit(ctx)
		}
		if !errors.Is(readErr, ErrNotFound) {
			return secretIssue{}, false, readErr
		}
	}
	issue, err := r.issueTx(ctx, tx, b.MissionRef, Pickup, b.ProviderRef, b.ReceiverRef, b.ReceiverPhone, b.ManifestRef, b.ManifestVersion, 0, now)
	if err != nil {
		return secretIssue{}, false, err
	}
	if err = insertOperation(ctx, tx, "vendor-service", b.OperationRef, fp, b.MissionRef, issue.HandoverRef, issue.Status); err != nil {
		return secretIssue{}, false, err
	}
	if err = emit(ctx, tx, "dispatch.vendor_delivery.pickup_handover_required.v1", b.MissionRef, issue.SourceVersion, b.CorrelationID, map[string]any{"mission_ref": b.MissionRef, "fulfilment_ref": b.FulfilmentRef, "stage": "PICKUP", "status": "ACTIVE"}); err != nil {
		return secretIssue{}, false, err
	}
	return issue, false, tx.Commit(ctx)
}

func (r *Repository) issueTx(ctx context.Context, tx pgx.Tx, mission string, stage Stage, provider, receiver, phone, manifest string, manifestVersion int64, resends int, now time.Time) (secretIssue, error) {
	var version int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM dispatch_vendor_handover_challenges WHERE mission_ref=$1 AND stage=$2`, mission, stage).Scan(&version); err != nil {
		return secretIssue{}, err
	}
	ref := "vho_" + uuid.NewString()
	rec, err := sharedhandover.NewSecureCodeRecord(r.secret, "dispatch-vendor-owned", sharedhandover.Step(strings.ToLower(string(stage))), ref, version, mission, provider, receiver, resends, now)
	if err != nil {
		return secretIssue{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_vendor_handover_challenges(challenge_ref,mission_ref,stage,version,provider_ref,receiver_ref,manifest_ref,manifest_version,otp_keyed_hash,qr_keyed_hash,max_attempts,max_resends,resend_count,resend_cooldown_until,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, ref, mission, stage, version, provider, receiver, manifest, manifestVersion, rec.Hash, rec.QRHash, r.policy.MaxAttempts, r.policy.MaxResends, resends, now.Add(r.policy.Cooldown), rec.ExpiresAt)
	if err != nil {
		return secretIssue{}, err
	}
	return secretIssue{Observation: Observation{MissionRef: mission, HandoverRef: ref, Stage: stage, Status: "ACTIVE", SourceVersion: version}, OTP: rec.Code, QR: rec.QRToken, RecipientRef: map[bool]string{true: receiver, false: provider}[stage == Delivery], RecipientPhone: phone, ExpiresAt: rec.ExpiresAt}, nil
}

func (r *Repository) Read(ctx context.Context, mission string, stage Stage) (Observation, error) {
	if mission == "" || (stage != Pickup && stage != Delivery) {
		return Observation{}, ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Observation{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	o, err := readTx(ctx, tx, mission, stage)
	return o, commitWith(tx, err)
}

// ReadCredential reconstructs only a live challenge for its current bound
// holder. The stored keyed hashes also reject older random-format challenges
// until they are rotated by the existing resend contract.
func (r *Repository) ReadCredential(ctx context.Context, mission, ref string, stage Stage, holder string) (sharedhandover.CodeRecord, error) {
	var record sharedhandover.CodeRecord
	var status, provider, receiver, branch string
	err := r.db.QueryRow(ctx, `SELECT h.challenge_ref,h.version,h.provider_ref,h.receiver_ref,h.otp_keyed_hash,h.qr_keyed_hash,h.expires_at,h.status,m.provider_ref,m.receiver_ref,m.branch_id
		FROM dispatch_vendor_handover_challenges h JOIN dispatch_vendor_delivery_missions m ON m.mission_ref=h.mission_ref
		WHERE h.mission_ref=$1 AND h.challenge_ref=$2 AND h.stage=$3`, mission, ref, stage).
		Scan(&record.ChallengeRef, &record.Version, &record.ProviderID, &record.OwnerID, &record.Hash, &record.QRHash, &record.ExpiresAt, &status, &provider, &receiver, &branch)
	if errors.Is(err, pgx.ErrNoRows) {
		return sharedhandover.CodeRecord{}, ErrNotFound
	}
	if err != nil {
		return sharedhandover.CodeRecord{}, err
	}
	expectedOwner := receiver
	if stage == Return {
		expectedOwner = branch
	}
	if status != "ACTIVE" || !record.ExpiresAt.After(time.Now().UTC()) || provider != record.ProviderID || expectedOwner != record.OwnerID ||
		(stage == Delivery && holder != receiver) || (stage != Delivery && holder != provider) {
		return sharedhandover.CodeRecord{}, ErrNotFound
	}
	record.Service, record.Stage, record.BookingID = "dispatch-vendor-owned", sharedhandover.Step(strings.ToLower(string(stage))), mission
	result, ok := sharedhandover.RecoverSecureCredentials(r.secret, record)
	if !ok {
		return sharedhandover.CodeRecord{}, ErrConflict
	}
	return result, nil
}

func readTx(ctx context.Context, tx pgx.Tx, mission string, stage Stage) (Observation, error) {
	var o Observation
	err := tx.QueryRow(ctx, `SELECT mission_ref,challenge_ref,stage,status,version FROM dispatch_vendor_handover_challenges WHERE mission_ref=$1 AND stage=$2 ORDER BY version DESC LIMIT 1`, mission, stage).Scan(&o.MissionRef, &o.HandoverRef, &o.Stage, &o.Status, &o.SourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Observation{}, ErrNotFound
	}
	return o, err
}

func (r *Repository) Confirm(ctx context.Context, c ConfirmCommand, caller, correlation string, now time.Time) (Observation, bool, *secretIssue, error) {
	if c.MissionRef == "" || c.ChallengeRef == "" || c.ProviderRef == "" || c.Credential == "" || c.OperationRef == "" || (c.Stage != Pickup && c.Stage != Delivery && c.Stage != Return) || (c.Method != "OTP" && c.Method != "QR") || c.ManifestRef == "" || c.ManifestVersion < 1 {
		return Observation{}, false, nil, ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Observation{}, false, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	fp := fingerprint(c)
	if o, found, opErr := operation(ctx, tx, caller, c.OperationRef, fp); opErr != nil {
		return Observation{}, false, nil, opErr
	} else if found {
		return o, true, nil, commitWith(tx, nil)
	}
	var missionStatus, provider, receiver, phone, manifest, status, otpHash, qrHash string
	var manifestVersion, version int64
	var expires time.Time
	var attempts, maxAttempts int
	err = tx.QueryRow(ctx, `SELECT m.status,m.provider_ref,h.receiver_ref,m.receiver_phone,h.manifest_ref,h.status,h.otp_keyed_hash,h.qr_keyed_hash,h.manifest_version,h.version,h.expires_at,h.attempt_count,h.max_attempts FROM dispatch_vendor_delivery_missions m JOIN dispatch_vendor_handover_challenges h ON h.mission_ref=m.mission_ref WHERE m.mission_ref=$1 AND h.challenge_ref=$2 AND h.stage=$3 FOR UPDATE OF m,h`, c.MissionRef, c.ChallengeRef, c.Stage).Scan(&missionStatus, &provider, &receiver, &phone, &manifest, &status, &otpHash, &qrHash, &manifestVersion, &version, &expires, &attempts, &maxAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Observation{}, false, nil, ErrNotFound
	}
	if err != nil {
		return Observation{}, false, nil, err
	}
	// A concurrent retry can miss the operation record before blocking on the
	// challenge row: under READ COMMITTED, the winner may insert that record
	// while this transaction waits for FOR UPDATE. Re-read after acquiring the
	// custody lock so the retry returns the canonical result instead of seeing
	// the now-consumed challenge as a rejected credential.
	if o, found, opErr := operation(ctx, tx, caller, c.OperationRef, fp); opErr != nil {
		return Observation{}, false, nil, opErr
	} else if found {
		return o, true, nil, commitWith(tx, nil)
	}
	if missionStatus == "CANCELLED" || missionStatus == "FAILED" || status != "ACTIVE" || provider != c.ProviderRef || manifest != c.ManifestRef || manifestVersion != c.ManifestVersion {
		return Observation{}, false, nil, ErrRejected
	}
	if !now.Before(expires) {
		_, _ = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='EXPIRED',updated_at=$2 WHERE challenge_ref=$1`, c.ChallengeRef, now)
		_ = tx.Commit(ctx)
		return Observation{}, false, nil, ErrRejected
	}
	rec := sharedhandover.CodeRecord{ChallengeRef: c.ChallengeRef, Version: version, Service: "dispatch-vendor-owned", Stage: sharedhandover.Step(strings.ToLower(string(c.Stage))), BookingID: c.MissionRef, ProviderID: provider, OwnerID: receiver, ExpiresAt: expires}
	expected := otpHash
	if c.Method == "QR" {
		expected = qrHash
	}
	if !sharedhandover.VerifySecureCredential(r.secret, c.Method, rec, c.Credential, expected) {
		attempts++
		newStatus := "ACTIVE"
		if attempts >= maxAttempts {
			newStatus = "LOCKED"
		}
		_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET attempt_count=$2,status=$3,updated_at=$4 WHERE challenge_ref=$1`, c.ChallengeRef, attempts, newStatus, now)
		if err != nil {
			return Observation{}, false, nil, err
		}
		if err = tx.Commit(ctx); err != nil {
			return Observation{}, false, nil, err
		}
		return Observation{}, false, nil, ErrRejected
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='CONSUMED',consumed_at=$2,updated_at=$2 WHERE challenge_ref=$1 AND status='ACTIVE'`, c.ChallengeRef, now)
	if err != nil {
		return Observation{}, false, nil, err
	}
	missionNext := "PICKED_UP"
	event := "dispatch.vendor_delivery.picked_up.v1"
	if c.Stage == Delivery {
		missionNext = "COMPLETED"
		event = "dispatch.vendor_delivery.completed.v1"
	} else if c.Stage == Return {
		missionNext = "RETURNED"
		event = "dispatch.vendor_delivery.return_completed.v1"
	}
	var sourceVersion int64
	if err = tx.QueryRow(ctx, `UPDATE dispatch_vendor_delivery_missions SET status=$2,source_version=source_version+1,updated_at=$3 WHERE mission_ref=$1 RETURNING source_version`, c.MissionRef, missionNext, now).Scan(&sourceVersion); err != nil {
		return Observation{}, false, nil, err
	}
	o := Observation{MissionRef: c.MissionRef, HandoverRef: c.ChallengeRef, Stage: c.Stage, Status: "CONSUMED", SourceVersion: sourceVersion}
	if err = insertOperation(ctx, tx, caller, c.OperationRef, fp, c.MissionRef, c.ChallengeRef, "CONSUMED"); err != nil {
		return Observation{}, false, nil, err
	}
	if err = emit(ctx, tx, event, c.MissionRef, sourceVersion, correlation, map[string]any{"mission_ref": c.MissionRef, "stage": c.Stage, "status": missionNext, "handover_ref": c.ChallengeRef}); err != nil {
		return Observation{}, false, nil, err
	}
	var next *secretIssue
	if c.Stage == Pickup {
		issue, issueErr := r.issueTx(ctx, tx, c.MissionRef, Delivery, provider, receiver, phone, manifest, manifestVersion, 0, now)
		if issueErr != nil {
			return Observation{}, false, nil, issueErr
		}
		next = &issue
		_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_delivery_missions SET status='DELIVERY_REQUIRED',source_version=source_version+1,updated_at=$2 WHERE mission_ref=$1`, c.MissionRef, now)
		if err != nil {
			return Observation{}, false, nil, err
		}
	}
	return o, false, next, tx.Commit(ctx)
}

func (r *Repository) IssueReturn(ctx context.Context, mission, operationRef, correlation string, now time.Time) (secretIssue, bool, error) {
	if mission == "" || operationRef == "" {
		return secretIssue{}, false, ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return secretIssue{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	fp := fingerprint(map[string]string{"mission_ref": mission, "stage": string(Return)})
	if o, found, opErr := operation(ctx, tx, "vendor-service", operationRef, fp); opErr != nil {
		return secretIssue{}, false, opErr
	} else if found {
		var issue secretIssue
		if err = tx.QueryRow(ctx, `SELECT h.challenge_ref,h.version,h.expires_at,m.provider_ref,m.branch_id FROM dispatch_vendor_handover_challenges h JOIN dispatch_vendor_delivery_missions m USING(mission_ref) WHERE h.challenge_ref=$1`, o.HandoverRef).Scan(&issue.HandoverRef, &issue.SourceVersion, &issue.ExpiresAt, &issue.RecipientRef, &issue.RecipientPhone); err != nil {
			return secretIssue{}, false, err
		}
		issue.MissionRef, issue.Stage, issue.Status = mission, Return, "ACTIVE"
		return issue, true, tx.Commit(ctx)
	}
	var status, provider, branch, manifest string
	var manifestVersion int64
	if err = tx.QueryRow(ctx, `SELECT status,provider_ref,branch_id,manifest_ref,manifest_version FROM dispatch_vendor_delivery_missions WHERE mission_ref=$1 FOR UPDATE`, mission).Scan(&status, &provider, &branch, &manifest, &manifestVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return secretIssue{}, false, ErrNotFound
		}
		return secretIssue{}, false, err
	}
	if status != "PICKED_UP" && status != "DELIVERY_REQUIRED" && status != "RETURN_REQUIRED" {
		return secretIssue{}, false, ErrConflict
	}
	if status == "RETURN_REQUIRED" {
		var issue secretIssue
		if err = tx.QueryRow(ctx, `SELECT challenge_ref,version,expires_at FROM dispatch_vendor_handover_challenges WHERE mission_ref=$1 AND stage='RETURN' AND status='ACTIVE'`, mission).Scan(&issue.HandoverRef, &issue.SourceVersion, &issue.ExpiresAt); err != nil {
			return secretIssue{}, false, ErrConflict
		}
		issue.MissionRef, issue.Stage, issue.Status, issue.RecipientRef = mission, Return, "ACTIVE", provider
		return issue, true, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='INVALIDATED',invalidated_at=$2,invalidated_reason='RETURN_STARTED',updated_at=$2 WHERE mission_ref=$1 AND status='ACTIVE'`, mission, now); err != nil {
		return secretIssue{}, false, err
	}
	issue, err := r.issueTx(ctx, tx, mission, Return, provider, branch, "", manifest, manifestVersion, 0, now)
	if err != nil {
		return secretIssue{}, false, err
	}
	var sourceVersion int64
	if err = tx.QueryRow(ctx, `UPDATE dispatch_vendor_delivery_missions SET status='RETURN_REQUIRED',source_version=source_version+1,updated_at=$2 WHERE mission_ref=$1 RETURNING source_version`, mission, now).Scan(&sourceVersion); err != nil {
		return secretIssue{}, false, err
	}
	issue.SourceVersion = sourceVersion
	if err = insertOperation(ctx, tx, "vendor-service", operationRef, fp, mission, issue.HandoverRef, "ACTIVE"); err != nil {
		return secretIssue{}, false, err
	}
	if err = emit(ctx, tx, "dispatch.vendor_delivery.return_handover_required.v1", mission, sourceVersion, correlation, map[string]any{"mission_ref": mission, "handover_ref": issue.HandoverRef, "status": "RETURN_REQUIRED"}); err != nil {
		return secretIssue{}, false, err
	}
	return issue, false, tx.Commit(ctx)
}

func (r *Repository) Resend(ctx context.Context, mission string, stage Stage, provider, operation, caller, correlation string, now time.Time) (secretIssue, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return secretIssue{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var oldRef, mProvider, receiver, phone, manifest, status string
	var manifestVersion int64
	var resends, maxResends int
	var cooldown time.Time
	err = tx.QueryRow(ctx, `SELECT h.challenge_ref,m.provider_ref,m.receiver_ref,m.receiver_phone,h.manifest_ref,h.status,h.manifest_version,h.resend_count,h.max_resends,h.resend_cooldown_until FROM dispatch_vendor_delivery_missions m JOIN dispatch_vendor_handover_challenges h ON h.mission_ref=m.mission_ref WHERE m.mission_ref=$1 AND h.stage=$2 ORDER BY h.version DESC LIMIT 1 FOR UPDATE OF m,h`, mission, stage).Scan(&oldRef, &mProvider, &receiver, &phone, &manifest, &status, &manifestVersion, &resends, &maxResends, &cooldown)
	if err != nil {
		return secretIssue{}, ErrNotFound
	}
	recoverNotification := status == "INVALIDATED"
	if (status != "ACTIVE" && !recoverNotification) || mProvider != provider || resends >= maxResends || (!recoverNotification && now.Before(cooldown)) {
		return secretIssue{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='INVALIDATED',invalidated_at=$2,invalidated_reason='RESEND',updated_at=$2 WHERE challenge_ref=$1`, oldRef, now)
	if err != nil {
		return secretIssue{}, err
	}
	issue, err := r.issueTx(ctx, tx, mission, stage, mProvider, receiver, phone, manifest, manifestVersion, resends+1, now)
	if err != nil {
		return secretIssue{}, err
	}
	if err = insertOperation(ctx, tx, caller, operation, fingerprint([]string{mission, string(stage), provider}), mission, issue.HandoverRef, "ACTIVE"); err != nil {
		return secretIssue{}, err
	}
	if err = emit(ctx, tx, "dispatch.vendor_delivery.handover_rotated.v1", mission, issue.SourceVersion, correlation, map[string]any{"mission_ref": mission, "stage": stage, "status": "ACTIVE"}); err != nil {
		return secretIssue{}, err
	}
	return issue, tx.Commit(ctx)
}

func (r *Repository) ChangeBinding(ctx context.Context, c ChangeCommand, providerChange bool, correlation string, now time.Time) (secretIssue, error) {
	if c.MissionRef == "" || c.OperationRef == "" {
		return secretIssue{}, ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return secretIssue{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	fp := fingerprint(struct {
		Command        ChangeCommand
		ProviderChange bool
	}{c, providerChange})
	if o, found, opErr := operation(ctx, tx, "vendor-service", c.OperationRef, fp); opErr != nil {
		return secretIssue{}, opErr
	} else if found {
		return secretIssue{Observation: o}, tx.Commit(ctx)
	}
	var provider, receiver, phone, manifest, status string
	var manifestVersion int64
	err = tx.QueryRow(ctx, `SELECT provider_ref,receiver_ref,receiver_phone,manifest_ref,status,manifest_version FROM dispatch_vendor_delivery_missions WHERE mission_ref=$1 FOR UPDATE`, c.MissionRef).Scan(&provider, &receiver, &phone, &manifest, &status, &manifestVersion)
	if err != nil {
		return secretIssue{}, ErrNotFound
	}
	stage := Pickup
	if status == "DELIVERY_REQUIRED" {
		stage = Delivery
	} else if status != "ASSIGNED" && status != "PICKUP_REQUIRED" {
		return secretIssue{}, ErrConflict
	}
	if providerChange {
		if c.ProviderRef == "" || c.ProviderRef == provider {
			return secretIssue{}, ErrInvalid
		}
		provider = c.ProviderRef
	} else {
		if c.ReceiverRef == "" || c.ReceiverRef == receiver {
			return secretIssue{}, ErrInvalid
		}
		receiver = c.ReceiverRef
		phone = c.ReceiverPhone
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='INVALIDATED',invalidated_at=$2,invalidated_reason=$3,updated_at=$2 WHERE mission_ref=$1 AND status='ACTIVE'`, c.MissionRef, now, map[bool]string{true: "PROVIDER_REASSIGNED", false: "RECEIVER_CHANGED"}[providerChange])
	if err != nil {
		return secretIssue{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_delivery_missions SET provider_ref=$2,receiver_ref=$3,receiver_phone=$4,source_version=source_version+1,updated_at=$5 WHERE mission_ref=$1`, c.MissionRef, provider, receiver, phone, now)
	if err != nil {
		return secretIssue{}, err
	}
	issue, err := r.issueTx(ctx, tx, c.MissionRef, stage, provider, receiver, phone, manifest, manifestVersion, 0, now)
	if err != nil {
		return secretIssue{}, err
	}
	event := "dispatch.vendor_delivery.provider_reassigned.v1"
	if !providerChange {
		event = "dispatch.vendor_delivery.receiver_changed.v1"
	}
	if err = emit(ctx, tx, event, c.MissionRef, issue.SourceVersion, correlation, map[string]any{"mission_ref": c.MissionRef, "stage": stage, "status": "ACTIVE"}); err != nil {
		return secretIssue{}, err
	}
	if err = insertOperation(ctx, tx, "vendor-service", c.OperationRef, fp, c.MissionRef, issue.HandoverRef, issue.Status); err != nil {
		return secretIssue{}, err
	}
	return issue, tx.Commit(ctx)
}

func operation(ctx context.Context, tx pgx.Tx, caller, op, fp string) (Observation, bool, error) {
	var savedFP, mission, ref, status string
	err := tx.QueryRow(ctx, `SELECT request_fingerprint,mission_ref,challenge_ref,result_status FROM dispatch_vendor_handover_operations WHERE caller=$1 AND operation_ref=$2`, caller, op).Scan(&savedFP, &mission, &ref, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	if savedFP != fp {
		return Observation{}, false, ErrConflict
	}
	var stage Stage
	var version int64
	if err = tx.QueryRow(ctx, `SELECT stage,version FROM dispatch_vendor_handover_challenges WHERE challenge_ref=$1`, ref).Scan(&stage, &version); err != nil {
		return Observation{}, false, err
	}
	return Observation{MissionRef: mission, HandoverRef: ref, Stage: stage, Status: status, SourceVersion: version}, true, nil
}
func insertOperation(ctx context.Context, tx pgx.Tx, caller, op, fp, mission, ref, status string) error {
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_vendor_handover_operations(caller,operation_ref,request_fingerprint,mission_ref,challenge_ref,result_status) VALUES($1,$2,$3,$4,$5,$6)`, caller, op, fp, mission, ref, status)
	return err
}
func emit(ctx context.Context, tx pgx.Tx, event, mission string, version int64, correlation string, payload map[string]any) error {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,correlation_id) VALUES($1,1,'VENDOR_DELIVERY_MISSION',$2,$3,$4,$5,$6) ON CONFLICT(aggregate_type,aggregate_id,event_type,aggregate_version) DO NOTHING`, event, mission, version, raw, hex.EncodeToString(sum[:]), correlation)
	return err
}
func commitWith(tx pgx.Tx, err error) error {
	if err != nil {
		return err
	}
	return tx.Commit(context.Background())
}

func (r *Repository) Invalidate(ctx context.Context, ref, reason string, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='INVALIDATED',invalidated_at=$2,invalidated_reason=$3,updated_at=$2 WHERE challenge_ref=$1 AND status='ACTIVE'`, ref, now, reason)
	return err
}

func (r *Repository) Cancel(ctx context.Context, mission, operation, correlation string, now time.Time) error {
	if mission == "" || operation == "" {
		return ErrInvalid
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var status string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT status,source_version FROM dispatch_vendor_delivery_missions WHERE mission_ref=$1 FOR UPDATE`, mission).Scan(&status, &version); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "CANCELLED" {
		return tx.Commit(ctx)
	}
	if status == "DELIVERED" || status == "COMPLETED" {
		return ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_handover_challenges SET status='INVALIDATED',invalidated_at=$2,invalidated_reason='MISSION_CANCELLED',updated_at=$2 WHERE mission_ref=$1 AND status='ACTIVE'`, mission, now)
	if err != nil {
		return err
	}
	version++
	_, err = tx.Exec(ctx, `UPDATE dispatch_vendor_delivery_missions SET status='CANCELLED',source_version=$2,updated_at=$3 WHERE mission_ref=$1`, mission, version, now)
	if err != nil {
		return err
	}
	if err = emit(ctx, tx, "dispatch.vendor_delivery.cancelled.v1", mission, version, correlation, map[string]any{"mission_ref": mission, "status": "CANCELLED"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var _ = fmt.Sprintf
