// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PutDeliveryBranchesBranchIdPolicyRequest extends VendorPayload {
  PutDeliveryBranchesBranchIdPolicyRequest({bool? enabled, String? deliveryMode, String? feeRule, String? currency, int? baseFeeMinor, int? perKmFeeMinor, int? minimumOrderMinor, int? freeDeliveryThresholdMinor, int? maxDeliveryRadiusMeters, int? maxPackageWeightGrams, int? maxPackageCount, int? maxVolumeMl, String? operatingTimezone, List<dynamic>? serviceZones, List<dynamic>? distanceTiers, List<dynamic>? operatingPeriods, List<dynamic>? supportedVehicleClasses, List<dynamic>? fallbackTriggers, String? hybridFallbackFundingSponsor, int? hybridVendorShareBps, String? hybridPlatformApprovalRef, String? riderAvailability, int? estimatedMinutesBase, int? estimatedMinutesPerKm, String? reason}) : super({'enabled': ?enabled, 'delivery_mode': ?deliveryMode, 'fee_rule': ?feeRule, 'currency': ?currency, 'base_fee_minor': ?baseFeeMinor, 'per_km_fee_minor': ?perKmFeeMinor, 'minimum_order_minor': ?minimumOrderMinor, 'free_delivery_threshold_minor': ?freeDeliveryThresholdMinor, 'max_delivery_radius_meters': ?maxDeliveryRadiusMeters, 'max_package_weight_grams': ?maxPackageWeightGrams, 'max_package_count': ?maxPackageCount, 'max_volume_ml': ?maxVolumeMl, 'operating_timezone': ?operatingTimezone, 'service_zones': ?serviceZones, 'distance_tiers': ?distanceTiers, 'operating_periods': ?operatingPeriods, 'supported_vehicle_classes': ?supportedVehicleClasses, 'fallback_triggers': ?fallbackTriggers, 'hybrid_fallback_funding_sponsor': ?hybridFallbackFundingSponsor, 'hybrid_vendor_share_bps': ?hybridVendorShareBps, 'hybrid_platform_approval_ref': ?hybridPlatformApprovalRef, 'rider_availability': ?riderAvailability, 'estimated_minutes_base': ?estimatedMinutesBase, 'estimated_minutes_per_km': ?estimatedMinutesPerKm, 'reason': ?reason});
  PutDeliveryBranchesBranchIdPolicyRequest.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  String? get deliveryMode => value('delivery_mode') as String?;
  String? get feeRule => value('fee_rule') as String?;
  String? get currency => value('currency') as String?;
  int? get baseFeeMinor => value('base_fee_minor') as int?;
  int? get perKmFeeMinor => value('per_km_fee_minor') as int?;
  int? get minimumOrderMinor => value('minimum_order_minor') as int?;
  int? get freeDeliveryThresholdMinor => value('free_delivery_threshold_minor') as int?;
  int? get maxDeliveryRadiusMeters => value('max_delivery_radius_meters') as int?;
  int? get maxPackageWeightGrams => value('max_package_weight_grams') as int?;
  int? get maxPackageCount => value('max_package_count') as int?;
  int? get maxVolumeMl => value('max_volume_ml') as int?;
  String? get operatingTimezone => value('operating_timezone') as String?;
  List<dynamic>? get serviceZones => value('service_zones') as List<dynamic>?;
  List<dynamic>? get distanceTiers => value('distance_tiers') as List<dynamic>?;
  List<dynamic>? get operatingPeriods => value('operating_periods') as List<dynamic>?;
  List<dynamic>? get supportedVehicleClasses => value('supported_vehicle_classes') as List<dynamic>?;
  List<dynamic>? get fallbackTriggers => value('fallback_triggers') as List<dynamic>?;
  String? get hybridFallbackFundingSponsor => value('hybrid_fallback_funding_sponsor') as String?;
  int? get hybridVendorShareBps => value('hybrid_vendor_share_bps') as int?;
  String? get hybridPlatformApprovalRef => value('hybrid_platform_approval_ref') as String?;
  String? get riderAvailability => value('rider_availability') as String?;
  int? get estimatedMinutesBase => value('estimated_minutes_base') as int?;
  int? get estimatedMinutesPerKm => value('estimated_minutes_per_km') as int?;
  String? get reason => value('reason') as String?;
}

