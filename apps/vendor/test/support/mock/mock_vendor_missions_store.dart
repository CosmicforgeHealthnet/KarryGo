import 'mock_delivery_policy_store.dart';
import 'mock_orchestration_store.dart';
import 'package:vendor/features/delivery/state/hybrid_fallback_controller.dart';
import 'mock_fulfilment_store.dart';
import 'dart:convert';
import 'mock_settings_store.dart';
import 'package:vendor/features/delivery/state/rider_workspace_controller.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/delivery/state/vendor_missions_controller.dart';
import 'mock_dashboard_data.dart';

class MockVendorMissionsStore {
  MockVendorMissionsStore(this.dashboard, this.settings);
  final MockSettingsStore settings;
  late final fulfilment = MockFulfilmentStore(this);
  late final policies = MockDeliveryPolicyStore(dashboard.store);
  late final orchestration = MockOrchestrationStore(this);
  final fallbackRecords = <String, JsonObject>{};
  final Map<String, JsonObject> availability = {};
  final Map<String, JsonObject> runtimes = {};
  final Map<String, ({String fingerprint, JsonObject response})> commands = {};
  final MockDashboardData dashboard;
  List<JsonObject>? _missions;
  List<JsonObject> get missions =>
      _missions ??= dashboard.scenario == MockDashboardScenario.empty
      ? []
      : [
          for (var i = 0; i < 4; i++)
            {
              'vendor_mission_id': 'demo-vendor-mission-${i + 1}',
              'fulfilment_group_id': 'demo-fulfilment-group-${i + 1}',
              'plan_id': 'demo-plan-${i + 1}',
              'plan_mission_id': 'demo-plan-mission-${i + 1}',
              'business_id': dashboard.store.business['id'],
              'branch_id': dashboard.store.branches.first['id'],
              if (i < 3 &&
                  !(i == 0 &&
                      dashboard.scenario == MockDashboardScenario.riderSetup))
                'assigned_staff_id': i == 0
                    ? 'mock-staff-1'
                    : 'mock-staff-rider',
              'status': [
                'ASSIGNED',
                'READY_FOR_PICKUP',
                'OUT_FOR_DELIVERY',
                'PLANNED',
              ][i],
              'version': 1,
              'created_at': DateTime.now()
                  .subtract(Duration(minutes: 20 + i * 10))
                  .toUtc()
                  .toIso8601String(),
              'updated_at': DateTime.now().toUtc().toIso8601String(),
              'stops': [
                {
                  'sequence': 1,
                  'stop_type': 'PICKUP',
                  'address':
                      dashboard.store.branches.first['address']?.toString() ??
                      'Vendor branch',
                },
                {
                  'sequence': 2,
                  'stop_type': 'DROPOFF',
                  'address': '14 Admiralty Way, Lekki, Lagos',
                },
              ],
              'parcel_ids': ['demo-delivery-parcel-${i + 1}'],
            },
        ];
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    if (request.url.path.contains('/orchestration')) {
      final result = orchestration.handle(request, body);
      if (result != null) return result;
    }
    final handover = handoverHandle(request, body);
    if (handover != null) return handover;
    final policy = policies.handle(request, body);
    if (policy != null) return policy;
    final fallback = fallbackHandle(request, body);
    if (fallback != null) return fallback;
    if (request.url.path.startsWith('/api/v1/vendor/delivery/rider/')) {
      return rider(request, body);
    }
    final fulfilmentResult = fulfilment.handle(request, body);
    if (fulfilmentResult != null) return fulfilmentResult;
    final match = RegExp(
      r'^/api/v1/vendor/fulfilment/vendor-missions(?:/([^/]+)(?:/(status|rider))?)?$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    JsonObject fail(int code, String name, String message) => {
      '_status': code,
      '_code': name,
      '_message': message,
    };
    JsonObject ok(Object value) => {'_status': 200, '_data': value};
    if (dashboard.scenario == MockDashboardScenario.unavailable) {
      return fail(
        503,
        'FULFILMENT_UNAVAILABLE',
        'Fulfilment is temporarily unavailable.',
      );
    }
    final id = match[1];
    if (id == null && request.method == 'GET') {
      return ok({
        'vendor_missions': missions.map((m) => JsonObject.from(m)).toList(),
      });
    }
    final value = missions
        .where((m) => m['vendor_mission_id'] == id)
        .firstOrNull;
    if (value == null) {
      return fail(404, 'FULFILMENT_NOT_FOUND', 'Delivery not found.');
    }
    if (request.method == 'GET' && match[2] == null) {
      return ok(JsonObject.from(value));
    }
    if (request.method == 'PUT' && match[2] == 'rider') {
      final key = header(request, 'Idempotency-Key');
      final fingerprint =
          '${request.url.path}|${jsonEncode(body)}|${header(request, 'If-Match')}';
      if (key.isEmpty) {
        return fail(
          422,
          'FULFILMENT_VALIDATION_FAILED',
          'A retry key is required.',
        );
      }
      final prior = commands[key];
      if (prior != null) {
        return prior.fingerprint == fingerprint
            ? prior.response
            : fail(409, 'FULFILMENT_CONFLICT', 'Retry details changed.');
      }
      if (int.tryParse(header(request, 'If-Match')) != value['version']) {
        return fail(
          409,
          'FULFILMENT_VERSION_CONFLICT',
          'This delivery changed. Refresh.',
        );
      }
      final staff = body['staff_id'];
      if (!eligible(staff, value['branch_id'])) {
        return fail(
          403,
          'FULFILMENT_FORBIDDEN',
          'Choose an active Vendor Rider for this branch.',
        );
      }
      final a = availabilityFor(staff.toString());
      if (a['state'] != 'AVAILABLE' ||
          a['branch_id'] != value['branch_id'] ||
          [
            'CUSTODY_TRANSFERRED',
            'OUT_FOR_DELIVERY',
            'RETURN_REQUIRED',
            'DELIVERED',
            'RETURNED',
          ].contains(value['status'])) {
        return fail(
          409,
          'FULFILMENT_CONFLICT',
          'The rider is unavailable or this delivery cannot be reassigned.',
        );
      }
      final old = value['assigned_staff_id'];
      if (old != null && old != staff) {
        final previous = availabilityFor(old.toString());
        previous['state'] = 'AVAILABLE';
        previous.remove('current_vendor_mission_id');
        previous['version'] = (previous['version'] as int) + 1;
      }
      value['assigned_staff_id'] = staff;
      value['status'] = 'ASSIGNED';
      value['version'] = (value['version'] as int) + 1;
      a['state'] = 'BUSY';
      a['current_vendor_mission_id'] = id;
      a['version'] = (a['version'] as int) + 1;
      runtimes[id!] = {'stage': 'ASSIGNED', 'runtime_version': 1};
      final result = ok(JsonObject.from(value));
      commands[key] = (fingerprint: fingerprint, response: result);
      return result;
    }
    if (request.method != 'PUT' || match[2] != 'status') {
      return fail(405, 'METHOD_NOT_ALLOWED', 'Method not allowed.');
    }
    final version = request.headers.entries
        .where((e) => e.key.toLowerCase() == 'if-match')
        .firstOrNull
        ?.value
        .replaceAll('"', '');
    if (int.tryParse(version ?? '') != value['version']) {
      return fail(
        409,
        'FULFILMENT_VERSION_CONFLICT',
        'This delivery changed. Refresh before continuing.',
      );
    }
    if (!(vendorMissionStaffTransitions[value['status']] ?? []).contains(
      body['status'],
    )) {
      return fail(
        409,
        'FULFILMENT_CONFLICT',
        'This update is not available to staff.',
      );
    }
    if (body['status'] == 'FAILED' &&
        body['failure_reason']?.toString().trim().isNotEmpty != true) {
      return fail(
        422,
        'FULFILMENT_VALIDATION_FAILED',
        'A failure reason is required.',
      );
    }
    value['status'] = body['status'];
    value['version'] = (value['version'] as int) + 1;
    value['updated_at'] = DateTime.now().toUtc().toIso8601String();
    if (body['status'] == 'FAILED') {
      value['failure_reason'] = body['failure_reason'];
    }
    return ok(JsonObject.from(value));
  }

  String header(http.BaseRequest request, String name) =>
      request.headers.entries
          .where((e) => e.key.toLowerCase() == name.toLowerCase())
          .firstOrNull
          ?.value ??
      '';
  bool eligible(Object? staff, Object? branch) =>
      settings.staff.any((s) => s['id'] == staff && s['status'] == 'active') &&
      settings.assignments.any(
        (a) =>
            a['staff_id'] == staff &&
            a['role_id'] == 'role_vendor_rider' &&
            a['revoked_at'] == null &&
            (a['branch_id'] == null || a['branch_id'] == branch),
      );
  JsonObject availabilityFor(String staff) => availability.putIfAbsent(
    staff,
    () => {
      'staff_id': staff,
      'business_id': dashboard.store.vendorId,
      'branch_id': dashboard.store.branches.first['id'],
      'state':
          missions.any(
            (m) =>
                m['assigned_staff_id'] == staff &&
                !['DELIVERED', 'RETURNED', 'FAILED'].contains(m['status']),
          )
          ? 'BUSY'
          : 'AVAILABLE',
      'version': 1,
      'updated_at': DateTime.now().toUtc().toIso8601String(),
      if (staff == 'mock-staff-1' && missions.isNotEmpty)
        'current_vendor_mission_id': missions.first['vendor_mission_id'],
    },
  );
  JsonObject runtime(JsonObject mission) {
    final id = mission['vendor_mission_id'] as String;
    final state = runtimes.putIfAbsent(
      id,
      () => {
        'stage': switch (mission['status']) {
          'ASSIGNED' => 'ASSIGNED',
          'READY_FOR_PICKUP' => 'ARRIVED_PICKUP',
          'OUT_FOR_DELIVERY' => 'EN_ROUTE_RECEIVER',
          _ => 'PLANNED',
        },
        'runtime_version': 1,
      },
    );
    return {...state, 'mission': jsonObject(jsonDecode(jsonEncode(mission)))};
  }

  JsonObject rider(http.BaseRequest request, JsonObject body) {
    JsonObject fail(int code, String name, String message) => {
      '_status': code,
      '_code': name,
      '_message': message,
    };
    JsonObject ok(Object value) => {
      '_status': 200,
      '_data': jsonDecode(jsonEncode(value)),
    };
    const staff = 'mock-staff-1';
    final path = request.url.path.replaceFirst(
      '/api/v1/vendor/delivery/rider/',
      '',
    );
    if (path == 'branches' && request.method == 'GET') {
      final q = request.url.queryParameters;
      final limit = int.tryParse(q['limit'] ?? '200'),
          offset = int.tryParse(q['offset'] ?? '0');
      if (limit == null ||
          limit < 1 ||
          limit > 200 ||
          offset == null ||
          offset < 0) {
        return fail(
          422,
          'FULFILMENT_VALIDATION_FAILED',
          'Invalid rider branch page.',
        );
      }
      final rows = [
        for (final branch in dashboard.store.branches)
          if (branch['status'] == 'ACTIVE' && eligible(staff, branch['id']))
            {
              'id': branch['id'],
              'display_name': branch['display_name'] ?? branch['name'],
            },
      ];
      return ok({
        'items': rows.skip(offset).take(limit).toList(),
        'has_more': offset + limit < rows.length,
        'next_offset': offset + limit,
      });
    }
    if (path == 'availability') {
      final existing = availability[staff];
      final assigned = missions.any(
        (m) =>
            m['assigned_staff_id'] == staff &&
            !['DELIVERED', 'RETURNED', 'FAILED'].contains(m['status']),
      );
      final value = existing ?? (assigned ? availabilityFor(staff) : null);
      if (request.method == 'GET') {
        return value == null
            ? fail(
                404,
                'FULFILMENT_NOT_FOUND',
                'Rider availability has not been set.',
              )
            : ok(value);
      }
      if (request.method != 'PUT') {
        return fail(405, 'METHOD_NOT_ALLOWED', 'Method not allowed.');
      }
      final key = header(request, 'Idempotency-Key');
      final fingerprint =
          '$path|${jsonEncode(body)}|${header(request, 'If-Match')}';
      final previous = commands[key];
      if (previous != null) {
        return previous.fingerprint == fingerprint
            ? previous.response
            : fail(409, 'FULFILMENT_CONFLICT', 'Retry details changed.');
      }
      if (key.isEmpty || !['AVAILABLE', 'OFFLINE'].contains(body['state'])) {
        return fail(
          422,
          'FULFILMENT_VALIDATION_FAILED',
          'Choose a rider availability state.',
        );
      }
      if (int.tryParse(header(request, 'If-Match')) !=
          (value?['version'] ?? 0)) {
        return fail(
          409,
          'FULFILMENT_VERSION_CONFLICT',
          'Availability changed. Refresh.',
        );
      }
      if (!eligible(staff, body['branch_id'])) {
        return fail(
          403,
          'FULFILMENT_FORBIDDEN',
          'No rider role at this branch.',
        );
      }
      if (value?['state'] == 'BUSY') {
        return fail(
          409,
          'FULFILMENT_CONFLICT',
          'Finish the assigned delivery before changing availability.',
        );
      }
      final saved =
          value ??
          {
            'staff_id': staff,
            'business_id': dashboard.store.vendorId,
            'branch_id': body['branch_id'],
            'version': 0,
          };
      saved['state'] = body['state'];
      saved['branch_id'] = body['branch_id'];
      saved['version'] = (saved['version'] as int) + 1;
      saved['updated_at'] = DateTime.now().toUtc().toIso8601String();
      availability[staff] = saved;
      final result = ok(saved);
      commands[key] = (fingerprint: fingerprint, response: result);
      return result;
    }
    final owned = missions
        .where((m) => m['assigned_staff_id'] == staff)
        .toList();
    if (path == 'missions' && request.method == 'GET') {
      return ok({'vendor_missions': owned.map(runtime).toList()});
    }
    final parts = path.split('/');
    final id = path == 'missions/current'
        ? availabilityFor(staff)['current_vendor_mission_id']
        : parts.length > 1
        ? parts[1]
        : null;
    final mission = owned
        .where((m) => m['vendor_mission_id'] == id)
        .firstOrNull;
    if (mission == null) {
      return fail(
        404,
        'FULFILMENT_NOT_FOUND',
        'No assigned delivery was found.',
      );
    }
    if (request.method == 'GET') return ok(runtime(mission));
    if (parts.length != 4 ||
        parts[2] != 'actions' ||
        request.method != 'POST') {
      return fail(405, 'METHOD_NOT_ALLOWED', 'Method not allowed.');
    }
    final action = parts[3];
    final key = header(request, 'Idempotency-Key');
    final fingerprint =
        '$path|${jsonEncode(body)}|${header(request, 'If-Match')}';
    final previous = commands[key];
    if (previous != null) {
      return previous.fingerprint == fingerprint
          ? ok(runtime(mission))
          : fail(409, 'FULFILMENT_CONFLICT', 'Retry details changed.');
    }
    if (key.isEmpty) {
      return fail(
        422,
        'FULFILMENT_VALIDATION_FAILED',
        'A retry key is required.',
      );
    }
    if (int.tryParse(header(request, 'If-Match')) != mission['version']) {
      return fail(
        409,
        'FULFILMENT_VERSION_CONFLICT',
        'This delivery changed. Refresh.',
      );
    }
    final current = runtime(mission);
    if (!(riderActions[current['stage']] ?? []).contains(action)) {
      return fail(409, 'FULFILMENT_CONFLICT', 'Follow the delivery sequence.');
    }
    if (['complete-delivery', 'return'].contains(action) &&
        (body['evidence_ref'] !=
                'demo-handover-$id-${action == 'return' ? 'return' : 'delivery'}' ||
            !['OTP', 'QR'].contains(body['method']) ||
            body['credential'] != '654321')) {
      return fail(
        422,
        'FULFILMENT_VALIDATION_FAILED',
        'The handover reference or credential is incorrect.',
      );
    }
    if (action == 'receiver-unavailable' &&
        body['reason_code']?.toString().trim().isNotEmpty != true) {
      return fail(422, 'FULFILMENT_VALIDATION_FAILED', 'Enter a reason.');
    }
    final next = switch (action) {
      'acknowledge' => 'ACKNOWLEDGED',
      'start-pickup' => 'EN_ROUTE_PICKUP',
      'arrive-pickup' => 'ARRIVED_PICKUP',
      'complete-pickup' => 'IN_CUSTODY',
      'start-receiver' => 'EN_ROUTE_RECEIVER',
      'arrive-receiver' => 'ARRIVED_RECEIVER',
      'complete-delivery' => 'COMPLETED',
      'receiver-unavailable' => 'RECEIVER_UNAVAILABLE',
      _ => 'RETURNED',
    };
    final status = switch (action) {
      'arrive-pickup' => 'READY_FOR_PICKUP',
      'complete-pickup' => 'CUSTODY_TRANSFERRED',
      'start-receiver' =>
        current['stage'] == 'IN_CUSTODY'
            ? 'OUT_FOR_DELIVERY'
            : mission['status'],
      'complete-delivery' => 'DELIVERED',
      'receiver-unavailable' => 'RETURN_REQUIRED',
      'return' => 'RETURNED',
      _ => mission['status'],
    };
    fulfilment.riderCustody(mission, action, key);
    mission['status'] = status;
    mission['version'] = (mission['version'] as int) + 1;
    runtimes[id as String] = {
      'stage': next,
      'runtime_version': (current['runtime_version'] as int) + 1,
      if (action == 'receiver-unavailable')
        'receiver_unavailable_reason': body['reason_code'],
    };
    if (['COMPLETED', 'RETURNED'].contains(next)) {
      final a = availabilityFor(staff);
      a['state'] = 'AVAILABLE';
      a.remove('current_vendor_mission_id');
      a['version'] = (a['version'] as int) + 1;
    }
    final result = ok(runtime(mission));
    commands[key] = (fingerprint: fingerprint, response: result);
    return result;
  }

  JsonObject? fallbackHandle(http.BaseRequest request, JsonObject body) {
    JsonObject fail(int code, String message) => {
      '_status': code,
      '_code': code == 404 ? 'FULFILMENT_NOT_FOUND' : 'FULFILMENT_CONFLICT',
      '_message': message,
    };
    JsonObject ok(JsonObject v) => {
      '_status': 200,
      '_data': {...v},
    };
    final read = RegExp(
      r'^/api/v1/vendor/fulfilment/vendor-missions/fallbacks/([^/]+)$',
    ).firstMatch(request.url.path);
    if (read != null) {
      final value = fallbackRecords[read[1]];
      return value == null ? fail(404, 'Fallback not found.') : ok(value);
    }
    final match = RegExp(
      r'^/api/v1/vendor/fulfilment/vendor-missions/([^/]+)/fallbacks$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final id = match[1]!;
    final key = header(request, 'Idempotency-Key');
    final version = int.tryParse(header(request, 'If-Match'));
    final fingerprint = '$id|$version|${jsonEncode(body)}';
    final previous = commands[key];
    if (previous != null) {
      return previous.fingerprint == fingerprint
          ? previous.response
          : fail(409, 'Retry details changed.');
    }
    final mission = missions
        .where((m) => m['vendor_mission_id'] == id)
        .firstOrNull;
    if (mission == null) return fail(404, 'Mission not found.');
    if (request.method != 'POST' ||
        key.isEmpty ||
        !fallbackReasons.contains(body['reason_code'])) {
      return fail(422, 'Choose a supported fallback reason.');
    }
    if (version != mission['version'] ||
        [
          'CUSTODY_TRANSFERRED',
          'OUT_FOR_DELIVERY',
          'DELIVERED',
          'RETURN_REQUIRED',
          'RETURNED',
        ].contains(mission['status'])) {
      return fail(409, 'This mission changed or is already in custody.');
    }
    final now = DateTime.now().toUtc().toIso8601String();
    final group = fulfilment.groups[mission['fulfilment_group_id']];
    final value = <String, dynamic>{
      'ID': 'demo-fallback-$key',
      'OperationRef': key,
      'BusinessID': mission['business_id'],
      'BranchID': mission['branch_id'],
      'VendorMissionID': id,
      'GroupID': mission['fulfilment_group_id'],
      'SourcePlanID': mission['plan_id'],
      'SourcePlanVersion': 1,
      'SourcePlanMissionID': mission['plan_mission_id'],
      'TargetPlanID': 'demo-platform-plan-$key',
      'TargetPlanVersion': 2,
      'TargetPlanMissionID': 'demo-platform-plan-mission-$key',
      'PlatformMissionRefID': 'demo-platform-mission-$key',
      'ReasonCode': body['reason_code'],
      'State': 'COMPLETED',
      'FailureReason': '',
      'Version': 4,
      'CreatedAt': now,
      'UpdatedAt': now,
      'CompletedAt': now,
    };
    fallbackRecords[key] = value;
    if (group != null) {
      group['current_plan_id'] = value['TargetPlanID'];
      group['current_plan_version'] = 2;
    }
    mission['status'] = 'FAILED';
    mission['failure_reason'] = 'HYBRID_FALLBACK:${body['reason_code']}';
    mission['version'] = (mission['version'] as int) + 1;
    runtimes[id] = {'stage': 'FALLBACK_COMPLETED', 'runtime_version': 2};
    if (mission['assigned_staff_id'] != null) {
      final a = availabilityFor(mission['assigned_staff_id'] as String);
      if (a['current_vendor_mission_id'] == id) {
        a['state'] = 'AVAILABLE';
        a.remove('current_vendor_mission_id');
        a['version'] = (a['version'] as int) + 1;
      }
    }
    final response = ok(value);
    commands[key] = (fingerprint: fingerprint, response: response);
    return response;
  }

  JsonObject? handoverHandle(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/commerce/orders/([^/]+)/missions/(demo-vendor-mission-[0-9]+)/handover/(PICKUP|pickup/confirm)$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final order = match[1]!, mission = match[2]!;
    if (!fulfilment.parcels.values.any(
      (p) =>
          p['vendor_order_id'] == order &&
          missions.any(
            (m) =>
                m['vendor_mission_id'] == mission &&
                (m['parcel_ids'] as List).contains(p['parcel_id']),
          ),
    )) {
      return {
        '_status': 404,
        '_code': 'NOT_FOUND',
        '_message': 'Handover not found.',
      };
    }
    final reference = 'demo-vendor-pickup-$mission';
    if (request.method == 'POST' &&
        (body['handover_ref'] != reference ||
            !['OTP', 'QR'].contains(body['method']) ||
            body['credential'] != '654321')) {
      return {
        '_status': 409,
        '_code': 'HANDOVER_REJECTED',
        '_message': 'The pickup credential is incorrect.',
      };
    }
    return {
      '_status': 200,
      '_data': {
        'authority': 'VENDOR_OWN_DELIVERY',
        'mission_ref': mission,
        'fulfilment_ref': mission,
        'handover_ref': reference,
        'stage': 'PICKUP',
        'status': 'CONSUMED',
        'source_version': 1,
      },
    };
  }
}
