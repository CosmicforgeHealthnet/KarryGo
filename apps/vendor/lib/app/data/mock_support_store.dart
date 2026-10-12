import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';
import '../../features/support/data/support_routes.dart';
import 'mock_dashboard_data.dart';

class MockSupportStore {
  MockSupportStore(this.dashboard);
  final MockDashboardData dashboard;
  final Map<String, List<JsonObject>> records = {};
  final Map<String, ({String fingerprint, JsonObject value})> commands = {};
  int sequence = 0;
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/commerce/orders/([^/]+)/support-cases$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final id = match[1]!;
    final order = dashboard.response('/api/v1/vendor/commerce/orders/$id', {});
    JsonObject failure(int code, String message) => {
      '_status': code,
      '_message': message,
    };
    if (order == null) return failure(404, 'Order not found.');
    final cases = records.putIfAbsent(id, () => []);
    if (request.method == 'GET') {
      return {
        '_status': 200,
        '_data': {'cases': cases},
      };
    }
    if (request.method != 'POST') return failure(405, 'Method not allowed.');
    final key =
        request.headers.entries
            .where((e) => e.key.toLowerCase() == 'idempotency-key')
            .firstOrNull
            ?.value
            .trim() ??
        '';
    if (key.isEmpty || key.length > 160) {
      return failure(400, 'A valid retry key is required.');
    }
    final category = body['category'];
    final categories =
        (supportOperations
                    .firstWhere((op) => op.method == 'POST')
                    .requestSchema!['properties']
                as Map)['category']['enum']
            as List;
    final description = (body['description'] as String? ?? '')
        .trim()
        .replaceAll(RegExp(r'\b[0-9]{4,8}\b'), '[REDACTED]');
    if (!categories.contains(category) ||
        description.isEmpty ||
        utf8.encode(description).length > 1000) {
      return failure(409, 'Choose a valid issue and enter its description.');
    }
    final refs = <String>[];
    for (final raw in body['line_refs'] as List? ?? []) {
      final ref = raw.toString().trim();
      if (ref.isNotEmpty &&
          ref.length <= 300 &&
          refs.length < 20 &&
          !refs.contains(ref)) {
        refs.add(ref);
      }
    }
    final lines = (order['lines'] as List? ?? [])
        .map((line) => (line as Map)['id'])
        .toSet();
    if (!refs.every(lines.contains)) {
      return failure(404, 'An item does not belong to this order.');
    }
    final fingerprint = jsonEncode({
      'category': category,
      'description': description,
      'line_refs': refs,
    });
    final commandKey = '${order['business_id']}|$key';
    final previous = commands[commandKey];
    if (previous != null) {
      if (previous.fingerprint != '$id|$fingerprint') {
        return failure(409, 'This retry key was used for a different report.');
      }
      return {'_status': 200, '_data': previous.value};
    }
    final now = DateTime.now().toUtc().toIso8601String();
    final record = <String, dynamic>{
      'link_ref': 'demo-support-link-${++sequence}',
      'operation_ref': key,
      'case_ref': 'demo-support-case-$sequence',
      'category': category,
      'commerce_order_ref': order['parent_order_id'],
      'vendor_order_ref': id,
      'vendor_ref': order['business_id'],
      'status': 'open',
      'resolution_version': 0,
      'created_at': now,
      'updated_at': now,
    };
    cases.insert(0, record);
    commands[commandKey] = (fingerprint: '$id|$fingerprint', value: record);
    return {'_status': 201, '_data': record};
  }
}