class PutDeliveryRiderAvailabilityRequest extends VendorPayload {
  PutDeliveryRiderAvailabilityRequest({String? branchId, String? state}) : super({'branch_id': ?branchId, 'state': ?state});
  PutDeliveryRiderAvailabilityRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get state => value('state') as String?;
}

class PostDeliveryRiderMissionsMissionIdActionsActionRequest extends VendorPayload {
  PostDeliveryRiderMissionsMissionIdActionsActionRequest({String? evidenceRef, String? method, String? credential, String? reasonCode, List<dynamic>? parcelRefs}) : super({'evidence_ref': ?evidenceRef, 'method': ?method, 'credential': ?credential, 'reason_code': ?reasonCode, 'parcel_refs': ?parcelRefs});
  PostDeliveryRiderMissionsMissionIdActionsActionRequest.fromJson(super.json);
  String? get evidenceRef => value('evidence_ref') as String?;
  String? get method => value('method') as String?;
  String? get credential => value('credential') as String?;
  String? get reasonCode => value('reason_code') as String?;
  List<dynamic>? get parcelRefs => value('parcel_refs') as List<dynamic>?;
}

class PutFulfilmentParcelsParcelIdManifestRequest extends VendorPayload {
  PutFulfilmentParcelsParcelIdManifestRequest({List<dynamic>? lines, int? packageCount, String? notes, List<dynamic>? mediaRefs, List<dynamic>? specialHandlingRefs, String? sealedAt}) : super({'lines': ?lines, 'package_count': ?packageCount, 'notes': ?notes, 'media_refs': ?mediaRefs, 'special_handling_refs': ?specialHandlingRefs, 'sealed_at': ?sealedAt});
  PutFulfilmentParcelsParcelIdManifestRequest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  int? get packageCount => value('package_count') as int?;
  String? get notes => value('notes') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  List<dynamic>? get specialHandlingRefs => value('special_handling_refs') as List<dynamic>?;
  String? get sealedAt => value('sealed_at') as String?;
}

