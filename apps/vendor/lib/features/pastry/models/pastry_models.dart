// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostPastryChangesChangeIdDecisionRequest extends VendorPayload {
  PostPastryChangesChangeIdDecisionRequest({
    String? decision,
    String? note,
    int? expectedVersion,
  }) : super({
         'decision': ?decision,
         'note': ?note,
         'expected_version': ?expectedVersion,
       });
  PostPastryChangesChangeIdDecisionRequest.fromJson(super.json);
  String? get decision => value('decision') as String?;
  String? get note => value('note') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryCustomRequestsRequest extends VendorPayload {
  PostPastryCustomRequestsRequest({
    String? branchId,
    String? title,
    String? notes,
    String? customerRef,
    String? eventAt,
    JsonObject? logistics,
    List<dynamic>? riskFlags,
    String? corporateProfileRef,
    String? changeCutoffAt,
    JsonObject? changePolicy,
    JsonObject? cancellationPolicy,
  }) : super({
         'branch_id': ?branchId,
         'title': ?title,
         'notes': ?notes,
         'customer_ref': ?customerRef,
         'event_at': ?eventAt,
         'logistics': ?logistics,
         'risk_flags': ?riskFlags,
         'corporate_profile_ref': ?corporateProfileRef,
         'change_cutoff_at': ?changeCutoffAt,
         'change_policy': ?changePolicy,
         'cancellation_policy': ?cancellationPolicy,
       });
  PostPastryCustomRequestsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get title => value('title') as String?;
  String? get notes => value('notes') as String?;
  String? get customerRef => value('customer_ref') as String?;
  String? get eventAt => value('event_at') as String?;
  JsonObject? get logistics =>
      value('logistics') == null ? null : jsonObject(value('logistics'));
  List<dynamic>? get riskFlags => value('risk_flags') as List<dynamic>?;
  String? get corporateProfileRef => value('corporate_profile_ref') as String?;
  String? get changeCutoffAt => value('change_cutoff_at') as String?;
  JsonObject? get changePolicy => value('change_policy') == null
      ? null
      : jsonObject(value('change_policy'));
  JsonObject? get cancellationPolicy => value('cancellation_policy') == null
      ? null
      : jsonObject(value('cancellation_policy'));
}

