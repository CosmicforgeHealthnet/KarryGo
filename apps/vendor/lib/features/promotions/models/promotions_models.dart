// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostPromotionsRequest extends VendorPayload {
  PostPromotionsRequest({
    String? name,
    String? promotionType,
    String? currency,
    int? percentageBps,
    int? fixedAmountMinor,
    int? maxDiscountMinor,
    int? minimumOrderMinor,
    String? startsAt,
    String? endsAt,
    String? fundingSponsor,
    int? vendorFundingBps,
    bool? couponRequired,
    int? globalUsageLimit,
    int? perCustomerUsageLimit,
    String? combinability,
    int? priority,
    List<dynamic>? scopes,
    List<dynamic>? bundleRequirements,
    List<dynamic>? quantityTiers,
  }) : super({
         'name': ?name,
         'promotion_type': ?promotionType,
         'currency': ?currency,
         'percentage_bps': ?percentageBps,
         'fixed_amount_minor': ?fixedAmountMinor,
         'max_discount_minor': ?maxDiscountMinor,
         'minimum_order_minor': ?minimumOrderMinor,
         'starts_at': ?startsAt,
         'ends_at': ?endsAt,
         'funding_sponsor': ?fundingSponsor,
         'vendor_funding_bps': ?vendorFundingBps,
         'coupon_required': ?couponRequired,
         'global_usage_limit': ?globalUsageLimit,
         'per_customer_usage_limit': ?perCustomerUsageLimit,
         'combinability': ?combinability,
         'priority': ?priority,
         'scopes': ?scopes,
         'bundle_requirements': ?bundleRequirements,
         'quantity_tiers': ?quantityTiers,
       });
  PostPromotionsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get promotionType => value('promotion_type') as String?;
  String? get currency => value('currency') as String?;
  int? get percentageBps => value('percentage_bps') as int?;
  int? get fixedAmountMinor => value('fixed_amount_minor') as int?;
  int? get maxDiscountMinor => value('max_discount_minor') as int?;
  int? get minimumOrderMinor => value('minimum_order_minor') as int?;
  String? get startsAt => value('starts_at') as String?;
  String? get endsAt => value('ends_at') as String?;
  String? get fundingSponsor => value('funding_sponsor') as String?;
  int? get vendorFundingBps => value('vendor_funding_bps') as int?;
  bool? get couponRequired => value('coupon_required') as bool?;
  int? get globalUsageLimit => value('global_usage_limit') as int?;
  int? get perCustomerUsageLimit => value('per_customer_usage_limit') as int?;
  String? get combinability => value('combinability') as String?;
  int? get priority => value('priority') as int?;
  List<dynamic>? get scopes => value('scopes') as List<dynamic>?;
  List<dynamic>? get bundleRequirements =>
      value('bundle_requirements') as List<dynamic>?;
  List<dynamic>? get quantityTiers => value('quantity_tiers') as List<dynamic>?;
}