class PostFulfilmentParcelsParcelIdReturnsRequest extends VendorPayload {
  PostFulfilmentParcelsParcelIdReturnsRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostFulfilmentParcelsParcelIdReturnsRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostFulfilmentUnitsUnitIdParcelsRequest extends VendorPayload {
  PostFulfilmentUnitsUnitIdParcelsRequest({JsonObject? manifest, int? weightGrams, int? lengthMm, int? widthMm, int? heightMm, bool? fragile, String? temperatureControl}) : super({'manifest': ?manifest, 'weight_grams': ?weightGrams, 'length_mm': ?lengthMm, 'width_mm': ?widthMm, 'height_mm': ?heightMm, 'fragile': ?fragile, 'temperature_control': ?temperatureControl});
  PostFulfilmentUnitsUnitIdParcelsRequest.fromJson(super.json);
  JsonObject? get manifest => value('manifest') == null ? null : jsonObject(value('manifest'));
  int? get weightGrams => value('weight_grams') as int?;
  int? get lengthMm => value('length_mm') as int?;
  int? get widthMm => value('width_mm') as int?;
  int? get heightMm => value('height_mm') as int?;
  bool? get fragile => value('fragile') as bool?;
  String? get temperatureControl => value('temperature_control') as String?;
}

class PutFulfilmentUnitsUnitIdReadinessRequest extends VendorPayload {
  PutFulfilmentUnitsUnitIdReadinessRequest({String? readiness}) : super({'readiness': ?readiness});
  PutFulfilmentUnitsUnitIdReadinessRequest.fromJson(super.json);
  String? get readiness => value('readiness') as String?;
}

class PostFulfilmentVendorMissionsMissionIdFallbacksRequest extends VendorPayload {
  PostFulfilmentVendorMissionsMissionIdFallbacksRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostFulfilmentVendorMissionsMissionIdFallbacksRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PutFulfilmentVendorMissionsMissionIdRiderRequest extends VendorPayload {
  PutFulfilmentVendorMissionsMissionIdRiderRequest({String? staffId}) : super({'staff_id': ?staffId});
  PutFulfilmentVendorMissionsMissionIdRiderRequest.fromJson(super.json);
  String? get staffId => value('staff_id') as String?;
}

class PutFulfilmentVendorMissionsMissionIdStatusRequest extends VendorPayload {
  PutFulfilmentVendorMissionsMissionIdStatusRequest({String? status, String? handoverRef, String? proofRef, String? failureReason}) : super({'status': ?status, 'handover_ref': ?handoverRef, 'proof_ref': ?proofRef, 'failure_reason': ?failureReason});
  PutFulfilmentVendorMissionsMissionIdStatusRequest.fromJson(super.json);
  String? get status => value('status') as String?;
  String? get handoverRef => value('handover_ref') as String?;
  String? get proofRef => value('proof_ref') as String?;
  String? get failureReason => value('failure_reason') as String?;
}

class DeliveryFulfilmentGroupParcelMissionCustodyEvent extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionCustodyEvent.fromJson(super.json);
  String? get createdAt => value('created_at') as String?;
  String? get custodyEventId => value('custody_event_id') as String?;
  String? get eventType => value('event_type') as String?;
  String? get fromCustodianRef => value('from_custodian_ref') as String?;
  String? get fromCustodianType => value('from_custodian_type') as String?;
  String? get handoverRef => value('handover_ref') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get parcelId => value('parcel_id') as String?;
  String? get platformMissionRefId => value('platform_mission_ref_id') as String?;
  String? get proofRef => value('proof_ref') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get source => value('source') as String?;
  String? get sourceEventId => value('source_event_id') as String?;
  int? get sourceVersion => value('source_version') as int?;
  String? get toCustodianRef => value('to_custodian_ref') as String?;
  String? get toCustodianType => value('to_custodian_type') as String?;
  String? get vendorMissionId => value('vendor_mission_id') as String?;
}

class DeliveryFulfilmentGroupParcelMissionGroup extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionGroup.fromJson(super.json);
  String? get createdAt => value('created_at') as String?;
  String? get currentPlanId => value('current_plan_id') as String?;
  int? get currentPlanVersion => value('current_plan_version') as int?;
  String? get fulfilmentGroupId => value('fulfilment_group_id') as String?;
  List<dynamic>? get parcels => value('parcels') as List<dynamic>?;
  String? get parentOrderId => value('parent_order_id') as String?;
  List<dynamic>? get units => value('units') as List<dynamic>?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class DeliveryFulfilmentGroupParcelMissionGroupView extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionGroupView.fromJson(super.json);
  JsonObject? get currentPlan => value('current_plan') == null ? null : jsonObject(value('current_plan'));
  JsonObject? get group => value('group') == null ? null : jsonObject(value('group'));
  List<dynamic>? get platformMissions => value('platform_missions') as List<dynamic>?;
  List<dynamic>? get returns => value('returns') as List<dynamic>?;
  List<dynamic>? get vendorMissions => value('vendor_missions') as List<dynamic>?;
}

class DeliveryFulfilmentGroupParcelMissionManifest extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionManifest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  String? get notes => value('notes') as String?;
  int? get packageCount => value('package_count') as int?;
  String? get sealedAt => value('sealed_at') as String?;
  List<dynamic>? get specialHandlingRefs => value('special_handling_refs') as List<dynamic>?;
}

class DeliveryFulfilmentGroupParcelMissionParcel extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionParcel.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get businessId => value('business_id') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get custodianRef => value('custodian_ref') as String?;
  String? get custodianType => value('custodian_type') as String?;
  bool? get fragile => value('fragile') as bool?;
  String? get fulfilmentGroupId => value('fulfilment_group_id') as String?;
  String? get fulfilmentUnitId => value('fulfilment_unit_id') as String?;
  int? get heightMm => value('height_mm') as int?;
  int? get lengthMm => value('length_mm') as int?;
  JsonObject? get manifest => value('manifest') == null ? null : jsonObject(value('manifest'));
  int? get manifestVersion => value('manifest_version') as int?;
  String? get parcelId => value('parcel_id') as String?;
  String? get parcelRef => value('parcel_ref') as String?;
  String? get parentOrderId => value('parent_order_id') as String?;
  int? get sequence => value('sequence') as int?;
  String? get status => value('status') as String?;
  String? get temperatureControl => value('temperature_control') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get vendorOrderId => value('vendor_order_id') as String?;
  int? get version => value('version') as int?;
  int? get weightGrams => value('weight_grams') as int?;
  int? get widthMm => value('width_mm') as int?;
}