class PatchPastryCustomRequestsRequestIdRequest extends VendorPayload {
  PatchPastryCustomRequestsRequestIdRequest({
    String? branchId,
    String? title,
    String? notes,
    String? eventAt,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'title': ?title,
         'notes': ?notes,
         'event_at': ?eventAt,
         'expected_version': ?expectedVersion,
       });
  PatchPastryCustomRequestsRequestIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get title => value('title') as String?;
  String? get notes => value('notes') as String?;
  String? get eventAt => value('event_at') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryCustomRequestsRequestIdCancelRequest extends VendorPayload {
  PostPastryCustomRequestsRequestIdCancelRequest({
    String? reason,
    bool? reviewApproved,
    int? expectedVersion,
  }) : super({
         'reason': ?reason,
         'review_approved': ?reviewApproved,
         'expected_version': ?expectedVersion,
       });
  PostPastryCustomRequestsRequestIdCancelRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
  bool? get reviewApproved => value('review_approved') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryCustomRequestsRequestIdChangesRequest extends VendorPayload {
  PostPastryCustomRequestsRequestIdChangesRequest({
    String? reason,
    JsonObject? specification,
    int? expectedVersion,
  }) : super({
         'reason': ?reason,
         'specification': ?specification,
         'expected_version': ?expectedVersion,
       });
  PostPastryCustomRequestsRequestIdChangesRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
  JsonObject? get specification => value('specification') == null
      ? null
      : jsonObject(value('specification'));
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryCustomRequestsRequestIdMilestonesMilestoneTypeStateRequest
    extends VendorPayload {
  PostPastryCustomRequestsRequestIdMilestonesMilestoneTypeStateRequest({
    String? note,
    int? expectedVersion,
  }) : super({'note': ?note, 'expected_version': ?expectedVersion});
  PostPastryCustomRequestsRequestIdMilestonesMilestoneTypeStateRequest.fromJson(
    super.json,
  );
  String? get note => value('note') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryCustomRequestsRequestIdQuotesRequest extends VendorPayload {
  PostPastryCustomRequestsRequestIdQuotesRequest({
    String? specificationVersionId,
    int? amountMinor,
    String? expiresAt,
    String? capacityPoolId,
    String? productionStart,
    String? productionEnd,
    List<dynamic>? capacityAllocations,
    int? holdForMinutes,
    bool? approvalRequired,
    String? vendorTerms,
    String? negotiationReason,
    String? negotiationNote,
    int? expectedRequestVersion,
  }) : super({
         'specification_version_id': ?specificationVersionId,
         'amount_minor': ?amountMinor,
         'expires_at': ?expiresAt,
         'capacity_pool_id': ?capacityPoolId,
         'production_start': ?productionStart,
         'production_end': ?productionEnd,
         'capacity_allocations': ?capacityAllocations,
         'hold_for_minutes': ?holdForMinutes,
         'approval_required': ?approvalRequired,
         'vendor_terms': ?vendorTerms,
         'negotiation_reason': ?negotiationReason,
         'negotiation_note': ?negotiationNote,
         'expected_request_version': ?expectedRequestVersion,
       });
  PostPastryCustomRequestsRequestIdQuotesRequest.fromJson(super.json);
  String? get specificationVersionId =>
      value('specification_version_id') as String?;
  int? get amountMinor => value('amount_minor') as int?;
  String? get expiresAt => value('expires_at') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionStart => value('production_start') as String?;
  String? get productionEnd => value('production_end') as String?;
  List<dynamic>? get capacityAllocations =>
      value('capacity_allocations') as List<dynamic>?;
  int? get holdForMinutes => value('hold_for_minutes') as int?;
  bool? get approvalRequired => value('approval_required') as bool?;
  String? get vendorTerms => value('vendor_terms') as String?;
  String? get negotiationReason => value('negotiation_reason') as String?;
  String? get negotiationNote => value('negotiation_note') as String?;
  int? get expectedRequestVersion => value('expected_request_version') as int?;
}

class PostPastryCustomRequestsRequestIdSpecificationsRequest
    extends VendorPayload {
  PostPastryCustomRequestsRequestIdSpecificationsRequest({
    int? expectedVersion,
    JsonObject? specification,
  }) : super({
         'expected_version': ?expectedVersion,
         'specification': ?specification,
       });
  PostPastryCustomRequestsRequestIdSpecificationsRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  JsonObject? get specification => value('specification') == null
      ? null
      : jsonObject(value('specification'));
}

class PostPastryCustomRequestsRequestIdSubmitRequest extends VendorPayload {
  PostPastryCustomRequestsRequestIdSubmitRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryCustomRequestsRequestIdSubmitRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryOptionGroupsRequest extends VendorPayload {
  PostPastryOptionGroupsRequest({
    String? name,
    String? groupType,
    int? minSelect,
    int? maxSelect,
  }) : super({
         'name': ?name,
         'group_type': ?groupType,
         'min_select': ?minSelect,
         'max_select': ?maxSelect,
       });
  PostPastryOptionGroupsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get groupType => value('group_type') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
}

class PatchPastryOptionGroupsGroupIdRequest extends VendorPayload {
  PatchPastryOptionGroupsGroupIdRequest({
    String? name,
    int? minSelect,
    int? maxSelect,
    int? version,
  }) : super({
         'name': ?name,
         'min_select': ?minSelect,
         'max_select': ?maxSelect,
         'version': ?version,
       });
  PatchPastryOptionGroupsGroupIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
  int? get version => value('version') as int?;
}

class PostPastryOptionGroupsGroupIdArchiveRequest extends VendorPayload {
  PostPastryOptionGroupsGroupIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryOptionGroupsGroupIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryOptionGroupsGroupIdOptionsRequest extends VendorPayload {
  PostPastryOptionGroupsGroupIdOptionsRequest({
    String? name,
    String? code,
    int? priceDeltaMinor,
    int? workloadWeight,
    int? position,
    String? capacityClass,
  }) : super({
         'name': ?name,
         'code': ?code,
         'price_delta_minor': ?priceDeltaMinor,
         'workload_weight': ?workloadWeight,
         'position': ?position,
         'capacity_class': ?capacityClass,
       });
  PostPastryOptionGroupsGroupIdOptionsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get code => value('code') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
  int? get workloadWeight => value('workload_weight') as int?;
  int? get position => value('position') as int?;
  String? get capacityClass => value('capacity_class') as String?;
}

class PatchPastryOptionsOptionIdRequest extends VendorPayload {
  PatchPastryOptionsOptionIdRequest({
    String? name,
    String? code,
    int? priceDeltaMinor,
    int? workloadWeight,
    int? position,
    String? capacityClass,
    int? version,
  }) : super({
         'name': ?name,
         'code': ?code,
         'price_delta_minor': ?priceDeltaMinor,
         'workload_weight': ?workloadWeight,
         'position': ?position,
         'capacity_class': ?capacityClass,
         'version': ?version,
       });
  PatchPastryOptionsOptionIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get code => value('code') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
  int? get workloadWeight => value('workload_weight') as int?;
  int? get position => value('position') as int?;
  String? get capacityClass => value('capacity_class') as String?;
  int? get version => value('version') as int?;
}

class PostPastryOptionsOptionIdArchiveRequest extends VendorPayload {
  PostPastryOptionsOptionIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryOptionsOptionIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryProductsRequest extends VendorPayload {
  PostPastryProductsRequest({
    String? id,
    String? branchId,
    String? name,
    String? description,
    String? productType,
    String? currency,
    String? status,
    int? basePriceMinor,
    bool? customizable,
    String? productionClass,
    String? capacityPoolId,
    int? setupBufferMinutes,
    int? cleanupBufferMinutes,
    int? workloadUnitsPerItem,
    JsonObject? logistics,
    int? version,
    int? position,
  }) : super({
         'id': ?id,
         'branch_id': ?branchId,
         'name': ?name,
         'description': ?description,
         'product_type': ?productType,
         'currency': ?currency,
         'status': ?status,
         'base_price_minor': ?basePriceMinor,
         'customizable': ?customizable,
         'production_class': ?productionClass,
         'capacity_pool_id': ?capacityPoolId,
         'setup_buffer_minutes': ?setupBufferMinutes,
         'cleanup_buffer_minutes': ?cleanupBufferMinutes,
         'workload_units_per_item': ?workloadUnitsPerItem,
         'logistics': ?logistics,
         'version': ?version,
         'position': ?position,
       });
  PostPastryProductsRequest.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get productType => value('product_type') as String?;
  String? get currency => value('currency') as String?;
  String? get status => value('status') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  bool? get customizable => value('customizable') as bool?;
  String? get productionClass => value('production_class') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  int? get workloadUnitsPerItem => value('workload_units_per_item') as int?;
  JsonObject? get logistics =>
      value('logistics') == null ? null : jsonObject(value('logistics'));
  int? get version => value('version') as int?;
  int? get position => value('position') as int?;
}

class PatchPastryProductsProductIdRequest extends VendorPayload {
  PatchPastryProductsProductIdRequest({
    String? id,
    String? branchId,
    String? name,
    String? description,
    String? productType,
    String? currency,
    String? status,
    int? basePriceMinor,
    bool? customizable,
    String? productionClass,
    String? capacityPoolId,
    int? setupBufferMinutes,
    int? cleanupBufferMinutes,
    int? workloadUnitsPerItem,
    JsonObject? logistics,
    int? version,
    int? position,
  }) : super({
         'id': ?id,
         'branch_id': ?branchId,
         'name': ?name,
         'description': ?description,
         'product_type': ?productType,
         'currency': ?currency,
         'status': ?status,
         'base_price_minor': ?basePriceMinor,
         'customizable': ?customizable,
         'production_class': ?productionClass,
         'capacity_pool_id': ?capacityPoolId,
         'setup_buffer_minutes': ?setupBufferMinutes,
         'cleanup_buffer_minutes': ?cleanupBufferMinutes,
         'workload_units_per_item': ?workloadUnitsPerItem,
         'logistics': ?logistics,
         'version': ?version,
         'position': ?position,
       });
  PatchPastryProductsProductIdRequest.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get productType => value('product_type') as String?;
  String? get currency => value('currency') as String?;
  String? get status => value('status') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  bool? get customizable => value('customizable') as bool?;
  String? get productionClass => value('production_class') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  int? get workloadUnitsPerItem => value('workload_units_per_item') as int?;
  JsonObject? get logistics =>
      value('logistics') == null ? null : jsonObject(value('logistics'));
  int? get version => value('version') as int?;
  int? get position => value('position') as int?;
}

class PostPastryProductsProductIdArchiveRequest extends VendorPayload {
  PostPastryProductsProductIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryProductsProductIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryProductsProductIdCapacityPreviewRequest extends VendorPayload {
  PostPastryProductsProductIdCapacityPreviewRequest({
    int? quantity,
    String? windowStart,
    String? windowEnd,
    String? productionLineId,
  }) : super({
         'quantity': ?quantity,
         'window_start': ?windowStart,
         'window_end': ?windowEnd,
         'production_line_id': ?productionLineId,
       });
  PostPastryProductsProductIdCapacityPreviewRequest.fromJson(super.json);
  int? get quantity => value('quantity') as int?;
  String? get windowStart => value('window_start') as String?;
  String? get windowEnd => value('window_end') as String?;
  String? get productionLineId => value('production_line_id') as String?;
}

class PostPastryProductsProductIdOptionGroupsRequest extends VendorPayload {
  PostPastryProductsProductIdOptionGroupsRequest({
    String? groupId,
    int? position,
    bool? requiredValue,
    int? expectedVersion,
  }) : super({
         'group_id': ?groupId,
         'position': ?position,
         'required': ?requiredValue,
         'expected_version': ?expectedVersion,
       });
  PostPastryProductsProductIdOptionGroupsRequest.fromJson(super.json);
  String? get groupId => value('group_id') as String?;
  int? get position => value('position') as int?;
  bool? get requiredValue => value('required') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class DeletePastryProductsProductIdOptionGroupsGroupIdRequest
    extends VendorPayload {
  DeletePastryProductsProductIdOptionGroupsGroupIdRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  DeletePastryProductsProductIdOptionGroupsGroupIdRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryProductsProductIdProductionBatchesRequest
    extends VendorPayload {
  PostPastryProductsProductIdProductionBatchesRequest({
    int? quantity,
    String? windowStart,
    String? windowEnd,
    String? productionLineId,
  }) : super({
         'quantity': ?quantity,
         'window_start': ?windowStart,
         'window_end': ?windowEnd,
         'production_line_id': ?productionLineId,
       });
  PostPastryProductsProductIdProductionBatchesRequest.fromJson(super.json);
  int? get quantity => value('quantity') as int?;
  String? get windowStart => value('window_start') as String?;
  String? get windowEnd => value('window_end') as String?;
  String? get productionLineId => value('production_line_id') as String?;
}

class PostPastryQuotesQuoteIdAcceptRequest extends VendorPayload {
  PostPastryQuotesQuoteIdAcceptRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryQuotesQuoteIdAcceptRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryQuotesQuoteIdApproveRequest extends VendorPayload {
  PostPastryQuotesQuoteIdApproveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryQuotesQuoteIdApproveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryQuotesQuoteIdDeclineRequest extends VendorPayload {
  PostPastryQuotesQuoteIdDeclineRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPastryQuotesQuoteIdDeclineRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPastryQuotesQuoteIdReviseRequest extends VendorPayload {
  PostPastryQuotesQuoteIdReviseRequest({
    String? specificationVersionId,
    int? amountMinor,
    String? expiresAt,
    String? reasonCode,
    String? note,
    String? vendorTerms,
    int? expectedVersion,
  }) : super({
         'specification_version_id': ?specificationVersionId,
         'amount_minor': ?amountMinor,
         'expires_at': ?expiresAt,
         'reason_code': ?reasonCode,
         'note': ?note,
         'vendor_terms': ?vendorTerms,
         'expected_version': ?expectedVersion,
       });
  PostPastryQuotesQuoteIdReviseRequest.fromJson(super.json);
  String? get specificationVersionId =>
      value('specification_version_id') as String?;
  int? get amountMinor => value('amount_minor') as int?;
  String? get expiresAt => value('expires_at') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
  String? get vendorTerms => value('vendor_terms') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PastrySections1011ProductionOccurrenceRetryCommand extends VendorPayload {
  PastrySections1011ProductionOccurrenceRetryCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get reason => value('reason') as String?;
}

class PastrySections1011ProductionOccurrenceRescheduleCommand
    extends VendorPayload {
  PastrySections1011ProductionOccurrenceRescheduleCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get startsAt => value('starts_at') as String?;
  String? get endsAt => value('ends_at') as String?;
}

class PastrySections1011ProductionPastryQuoteCreateCommand
    extends VendorPayload {
  PastrySections1011ProductionPastryQuoteCreateCommand.fromJson(super.json);
  String? get specificationVersionId =>
      value('specification_version_id') as String?;
  int? get amountMinor => value('amount_minor') as int?;
  String? get expiresAt => value('expires_at') as String?;
  bool? get approvalRequired => value('approval_required') as bool?;
  int? get expectedRequestVersion => value('expected_request_version') as int?;
  String? get productionStart => value('production_start') as String?;
  String? get productionEnd => value('production_end') as String?;
  int? get holdForMinutes => value('hold_for_minutes') as int?;
  String? get vendorTerms => value('vendor_terms') as String?;
}
