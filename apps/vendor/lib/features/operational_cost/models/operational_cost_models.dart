// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCostPoliciesRequest extends VendorPayload {
  PostCostPoliciesRequest({
    String? branchId,
    String? subjectType,
    String? subjectRef,
    String? strategy,
    String? currency,
    int? weightedAverageReceiptLimit,
    int? manualUnitCostMinor,
    String? manualUnitCode,
    String? manualCostReason,
    String? reason,
  }) : super({
         'branch_id': ?branchId,
         'subject_type': ?subjectType,
         'subject_ref': ?subjectRef,
         'strategy': ?strategy,
         'currency': ?currency,
         'weighted_average_receipt_limit': ?weightedAverageReceiptLimit,
         'manual_unit_cost_minor': ?manualUnitCostMinor,
         'manual_unit_code': ?manualUnitCode,
         'manual_cost_reason': ?manualCostReason,
         'reason': ?reason,
       });
  PostCostPoliciesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get subjectType => value('subject_type') as String?;
  String? get subjectRef => value('subject_ref') as String?;
  String? get strategy => value('strategy') as String?;
  String? get currency => value('currency') as String?;
  int? get weightedAverageReceiptLimit =>
      value('weighted_average_receipt_limit') as int?;
  int? get manualUnitCostMinor => value('manual_unit_cost_minor') as int?;
  String? get manualUnitCode => value('manual_unit_code') as String?;
  String? get manualCostReason => value('manual_cost_reason') as String?;
  String? get reason => value('reason') as String?;
}

class PutCostPoliciesPolicyIdRequest extends VendorPayload {
  PutCostPoliciesPolicyIdRequest({
    String? branchId,
    String? subjectType,
    String? subjectRef,
    String? strategy,
    String? currency,
    int? weightedAverageReceiptLimit,
    int? manualUnitCostMinor,
    String? manualUnitCode,
    String? manualCostReason,
    String? reason,
  }) : super({
         'branch_id': ?branchId,
         'subject_type': ?subjectType,
         'subject_ref': ?subjectRef,
         'strategy': ?strategy,
         'currency': ?currency,
         'weighted_average_receipt_limit': ?weightedAverageReceiptLimit,
         'manual_unit_cost_minor': ?manualUnitCostMinor,
         'manual_unit_code': ?manualUnitCode,
         'manual_cost_reason': ?manualCostReason,
         'reason': ?reason,
       });
  PutCostPoliciesPolicyIdRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get subjectType => value('subject_type') as String?;
  String? get subjectRef => value('subject_ref') as String?;
  String? get strategy => value('strategy') as String?;
  String? get currency => value('currency') as String?;
  int? get weightedAverageReceiptLimit =>
      value('weighted_average_receipt_limit') as int?;
  int? get manualUnitCostMinor => value('manual_unit_cost_minor') as int?;
  String? get manualUnitCode => value('manual_unit_code') as String?;
  String? get manualCostReason => value('manual_cost_reason') as String?;
  String? get reason => value('reason') as String?;
}

class PostCostPoliciesPolicyIdArchiveRequest extends VendorPayload {
  PostCostPoliciesPolicyIdArchiveRequest({String? reason})
    : super({'reason': ?reason});
  PostCostPoliciesPolicyIdArchiveRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
}