class DeliveryFulfilmentGroupParcelMissionParcelReturn extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionParcelReturn.fromJson(super.json);
  String? get completedAt => value('completed_at') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get originPlatformMissionRefId => value('origin_platform_mission_ref_id') as String?;
  String? get originVendorMissionId => value('origin_vendor_mission_id') as String?;
  String? get parcelId => value('parcel_id') as String?;
  String? get parcelReturnId => value('parcel_return_id') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get returnPlatformMissionRefId => value('return_platform_mission_ref_id') as String?;
  String? get returnVendorMissionId => value('return_vendor_mission_id') as String?;
  String? get state => value('state') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class DeliveryFulfilmentGroupParcelMissionPlan extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionPlan.fromJson(super.json);
  String? get activatedAt => value('activated_at') as String?;
  JsonObject? get costSnapshot => value('cost_snapshot') == null ? null : jsonObject(value('cost_snapshot'));
  String? get createdAt => value('created_at') as String?;
  String? get currency => value('currency') as String?;
  String? get deliveryMode => value('delivery_mode') as String?;
  String? get fingerprint => value('fingerprint') as String?;
  String? get fulfilmentGroupId => value('fulfilment_group_id') as String?;
  List<dynamic>? get missions => value('missions') as List<dynamic>?;
  String? get planId => value('plan_id') as String?;
  int? get plannedCostMinor => value('planned_cost_minor') as int?;
  String? get plannerVersion => value('planner_version') as String?;
  int? get policyVersion => value('policy_version') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get reasonDetail => value('reason_detail') as String?;
  JsonObject? get routingEvidence => value('routing_evidence') == null ? null : jsonObject(value('routing_evidence'));
  String? get status => value('status') as String?;
  String? get supersededAt => value('superseded_at') as String?;
  int? get supersedesVersion => value('supersedes_version') as int?;
  int? get version => value('version') as int?;
}

class DeliveryFulfilmentGroupParcelMissionPlanMission extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionPlanMission.fromJson(super.json);
  String? get authority => value('authority') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get currency => value('currency') as String?;
  String? get owner => value('owner') as String?;
  List<dynamic>? get parcelIds => value('parcel_ids') as List<dynamic>?;
  String? get planId => value('plan_id') as String?;
  String? get planMissionId => value('plan_mission_id') as String?;
  int? get plannedCostMinor => value('planned_cost_minor') as int?;
  int? get sequence => value('sequence') as int?;
  String? get serviceClass => value('service_class') as String?;
  String? get slotRef => value('slot_ref') as String?;
  List<dynamic>? get stops => value('stops') as List<dynamic>?;
  String? get vehicleClass => value('vehicle_class') as String?;
}

class DeliveryFulfilmentGroupParcelMissionPlatformMissionRef extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionPlatformMissionRef.fromJson(super.json);
  String? get authority => value('authority') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get externalRef => value('external_ref') as String?;
  String? get failureReason => value('failure_reason') as String?;
  String? get fulfilmentGroupId => value('fulfilment_group_id') as String?;
  String? get linkState => value('link_state') as String?;
  String? get linkedAt => value('linked_at') as String?;
  String? get missionRef => value('mission_ref') as String?;
  String? get planId => value('plan_id') as String?;
  String? get planMissionId => value('plan_mission_id') as String?;
  String? get platformMissionRefId => value('platform_mission_ref_id') as String?;
  String? get refusalCode => value('refusal_code') as String?;
  String? get replacementReason => value('replacement_reason') as String?;
  String? get replacesId => value('replaces_id') as String?;
  String? get statusProjection => value('status_projection') as String?;
  String? get statusSourceRef => value('status_source_ref') as String?;
  int? get statusSourceVersion => value('status_source_version') as int?;
  String? get statusUpdatedAt => value('status_updated_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class DeliveryFulfilmentGroupParcelMissionStop extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionStop.fromJson(super.json);
  String? get address => value('address') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get businessId => value('business_id') as String?;
  String? get fulfilmentUnitId => value('fulfilment_unit_id') as String?;
  int? get sequence => value('sequence') as int?;
  String? get stopId => value('stop_id') as String?;
  String? get stopType => value('stop_type') as String?;
}

