import '../../../core/contracts/vendor_contract.dart';

const deliveryVehicleClasses = ['MOTORCYCLE', 'VAN', 'TRUCK'];
const deliveryFallbackTriggers = {
  'OWN_RIDER_FIRST': 'Own rider first',
  'DISTANCE_OVERFLOW': 'Beyond delivery distance',
  'TIME_OVERFLOW': 'Outside delivery hours',
  'CAPACITY_OVERFLOW': 'Package capacity exceeded',
  'VEHICLE_OVERFLOW': 'Vehicle requirements unmet',
  'SLA_OVERFLOW': 'Delivery time exceeds SLA',
};
const deliveryWeekdays = [
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
  'Sunday',
];
String policyClock(int minute) =>
    '${(minute ~/ 60).toString().padLeft(2, '0')}:${(minute % 60).toString().padLeft(2, '0')}';
int? policyMoney(String text) {
  final match = RegExp(r'^(\d+)(?:\.(\d{1,2}))?$').firstMatch(text.trim());
  if (match == null) return null;
  final whole = int.tryParse(match[1]!);
  return whole == null
      ? null
      : whole * 100 + int.parse((match[2] ?? '').padRight(2, '0'));
}

String? validateAdvancedPolicy(JsonObject body) {
  final zones = body['service_zones'] ?? [],
      tiers = body['distance_tiers'] ?? [],
      periods = body['operating_periods'] ?? [];
  if (zones is! List || tiers is! List || periods is! List) {
    return 'Check delivery rules.';
  }
  for (final zone in zones) {
    if (zone is! Map ||
        zone['id'] is! String ||
        (zone['id'] as String).trim().isEmpty ||
        zone['min_distance_meters'] is! int ||
        zone['min_distance_meters'] < 0 ||
        zone['max_distance_meters'] is! int ||
        zone['max_distance_meters'] < 0 ||
        zone['max_distance_meters'] > 0 &&
            zone['max_distance_meters'] <= zone['min_distance_meters'] ||
        zone['fee_minor'] is! int ||
        zone['fee_minor'] < 0 ||
        zone['priority'] is! int ||
        zone['enabled'] is! bool) {
      return 'Check each zone reference, distance range and fee.';
    }
  }
  for (var i = 0; i < tiers.length; i++) {
    final tier = tiers[i];
    if (tier is! Map ||
        tier['min_distance_meters'] is! int ||
        tier['min_distance_meters'] < 0 ||
        tier['max_distance_meters'] is! int ||
        tier['fee_minor'] is! int ||
        tier['fee_minor'] < 0 ||
        tier['max_distance_meters'] != 0 &&
            tier['max_distance_meters'] <= tier['min_distance_meters'] ||
        tier['max_distance_meters'] == 0 && i != tiers.length - 1 ||
        i > 0 &&
            tiers[i - 1]['max_distance_meters'] !=
                tier['min_distance_meters']) {
      return 'Distance tiers must be consecutive. Only the final tier may have no upper limit.';
    }
  }
  for (final period in periods) {
    if (period is! Map ||
        period['weekday'] is! int ||
        period['weekday'] < 1 ||
        period['weekday'] > 7 ||
        period['start_minute'] is! int ||
        period['start_minute'] < 0 ||
        period['start_minute'] >= 1440 ||
        period['end_minute'] is! int ||
        period['end_minute'] < 0 ||
        period['end_minute'] >= 1440 ||
        period['start_minute'] == period['end_minute']) {
      return 'Choose a weekday and different start/end times.';
    }
  }
  final vehicles = body['supported_vehicle_classes'] ?? [],
      triggers = body['fallback_triggers'] ?? [];
  if (vehicles is! List ||
      vehicles.any((v) => !deliveryVehicleClasses.contains(v))) {
    return 'Choose supported vehicle classes.';
  }
  if (triggers is! List ||
      triggers.any((v) => !deliveryFallbackTriggers.containsKey(v))) {
    return 'Choose supported fallback triggers.';
  }
  if (body['rider_availability'] != null &&
      ![
        'AVAILABLE',
        'UNAVAILABLE',
        'UNKNOWN',
      ].contains(body['rider_availability'])) {
    return 'Check rider availability.';
  }
  return null;
}

String? validateDeliveryPolicy(JsonObject body) {
  final advanced = validateAdvancedPolicy(body);
  if (advanced != null) return advanced;
  if (!RegExp(r'^[A-Z]{3}$').hasMatch(body['currency']?.toString() ?? '') ||
      !['PLATFORM', 'VENDOR', 'HYBRID'].contains(body['delivery_mode']) ||
      !['FLAT', 'ZONE_BASED', 'DISTANCE_TIER'].contains(body['fee_rule'])) {
    return 'Check currency, delivery mode and pricing.';
  }
  for (final key in [
    'base_fee_minor',
    'per_km_fee_minor',
    'minimum_order_minor',
  ]) {
    if (body[key] is! int || body[key] < 0) {
      return 'Enter non-negative delivery charges.';
    }
  }
  final free = body['free_delivery_threshold_minor'];
  if (free != null && (free is! int || free < body['minimum_order_minor'])) {
    return 'Free delivery must meet the minimum order amount.';
  }
  for (final key in [
    'max_delivery_radius_meters',
    'max_package_weight_grams',
    'max_package_count',
    'max_volume_ml',
  ]) {
    final value = body[key];
    if (value != null &&
        (value is! int ||
            value < 1 ||
            key == 'max_delivery_radius_meters' && value > 200000 ||
            key == 'max_package_weight_grams' && value > 100000000)) {
      return 'Check your delivery radius and package limits.';
    }
  }
  for (final key in ['estimated_minutes_base', 'estimated_minutes_per_km']) {
    final value = body[key];
    if (value != null && (value is! int || value < 0 || value > 1440)) {
      return 'Delivery time settings must be between 0 and 1440 minutes.';
    }
  }
  return null;
}
