package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	handover "cosmicforge/logistics/services/vendor-service/internal/integrations/handover"
)

type vendorOwnedRuntimeHandover struct {
	db        *pgxpool.Pool
	authority *handover.VendorOwned
}

func newVendorOwnedRuntimeHandover(db *pgxpool.Pool, origin, secret string, enabled bool) fulfilmentgroup.VendorHandoverAuthority {
	if !enabled || db == nil || origin == "" || secret == "" {
		return nil
	}
	return &vendorOwnedRuntimeHandover{db: db, authority: &handover.VendorOwned{Origin: origin, Secret: []byte(secret), Enabled: true}}
}

func (a *vendorOwnedRuntimeHandover) binding(ctx context.Context, r fulfilmentgroup.VendorHandoverRequest) (handover.Binding, error) {
	if a == nil || a.db == nil || a.authority == nil {
		return handover.Binding{}, fulfilmentgroup.ErrHandoverUnavailable
	}
	var b handover.Binding
	var parcels []byte
	err := a.db.QueryRow(ctx, `SELECT vm.id,vm.id,vm.business_id,vm.branch_id,COALESCE(vm.assigned_staff_id,''),po.customer_id,COALESCE(po.receiver_snapshot->>'receiver_phone',''),vm.plan_id||':'||vm.plan_mission_id,pv.version,jsonb_agg(p.id ORDER BY p.id) FROM commerce_vendor_missions vm JOIN commerce_fulfilment_groups g ON g.id=vm.group_id JOIN commerce_parent_orders po ON po.id=g.parent_order_id JOIN commerce_fulfilment_plan_versions pv ON pv.id=vm.plan_id JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=vm.plan_mission_id JOIN commerce_fulfilment_parcels p ON p.id=mp.parcel_id WHERE vm.id=$1 AND vm.business_id=$2 AND vm.branch_id=$3 AND vm.assigned_staff_id=$4 GROUP BY vm.id,po.customer_id,po.receiver_snapshot,pv.version`, r.MissionID, r.BusinessID, r.BranchID, r.RiderStaffID).Scan(&b.MissionRef, &b.FulfilmentRef, &b.BusinessID, &b.BranchID, &b.ProviderRef, &b.ReceiverRef, &b.ReceiverPhone, &b.ManifestRef, &b.ManifestVersion, &parcels)
	if err != nil || json.Unmarshal(parcels, &b.ParcelRefs) != nil {
		return handover.Binding{}, fulfilmentgroup.ErrHandoverUnavailable
	}
	b.Authority = "VENDOR_OWN_DELIVERY"
	return b, nil
}
func (a *vendorOwnedRuntimeHandover) VerifyPickup(ctx context.Context, r fulfilmentgroup.VendorHandoverRequest) (fulfilmentgroup.HandoverVerification, error) {
	b, err := a.binding(ctx, r)
	if err != nil {
		return fulfilmentgroup.HandoverVerification{}, err
	}
	if _, err = a.authority.Bind(ctx, b, "bind:"+b.FulfilmentRef, r.OperationRef); err != nil && !errors.Is(err, handover.ErrRejected) {
		return fulfilmentgroup.HandoverVerification{}, err
	}
	o, err := a.authority.Read(ctx, b, "PICKUP", r.OperationRef)
	if err != nil || o.Status != "CONSUMED" || (r.EvidenceRef != "" && r.EvidenceRef != o.HandoverRef) {
		return fulfilmentgroup.HandoverVerification{}, fulfilmentgroup.ErrHandoverUnavailable
	}
	return fulfilmentgroup.HandoverVerification{OpaqueRef: o.HandoverRef, EvidenceRef: "dispatch:" + o.HandoverRef}, nil
}
func (a *vendorOwnedRuntimeHandover) VerifyReceiver(ctx context.Context, r fulfilmentgroup.VendorHandoverRequest) (fulfilmentgroup.HandoverVerification, error) {
	b, err := a.binding(ctx, r)
	if err != nil {
		return fulfilmentgroup.HandoverVerification{}, err
	}
	if r.EvidenceRef == "" || r.Method == "" || r.Credential == "" {
		return fulfilmentgroup.HandoverVerification{}, fulfilmentgroup.ErrInvalid
	}
	o, err := a.authority.ConfirmDelivery(ctx, b, r.EvidenceRef, r.Method, r.Credential, "vendor-delivery:"+r.OperationRef, r.OperationRef)
	if err != nil || o.Status != "CONSUMED" {
		return fulfilmentgroup.HandoverVerification{}, fulfilmentgroup.ErrHandoverUnavailable
	}
	return fulfilmentgroup.HandoverVerification{OpaqueRef: o.HandoverRef, EvidenceRef: "dispatch:" + o.HandoverRef}, nil
}
func (a *vendorOwnedRuntimeHandover) VerifyReturn(ctx context.Context, r fulfilmentgroup.VendorHandoverRequest) (fulfilmentgroup.HandoverVerification, error) {
	b, err := a.binding(ctx, r)
	if err != nil {
		return fulfilmentgroup.HandoverVerification{}, err
	}
	if _, err = a.authority.EnsureReturn(ctx, b, "return:"+r.OperationRef, r.OperationRef); err != nil && !errors.Is(err, handover.ErrRejected) {
		return fulfilmentgroup.HandoverVerification{}, err
	}
	if r.EvidenceRef == "" || r.Method == "" || r.Credential == "" {
		return fulfilmentgroup.HandoverVerification{}, fulfilmentgroup.ErrHandoverUnavailable
	}
	o, err := a.authority.ConfirmReturn(ctx, b, r.EvidenceRef, r.Method, r.Credential, "vendor-return:"+r.OperationRef, r.OperationRef)
	if err != nil || o.Status != "CONSUMED" {
		return fulfilmentgroup.HandoverVerification{}, fulfilmentgroup.ErrHandoverUnavailable
	}
	return fulfilmentgroup.HandoverVerification{OpaqueRef: o.HandoverRef, EvidenceRef: "dispatch:" + o.HandoverRef}, nil
}
