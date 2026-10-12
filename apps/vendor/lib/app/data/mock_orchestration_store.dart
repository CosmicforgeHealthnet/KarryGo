import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import '../../features/orchestration/state/orchestration_workspace_controller.dart';
import 'mock_vendor_missions_store.dart';

class MockOrchestrationStore {
  MockOrchestrationStore(this.delivery) {
    final now = DateTime.now().toUtc();
    for (final entry in delivery.fulfilment.groups.entries) {
      final index = records.length;
      final id = 'mock-orchestration-${index + 1}';
      final unit = delivery.fulfilment.units.values.firstWhere(
        (u) => u['fulfilment_group_id'] == entry.key,
      );
      final waiting = index == 3;
      records[entry.key] = {
        'id': id,
        'fulfilment_group_ref': entry.key,
        'parent_order_id': entry.value['parent_order_id'],
        'plan_version': 1,
        'state': waiting ? 'WAITING_READINESS' : 'COMMITTED',
        'selected_authority': 'DISPATCH',
        'selected_product_code': '',
        'selection_source': 'AUTOMATIC',
        'desired_pickup_start': now
            .add(const Duration(minutes: 20))
            .toIso8601String(),
        'desired_pickup_end': now
            .add(const Duration(minutes: 35))
            .toIso8601String(),
        'commit_not_before': now
            .add(const Duration(minutes: 12))
            .toIso8601String(),
        'recovery_state': 'NONE',
        'last_reason_code': waiting ? 'WAITING_READINESS' : 'MISSION_COMMITTED',
        'current_mission_ref': waiting
            ? ''
            : delivery.missions[index]['vendor_mission_id'],
        'requirements_snapshot': {
          'weight_grams': 1500,
          'package_count': 1,
          'pickup_count': 1,
          'dropoff_count': 1,
          'secure_pickup': true,
          'secure_delivery': true,
        },
        'version': 1,
        'created_at': now.toIso8601String(),
        'updated_at': now.toIso8601String(),
      };
      pickups[entry.key] = [
        {
          'vendor_order_id': unit['vendor_order_id'],
          'business_id': unit['business_id'],
          'branch_id': unit['branch_id'],
          'sequence': 1,
          'package_count': 1,
          'weight_grams': 1500,
        },
      ];
      decisions[entry.key] = [
        {
          'id': 'mock-decision-$id',
          'orchestration_id': id,
          'plan_version': 1,
          'event_type': 'AUTHORITY_SELECTED',
          'candidate_authority': 'DISPATCH',
          'candidate_product_code': '',
          'selected': true,
          'serviceable': true,
          'reason_code': 'ELIGIBLE_AUTHORITY',
          'created_at': now.toIso8601String(),
        },
      ];
    }
  }
  final MockVendorMissionsStore delivery;
  final records = <String, JsonObject>{};
  final pickups = <String, List<JsonObject>>{},
      decisions = <String, List<JsonObject>>{};
  int sequence = 0;
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/fulfilment/groups/([^/]+)/orchestration(?:/(decisions|authority-override))?$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final group = match[1]!;
    JsonObject fail(int status, String code, String message) => {
      '_status': status,
      '_code': code,
      '_message': message,
    };
    final current = records[group];
    if (current == null) {
      return fail(
        404,
        'ORCHESTRATION_NOT_FOUND',
        'No orchestration for this fulfilment group.',
      );
    }
    if (request.method == 'GET' && match[2] == null) {
      return {
        '_status': 200,
        '_data': {'orchestration': current, 'pickups': pickups[group]},
      };
    }
    if (request.method == 'GET' && match[2] == 'decisions') {
      return {
        '_status': 200,
        '_data': {'decisions': decisions[group]},
      };
    }
    if (request.method != 'POST' || match[2] != 'authority-override') {
      return fail(405, 'METHOD_NOT_ALLOWED', 'Method not allowed.');
    }
    if ([
          'COMMITTING',
          'COMMITTED',
          'IN_EXECUTION',
          'COMPLETED',
          'RECOVERING',
          'CANCELLED',
          'FAILED',
        ].contains(current['state']) ||
        current['current_mission_ref'] != '') {
      return fail(
        409,
        'ORCHESTRATION_COMMITTED',
        'An execution authority already owns this work.',
      );
    }
    if (!executionAuthorities.contains(body['authority']) ||
        body['reason'] is! String ||
        (body['reason'] as String).trim().isEmpty) {
      return fail(
        400,
        'ORCHESTRATION_INVALID',
        'Choose an execution authority and provide a reason.',
      );
    }
    final now = DateTime.now().toUtc().toIso8601String();
    decisions[group]!.insert(0, {
      'id': 'mock-override-${++sequence}',
      'orchestration_id': current['id'],
      'plan_version': current['plan_version'],
      'event_type': 'OVERRIDDEN',
      'candidate_authority': body['authority'],
      'candidate_product_code': body['product_code'] ?? '',
      'selected': true,
      'actor_ref': 'mock-staff-1',
      'override_reason': body['reason'],
      'reason_code': 'OVERRIDDEN_FROM_${current['selected_authority']}',
      'created_at': now,
    });
    current.addAll({
      'selected_authority': body['authority'],
      'selected_product_code': body['product_code'] ?? '',
      'selection_source': 'ADMIN_OVERRIDE',
      'last_reason_code': 'AUTHORITY_OVERRIDDEN',
      'version': (current['version'] as int) + 1,
      'updated_at': now,
    });
    return {'_status': 200, '_data': current};
  }
}
