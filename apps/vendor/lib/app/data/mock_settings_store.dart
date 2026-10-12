import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';
import 'mock_dashboard_data.dart';
import 'mock_settings_permissions.dart';

class MockSettingsStore {
  MockSettingsStore(this.business, this.scenario) {
    roles.addAll(mockSettingsRoles.map(_copy));
    for (final role in roles) {
      role['permission_codes'] = role['id'] == 'role_owner'
          ? mockSettingsPermissions.map((p) => p['code']).toList()
          : role['id'] == 'role_branch_manager'
          ? [
              'vendor.orders.read',
              'vendor.orders.manage',
              'vendor.branches.read',
              'vendor.staff.read',
            ]
          : ['vendor.orders.read'];
    }
    if (scenario != MockDashboardScenario.empty) {
      staff.addAll([
        {
          'id': 'mock-staff-1',
          'vendor_id': business.vendorId,
          'status': 'active',
          'version': 1,
          'created_at': '2026-10-09T08:00:00Z',
        },
        {
          'id': 'mock-staff-2',
          'vendor_id': business.vendorId,
          'status': 'active',
          'version': 1,
          'created_at': '2026-10-09T08:10:00Z',
        },
      ]);
      staff.add({
        'id': 'mock-staff-free-rider',
        'vendor_id': business.vendorId,
        'status': 'active',
        'version': 1,
        'created_at': '2026-10-09T08:00:00Z',
      });
      assignments.add({
        'id': 'mock-free-rider-role',
        'staff_id': 'mock-staff-free-rider',
        'role_id': 'role_vendor_rider',
        'branch_id': null,
        'assigned_by': 'mock-staff-1',
        'created_at': '2026-10-09T08:00:00Z',
      });
      assignments.addAll([
        {
          'id': 'mock-assignment-owner',
          'staff_id': 'mock-staff-1',
          'role_id': 'role_owner',
          'created_at': '2026-10-09T08:00:00Z',
        },
        {
          'id': 'mock-assignment-manager',
          'staff_id': 'mock-staff-2',
          'role_id': 'role_branch_manager',
          'branch_id': 'mock-branch',
          'assigned_by': 'mock-staff-1',
          'created_at': '2026-10-09T08:10:00Z',
        },
      ]);
      staff.add({
        'id': 'mock-staff-rider',
        'vendor_id': business.vendorId,
        'status': 'active',
        'version': 1,
        'created_at': '2026-10-09T08:00:00Z',
      });
      assignments.addAll([
        {
          'id': 'mock-role-self-rider',
          'staff_id': 'mock-staff-1',
          'role_id': 'role_vendor_rider',
          'branch_id': null,
          'assigned_by': 'mock-staff-1',
          'created_at': '2026-10-09T08:00:00Z',
        },
        {
          'id': 'mock-role-rider',
          'staff_id': 'mock-staff-rider',
          'role_id': 'role_vendor_rider',
          'branch_id': null,
          'assigned_by': 'mock-staff-1',
          'created_at': '2026-10-09T08:00:00Z',
        },
      ]);
    }
  }
  final MockOnboardingStore business;
  final MockDashboardScenario scenario;
  final List<JsonObject> staff = [], roles = [], assignments = [];
  final Map<String, ({String fingerprint, JsonObject response})> _replays = {};
  final Set<String> _contacts = {};
  int _sequence = 0;
  static JsonObject _copy(JsonObject value) =>
      jsonObject(jsonDecode(jsonEncode(value)));
  JsonObject _ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject _error(int status, String message) => {
    '_status': status,
    '_message': message,
  };
  JsonObject _page(List<JsonObject> values, Map<String, String> query) {
    final limit = (int.tryParse(query['limit'] ?? '') ?? 50).clamp(1, 100),
        offset = (int.tryParse(query['offset'] ?? '') ?? 0).clamp(0, 1000000);
    return _ok({
      'items': values.skip(offset).take(limit).toList(),
      'pagination': {
        'limit': limit,
        'offset': offset,
        'has_more': values.length > offset + limit,
      },
    });
  }

