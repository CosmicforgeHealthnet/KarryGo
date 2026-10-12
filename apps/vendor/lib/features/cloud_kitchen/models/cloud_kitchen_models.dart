// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCloudKitchenBulkPackagesRequest extends VendorPayload {
  PostCloudKitchenBulkPackagesRequest({String? id, String? branchId, String? name, String? description, String? status, int? minimumQuantity, int? maximumQuantity, String? capacityPoolId, String? productionClass, int? leadTimeMinutes, int? setupBufferMinutes, int? cleanupBufferMinutes, JsonObject? logistics, int? version}) : super({'id': ?id, 'branch_id': ?branchId, 'name': ?name, 'description': ?description, 'status': ?status, 'minimum_quantity': ?minimumQuantity, 'maximum_quantity': ?maximumQuantity, 'capacity_pool_id': ?capacityPoolId, 'production_class': ?productionClass, 'lead_time_minutes': ?leadTimeMinutes, 'setup_buffer_minutes': ?setupBufferMinutes, 'cleanup_buffer_minutes': ?cleanupBufferMinutes, 'logistics': ?logistics, 'version': ?version});
  PostCloudKitchenBulkPackagesRequest.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get status => value('status') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get maximumQuantity => value('maximum_quantity') as int?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionClass => value('production_class') as String?;
  int? get leadTimeMinutes => value('lead_time_minutes') as int?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  JsonObject? get logistics => value('logistics') == null ? null : jsonObject(value('logistics'));
  int? get version => value('version') as int?;
}

class PatchCloudKitchenBulkPackagesPackageIdRequest extends VendorPayload {
  PatchCloudKitchenBulkPackagesPackageIdRequest({String? id, String? branchId, String? name, String? description, String? status, int? minimumQuantity, int? maximumQuantity, String? capacityPoolId, String? productionClass, int? leadTimeMinutes, int? setupBufferMinutes, int? cleanupBufferMinutes, JsonObject? logistics, int? version}) : super({'id': ?id, 'branch_id': ?branchId, 'name': ?name, 'description': ?description, 'status': ?status, 'minimum_quantity': ?minimumQuantity, 'maximum_quantity': ?maximumQuantity, 'capacity_pool_id': ?capacityPoolId, 'production_class': ?productionClass, 'lead_time_minutes': ?leadTimeMinutes, 'setup_buffer_minutes': ?setupBufferMinutes, 'cleanup_buffer_minutes': ?cleanupBufferMinutes, 'logistics': ?logistics, 'version': ?version});
  PatchCloudKitchenBulkPackagesPackageIdRequest.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get status => value('status') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get maximumQuantity => value('maximum_quantity') as int?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionClass => value('production_class') as String?;
  int? get leadTimeMinutes => value('lead_time_minutes') as int?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  JsonObject? get logistics => value('logistics') == null ? null : jsonObject(value('logistics'));
  int? get version => value('version') as int?;
}

