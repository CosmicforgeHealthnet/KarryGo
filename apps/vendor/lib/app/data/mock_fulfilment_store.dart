import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';
import 'mock_vendor_missions_store.dart';

class MockFulfilmentStore {
  MockFulfilmentStore(this.delivery) {
    final orders =
        delivery.dashboard.response(
              '/api/v1/vendor/commerce/orders',
              {},
            )?['vendor_orders']
            as List? ??
        [];
    for (var i = 0; i < delivery.missions.length && i < orders.length; i++) {
      final order = jsonObject(orders[i]);
      final mission = delivery.missions[i];
      final group = mission['fulfilment_group_id'] as String;
      final unit = 'demo-fulfilment-unit-${i + 1}';
      final parcel = mission['parcel_ids'][0] as String;
      final now = DateTime.now().toUtc().toIso8601String();
      units[unit] = {
        'fulfilment_unit_id': unit,
        'fulfilment_group_id': group,
        'parent_order_id': order['parent_order_id'],
        'vendor_order_id': order['vendor_order_id'],
        'business_id': order['business_id'],
        'branch_id': order['branch_id'],
        'pickup_address': 'Vendor branch',
        'readiness': ['PACKING', 'READY', 'READY', 'NOT_READY'][i],
        'version': 1,
        'created_at': now,
        'updated_at': now,
      };
      parcels[parcel] = {
        'parcel_id': parcel,
        'parcel_ref': 'KRY-P-${i + 1}',
        'fulfilment_group_id': group,
        'fulfilment_unit_id': unit,
        'parent_order_id': order['parent_order_id'],
        'vendor_order_id': order['vendor_order_id'],
        'business_id': order['business_id'],
        'branch_id': order['branch_id'],
        'sequence': 1,
        'status': ['PACKED', 'READY', 'IN_TRANSIT', 'CREATED'][i],
        'custodian_type': 'VENDOR',
        'manifest': {
          'package_count': 1,
          'lines': [
            for (final raw in order['lines'] as List)
              {
                'order_line_ref': raw['id'],
                'description': raw['item_name'],
                'quantity': raw['quantity'],
              },
          ],
          if (i == 2) 'sealed_at': now,
        },
        'manifest_version': 1,
        'weight_grams': 1500,
        'fragile': false,
        'temperature_control': 'NONE',
        'version': 1,
        'created_at': now,
        'updated_at': now,
      };
      groups[group] = {
        'fulfilment_group_id': group,
        'parent_order_id': order['parent_order_id'],
        'current_plan_id': mission['plan_id'],
        'current_plan_version': 1,
        'version': 1,
        'created_at': now,
        'updated_at': now,
      };
      history[parcel] = [
        {
          'custody_event_id': 'demo-created-$parcel',
          'parcel_id': parcel,
          'event_type': 'CREATED',
          'from_custodian_type': 'NONE',
          'to_custodian_type': 'VENDOR',
          'source': 'VENDOR',
          'source_event_id': 'demo-create-$parcel',
          'source_version': 1,
          'occurred_at': now,
          'created_at': now,
        },
      ];
    }
  }
  final MockVendorMissionsStore delivery;
  final groups = <String, JsonObject>{},
      units = <String, JsonObject>{},
      parcels = <String, JsonObject>{};
  final history = <String, List<JsonObject>>{};
  final returns = <JsonObject>[];
  final replays = <String, ({String fingerprint, JsonObject response})>{};
  int sequence = 0;
  JsonObject fail(int status, String message) => {
    '_status': status,
    '_code': status == 409
        ? 'FULFILMENT_VERSION_CONFLICT'
        : 'FULFILMENT_VALIDATION_FAILED',
    '_message': message,
  };
  JsonObject ok(Object value) => {
    '_status': 200,
    '_data': jsonDecode(jsonEncode(value)),
  };
  bool validManifest(JsonObject m) =>
      m['package_count'] is int &&
      m['package_count'] >= 0 &&
      m['lines'] is List &&
      (m['lines'] as List).length <= 500 &&
      (m['lines'] as List).every(
        (l) =>
            l is Map &&
            l['order_line_ref'] is String &&
            l['order_line_ref'].toString().isNotEmpty &&
            l['quantity'] is int &&
            l['quantity'] >= 0,
      );
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final path = request.url.path.replaceFirst(
      '/api/v1/vendor/fulfilment/',
      '',
    );
    final pieces = path.split('/');
    if (pieces.length < 2) return null;
    final kind = pieces[0], id = pieces[1];
    if (!['groups', 'units', 'parcels'].contains(kind)) return null;
    if (request.method == 'GET') {
      if (kind == 'groups') {
        final group = groups[id];
        if (group == null) return fail(404, 'Fulfilment not found.');
        final plan = {
          'plan_id': group['current_plan_id'],
          'fulfilment_group_id': id,
          'version': 1,
          'status': 'ACTIVE',
          'reason_code': 'INITIAL_PLAN',
          'delivery_mode': 'VENDOR',
          'created_at': group['created_at'],
        };
        if (pieces.length == 3 && pieces[2] == 'plans') {
          return ok({
            'plans': [plan],
          });
        }
        return ok({
          'group': {
            ...group,
            'units': units.values
                .where((u) => u['fulfilment_group_id'] == id)
                .toList(),
            'parcels': parcels.values
                .where((p) => p['fulfilment_group_id'] == id)
                .toList(),
          },
          'current_plan': plan,
          'platform_missions': [],
          'vendor_missions': delivery.missions
              .where((m) => m['fulfilment_group_id'] == id)
              .toList(),
          'returns': returns
              .where(
                (r) => parcels[r['parcel_id']]?['fulfilment_group_id'] == id,
              )
              .toList(),
        });
      }
      if (kind == 'units' && pieces.length == 3 && pieces[2] == 'parcels') {
        return ok({
          'parcels': parcels.values
              .where((p) => p['fulfilment_unit_id'] == id)
              .toList(),
        });
      }
      final parcel = parcels[id];
      if (parcel == null) return fail(404, 'Parcel not found.');
      return pieces.length == 3 && pieces[2] == 'custody'
          ? ok({'custody_events': history[id] ?? []})
          : ok(parcel);
    }
    final key = delivery.header(request, 'Idempotency-Key');
    final version = int.tryParse(delivery.header(request, 'If-Match'));
    final fingerprint = '${request.method}|$path|${jsonEncode(body)}|$version';
    final replay = replays[key];
    if (replay != null) {
      return replay.fingerprint == fingerprint
          ? replay.response
          : fail(409, 'Retry details changed.');
    }
    final now = DateTime.now().toUtc().toIso8601String();
    JsonObject? result;
    if (kind == 'units') {
      final unit = units[id];
      if (unit == null) return fail(404, 'Pickup obligation not found.');
      if (pieces.length == 3 &&
          pieces[2] == 'readiness' &&
          request.method == 'PUT') {
        if (version != unit['version']) {
          return fail(409, 'Pickup obligation changed. Refresh.');
        }
        const rank = {'NOT_READY': 0, 'PACKING': 1, 'READY': 2, 'COLLECTED': 3};
        final state = body['readiness'];
        final current = unit['readiness'];
        if (state != current &&
            !(state == 'CANCELLED' && current != 'COLLECTED') &&
            !(rank.containsKey(current) &&
                rank.containsKey(state) &&
                rank[state]! > rank[current]!)) {
          return fail(422, 'This pickup state is unavailable.');
        }
        unit['readiness'] = state;
        unit['version'] = (unit['version'] as int) + 1;
        result = unit;
      } else if (pieces.length == 3 &&
          pieces[2] == 'parcels' &&
          request.method == 'POST') {
        if (key.isEmpty || !validManifest(jsonObject(body['manifest']))) {
          return fail(422, 'Check the packed contents.');
        }
        for (final field in [
          'weight_grams',
          'length_mm',
          'width_mm',
          'height_mm',
        ]) {
          if (body[field] != null && (body[field] is! int || body[field] < 0)) {
            return fail(422, 'Physical measurements must be non-negative.');
          }
        }
        if (body['temperature_control'] != null &&
            ![
              'NONE',
              'CHILLED',
              'FROZEN',
              'HOT',
            ].contains(body['temperature_control'])) {
          return fail(422, 'Choose a temperature requirement.');
        }
        final parcel = 'demo-packed-parcel-${++sequence}';
        result = {
          ...unit,
          'parcel_id': parcel,
          'parcel_ref': 'KRY-P-NEW-$sequence',
          'status': 'PACKED',
          'custodian_type': 'VENDOR',
          'manifest': body['manifest'],
          'manifest_version': 1,
          'version': 1,
          'weight_grams': body['weight_grams'] ?? 0,
          'fragile': body['fragile'] ?? false,
          'temperature_control': body['temperature_control'] ?? 'NONE',
          for (final field in ['length_mm', 'width_mm', 'height_mm'])
            if (body[field] != null) field: body[field],
          'created_at': now,
          'updated_at': now,
        };
        result.remove('readiness');
        parcels[parcel] = result;
      }
    } else if (kind == 'parcels') {
      final parcel = parcels[id];
      if (parcel == null) return fail(404, 'Parcel not found.');
      if (pieces.length == 3 &&
          pieces[2] == 'manifest' &&
          request.method == 'PUT') {
        if (version != parcel['version']) {
          return fail(409, 'Parcel changed. Refresh.');
        }
        if (parcel['manifest']['sealed_at'] != null ||
            ![
              'CREATED',
              'PACKED',
              'READY',
              'CANCELLED',
            ].contains(parcel['status'])) {
          return fail(409, 'The manifest is sealed by custody.');
        }
        if (!validManifest(body)) {
          return fail(422, 'Check the packed contents.');
        }
        parcel['manifest'] = body;
        parcel['manifest_version'] = (parcel['manifest_version'] as int) + 1;
        parcel['version'] = (parcel['version'] as int) + 1;
        result = parcel;
      } else if (pieces.length == 3 &&
          pieces[2] == 'returns' &&
          request.method == 'POST') {
        if (key.isEmpty ||
            body['reason_code']?.toString().trim().isNotEmpty != true) {
          return fail(422, 'A return reason and retry key are required.');
        }
        result = {
          'parcel_return_id': 'demo-parcel-return-${++sequence}',
          'parcel_id': id,
          'reason_code': body['reason_code'],
          'state': 'INITIATED',
          'version': 1,
          'created_at': now,
          'updated_at': now,
        };
        returns.add(result);
        if (['IN_TRANSIT', 'FAILED', 'DELIVERED'].contains(parcel['status'])) {
          parcel['status'] = 'RETURN_REQUIRED';
          parcel['version'] = (parcel['version'] as int) + 1;
        }
      }
    }
    if (result == null) return fail(405, 'Operation unavailable.');
    final response = ok(result);
    if (key.isNotEmpty) {
      replays[key] = (fingerprint: fingerprint, response: response);
    }
    return response;
  }

  void riderCustody(JsonObject mission, String action, String key) {
    final event = switch (action) {
      'complete-pickup' => 'HANDED_OVER',
      'complete-delivery' => 'DELIVERED',
      'return' => 'RETURNED',
      _ => null,
    };
    if (event == null) return;
    final now = DateTime.now().toUtc().toIso8601String();
    for (final id in mission['parcel_ids'] as List? ?? []) {
      final parcel = parcels[id];
      if (parcel == null) continue;
      parcel['status'] = switch (action) {
        'complete-pickup' => 'IN_TRANSIT',
        'complete-delivery' => 'DELIVERED',
        _ => 'RETURNED',
      };
      parcel['custodian_type'] = action == 'complete-delivery'
          ? 'CUSTOMER'
          : 'VENDOR';
      parcel['manifest']['sealed_at'] ??= now;
      parcel['version'] = (parcel['version'] as int) + 1;
      history.putIfAbsent(id.toString(), () => []).add({
        'custody_event_id': 'demo-custody-$key-$id',
        'parcel_id': id,
        'event_type': event,
        'from_custodian_type': 'VENDOR',
        'to_custodian_type': parcel['custodian_type'],
        'source': 'VENDOR',
        'source_event_id': 'rider-action-$key-$id',
        'source_version': mission['version'],
        'vendor_mission_id': mission['vendor_mission_id'],
        'occurred_at': now,
        'created_at': now,
      });
    }
  }
}