  JsonObject? handle(http.BaseRequest req, JsonObject body) {
    final path = req.url.path,
        method = req.method,
        segments = path.split('/').skip(4).toList();
    final iam =
        segments.isNotEmpty &&
        {
          'staff',
          'roles',
          'permissions',
          'assignments',
        }.contains(segments.first);
    final vendor = path.startsWith('/api/v1/vendor/businesses/');
    if (!iam && !vendor) return null;
    if (vendor &&
        segments.length > 2 &&
        !{
          'brands',
          'branches',
          'capabilities',
          'pause',
          'resume',
        }.contains(segments[2])) {
      return null;
    }
    if (business.scenario == MockOnboardingScenario.permissionDenied) {
      return _error(
        403,
        'You do not have permission to manage business settings.',
      );
    }
    if (business.scenario == MockOnboardingScenario.unavailable) {
      return _error(503, 'Business settings are temporarily unavailable.');
    }
    if (vendor && segments[1] != business.vendorId) {
      return _error(404, 'Business not found.');
    }
    if (method == 'GET' && path == '/api/v1/vendor/staff/me/access') {
      if (scenario == MockDashboardScenario.settingsUnavailable) {
        return _error(503, 'Access details are temporarily unavailable.');
      }
      final grants = <JsonObject>[];
      final seen = <String>{};
      for (final assignment in assignments.where(
        (a) => a['staff_id'] == 'mock-staff-1' && a['revoked_at'] == null,
      )) {
        final role = roles
            .where((r) => r['id'] == assignment['role_id'])
            .firstOrNull;
        for (final code in (role?['permission_codes'] as List? ?? const [])) {
          if (!seen.add('$code:${assignment['branch_id']}')) continue;
          final permission = mockSettingsPermissions
              .where((p) => p['code'] == code)
              .firstOrNull;
          grants.add({
            'permission_code': code,
            'branch_id': assignment['branch_id'],
            'is_high_risk': permission?['is_high_risk'] == true,
            'description': permission?['description'] ?? '',
            'category': permission?['category'] ?? '',
          });
        }
      }
      return _ok({
        'staff_id': 'mock-staff-1',
        'vendor_id': business.vendorId,
        'grants': grants,
      });
    }
    if (scenario == MockDashboardScenario.settingsRestricted &&
        (iam || method != 'GET')) {
      return _error(403, 'You do not have access to this action.');
    }
    if (scenario == MockDashboardScenario.settingsUnavailable &&
        (iam || segments.length > 2 || method != 'GET')) {
      return _error(503, 'Settings are temporarily unavailable.');
    }
    if (iam) return _iam(method, segments, body);
    if (method == 'GET') {
      return _businessRead(segments.skip(2).toList(), req.url.queryParameters);
    }
    final key = req.headers.entries
        .where((e) => e.key.toLowerCase() == 'idempotency-key')
        .firstOrNull
        ?.value;
    if (key == null || key.isEmpty) {
      return _error(400, 'Idempotency-Key is required.');
    }
    final fingerprint = jsonEncode([method, path, body]),
        cached = _replays[key];
    if (cached != null) {
      return cached.fingerprint == fingerprint
          ? _copy(cached.response)
          : _error(409, 'This idempotency key has different details.');
    }
    final result = _businessWrite(method, segments.skip(2).toList(), body);
    if (result['_message'] == null) {
      _replays[key] = (fingerprint: fingerprint, response: _copy(result));
    }
    return result;
  }