class PostCloudKitchenBulkPackagesPackageIdArchiveRequest extends VendorPayload {
  PostCloudKitchenBulkPackagesPackageIdArchiveRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PostCloudKitchenBulkPackagesPackageIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenBulkPackagesPackageIdBandsRequest extends VendorPayload {
  PostCloudKitchenBulkPackagesPackageIdBandsRequest({int? minQuantity, int? maxQuantity, int? unitPriceMinor, int? packagePriceMinor, int? leadTimeMinutes, int? capacityUnitsPerItem, int? expectedVersion}) : super({'min_quantity': ?minQuantity, 'max_quantity': ?maxQuantity, 'unit_price_minor': ?unitPriceMinor, 'package_price_minor': ?packagePriceMinor, 'lead_time_minutes': ?leadTimeMinutes, 'capacity_units_per_item': ?capacityUnitsPerItem, 'expected_version': ?expectedVersion});
  PostCloudKitchenBulkPackagesPackageIdBandsRequest.fromJson(super.json);
  int? get minQuantity => value('min_quantity') as int?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  int? get packagePriceMinor => value('package_price_minor') as int?;
  int? get leadTimeMinutes => value('lead_time_minutes') as int?;
  int? get capacityUnitsPerItem => value('capacity_units_per_item') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCloudKitchenBulkPackagesPackageIdBandsBandIdRequest extends VendorPayload {
  PatchCloudKitchenBulkPackagesPackageIdBandsBandIdRequest({int? minQuantity, int? maxQuantity, int? unitPriceMinor, int? packagePriceMinor, int? leadTimeMinutes, int? capacityUnitsPerItem, int? expectedPackageVersion, int? expectedBandVersion}) : super({'min_quantity': ?minQuantity, 'max_quantity': ?maxQuantity, 'unit_price_minor': ?unitPriceMinor, 'package_price_minor': ?packagePriceMinor, 'lead_time_minutes': ?leadTimeMinutes, 'capacity_units_per_item': ?capacityUnitsPerItem, 'expected_package_version': ?expectedPackageVersion, 'expected_band_version': ?expectedBandVersion});
  PatchCloudKitchenBulkPackagesPackageIdBandsBandIdRequest.fromJson(super.json);
  int? get minQuantity => value('min_quantity') as int?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  int? get packagePriceMinor => value('package_price_minor') as int?;
  int? get leadTimeMinutes => value('lead_time_minutes') as int?;
  int? get capacityUnitsPerItem => value('capacity_units_per_item') as int?;
  int? get expectedPackageVersion => value('expected_package_version') as int?;
  int? get expectedBandVersion => value('expected_band_version') as int?;
}

class PostCloudKitchenBulkPackagesPackageIdBandsBandIdArchiveRequest extends VendorPayload {
  PostCloudKitchenBulkPackagesPackageIdBandsBandIdArchiveRequest({int? expectedPackageVersion, int? expectedBandVersion}) : super({'expected_package_version': ?expectedPackageVersion, 'expected_band_version': ?expectedBandVersion});
  PostCloudKitchenBulkPackagesPackageIdBandsBandIdArchiveRequest.fromJson(super.json);
  int? get expectedPackageVersion => value('expected_package_version') as int?;
  int? get expectedBandVersion => value('expected_band_version') as int?;
}

class PostCloudKitchenBulkPackagesPackageIdComponentsRequest extends VendorPayload {
  PostCloudKitchenBulkPackagesPackageIdComponentsRequest({String? itemRef, int? defaultQuantity, String? variantRef, int? position, int? expectedVersion}) : super({'item_ref': ?itemRef, 'default_quantity': ?defaultQuantity, 'variant_ref': ?variantRef, 'position': ?position, 'expected_version': ?expectedVersion});
  PostCloudKitchenBulkPackagesPackageIdComponentsRequest.fromJson(super.json);
  String? get itemRef => value('item_ref') as String?;
  int? get defaultQuantity => value('default_quantity') as int?;
  String? get variantRef => value('variant_ref') as String?;
  int? get position => value('position') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class DeleteCloudKitchenBulkPackagesPackageIdComponentsComponentIdRequest extends VendorPayload {
  DeleteCloudKitchenBulkPackagesPackageIdComponentsComponentIdRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  DeleteCloudKitchenBulkPackagesPackageIdComponentsComponentIdRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansRequest({String? id, String? branchId, String? corporateProfileRef, String? name, String? packageId, String? recurrenceType, String? timezone, String? localTime, String? status, int? quantity, List<dynamic>? weekdays, String? effectiveFrom, String? effectiveUntil, int? productionMinutes, int? leadTimeMinutes, int? reservationHorizonDays, int? version}) : super({'id': ?id, 'branch_id': ?branchId, 'corporate_profile_ref': ?corporateProfileRef, 'name': ?name, 'package_id': ?packageId, 'recurrence_type': ?recurrenceType, 'timezone': ?timezone, 'local_time': ?localTime, 'status': ?status, 'quantity': ?quantity, 'weekdays': ?weekdays, 'effective_from': ?effectiveFrom, 'effective_until': ?effectiveUntil, 'production_minutes': ?productionMinutes, 'lead_time_minutes': ?leadTimeMinutes, 'reservation_horizon_days': ?reservationHorizonDays, 'version': ?version});
  PostCloudKitchenCorporatePlansRequest.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get corporateProfileRef => value('corporate_profile_ref') as String?;
  String? get name => value('name') as String?;
  String? get packageId => value('package_id') as String?;
  String? get recurrenceType => value('recurrence_type') as String?;
  String? get timezone => value('timezone') as String?;
  String? get localTime => value('local_time') as String?;
  String? get status => value('status') as String?;
  int? get quantity => value('quantity') as int?;
  List<dynamic>? get weekdays => value('weekdays') as List<dynamic>?;
  String? get effectiveFrom => value('effective_from') as String?;
  String? get effectiveUntil => value('effective_until') as String?;
  int? get productionMinutes => value('production_minutes') as int?;
  int? get leadTimeMinutes => value('lead_time_minutes') as int?;
  int? get reservationHorizonDays => value('reservation_horizon_days') as int?;
  int? get version => value('version') as int?;
}

class PatchCloudKitchenCorporatePlansPlanIdRequest extends VendorPayload {
  PatchCloudKitchenCorporatePlansPlanIdRequest({String? id, String? branchId, String? corporateProfileRef, String? name, String? packageId, String? recurrenceType, String? timezone, String? localTime, String? status, int? quantity, List<dynamic>? weekdays, String? effectiveFrom, String? effectiveUntil, int? productionMinutes, int? leadTimeMinutes, int? reservationHorizonDays, int? version}) : super({'id': ?id, 'branch_id': ?branchId, 'corporate_profile_ref': ?corporateProfileRef, 'name': ?name, 'package_id': ?packageId, 'recurrence_type': ?recurrenceType, 'timezone': ?timezone, 'local_time': ?localTime, 'status': ?status, 'quantity': ?quantity, 'weekdays': ?weekdays, 'effective_from': ?effectiveFrom, 'effective_until': ?effectiveUntil, 'production_minutes': ?productionMinutes, 'lead_time_minutes': ?leadTimeMinutes, 'reservation_horizon_days': ?reservationHorizonDays, 'version': ?version});
  PatchCloudKitchenCorporatePlansPlanIdRequest.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get corporateProfileRef => value('corporate_profile_ref') as String?;
  String? get name => value('name') as String?;
  String? get packageId => value('package_id') as String?;
  String? get recurrenceType => value('recurrence_type') as String?;
  String? get timezone => value('timezone') as String?;
  String? get localTime => value('local_time') as String?;
  String? get status => value('status') as String?;
  int? get quantity => value('quantity') as int?;
  List<dynamic>? get weekdays => value('weekdays') as List<dynamic>?;
  String? get effectiveFrom => value('effective_from') as String?;
  String? get effectiveUntil => value('effective_until') as String?;
  int? get productionMinutes => value('production_minutes') as int?;
  int? get leadTimeMinutes => value('lead_time_minutes') as int?;
  int? get reservationHorizonDays => value('reservation_horizon_days') as int?;
  int? get version => value('version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdActivateRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdActivateRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdActivateRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdArchiveRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdArchiveRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdExceptionsRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdExceptionsRequest({String? occurrenceDate, String? action, String? replacementDate, int? quantityOverride, String? packageOverride}) : super({'occurrence_date': ?occurrenceDate, 'action': ?action, 'replacement_date': ?replacementDate, 'quantity_override': ?quantityOverride, 'package_override': ?packageOverride});
  PostCloudKitchenCorporatePlansPlanIdExceptionsRequest.fromJson(super.json);
  String? get occurrenceDate => value('occurrence_date') as String?;
  String? get action => value('action') as String?;
  String? get replacementDate => value('replacement_date') as String?;
  int? get quantityOverride => value('quantity_override') as int?;
  String? get packageOverride => value('package_override') as String?;
}

class PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdCancelRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdCancelRequest({String? reason, int? expectedVersion}) : super({'reason': ?reason, 'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdCancelRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdReplayRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdReplayRequest({String? reason, int? expectedVersion}) : super({'reason': ?reason, 'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdReplayRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdRescheduleRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdRescheduleRequest({String? startsAt, String? endsAt, int? expectedVersion}) : super({'starts_at': ?startsAt, 'ends_at': ?endsAt, 'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdRescheduleRequest.fromJson(super.json);
  String? get startsAt => value('starts_at') as String?;
  String? get endsAt => value('ends_at') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdRetryRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdRetryRequest({String? reason, int? expectedVersion}) : super({'reason': ?reason, 'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdOccurrencesOccurrenceIdRetryRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdResumeRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdResumeRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdResumeRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdSuspendRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdSuspendRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdSuspendRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCloudKitchenCorporatePlansPlanIdStatusRequest extends VendorPayload {
  PostCloudKitchenCorporatePlansPlanIdStatusRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PostCloudKitchenCorporatePlansPlanIdStatusRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutCloudKitchenSettingsBranchIdRequest extends VendorPayload {
  PutCloudKitchenSettingsBranchIdRequest({String? branchId, int? bulkOrderThreshold, int? minimumLeadTimeMinutes, String? defaultCapacityPoolRef, int? reservationHoldDurationMinutes, int? corporateReservationHorizonDays, int? version}) : super({'branch_id': ?branchId, 'bulk_order_threshold': ?bulkOrderThreshold, 'minimum_lead_time_minutes': ?minimumLeadTimeMinutes, 'default_capacity_pool_ref': ?defaultCapacityPoolRef, 'reservation_hold_duration_minutes': ?reservationHoldDurationMinutes, 'corporate_reservation_horizon_days': ?corporateReservationHorizonDays, 'version': ?version});
  PutCloudKitchenSettingsBranchIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  int? get bulkOrderThreshold => value('bulk_order_threshold') as int?;
  int? get minimumLeadTimeMinutes => value('minimum_lead_time_minutes') as int?;
  String? get defaultCapacityPoolRef => value('default_capacity_pool_ref') as String?;
  int? get reservationHoldDurationMinutes => value('reservation_hold_duration_minutes') as int?;
  int? get corporateReservationHorizonDays => value('corporate_reservation_horizon_days') as int?;
  int? get version => value('version') as int?;
}

class CloudKitchenSections1011ProductionOccurrenceRetryCommand extends VendorPayload {
  CloudKitchenSections1011ProductionOccurrenceRetryCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get reason => value('reason') as String?;
}

class CloudKitchenSections1011ProductionOccurrenceRescheduleCommand extends VendorPayload {
  CloudKitchenSections1011ProductionOccurrenceRescheduleCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get startsAt => value('starts_at') as String?;
  String? get endsAt => value('ends_at') as String?;
}

class CloudKitchenSections1011ProductionPastryQuoteCreateCommand extends VendorPayload {
  CloudKitchenSections1011ProductionPastryQuoteCreateCommand.fromJson(super.json);
  String? get specificationVersionId => value('specification_version_id') as String?;
  int? get amountMinor => value('amount_minor') as int?;
  String? get expiresAt => value('expires_at') as String?;
  bool? get approvalRequired => value('approval_required') as bool?;
  int? get expectedRequestVersion => value('expected_request_version') as int?;
  String? get productionStart => value('production_start') as String?;
  String? get productionEnd => value('production_end') as String?;
  int? get holdForMinutes => value('hold_for_minutes') as int?;
  String? get vendorTerms => value('vendor_terms') as String?;
}
