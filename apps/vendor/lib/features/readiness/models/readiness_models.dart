// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCommerceOrdersVendorOrderIdReadinessBlockRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadinessBlockRequest({
    String? substate,
    String? reasonCode,
    String? estimatedReadyAt,
  }) : super({
         'substate': ?substate,
         'reason_code': ?reasonCode,
         'estimated_ready_at': ?estimatedReadyAt,
       });
  PostCommerceOrdersVendorOrderIdReadinessBlockRequest.fromJson(super.json);
  String? get substate => value('substate') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get estimatedReadyAt => value('estimated_ready_at') as String?;
}

class PostCommerceOrdersVendorOrderIdReadinessDelayRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadinessDelayRequest({
    String? substate,
    String? reasonCode,
    String? estimatedReadyAt,
  }) : super({
         'substate': ?substate,
         'reason_code': ?reasonCode,
         'estimated_ready_at': ?estimatedReadyAt,
       });
  PostCommerceOrdersVendorOrderIdReadinessDelayRequest.fromJson(super.json);
  String? get substate => value('substate') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get estimatedReadyAt => value('estimated_ready_at') as String?;
}

class PostCommerceOrdersVendorOrderIdReadinessEstimateRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadinessEstimateRequest({
    String? substate,
    String? reasonCode,
    String? estimatedReadyAt,
  }) : super({
         'substate': ?substate,
         'reason_code': ?reasonCode,
         'estimated_ready_at': ?estimatedReadyAt,
       });
  PostCommerceOrdersVendorOrderIdReadinessEstimateRequest.fromJson(super.json);
  String? get substate => value('substate') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get estimatedReadyAt => value('estimated_ready_at') as String?;
}

class PostCommerceOrdersVendorOrderIdReadinessReadyRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadinessReadyRequest({
    String? substate,
    String? reasonCode,
    String? estimatedReadyAt,
  }) : super({
         'substate': ?substate,
         'reason_code': ?reasonCode,
         'estimated_ready_at': ?estimatedReadyAt,
       });
  PostCommerceOrdersVendorOrderIdReadinessReadyRequest.fromJson(super.json);
  String? get substate => value('substate') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get estimatedReadyAt => value('estimated_ready_at') as String?;
}

class PostCommerceOrdersVendorOrderIdReadinessReadySoonRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadinessReadySoonRequest({
    String? substate,
    String? reasonCode,
    String? estimatedReadyAt,
  }) : super({
         'substate': ?substate,
         'reason_code': ?reasonCode,
         'estimated_ready_at': ?estimatedReadyAt,
       });
  PostCommerceOrdersVendorOrderIdReadinessReadySoonRequest.fromJson(super.json);
  String? get substate => value('substate') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get estimatedReadyAt => value('estimated_ready_at') as String?;
}

class PostCommerceOrdersVendorOrderIdReadinessStartRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadinessStartRequest({
    String? parentOrderId,
    String? branchId,
    String? vendorType,
    String? estimatedReadyAt,
  }) : super({
         'parent_order_id': ?parentOrderId,
         'branch_id': ?branchId,
         'vendor_type': ?vendorType,
         'estimated_ready_at': ?estimatedReadyAt,
       });
  PostCommerceOrdersVendorOrderIdReadinessStartRequest.fromJson(super.json);
  String? get parentOrderId => value('parent_order_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get vendorType => value('vendor_type') as String?;
  String? get estimatedReadyAt => value('estimated_ready_at') as String?;
}