  JsonObject? _businessRead(List<String> p, Map<String, String> q) {
    if (p.isEmpty) return _ok(business.business);
    if (p.first == 'branches') {
      if (p.length == 1) return _page(business.branches, q);
      final branch = business.branches
          .where((b) => b['id'] == p[1])
          .firstOrNull;
      if (branch == null) return _error(404, 'Branch not found.');
      if (p.length == 2) return _ok(branch);
      if (p[2] == 'hours') return _ok(branch['hours'] ?? []);
      if (p[2] == 'service-area' || p[2] == 'operations-settings') {
        final value =
            branch[p[2] == 'service-area'
                ? 'service_area'
                : 'operations_settings'];
        return value == null
            ? _error(404, 'Branch settings not found.')
            : _ok(value);
      }
      if (p[2] == 'capabilities') return _ok(_branchCaps(branch));
    }
    if (p.first == 'brands' || p.first == 'capabilities') {
      final values = p.first == 'brands'
          ? business.brands
          : business.capabilities;
      if (p.length == 1) return _page(values, q);
      final value = values.where((b) => b['id'] == p[1]).firstOrNull;
      return value == null ? _error(404, 'Record not found.') : _ok(value);
    }
    return null;
  }

  bool _version(JsonObject value, JsonObject body) =>
      (value['version'] ?? 0) == body['expected_version'];
  List<JsonObject> _branchCaps(JsonObject branch) {
    final values = ((branch['capabilities'] ?? []) as List)
        .map(jsonObject)
        .toList();
    for (final value in values) {
      final capId = value['vendor_capability_id'] ?? value['capability_id'];
      value.remove('capability_id');
      value.addAll({
        'id': value['id'] ?? 'mock-link-${branch['id']}-$capId',
        'business_id': business.vendorId,
        'branch_id': branch['id'],
        'vendor_capability_id': capId,
      });
      final cap = business.capabilities
          .where((c) => c['id'] == capId)
          .firstOrNull;
      final reason = business.business['status'] != 'ACTIVE'
          ? 'BUSINESS_NOT_ACTIVE'
          : cap?['status'] != 'ACTIVE'
          ? 'BUSINESS_CAPABILITY_NOT_ACTIVE'
          : branch['status'] != 'ACTIVE'
          ? 'BRANCH_NOT_ACTIVE'
          : value['status'] != 'ACTIVE'
          ? 'BRANCH_CAPABILITY_NOT_ACTIVE'
          : 'ACTIVE';
      value['effective'] = reason == 'ACTIVE';
      value['effective_reason'] = reason;
    }
    branch['capabilities'] = values;
    return values;
  }

  JsonObject _status(JsonObject value, JsonObject body, String command) {
    if (!_version(value, body)) {
      return _error(409, 'This record changed. Refresh before saving.');
    }
    final from = command == 'pause' ? 'ACTIVE' : 'PAUSED';
    if (value['status'] != from) {
      return _error(
        409,
        'Only ACTIVE records can be paused and only PAUSED records can be resumed.',
      );
    }
    value['status'] = command == 'pause' ? 'PAUSED' : 'ACTIVE';
    value['version'] = (value['version'] as int) + 1;
    return _ok(value);
  }

