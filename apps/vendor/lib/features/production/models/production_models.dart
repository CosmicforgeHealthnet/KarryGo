// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostProductionAlertsAlertIdAcknowledgeRequest extends VendorPayload {
  PostProductionAlertsAlertIdAcknowledgeRequest({
    int? expectedVersion,
    String? note,
  }) : super({'expected_version': ?expectedVersion, 'note': ?note});
  PostProductionAlertsAlertIdAcknowledgeRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get note => value('note') as String?;
}

class PostProductionAlertsAlertIdResolveRequest extends VendorPayload {
  PostProductionAlertsAlertIdResolveRequest({
    int? expectedVersion,
    String? note,
  }) : super({'expected_version': ?expectedVersion, 'note': ?note});
  PostProductionAlertsAlertIdResolveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get note => value('note') as String?;
}

class PostProductionBlackoutsRequest extends VendorPayload {
  PostProductionBlackoutsRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? startAt,
    String? endAt,
    String? reasonCode,
    String? note,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'start_at': ?startAt,
         'end_at': ?endAt,
         'reason_code': ?reasonCode,
         'note': ?note,
         'expected_version': ?expectedVersion,
       });
  PostProductionBlackoutsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get startAt => value('start_at') as String?;
  String? get endAt => value('end_at') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionBlackoutsBlackoutIdCancelRequest extends VendorPayload {
  PostProductionBlackoutsBlackoutIdCancelRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostProductionBlackoutsBlackoutIdCancelRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionCapacityPreviewRequest extends VendorPayload {
  PostProductionCapacityPreviewRequest({
    String? branchId,
    String? capability,
    String? windowStart,
    String? windowEnd,
    List<dynamic>? allocations,
    String? sourceType,
    String? sourceRef,
    String? note,
    String? priority,
    int? holdForMinutes,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'window_start': ?windowStart,
         'window_end': ?windowEnd,
         'allocations': ?allocations,
         'source_type': ?sourceType,
         'source_ref': ?sourceRef,
         'note': ?note,
         'priority': ?priority,
         'hold_for_minutes': ?holdForMinutes,
       });
  PostProductionCapacityPreviewRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get windowStart => value('window_start') as String?;
  String? get windowEnd => value('window_end') as String?;
  List<dynamic>? get allocations => value('allocations') as List<dynamic>?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get note => value('note') as String?;
  String? get priority => value('priority') as String?;
  int? get holdForMinutes => value('hold_for_minutes') as int?;
}

class PostProductionJobsRequest extends VendorPayload {
  PostProductionJobsRequest({
    String? vendorOrderId,
    String? branchId,
    String? capability,
    String? sourceType,
    String? title,
    String? reservationId,
    String? productionLineId,
    String? sourceRef,
    int? sourceVersion,
    int? plannedQuantity,
    String? scheduledStart,
    String? scheduledEnd,
    String? priority,
    String? expectedReadyWindowStart,
    String? expectedReadyWindowEnd,
    String? promisedReadyAt,
    List<dynamic>? riskFlags,
  }) : super({
         'vendor_order_id': ?vendorOrderId,
         'branch_id': ?branchId,
         'capability': ?capability,
         'source_type': ?sourceType,
         'title': ?title,
         'reservation_id': ?reservationId,
         'production_line_id': ?productionLineId,
         'source_ref': ?sourceRef,
         'source_version': ?sourceVersion,
         'planned_quantity': ?plannedQuantity,
         'scheduled_start': ?scheduledStart,
         'scheduled_end': ?scheduledEnd,
         'priority': ?priority,
         'expected_ready_window_start': ?expectedReadyWindowStart,
         'expected_ready_window_end': ?expectedReadyWindowEnd,
         'promised_ready_at': ?promisedReadyAt,
         'risk_flags': ?riskFlags,
       });
  PostProductionJobsRequest.fromJson(super.json);
  String? get vendorOrderId => value('vendor_order_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get title => value('title') as String?;
  String? get reservationId => value('reservation_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  int? get sourceVersion => value('source_version') as int?;
  int? get plannedQuantity => value('planned_quantity') as int?;
  String? get scheduledStart => value('scheduled_start') as String?;
  String? get scheduledEnd => value('scheduled_end') as String?;
  String? get priority => value('priority') as String?;
  String? get expectedReadyWindowStart =>
      value('expected_ready_window_start') as String?;
  String? get expectedReadyWindowEnd =>
      value('expected_ready_window_end') as String?;
  String? get promisedReadyAt => value('promised_ready_at') as String?;
  List<dynamic>? get riskFlags => value('risk_flags') as List<dynamic>?;
}

class PostProductionJobsJobIdCompleteRequest extends VendorPayload {
  PostProductionJobsJobIdCompleteRequest() : super({});
  PostProductionJobsJobIdCompleteRequest.fromJson(super.json);
}

class PostProductionJobsJobIdDelayRequest extends VendorPayload {
  PostProductionJobsJobIdDelayRequest({
    String? reasonCode,
    String? note,
    String? newWindowStart,
    String? newWindowEnd,
    String? idempotencyKey,
    int? expectedVersion,
  }) : super({
         'reason_code': ?reasonCode,
         'note': ?note,
         'new_window_start': ?newWindowStart,
         'new_window_end': ?newWindowEnd,
         'idempotency_key': ?idempotencyKey,
         'expected_version': ?expectedVersion,
       });
  PostProductionJobsJobIdDelayRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
  String? get newWindowStart => value('new_window_start') as String?;
  String? get newWindowEnd => value('new_window_end') as String?;
  String? get idempotencyKey => value('idempotency_key') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionJobsJobIdReadyRequest extends VendorPayload {
  PostProductionJobsJobIdReadyRequest() : super({});
  PostProductionJobsJobIdReadyRequest.fromJson(super.json);
}

class PostProductionJobsJobIdStartRequest extends VendorPayload {
  PostProductionJobsJobIdStartRequest() : super({});
  PostProductionJobsJobIdStartRequest.fromJson(super.json);
}

class PostProductionJobsJobIdStepsRequest extends VendorPayload {
  PostProductionJobsJobIdStepsRequest({
    String? productionLineId,
    int? sequenceNo,
    int? phase,
    String? name,
    bool? requiredValue,
    int? expectedJobVersion,
  }) : super({
         'production_line_id': ?productionLineId,
         'sequence_no': ?sequenceNo,
         'phase': ?phase,
         'name': ?name,
         'required': ?requiredValue,
         'expected_job_version': ?expectedJobVersion,
       });
  PostProductionJobsJobIdStepsRequest.fromJson(super.json);
  String? get productionLineId => value('production_line_id') as String?;
  int? get sequenceNo => value('sequence_no') as int?;
  int? get phase => value('phase') as int?;
  String? get name => value('name') as String?;
  bool? get requiredValue => value('required') as bool?;
  int? get expectedJobVersion => value('expected_job_version') as int?;
}

class PostProductionJobsJobIdStepsStepIdStatusRequest extends VendorPayload {
  PostProductionJobsJobIdStepsStepIdStatusRequest({
    int? expectedVersion,
    String? status,
    String? blockedReason,
    String? note,
  }) : super({
         'expected_version': ?expectedVersion,
         'status': ?status,
         'blocked_reason': ?blockedReason,
         'note': ?note,
       });
  PostProductionJobsJobIdStepsStepIdStatusRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get status => value('status') as String?;
  String? get blockedReason => value('blocked_reason') as String?;
  String? get note => value('note') as String?;
}

class PostProductionJobsJobIdStatusRequest extends VendorPayload {
  PostProductionJobsJobIdStatusRequest({
    String? note,
    String? blockedReason,
    int? producedQuantity,
    int? wasteQuantity,
    Object? packagingProfile,
    Object? packagingEvidence,
  }) : super({
         'note': ?note,
         'blocked_reason': ?blockedReason,
         'produced_quantity': ?producedQuantity,
         'waste_quantity': ?wasteQuantity,
         'packaging_profile': ?packagingProfile,
         'packaging_evidence': ?packagingEvidence,
       });
  PostProductionJobsJobIdStatusRequest.fromJson(super.json);
  String? get note => value('note') as String?;
  String? get blockedReason => value('blocked_reason') as String?;
  int? get producedQuantity => value('produced_quantity') as int?;
  int? get wasteQuantity => value('waste_quantity') as int?;
  Object? get packagingProfile => value('packaging_profile');
  Object? get packagingEvidence => value('packaging_evidence');
}

class PostProductionLinesRequest extends VendorPayload {
  PostProductionLinesRequest({
    String? branchId,
    String? capability,
    String? name,
    String? code,
    String? productionClass,
    String? capacityPoolId,
    int? setupBufferMinutes,
    int? cleanupBufferMinutes,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'name': ?name,
         'code': ?code,
         'production_class': ?productionClass,
         'capacity_pool_id': ?capacityPoolId,
         'setup_buffer_minutes': ?setupBufferMinutes,
         'cleanup_buffer_minutes': ?cleanupBufferMinutes,
         'expected_version': ?expectedVersion,
       });
  PostProductionLinesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get name => value('name') as String?;
  String? get code => value('code') as String?;
  String? get productionClass => value('production_class') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchProductionLinesLineIdRequest extends VendorPayload {
  PatchProductionLinesLineIdRequest({
    String? branchId,
    String? capability,
    String? name,
    String? code,
    String? productionClass,
    String? capacityPoolId,
    int? setupBufferMinutes,
    int? cleanupBufferMinutes,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'name': ?name,
         'code': ?code,
         'production_class': ?productionClass,
         'capacity_pool_id': ?capacityPoolId,
         'setup_buffer_minutes': ?setupBufferMinutes,
         'cleanup_buffer_minutes': ?cleanupBufferMinutes,
         'expected_version': ?expectedVersion,
       });
  PatchProductionLinesLineIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get name => value('name') as String?;
  String? get code => value('code') as String?;
  String? get productionClass => value('production_class') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionLinesLineIdArchiveRequest extends VendorPayload {
  PostProductionLinesLineIdArchiveRequest() : super({});
  PostProductionLinesLineIdArchiveRequest.fromJson(super.json);
}

class PostProductionPoolsRequest extends VendorPayload {
  PostProductionPoolsRequest({
    String? branchId,
    String? capability,
    String? name,
    String? code,
    String? scopeType,
    String? scopeRef,
    String? unitType,
    int? defaultCapacity,
    int? setupBufferMinutes,
    int? cleanupBufferMinutes,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'name': ?name,
         'code': ?code,
         'scope_type': ?scopeType,
         'scope_ref': ?scopeRef,
         'unit_type': ?unitType,
         'default_capacity': ?defaultCapacity,
         'setup_buffer_minutes': ?setupBufferMinutes,
         'cleanup_buffer_minutes': ?cleanupBufferMinutes,
         'expected_version': ?expectedVersion,
       });
  PostProductionPoolsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get name => value('name') as String?;
  String? get code => value('code') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get scopeRef => value('scope_ref') as String?;
  String? get unitType => value('unit_type') as String?;
  int? get defaultCapacity => value('default_capacity') as int?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchProductionPoolsPoolIdRequest extends VendorPayload {
  PatchProductionPoolsPoolIdRequest({
    String? branchId,
    String? capability,
    String? name,
    String? code,
    String? scopeType,
    String? scopeRef,
    String? unitType,
    int? defaultCapacity,
    int? setupBufferMinutes,
    int? cleanupBufferMinutes,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'name': ?name,
         'code': ?code,
         'scope_type': ?scopeType,
         'scope_ref': ?scopeRef,
         'unit_type': ?unitType,
         'default_capacity': ?defaultCapacity,
         'setup_buffer_minutes': ?setupBufferMinutes,
         'cleanup_buffer_minutes': ?cleanupBufferMinutes,
         'expected_version': ?expectedVersion,
       });
  PatchProductionPoolsPoolIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get name => value('name') as String?;
  String? get code => value('code') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get scopeRef => value('scope_ref') as String?;
  String? get unitType => value('unit_type') as String?;
  int? get defaultCapacity => value('default_capacity') as int?;
  int? get setupBufferMinutes => value('setup_buffer_minutes') as int?;
  int? get cleanupBufferMinutes => value('cleanup_buffer_minutes') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionPoolsPoolIdArchiveRequest extends VendorPayload {
  PostProductionPoolsPoolIdArchiveRequest() : super({});
  PostProductionPoolsPoolIdArchiveRequest.fromJson(super.json);
}

class PostProductionReservationsRequest extends VendorPayload {
  PostProductionReservationsRequest({
    String? branchId,
    String? capability,
    String? windowStart,
    String? windowEnd,
    List<dynamic>? allocations,
    String? sourceType,
    String? sourceRef,
    String? note,
    String? priority,
    int? holdForMinutes,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'window_start': ?windowStart,
         'window_end': ?windowEnd,
         'allocations': ?allocations,
         'source_type': ?sourceType,
         'source_ref': ?sourceRef,
         'note': ?note,
         'priority': ?priority,
         'hold_for_minutes': ?holdForMinutes,
       });
  PostProductionReservationsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get windowStart => value('window_start') as String?;
  String? get windowEnd => value('window_end') as String?;
  List<dynamic>? get allocations => value('allocations') as List<dynamic>?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get note => value('note') as String?;
  String? get priority => value('priority') as String?;
  int? get holdForMinutes => value('hold_for_minutes') as int?;
}

class PostProductionReservationsReservationIdConfirmRequest
    extends VendorPayload {
  PostProductionReservationsReservationIdConfirmRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostProductionReservationsReservationIdConfirmRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionReservationsReservationIdReleaseRequest
    extends VendorPayload {
  PostProductionReservationsReservationIdReleaseRequest({
    int? expectedVersion,
    List<JsonObject>? partial,
    String? reason,
  }) : super({
         'expected_version': ?expectedVersion,
         'partial': ?partial,
         'reason': ?reason,
       });
  PostProductionReservationsReservationIdReleaseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  List<JsonObject>? get partial =>
      (value('partial') as List?)?.map(jsonObject).toList();
  String? get reason => value('reason') as String?;
}

class PostProductionReservationsReservationIdRescheduleRequest
    extends VendorPayload {
  PostProductionReservationsReservationIdRescheduleRequest({
    String? windowStart,
    String? windowEnd,
    List<dynamic>? allocations,
    int? expectedVersion,
  }) : super({
         'window_start': ?windowStart,
         'window_end': ?windowEnd,
         'allocations': ?allocations,
         'expected_version': ?expectedVersion,
       });
  PostProductionReservationsReservationIdRescheduleRequest.fromJson(super.json);
  String? get windowStart => value('window_start') as String?;
  String? get windowEnd => value('window_end') as String?;
  List<dynamic>? get allocations => value('allocations') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionRulesRequest extends VendorPayload {
  PostProductionRulesRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? itemFamily,
    int? maxQuantity,
    List<dynamic>? daysOfWeek,
    String? startLocalTime,
    String? endLocalTime,
    String? effectiveStartDate,
    String? effectiveEndDate,
    int? specificity,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'item_family': ?itemFamily,
         'max_quantity': ?maxQuantity,
         'days_of_week': ?daysOfWeek,
         'start_local_time': ?startLocalTime,
         'end_local_time': ?endLocalTime,
         'effective_start_date': ?effectiveStartDate,
         'effective_end_date': ?effectiveEndDate,
         'specificity': ?specificity,
         'expected_version': ?expectedVersion,
       });
  PostProductionRulesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get itemFamily => value('item_family') as String?;
  int? get maxQuantity => value('max_quantity') as int?;
  List<dynamic>? get daysOfWeek => value('days_of_week') as List<dynamic>?;
  String? get startLocalTime => value('start_local_time') as String?;
  String? get endLocalTime => value('end_local_time') as String?;
  String? get effectiveStartDate => value('effective_start_date') as String?;
  String? get effectiveEndDate => value('effective_end_date') as String?;
  int? get specificity => value('specificity') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchProductionRulesRuleIdRequest extends VendorPayload {
  PatchProductionRulesRuleIdRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? itemFamily,
    int? maxQuantity,
    List<dynamic>? daysOfWeek,
    String? startLocalTime,
    String? endLocalTime,
    String? effectiveStartDate,
    String? effectiveEndDate,
    int? specificity,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'item_family': ?itemFamily,
         'max_quantity': ?maxQuantity,
         'days_of_week': ?daysOfWeek,
         'start_local_time': ?startLocalTime,
         'end_local_time': ?endLocalTime,
         'effective_start_date': ?effectiveStartDate,
         'effective_end_date': ?effectiveEndDate,
         'specificity': ?specificity,
         'expected_version': ?expectedVersion,
       });
  PatchProductionRulesRuleIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get itemFamily => value('item_family') as String?;
  int? get maxQuantity => value('max_quantity') as int?;
  List<dynamic>? get daysOfWeek => value('days_of_week') as List<dynamic>?;
  String? get startLocalTime => value('start_local_time') as String?;
  String? get endLocalTime => value('end_local_time') as String?;
  String? get effectiveStartDate => value('effective_start_date') as String?;
  String? get effectiveEndDate => value('effective_end_date') as String?;
  int? get specificity => value('specificity') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionRulesRuleIdArchiveRequest extends VendorPayload {
  PostProductionRulesRuleIdArchiveRequest() : super({});
  PostProductionRulesRuleIdArchiveRequest.fromJson(super.json);
}

class PostProductionSlotTemplatesRequest extends VendorPayload {
  PostProductionSlotTemplatesRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? label,
    List<dynamic>? daysOfWeek,
    String? startLocalTime,
    String? endLocalTime,
    int? capacityOverride,
    String? effectiveStartDate,
    String? effectiveEndDate,
    String? timezone,
    int? generationHorizonDays,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'label': ?label,
         'days_of_week': ?daysOfWeek,
         'start_local_time': ?startLocalTime,
         'end_local_time': ?endLocalTime,
         'capacity_override': ?capacityOverride,
         'effective_start_date': ?effectiveStartDate,
         'effective_end_date': ?effectiveEndDate,
         'timezone': ?timezone,
         'generation_horizon_days': ?generationHorizonDays,
         'expected_version': ?expectedVersion,
       });
  PostProductionSlotTemplatesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get label => value('label') as String?;
  List<dynamic>? get daysOfWeek => value('days_of_week') as List<dynamic>?;
  String? get startLocalTime => value('start_local_time') as String?;
  String? get endLocalTime => value('end_local_time') as String?;
  int? get capacityOverride => value('capacity_override') as int?;
  String? get effectiveStartDate => value('effective_start_date') as String?;
  String? get effectiveEndDate => value('effective_end_date') as String?;
  String? get timezone => value('timezone') as String?;
  int? get generationHorizonDays => value('generation_horizon_days') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchProductionSlotTemplatesTemplateIdRequest extends VendorPayload {
  PatchProductionSlotTemplatesTemplateIdRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? label,
    List<dynamic>? daysOfWeek,
    String? startLocalTime,
    String? endLocalTime,
    int? capacityOverride,
    String? effectiveStartDate,
    String? effectiveEndDate,
    String? timezone,
    int? generationHorizonDays,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'label': ?label,
         'days_of_week': ?daysOfWeek,
         'start_local_time': ?startLocalTime,
         'end_local_time': ?endLocalTime,
         'capacity_override': ?capacityOverride,
         'effective_start_date': ?effectiveStartDate,
         'effective_end_date': ?effectiveEndDate,
         'timezone': ?timezone,
         'generation_horizon_days': ?generationHorizonDays,
         'expected_version': ?expectedVersion,
       });
  PatchProductionSlotTemplatesTemplateIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get label => value('label') as String?;
  List<dynamic>? get daysOfWeek => value('days_of_week') as List<dynamic>?;
  String? get startLocalTime => value('start_local_time') as String?;
  String? get endLocalTime => value('end_local_time') as String?;
  int? get capacityOverride => value('capacity_override') as int?;
  String? get effectiveStartDate => value('effective_start_date') as String?;
  String? get effectiveEndDate => value('effective_end_date') as String?;
  String? get timezone => value('timezone') as String?;
  int? get generationHorizonDays => value('generation_horizon_days') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionSlotTemplatesTemplateIdArchiveRequest
    extends VendorPayload {
  PostProductionSlotTemplatesTemplateIdArchiveRequest() : super({});
  PostProductionSlotTemplatesTemplateIdArchiveRequest.fromJson(super.json);
}

class PostProductionSlotTemplatesTemplateIdGenerateRequest
    extends VendorPayload {
  PostProductionSlotTemplatesTemplateIdGenerateRequest() : super({});
  PostProductionSlotTemplatesTemplateIdGenerateRequest.fromJson(super.json);
}

class PostProductionSlotsRequest extends VendorPayload {
  PostProductionSlotsRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? startAt,
    String? endAt,
    int? capacityOverride,
    String? label,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'start_at': ?startAt,
         'end_at': ?endAt,
         'capacity_override': ?capacityOverride,
         'label': ?label,
         'expected_version': ?expectedVersion,
       });
  PostProductionSlotsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get startAt => value('start_at') as String?;
  String? get endAt => value('end_at') as String?;
  int? get capacityOverride => value('capacity_override') as int?;
  String? get label => value('label') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchProductionSlotsSlotIdRequest extends VendorPayload {
  PatchProductionSlotsSlotIdRequest({
    String? branchId,
    String? capability,
    String? capacityPoolId,
    String? productionLineId,
    String? startAt,
    String? endAt,
    int? capacityOverride,
    String? label,
    int? expectedVersion,
  }) : super({
         'branch_id': ?branchId,
         'capability': ?capability,
         'capacity_pool_id': ?capacityPoolId,
         'production_line_id': ?productionLineId,
         'start_at': ?startAt,
         'end_at': ?endAt,
         'capacity_override': ?capacityOverride,
         'label': ?label,
         'expected_version': ?expectedVersion,
       });
  PatchProductionSlotsSlotIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capability => value('capability') as String?;
  String? get capacityPoolId => value('capacity_pool_id') as String?;
  String? get productionLineId => value('production_line_id') as String?;
  String? get startAt => value('start_at') as String?;
  String? get endAt => value('end_at') as String?;
  int? get capacityOverride => value('capacity_override') as int?;
  String? get label => value('label') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionSlotsSlotIdArchiveRequest extends VendorPayload {
  PostProductionSlotsSlotIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostProductionSlotsSlotIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionSlotsSlotIdBlackoutRequest extends VendorPayload {
  PostProductionSlotsSlotIdBlackoutRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostProductionSlotsSlotIdBlackoutRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionSlotsSlotIdCloseRequest extends VendorPayload {
  PostProductionSlotsSlotIdCloseRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostProductionSlotsSlotIdCloseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostProductionSlotsSlotIdOpenRequest extends VendorPayload {
  PostProductionSlotsSlotIdOpenRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostProductionSlotsSlotIdOpenRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class ProductionSections1011ProductionOccurrenceRetryCommand
    extends VendorPayload {
  ProductionSections1011ProductionOccurrenceRetryCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get reason => value('reason') as String?;
}

class ProductionSections1011ProductionOccurrenceRescheduleCommand
    extends VendorPayload {
  ProductionSections1011ProductionOccurrenceRescheduleCommand.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
  String? get startsAt => value('starts_at') as String?;
  String? get endsAt => value('ends_at') as String?;
}

class ProductionSections1011ProductionPastryQuoteCreateCommand
    extends VendorPayload {
  ProductionSections1011ProductionPastryQuoteCreateCommand.fromJson(super.json);
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