class PatchPromotionsPromotionIdRequest extends VendorPayload {
  PatchPromotionsPromotionIdRequest({
    String? name,
    String? promotionType,
    String? currency,
    int? percentageBps,
    int? fixedAmountMinor,
    int? maxDiscountMinor,
    int? minimumOrderMinor,
    String? startsAt,
    String? endsAt,
    String? fundingSponsor,
    int? vendorFundingBps,
    bool? couponRequired,
    int? globalUsageLimit,
    int? perCustomerUsageLimit,
    String? combinability,
    int? priority,
    List<dynamic>? scopes,
    List<dynamic>? bundleRequirements,
    List<dynamic>? quantityTiers,
  }) : super({
         'name': ?name,
         'promotion_type': ?promotionType,
         'currency': ?currency,
         'percentage_bps': ?percentageBps,
         'fixed_amount_minor': ?fixedAmountMinor,
         'max_discount_minor': ?maxDiscountMinor,
         'minimum_order_minor': ?minimumOrderMinor,
         'starts_at': ?startsAt,
         'ends_at': ?endsAt,
         'funding_sponsor': ?fundingSponsor,
         'vendor_funding_bps': ?vendorFundingBps,
         'coupon_required': ?couponRequired,
         'global_usage_limit': ?globalUsageLimit,
         'per_customer_usage_limit': ?perCustomerUsageLimit,
         'combinability': ?combinability,
         'priority': ?priority,
         'scopes': ?scopes,
         'bundle_requirements': ?bundleRequirements,
         'quantity_tiers': ?quantityTiers,
       });
  PatchPromotionsPromotionIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get promotionType => value('promotion_type') as String?;
  String? get currency => value('currency') as String?;
  int? get percentageBps => value('percentage_bps') as int?;
  int? get fixedAmountMinor => value('fixed_amount_minor') as int?;
  int? get maxDiscountMinor => value('max_discount_minor') as int?;
  int? get minimumOrderMinor => value('minimum_order_minor') as int?;
  String? get startsAt => value('starts_at') as String?;
  String? get endsAt => value('ends_at') as String?;
  String? get fundingSponsor => value('funding_sponsor') as String?;
  int? get vendorFundingBps => value('vendor_funding_bps') as int?;
  bool? get couponRequired => value('coupon_required') as bool?;
  int? get globalUsageLimit => value('global_usage_limit') as int?;
  int? get perCustomerUsageLimit => value('per_customer_usage_limit') as int?;
  String? get combinability => value('combinability') as String?;
  int? get priority => value('priority') as int?;
  List<dynamic>? get scopes => value('scopes') as List<dynamic>?;
  List<dynamic>? get bundleRequirements =>
      value('bundle_requirements') as List<dynamic>?;
  List<dynamic>? get quantityTiers => value('quantity_tiers') as List<dynamic>?;
}

class PostPromotionsPromotionIdCouponsRequest extends VendorPayload {
  PostPromotionsPromotionIdCouponsRequest({
    required String code,
    int? usageLimit,
    int? perCustomerUsageLimit,
  }) : super({
         'code': code,
         'usage_limit': ?usageLimit,
         'per_customer_usage_limit': ?perCustomerUsageLimit,
       });
  PostPromotionsPromotionIdCouponsRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  int? get usageLimit => value('usage_limit') as int?;
  int? get perCustomerUsageLimit => value('per_customer_usage_limit') as int?;
}

class PostPromotionsPromotionIdStatusRequest extends VendorPayload {
  PostPromotionsPromotionIdStatusRequest({required String status})
    : super({'status': status});
  PostPromotionsPromotionIdStatusRequest.fromJson(super.json);
  String? get status => value('status') as String?;
}

class PromotionsPromotionsDeliveryFundingCampaignInput extends VendorPayload {
  PromotionsPromotionsDeliveryFundingCampaignInput.fromJson(super.json);
  List<dynamic>? get bundleRequirements =>
      value('bundle_requirements') as List<dynamic>?;
  String? get combinability => value('combinability') as String?;
  bool? get couponRequired => value('coupon_required') as bool?;
  String? get currency => value('currency') as String?;
  String? get endsAt => value('ends_at') as String?;
  int? get fixedAmountMinor => value('fixed_amount_minor') as int?;
  String? get fundingSponsor => value('funding_sponsor') as String?;
  int? get globalUsageLimit => value('global_usage_limit') as int?;
  int? get maxDiscountMinor => value('max_discount_minor') as int?;
  int? get minimumOrderMinor => value('minimum_order_minor') as int?;
  String? get name => value('name') as String?;
  int? get perCustomerUsageLimit => value('per_customer_usage_limit') as int?;
  int? get percentageBps => value('percentage_bps') as int?;
  int? get priority => value('priority') as int?;
  String? get promotionType => value('promotion_type') as String?;
  List<dynamic>? get quantityTiers => value('quantity_tiers') as List<dynamic>?;
  List<dynamic>? get scopes => value('scopes') as List<dynamic>?;
  String? get startsAt => value('starts_at') as String?;
}