  JsonObject _businessWrite(String method, List<String> p, JsonObject body) {
    if (p.isEmpty) {
      if (!_version(business.business, body)) {
        return _error(409, 'Business details changed. Refresh before saving.');
      }
      final values = Map<String, dynamic>.of(body)..remove('expected_version');
      business.business.addAll(values);
      business.business['version'] = (business.business['version'] as int) + 1;
      return _ok(business.business);
    }
    if (p.length == 1 && {'pause', 'resume'}.contains(p.first)) {
      return _status(business.business, body, p.first);
    }
    if (p.first == 'branches' && p.length >= 3) {
      final branch = business.branches
          .where((b) => b['id'] == p[1])
          .firstOrNull;
      if (branch == null) return _error(404, 'Branch not found.');
      if ({'pause', 'resume'}.contains(p[2])) {
        return _status(branch, body, p[2]);
      }
      if (p[2] == 'hours') {
        if (!_version(branch, body)) {
          return _error(409, 'Branch hours changed. Refresh before saving.');
        }
        final windows = (body['hours'] as List).map(jsonObject).toList();
        for (final window in windows) {
          if (window['weekday'] is! int ||
              (window['weekday'] as int) < 1 ||
              (window['weekday'] as int) > 7 ||
              window['is_closed'] != true &&
                  (window['opens_at'].toString().compareTo(
                        window['closes_at'].toString(),
                      ) >=
                      0)) {
            return _error(
              422,
              'Operating hours must use valid, non-overnight intervals.',
            );
          }
        }
        for (final day in windows.map((w) => w['weekday']).toSet()) {
          final same = windows.where((w) => w['weekday'] == day).toList();
          if (same.any((w) => w['is_closed'] == true) && same.length > 1) {
            return _error(422, 'A closed day cannot have other intervals.');
          }
          final open = same.where((w) => w['is_closed'] != true).toList()
            ..sort(
              (a, b) =>
                  a['opens_at'].toString().compareTo(b['opens_at'].toString()),
            );
          for (var i = 0; i < open.length; i++) {
            if (!RegExp(
                  r'^(?:[01][0-9]|2[0-3]):[0-5][0-9]$',
                ).hasMatch(open[i]['opens_at'].toString()) ||
                !RegExp(
                  r'^(?:[01][0-9]|2[0-3]):[0-5][0-9]$',
                ).hasMatch(open[i]['closes_at'].toString())) {
              return _error(422, 'Use a valid time.');
            }
            if (i > 0 &&
                open[i]['opens_at'].toString().compareTo(
                      open[i - 1]['closes_at'].toString(),
                    ) <
                    0) {
              return _error(422, 'Operating intervals cannot overlap.');
            }
          }
        }
        branch['hours'] = windows;
        branch['version'] = (branch['version'] as int) + 1;
        return _ok(windows);
      }
      if (p[2] == 'service-area' || p[2] == 'operations-settings') {
        final field = p[2] == 'service-area'
                ? 'service_area'
                : 'operations_settings',
            old = jsonObject(branch[field] ?? {});
        if (!_version(old, body)) {
          return _error(409, 'Branch settings changed. Refresh before saving.');
        }
        if (field == 'operations_settings' && body['schema_version'] != 1) {
          return _error(422, 'Only settings schema version 1 is supported.');
        }
        if (field == 'service_area' &&
            (body['mode'] != 'RADIUS' ||
                (body['radius_meters'] as int) < 1 ||
                (body['radius_meters'] as int) > 200000)) {
          return _error(422, 'Use a radius from 1 to 200000 metres.');
        }
        final value = Map<String, dynamic>.of(body)..remove('expected_version');
        value['version'] = (old['version'] as int? ?? 0) + 1;
        branch[field] = value;
        return _ok(value);
      }
      if (p[2] == 'capabilities' && p.length >= 4) {
        final cap = business.capabilities
            .where((c) => c['id'] == p[3])
            .firstOrNull;
        if (cap == null) return _error(404, 'Business type not found.');
        final values = _branchCaps(branch),
            linked = values
                .where((c) => c['vendor_capability_id'] == p[3])
                .firstOrNull;
        if (p.length == 5 && linked != null) {
          final result = _status(linked, body, p[4]);
          _branchCaps(branch);
          return result;
        }
        if (linked != null && !_version(linked, body)) {
          return _error(409, 'Branch type changed. Refresh before saving.');
        }
        if (linked != null) {
          linked['version'] = (linked['version'] as int) + 1;
          return _ok(linked);
        }
        if (body['expected_version'] != 0) {
          return _error(409, 'Refresh before linking this type.');
        }
        final value = {
          'id': 'mock-link-${++_sequence}',
          'business_id': business.vendorId,
          'branch_id': branch['id'],
          'vendor_capability_id': cap['id'],
          'capability_type': cap['capability_type'],
          'status': 'DRAFT',
          'version': 1,
          'effective': false,
          'effective_reason': 'BRANCH_CAPABILITY_NOT_ACTIVE',
        };
        values.add(value);
        branch['capabilities'] = values;
        return _ok(value);
      }
    }
    final values = switch (p.first) {
      'brands' => business.brands,
      'branches' => business.branches,
      'capabilities' => business.capabilities,
      _ => <JsonObject>[],
    };
    if (!{'brands', 'branches', 'capabilities'}.contains(p.first)) {
      return _error(404, 'Settings route not found.');
    }
    if (p.length == 1) {
      if (p.first == 'capabilities' &&
          values.any((v) => v['capability_type'] == body['capability_type'])) {
        return _error(409, 'This business type already exists.');
      }
      final value = Map<String, dynamic>.of(body)..remove('expected_version');
      value.addAll({
        'id': 'mock-${p.first}-${++_sequence}',
        'business_id': business.vendorId,
        'version': 1,
      });
      if (p.first == 'brands') {
        value['verification_state'] = 'NOT_STARTED';
        value.putIfAbsent('media_refs', () => []);
      } else {
        value['status'] = 'DRAFT';
      }
      values.add(value);
      return _ok(value, 201);
    }
    final value = values.where((v) => v['id'] == p[1]).firstOrNull;
    if (value == null) return _error(404, 'Record not found.');
    if (p.length == 3 && {'pause', 'resume'}.contains(p[2])) {
      return _status(value, body, p[2]);
    }
    if (!_version(value, body)) {
      return _error(409, 'This record changed. Refresh before saving.');
    }
    final update = Map<String, dynamic>.of(body)..remove('expected_version');
    value.addAll(update);
    value['version'] = (value['version'] as int) + 1;
    return _ok(value);
  }

