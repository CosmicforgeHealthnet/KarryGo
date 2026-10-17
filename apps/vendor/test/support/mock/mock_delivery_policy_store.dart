import 'package:http/http.dart' as http;
import 'dart:convert';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';
import 'package:vendor/features/delivery/state/delivery_policy_rules.dart';

class MockDeliveryPolicyStore {
  MockDeliveryPolicyStore(this.business);
  final MockOnboardingStore business;
  final records = <String, JsonObject>{};
  final receipts = <String, JsonObject>{};
  final fingerprints = <String, String>{};
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/delivery/branches/([^/]+)/policy$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final id = match[1]!;
    JsonObject fail(int code, String name, String message) => {
      '_status': code,
      '_code': name,
      '_message': message,
    };
    if (!business.branches.any((b) => b['id'] == id)) {
      return fail(
        404,
        'DELIVERY_POLICY_NOT_FOUND',
        'Delivery policy was not found.',
      );
    }
    final value = records.putIfAbsent(
      id,
      () => {
        'id': 'demo-delivery-policy-$id',
        'branch_id': id,
        'delivery_mode': 'HYBRID',
        'fee_rule': 'DISTANCE_TIER',
        'enabled': true,
        'currency': business.business['currency'],
        'base_fee_minor': 50000,
        'per_km_fee_minor': 10000,
        'minimum_order_minor': 100000,
        'free_delivery_threshold_minor': null,
        'max_delivery_radius_meters': 10000,
        'operating_timezone': business.business['timezone'],
        'supported_vehicle_classes': ['MOTORCYCLE'],
        'fallback_triggers': [],
        'hybrid_fallback_funding_sponsor': 'VENDOR',
        'hybrid_vendor_share_bps': 10000,
        'rider_availability': 'UNKNOWN',
        'estimated_minutes_base': 20,
        'estimated_minutes_per_km': 2,
        'version': 1,
      },
    );
    if (request.method == 'GET') {
      return {
        '_status': 200,
        '_data': {...value},
      };
    }
    if (request.method != 'PUT') {
      return fail(405, 'METHOD_NOT_ALLOWED', 'Method not allowed.');
    }
    final headers = {
      for (final e in request.headers.entries) e.key.toLowerCase(): e.value,
    };
    final commandKey = headers['idempotency-key'];
    final fingerprint = jsonEncode([
      request.url.path,
      body,
      headers['if-match'],
    ]);
    if (commandKey == null || commandKey.isEmpty) {
      return fail(
        400,
        'IDEMPOTENCY_KEY_REQUIRED',
        'Idempotency-Key is required.',
      );
    }
    if (receipts.containsKey(commandKey)) {
      return fingerprints[commandKey] == fingerprint
          ? receipts[commandKey]!
          : fail(
              409,
              'IDEMPOTENCY_CONFLICT',
              'This retry key belongs to a different policy update.',
            );
    }
    final validation = validateDeliveryPolicy(body);
    if (validation != null) {
      return fail(422, 'DELIVERY_POLICY_VALIDATION_FAILED', validation);
    }
    final version = request.headers.entries
        .where((e) => e.key.toLowerCase() == 'if-match')
        .firstOrNull
        ?.value;
    if (int.tryParse(version ?? '') != value['version']) {
      return fail(
        409,
        'DELIVERY_POLICY_VERSION_CONFLICT',
        'This policy changed. Refresh before continuing.',
      );
    }
    if (!['PLATFORM', 'VENDOR', 'HYBRID'].contains(body['delivery_mode']) ||
        !['FLAT', 'ZONE_BASED', 'DISTANCE_TIER'].contains(body['fee_rule']) ||
        !RegExp(r'^[A-Z]{3}$').hasMatch(body['currency']?.toString() ?? '')) {
      return fail(
        422,
        'DELIVERY_POLICY_VALIDATION_FAILED',
        'Check delivery mode, fee rule and currency.',
      );
    }
    if (body['hybrid_fallback_funding_sponsor'] != null &&
        body['hybrid_fallback_funding_sponsor'] != 'VENDOR') {
      return fail(
        403,
        'DELIVERY_POLICY_FORBIDDEN',
        'Vendor staff cannot authorize platform funding.',
      );
    }
    for (final field in [
      'base_fee_minor',
      'per_km_fee_minor',
      'minimum_order_minor',
    ]) {
      if (body[field] is! int || body[field] < 0) {
        return fail(
          422,
          'DELIVERY_POLICY_VALIDATION_FAILED',
          'Delivery charges must be non-negative.',
        );
      }
    }
    if (body['free_delivery_threshold_minor'] != null &&
        body['free_delivery_threshold_minor'] < body['minimum_order_minor']) {
      return fail(
        422,
        'DELIVERY_POLICY_VALIDATION_FAILED',
        'The free delivery threshold must meet the minimum order.',
      );
    }
    final saved = {...body}..remove('reason');
    records[id] = {
      ...saved,
      'id': value['id'],
      'branch_id': id,
      'version': (value['version'] as int) + 1,
    };
    final response = jsonObject(
      jsonDecode(jsonEncode({'_status': 200, '_data': records[id]})),
    );
    receipts[commandKey] = response;
    fingerprints[commandKey] = fingerprint;
    return response;
  }
}
