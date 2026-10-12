import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';

const promotionBase = '/api/v1/vendor/promotions';
const promotionTypes = {
  'PERCENTAGE_DISCOUNT': 'Percentage discount',
  'FIXED_DISCOUNT': 'Fixed discount',
  'COUPON': 'Coupon offer',
  'BUNDLE': 'Bundle offer',
  'QUANTITY_BREAK': 'Quantity discounts',
  'FREE_DELIVERY': 'Free delivery',
  'DELIVERY_PERCENTAGE_DISCOUNT': 'Delivery percentage discount',
  'DELIVERY_FLAT_DISCOUNT': 'Delivery fixed discount',
};
const promotionTransitions = {
  'DRAFT': ['SCHEDULED', 'ACTIVE', 'ARCHIVED'],
  'SCHEDULED': ['ACTIVE', 'PAUSED', 'ENDED', 'ARCHIVED'],
  'ACTIVE': ['PAUSED', 'ENDED', 'ARCHIVED'],
  'PAUSED': ['ACTIVE', 'ENDED', 'ARCHIVED'],
  'ENDED': ['ARCHIVED'],
  'ARCHIVED': <String>[],
};
const promotionScopeTypes = [
  'BRANCH',
  'CAPABILITY',
  'CATEGORY',
  'ITEM',
  'VARIANT',
  'CUSTOMER_SEGMENT',
];

int? promotionDecimal(String text) {
  final match = RegExp(r'^(\d+)(?:\.(\d{1,2}))?$').firstMatch(text.trim());
  if (match == null) return null;
  final whole = int.tryParse(match[1]!);
  if (whole == null) return null;
  return whole * 100 + int.parse((match[2] ?? '').padRight(2, '0'));
}

String promotionAmount(Object? value) => value is int
    ? '${value ~/ 100}.${(value % 100).toString().padLeft(2, '0')}'
    : '';

String promotionBenefit(JsonObject row) {
  if (row['promotion_type'] == 'FREE_DELIVERY') return 'Free delivery';
  if (row['promotion_type'] == 'QUANTITY_BREAK') {
    return '${(row['quantity_tiers'] as List?)?.length ?? 0} quantity tiers';
  }
  final bps = row['percentage_bps'];
  final discount = bps is int && bps > 0
      ? '${promotionAmount(bps).replaceFirst(RegExp(r'\.00$'), '')}% off'
      : '${dashboardMoney(row['fixed_amount_minor'], row['currency'])} off';
  return row['promotion_type'].toString().startsWith('DELIVERY_')
      ? '$discount delivery'
      : discount;
}

String? validatePromotion(JsonObject body) {
  if (body['name'] is! String || (body['name'] as String).trim().isEmpty) {
    return 'Give your promotion a name.';
  }
  if (!promotionTypes.containsKey(body['promotion_type'])) {
    return 'Choose an offer type.';
  }
  if (!RegExp(r'^[A-Z]{3}$').hasMatch(body['currency']?.toString() ?? '')) {
    return 'Use a three-letter currency code.';
  }
  final start = DateTime.tryParse(body['starts_at']?.toString() ?? '');
  final end = DateTime.tryParse(body['ends_at']?.toString() ?? '');
  if (start == null || end == null || !end.isAfter(start)) {
    return 'The end time must be after the start time.';
  }
  final percentage = body['percentage_bps'], fixed = body['fixed_amount_minor'];
  if (percentage is! int ||
      percentage < 0 ||
      percentage > 10000 ||
      fixed is! int ||
      fixed < 0 ||
      body['minimum_order_minor'] is! int ||
      body['minimum_order_minor'] < 0 ||
      body['priority'] is! int) {
    return 'Check your discount, minimum spend and priority.';
  }
  final type = body['promotion_type'];
  if (['PERCENTAGE_DISCOUNT', 'DELIVERY_PERCENTAGE_DISCOUNT'].contains(type) &&
      (percentage <= 0 || fixed != 0)) {
    return 'Enter a percentage between 0.01 and 100.';
  }
  if (['FIXED_DISCOUNT', 'DELIVERY_FLAT_DISCOUNT'].contains(type) &&
      (fixed <= 0 || percentage != 0)) {
    return 'Enter a discount greater than zero.';
  }
  if (['COUPON', 'BUNDLE'].contains(type) &&
      !((percentage > 0 && fixed == 0) || (fixed > 0 && percentage == 0))) {
    return 'Choose one discount: a percentage or a fixed amount.';
  }
  for (final key in [
    'max_discount_minor',
    'global_usage_limit',
    'per_customer_usage_limit',
  ]) {
    final value = body[key];
    if (value != null && (value is! int || value <= 0)) {
      return 'Optional limits must be greater than zero.';
    }
  }
  if (body['funding_sponsor'] != 'VENDOR' ||
      body['vendor_funding_bps'] != 10000) {
    return 'Only vendor-funded offers can be configured here.';
  }
  if (!['EXCLUSIVE', 'STACKABLE'].contains(body['combinability'])) {
    return 'Choose how offers combine.';
  }
  final scopes = body['scopes'];
  if (scopes is! List) return 'Check the offer audience.';
  final seen = <String>{};
  for (final raw in scopes) {
    if (raw is! Map ||
        !promotionScopeTypes.contains(raw['type']) ||
        raw['ref'] is! String ||
        (raw['ref'] as String).trim().isEmpty ||
        !seen.add('${raw['type']}|${raw['ref']}')) {
      return 'Check for missing or repeated audience references.';
    }
  }
  if (type == 'BUNDLE') {
    final requirements = body['bundle_requirements'];
    if (requirements is! List || requirements.isEmpty) {
      return 'Add the items required for this bundle.';
    }
    final keys = <String>{};
    for (final row in requirements) {
      if (row is! Map ||
          row['item_ref'] is! String ||
          (row['item_ref'] as String).trim().isEmpty ||
          row['minimum_quantity'] is! int ||
          row['minimum_quantity'] <= 0 ||
          !keys.add('${row['item_ref']}|${row['variant_ref'] ?? ''}')) {
        return 'Check bundle items and quantities.';
      }
    }
  }
  if (type == 'QUANTITY_BREAK') {
    final tiers = body['quantity_tiers'];
    if (tiers is! List || tiers.isEmpty) {
      return 'Add at least one quantity tier.';
    }
    int? previousMax;
    for (var i = 0; i < tiers.length; i++) {
      final tier = tiers[i];
      if (tier is! Map) return 'Check quantity tiers.';
      final min = tier['minimum_quantity'], max = tier['maximum_quantity'];
      final bps = tier['percentage_bps'], amount = tier['fixed_amount_minor'];
      if (min is! int ||
          min < 1 ||
          max != null && (max is! int || max < min) ||
          bps is! int ||
          bps < 0 ||
          bps > 10000 ||
          amount is! int ||
          amount < 0 ||
          (bps > 0) == (amount > 0) ||
          i > 0 && (previousMax == null || min != previousMax + 1)) {
        return 'Use consecutive quantity ranges and one discount per tier.';
      }
      previousMax = max as int?;
    }
  }
  return null;
}