  JsonObject? _iam(String method, List<String> p, JsonObject body) {
    if (p.first == 'permissions' && method == 'GET') {
      return _ok({'permissions': mockSettingsPermissions});
    }
    if (p.first == 'assignments') {
      final value = assignments.where((a) => a['id'] == p[1]).firstOrNull;
      if (value == null) return _error(404, 'Assignment not found.');
      assignments.remove(value);
      return _ok({'unassigned': true});
    }
    final values = p.first == 'staff' ? staff : roles;
    if (method == 'GET') {
      if (p.length == 1) {
        return _ok({
          p.first: values.map((v) {
            final copy = _copy(v);
            if (p.first == 'roles') copy.remove('permission_codes');
            return copy;
          }).toList(),
        });
      }
      final value = values.where((v) => v['id'] == p[1]).firstOrNull;
      if (value == null) return _error(404, 'Record not found.');
      if (p.length == 3 && p[2] == 'roles') {
        return _ok({
          'assignments': assignments
              .where((a) => a['staff_id'] == p[1])
              .toList(),
        });
      }
      return _ok(value);
    }
    if (p.first == 'staff') {
      if (p.length == 1) {
        if ((body['phone']?.toString().isNotEmpty != true) &&
            (body['email']?.toString().isNotEmpty != true)) {
          return _error(422, 'Phone or email is required.');
        }
        if (!roles.any((r) => r['id'] == body['role_id'])) {
          return _error(404, 'Role not found.');
        }
        if (body['branch_id'] != null &&
            !business.branches.any((b) => b['id'] == body['branch_id'])) {
          return _error(404, 'Branch not found.');
        }
        final contact = '${body['phone'] ?? ''}|${body['email'] ?? ''}';
        if (!_contacts.add(contact)) {
          return _error(409, 'This person is already staff at this business.');
        }
        final value = {
          'id': 'mock-team-${++_sequence}',
          'vendor_id': business.vendorId,
          'status': 'active',
          'version': 1,
          'created_at': DateTime.now().toUtc().toIso8601String(),
        };
        staff.add(value);
        assignments.add({
          'id': 'mock-assignment-${++_sequence}',
          'staff_id': value['id'],
          'role_id': body['role_id'],
          'branch_id': body['branch_id'],
          'assigned_by': 'mock-staff-1',
          'created_at': DateTime.now().toUtc().toIso8601String(),
        });
        return _ok(value, 201);
      }
      final value = staff.where((s) => s['id'] == p[1]).firstOrNull;
      if (value == null) return _error(404, 'Staff member not found.');
      if (!_version(value, body)) {
        return _error(409, 'Staff record changed. Refresh before saving.');
      }
      final status = p[2] == 'suspend' ? 'suspended' : 'active';
      if (value['status'] == status) {
        return _error(409, 'This staff member already has this status.');
      }
      value['status'] = status;
      value['version'] = (value['version'] as int) + 1;
      if (status == 'suspended') {
        value['suspended_reason'] = body['reason'];
      } else {
        value.remove('suspended_reason');
      }
      return _ok({status == 'suspended' ? 'suspended' : 'reactivated': true});
    }
    final codes = body['permission_codes'] is List
        ? List<String>.from(body['permission_codes'] as List)
        : <String>[];
    if (codes.any(
      (code) => !mockSettingsPermissions.any((p) => p['code'] == code),
    )) {
      return _error(422, 'Unknown permission code.');
    }
    final role = p.length > 1
        ? roles.where((r) => r['id'] == p[1]).firstOrNull
        : null;
    if (p.length > 1 && role == null) return _error(404, 'Role not found.');
    final effective = p.length > 2 && p[2] == 'assign'
        ? List<String>.from(role!['permission_codes'] as List)
        : codes;
    final highRisk = mockSettingsPermissions.any(
      (p) => p['is_high_risk'] == true && effective.contains(p['code']),
    );
    if (highRisk &&
        !business.consumeSettingsGrant(
          body['step_up_grant_id']?.toString() ?? '',
        )) {
      return _error(403, 'Step-up authentication is required for this change.');
    }
    if (p.length == 1) {
      if (body['code']?.toString().trim().isNotEmpty != true ||
          body['name']?.toString().trim().isNotEmpty != true) {
        return _error(422, 'Role code and name are required.');
      }
      if (roles.any((r) => r['code'] == body['code'])) {
        return _error(409, 'This role code already exists.');
      }
      final value = {
        'id': 'mock-role-${++_sequence}',
        'vendor_id': business.vendorId,
        'code': body['code'],
        'name': body['name'],
        'description': body['description'] ?? '',
        'is_system': false,
        'version': 2,
        'permission_codes': codes,
      };
      roles.add(value);
      return _ok(value, 201);
    }
    if (p[2] == 'permissions') {
      if (role!['is_system'] == true) {
        return _error(403, 'Default role families cannot be edited.');
      }
      if (!_version(role, body)) {
        return _error(409, 'This role changed. Refresh before saving.');
      }
      role['permission_codes'] = codes;
      role['version'] = (role['version'] as int) + 1;
      return _ok({'updated': true});
    }
    if (!staff.any((s) => s['id'] == body['staff_id']) ||
        body['role_id'] != role!['id']) {
      return _error(404, 'Staff or role not found.');
    }
    if (body['branch_id'] != null &&
        !business.branches.any((b) => b['id'] == body['branch_id'])) {
      return _error(404, 'Branch not found.');
    }
    if (assignments.any(
      (a) =>
          a['staff_id'] == body['staff_id'] &&
          a['role_id'] == role['id'] &&
          a['branch_id'] == body['branch_id'],
    )) {
      return _error(409, 'This role is already assigned at this scope.');
    }
    final value = {
      'id': 'mock-assignment-${++_sequence}',
      'staff_id': body['staff_id'],
      'role_id': role['id'],
      'branch_id': body['branch_id'],
      'assigned_by': 'mock-staff-1',
      'created_at': DateTime.now().toUtc().toIso8601String(),
    };
    assignments.add(value);
    return _ok(value, 201);
  }
}
