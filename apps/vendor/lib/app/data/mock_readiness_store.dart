import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';
import '../../features/readiness/state/order_readiness_controller.dart';
import 'mock_dashboard_data.dart';

class MockReadinessStore {
  MockReadinessStore(this.dashboard);
  final MockDashboardData dashboard;
  final Map<String, JsonObject> records = {};
  final Map<String, List<JsonObject>> history = {};
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/commerce/orders/([^/]+)/readiness(?:/([^/]+))?$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final id = match[1]!;
    final action = match[2];
    final order = dashboard.response('/api/v1/vendor/commerce/orders/$id', {});
    JsonObject fail(int code, String name, String message) => {
      '_status': code,
      '_code': name,
      '_message': message,
    };
    if (order == null) {
      return fail(
        404,
        'READINESS_NOT_FOUND',
        'No readiness record is available.',
      );
    }
    var record = records[id];
    JsonObject ok(Object data) => {'_status': 200, '_data': data};
    if (request.method == 'GET') {
      if (record == null) {
        return fail(
          404,
          'READINESS_NOT_FOUND',
          'No readiness record is available.',
        );
      }
      return action == 'history'
          ? ok({'history': history[id] ?? []})
          : action == null
          ? ok(JsonObject.from(record))
          : fail(404, 'NOT_FOUND', 'Route unavailable.');
    }
    if (request.method != 'POST') {
      return fail(405, 'METHOD_NOT_ALLOWED', 'Method not allowed.');
    }
    final now = DateTime.now().toUtc().toIso8601String();
    final starting = action == 'start';
    if (starting) {
      final types =
          (dashboard.store.branches
                          .where((b) => b['id'] == order['branch_id'])
                          .firstOrNull?['capabilities']
                      as List? ??
                  [])
              .map(jsonObject)
              .where((v) => v['status'] == 'ACTIVE')
              .map((v) => v['capability_type']);
      if (body['parent_order_id'] != order['parent_order_id'] ||
          body['branch_id'] != order['branch_id'] ||
          !types.contains(body['vendor_type'])) {
        return fail(
          400,
          'READINESS_INVALID',
          'Choose an active business type for this branch.',
        );
      }
      record ??= {
        'vendor_order_id': id,
        'parent_order_id': order['parent_order_id'],
        'business_id': order['business_id'],
        'branch_id': order['branch_id'],
        'vendor_type': body['vendor_type'],
        'state': 'NOT_STARTED',
        'readiness_version': 1,
        'confidence_score': 0.35,
        'confidence_band': 'LOW',
        'source': 'VENDOR_STAFF',
        'updated_at': now,
      };
    } else {
      if (record == null) {
        return fail(
          404,
          'READINESS_NOT_FOUND',
          'No readiness record is available.',
        );
      }
      final version = request.headers.entries
          .where((e) => e.key.toLowerCase() == 'if-match')
          .firstOrNull
          ?.value
          .replaceAll('"', '');
      if (int.tryParse(version ?? '') != record['readiness_version']) {
        return fail(
          409,
          'READINESS_VERSION_CONFLICT',
          'Preparation changed. Refresh before continuing.',
        );
      }
    }
    final target = starting ? 'PREPARING' : readinessTargets[action];
    if (target == null ||
        (!starting && !readinessAllows(record['state'] as String, target)) ||
        starting &&
            ![
              'NOT_STARTED',
              'ACCEPTED',
              'PREPARING',
              'BLOCKED',
              'DELAYED',
            ].contains(record['state'])) {
      return fail(
        409,
        'READINESS_TRANSITION_REFUSED',
        'This preparation update is not available.',
      );
    }
    final reason = body['reason_code']?.toString().trim() ?? '';
    if (['BLOCKED', 'DELAYED'].contains(target) && reason.isEmpty) {
      return fail(400, 'READINESS_INVALID', 'A reason is required.');
    }
    final substate = body['substate']?.toString();
    if (substate != null &&
        substate.isNotEmpty &&
        (target == 'READY' ||
            !(readinessSubstates[record['vendor_type']] ?? []).contains(
              substate,
            ))) {
      return fail(
        400,
        'READINESS_INVALID',
        'Choose a valid preparation activity.',
      );
    }
    final estimate = body['estimated_ready_at'];
    if (estimate != null && DateTime.tryParse(estimate.toString()) == null) {
      return fail(400, 'READINESS_INVALID', 'Choose a valid ready time.');
    }
    void move(String state) {
      final previous = record!['state'];
      final version = (record['readiness_version'] as int) + 1;
      history.putIfAbsent(id, () => []).insert(0, {
        'id': 'demo-readiness-$id-$version',
        'vendor_order_id': id,
        'from_state': previous,
        'to_state': state,
        'source': 'VENDOR_STAFF',
        'readiness_version': version,
        'created_at': now,
        if (reason.isNotEmpty) 'reason_code': reason,
        if (substate != null && substate.isNotEmpty) 'to_substate': substate,
      });
      record['state'] = state;
      record['readiness_version'] = version;
      record['updated_at'] = now;
      record['last_signal_at'] = now;
    }

    if (starting && record['state'] == 'NOT_STARTED') move('ACCEPTED');
    move(target);
    record.remove('substate');
    if (substate != null && substate.isNotEmpty) record['substate'] = substate;
    if (estimate != null) {
      final start =
          DateTime.tryParse(record['prep_started_at']?.toString() ?? '') ??
          DateTime.parse(now);
      final minimum = start.add(const Duration(minutes: 1));
      final maximum = start.add(const Duration(hours: 8));
      final requested = DateTime.parse(estimate.toString());
      record['vendor_estimated_ready_at'] =
          (requested.isBefore(minimum)
                  ? minimum
                  : requested.isAfter(maximum)
                  ? maximum
                  : requested)
              .toUtc()
              .toIso8601String();
    }
    if (!['BLOCKED', 'DELAYED'].contains(target)) {
      record.remove('block_reason');
      record.remove('delay_reason');
    }
    if (target == 'PREPARING') record.putIfAbsent('prep_started_at', () => now);
    if (target == 'READY') {
      record['actual_ready_at'] = now;
      record.putIfAbsent('first_actual_ready_at', () => now);
    }
    if (target == 'BLOCKED') record['block_reason'] = reason;
    if (target == 'DELAYED') record['delay_reason'] = reason;
    records[id] = record;
    return ok(JsonObject.from(record));
  }
}