class DeliveryFulfilmentGroupParcelMissionUnit extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionUnit.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get businessId => value('business_id') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get fulfilmentGroupId => value('fulfilment_group_id') as String?;
  String? get fulfilmentUnitId => value('fulfilment_unit_id') as String?;
  List<dynamic>? get handlingRequirements => value('handling_requirements') as List<dynamic>?;
  String? get parentOrderId => value('parent_order_id') as String?;
  String? get pickupAddress => value('pickup_address') as String?;
  String? get readiness => value('readiness') as String?;
  int? get requiredPackageCount => value('required_package_count') as int?;
  int? get requiredWeightGrams => value('required_weight_grams') as int?;
  String? get serviceClass => value('service_class') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get vehicleClass => value('vehicle_class') as String?;
  String? get vendorOrderId => value('vendor_order_id') as String?;
  int? get version => value('version') as int?;
}

class DeliveryFulfilmentGroupParcelMissionVendorMission extends VendorPayload {
  DeliveryFulfilmentGroupParcelMissionVendorMission.fromJson(super.json);
  String? get assignedAt => value('assigned_at') as String?;
  String? get assignedStaffId => value('assigned_staff_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get businessId => value('business_id') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get failureReason => value('failure_reason') as String?;
  String? get fulfilmentGroupId => value('fulfilment_group_id') as String?;
  String? get handoverRef => value('handover_ref') as String?;
  List<dynamic>? get parcelIds => value('parcel_ids') as List<dynamic>?;
  String? get planId => value('plan_id') as String?;
  String? get planMissionId => value('plan_mission_id') as String?;
  String? get proofRef => value('proof_ref') as String?;
  String? get status => value('status') as String?;
  List<dynamic>? get stops => value('stops') as List<dynamic>?;
  String? get updatedAt => value('updated_at') as String?;
  String? get vendorMissionId => value('vendor_mission_id') as String?;
  int? get version => value('version') as int?;
}

class DeliverySection17cVendorDeliveryPolicyPolicyInput extends VendorPayload {
  DeliverySection17cVendorDeliveryPolicyPolicyInput.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  String? get deliveryMode => value('delivery_mode') as String?;
  String? get feeRule => value('fee_rule') as String?;
  String? get currency => value('currency') as String?;
  int? get baseFeeMinor => value('base_fee_minor') as int?;
  int? get perKmFeeMinor => value('per_km_fee_minor') as int?;
  int? get minimumOrderMinor => value('minimum_order_minor') as int?;
  int? get freeDeliveryThresholdMinor => value('free_delivery_threshold_minor') as int?;
  int? get maxDeliveryRadiusMeters => value('max_delivery_radius_meters') as int?;
  int? get maxPackageWeightGrams => value('max_package_weight_grams') as int?;
  int? get maxPackageCount => value('max_package_count') as int?;
  int? get maxVolumeMl => value('max_volume_ml') as int?;
  String? get operatingTimezone => value('operating_timezone') as String?;
  String? get riderAvailability => value('rider_availability') as String?;
  List<dynamic>? get serviceZones => value('service_zones') as List<dynamic>?;
  List<dynamic>? get distanceTiers => value('distance_tiers') as List<dynamic>?;
  List<dynamic>? get operatingPeriods => value('operating_periods') as List<dynamic>?;
  List<dynamic>? get supportedVehicleClasses => value('supported_vehicle_classes') as List<dynamic>?;
  List<dynamic>? get fallbackTriggers => value('fallback_triggers') as List<dynamic>?;
  String? get hybridFallbackFundingSponsor => value('hybrid_fallback_funding_sponsor') as String?;
  int? get hybridVendorShareBps => value('hybrid_vendor_share_bps') as int?;
  String? get hybridPlatformApprovalRef => value('hybrid_platform_approval_ref') as String?;
  int? get estimatedMinutesBase => value('estimated_minutes_base') as int?;
  int? get estimatedMinutesPerKm => value('estimated_minutes_per_km') as int?;
  String? get reason => value('reason') as String?;
}

class DeliverySection17cVendorDeliveryPolicyPolicy extends VendorPayload {
  DeliverySection17cVendorDeliveryPolicyPolicy.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  String? get deliveryMode => value('delivery_mode') as String?;
  String? get feeRule => value('fee_rule') as String?;
  String? get currency => value('currency') as String?;
  int? get baseFeeMinor => value('base_fee_minor') as int?;
  int? get perKmFeeMinor => value('per_km_fee_minor') as int?;
  int? get minimumOrderMinor => value('minimum_order_minor') as int?;
  int? get freeDeliveryThresholdMinor => value('free_delivery_threshold_minor') as int?;
  int? get maxDeliveryRadiusMeters => value('max_delivery_radius_meters') as int?;
  int? get maxPackageWeightGrams => value('max_package_weight_grams') as int?;
  int? get maxPackageCount => value('max_package_count') as int?;
  int? get maxVolumeMl => value('max_volume_ml') as int?;
  String? get operatingTimezone => value('operating_timezone') as String?;
  String? get riderAvailability => value('rider_availability') as String?;
  List<dynamic>? get serviceZones => value('service_zones') as List<dynamic>?;
  List<dynamic>? get distanceTiers => value('distance_tiers') as List<dynamic>?;
  List<dynamic>? get operatingPeriods => value('operating_periods') as List<dynamic>?;
  List<dynamic>? get supportedVehicleClasses => value('supported_vehicle_classes') as List<dynamic>?;
  List<dynamic>? get fallbackTriggers => value('fallback_triggers') as List<dynamic>?;
  String? get hybridFallbackFundingSponsor => value('hybrid_fallback_funding_sponsor') as String?;
  int? get hybridVendorShareBps => value('hybrid_vendor_share_bps') as int?;
  String? get hybridPlatformApprovalRef => value('hybrid_platform_approval_ref') as String?;
  int? get estimatedMinutesBase => value('estimated_minutes_base') as int?;
  int? get estimatedMinutesPerKm => value('estimated_minutes_per_km') as int?;
  String? get reason => value('reason') as String?;
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class DeliverySection17cVendorDeliveryPolicyServiceZone extends VendorPayload {
  DeliverySection17cVendorDeliveryPolicyServiceZone.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get name => value('name') as String?;
  bool? get enabled => value('enabled') as bool?;
  int? get minDistanceMeters => value('min_distance_meters') as int?;
  int? get maxDistanceMeters => value('max_distance_meters') as int?;
  int? get feeMinor => value('fee_minor') as int?;
  int? get priority => value('priority') as int?;
}

class DeliverySection17cVendorDeliveryPolicyDistanceTier extends VendorPayload {
  DeliverySection17cVendorDeliveryPolicyDistanceTier.fromJson(super.json);
  int? get minDistanceMeters => value('min_distance_meters') as int?;
  int? get maxDistanceMeters => value('max_distance_meters') as int?;
  int? get feeMinor => value('fee_minor') as int?;
}

class DeliverySection17cVendorDeliveryPolicyOperatingPeriod extends VendorPayload {
  DeliverySection17cVendorDeliveryPolicyOperatingPeriod.fromJson(super.json);
  int? get weekday => value('weekday') as int?;
  int? get startMinute => value('start_minute') as int?;
  int? get endMinute => value('end_minute') as int?;
}

class DeliverySection17cVendorDeliveryPolicyPolicyEnvelope extends VendorPayload {
  DeliverySection17cVendorDeliveryPolicyPolicyEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data => value('data') == null ? null : jsonObject(value('data'));
}
