import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:crypto/crypto.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/promotions/state/promotion_rules.dart';
import 'mock_dashboard_data.dart';

class MockPromotionsStore {
  MockPromotionsStore(this.dashboard) {
    if (dashboard.scenario == MockDashboardScenario.empty) return;
    final now = DateTime.now().toUtc();
    final branch = dashboard.store.branches.first['id'];
    for (var i = 0; i < 3; i++) {
      final row = <String, dynamic>{
        'id': 'mock-promotion-$i',
        'business_id': dashboard.store.business['id'],
        'name': ['Lunch favourites', 'Delivery on us', 'Weekend welcome'][i],
        'promotion_type': ['PERCENTAGE_DISCOUNT', 'FREE_DELIVERY', 'COUPON'][i],
        'status': ['ACTIVE', 'PAUSED', 'DRAFT'][i],
        'currency': dashboard.store.business['currency'],
        'percentage_bps': i == 1 ? 0 : 1500,
        'fixed_amount_minor': 0,
        'max_discount_minor': i == 0 ? 200000 : null,
        'minimum_order_minor': 500000,
        'starts_at': now.subtract(const Duration(days: 1)).toIso8601String(),
        'ends_at': now.add(const Duration(days: 14)).toIso8601String(),
        'funding_sponsor': 'VENDOR',
        'vendor_funding_bps': 10000,
        'coupon_required': i == 2,
        'global_usage_limit': 200,
        'per_customer_usage_limit': 2,
        'combinability': 'EXCLUSIVE',
        'priority': 100,
        'version': 1,
        'scopes': [
          {'type': 'BRANCH', 'ref': branch},
        ],
        'bundle_requirements': <JsonObject>[],
        'quantity_tiers': <JsonObject>[],
      };
      campaigns.add(row);
      usage[row['id']] = i == 0 ? 24 : 0;
    }
  }
  final MockDashboardData dashboard;
  final List<JsonObject> campaigns = [];
  final usage = <String, int>{};
  final coupons = <String>{};
  final receipts = <String, JsonObject>{};
  final fingerprints = <String, String>{};
  int sequence = 0;
  JsonObject result(Object? value, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(value)),
    '_status': status,
  };
  JsonObject failure(int status, String message) => {
    '_status': status,
    '_code': status == 409 ? 'CONFLICT' : 'INVALID_PROMOTION',
    '_message': message,
  };

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final path = request.url.path;
    if (path != promotionBase && !path.startsWith('$promotionBase/')) {
      return null;
    }
    final headers = {
      for (final entry in request.headers.entries)
        entry.key.toLowerCase(): entry.value,
    };
    final key = headers['idempotency-key'];
    final write = request.method != 'GET';
    final fingerprint = jsonEncode([
      request.method,
      path,
      body,
      headers['if-match'],
    ]);
    if (write) {
      if (key == null || key.isEmpty) {
        return failure(400, 'Idempotency-Key is required.');
      }
      if (receipts.containsKey(key)) {
        return fingerprints[key] == fingerprint
            ? receipts[key]!
            : failure(409, 'This retry key belongs to a different update.');
      }
    }
    JsonObject finish(JsonObject response) {
      if (write && response['_message'] == null) {
        receipts[key!] = response;
        fingerprints[key] = fingerprint;
      }
      return response;
    }

    if (path == promotionBase) {
      if (request.method == 'GET') return result({'promotions': campaigns});
      if (request.method != 'POST') {
        return failure(405, 'Unsupported operation.');
      }
      final error = validatePromotion(body);
      if (error != null) return failure(422, error);
      final campaign = <String, dynamic>{
        ...jsonObject(jsonDecode(jsonEncode(body))),
        'id': 'mock-created-promotion-${++sequence}',
        'business_id': dashboard.store.business['id'],
        'status': 'DRAFT',
        'version': 1,
      };
      campaigns.add(campaign);
      return finish(result(campaign, 201));
    }
    final parts = path.substring(promotionBase.length + 1).split('/');
    final campaign = campaigns
        .where((row) => row['id'] == parts.first)
        .firstOrNull;
    if (campaign == null) return failure(404, 'Promotion not found.');
    if (request.method == 'GET') {
      if (parts.length == 1) return result(campaign);
      if (parts.last == 'usage') {
        return result({'used': usage[campaign['id']] ?? 0});
      }
    }
    if (parts.length == 1 && request.method == 'PATCH') {
      if (campaign['funding_sponsor'] != 'VENDOR') {
        return failure(403, 'Only vendor-funded offers can be edited.');
      }
      if (int.tryParse(headers['if-match'] ?? '') != campaign['version']) {
        return failure(
          409,
          'This promotion has changed. Refresh before editing.',
        );
      }
      final error = validatePromotion(body);
      if (error != null) return failure(422, error);
      final next = jsonObject(jsonDecode(jsonEncode(body)));
      campaign.addAll({...next, 'version': (campaign['version'] as int) + 1});
      return finish(result(campaign));
    }
    if (parts.last == 'status' && request.method == 'POST') {
      if (int.tryParse(headers['if-match'] ?? '') != campaign['version'] ||
          !(promotionTransitions[campaign['status']] ?? []).contains(
            body['status'],
          )) {
        return failure(
          409,
          'The promotion changed or this status transition is unavailable.',
        );
      }
      campaign['status'] = body['status'];
      campaign['version'] = (campaign['version'] as int) + 1;
      return finish(result(campaign));
    }
    if (parts.last == 'coupons' && request.method == 'POST') {
      if (['ENDED', 'ARCHIVED'].contains(campaign['status'])) {
        return failure(
          422,
          'Coupons cannot be added to ended or archived offers.',
        );
      }
      final code = body['code']?.toString().trim().toUpperCase() ?? '';
      if (code.isEmpty) return failure(422, 'Enter a coupon code.');
      for (final field in ['usage_limit', 'per_customer_usage_limit']) {
        final value = body[field];
        if (value != null && (value is! int || value < 1)) {
          return failure(422, 'Coupon limits must be positive.');
        }
      }
      final digest = sha256.convert(utf8.encode(code)).toString();
      if (!coupons.add(digest)) {
        return failure(422, 'Coupon code is already in use.');
      }
      return finish(result({'coupon_id': 'mock-coupon-${++sequence}'}, 201));
    }
    return failure(404, 'Operation not found.');
  }
}
