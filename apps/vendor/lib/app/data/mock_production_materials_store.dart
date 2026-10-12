import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';

class MockProductionMaterialsStore {
  MockProductionMaterialsStore(
    this.vendor, {
    required bool empty,
    required this.branchIds,
  }) {
    if (!empty) {
      for (final kind in rows.keys) {
        final ingredient = kind == 'ingredients';
        rows[kind]!.add({
          'id': ingredient
              ? 'mock-production-rice'
              : 'mock-production-container',
          'business_id': vendor,
          'code': ingredient ? 'RICE' : 'CONTAINER',
          'name': ingredient ? 'Rice' : 'Food container',
          'description': ingredient
              ? 'Rice for kitchen preparation.'
              : 'Takeaway food packaging.',
          'measurement_family': ingredient ? 'MASS' : 'COUNT',
          'default_unit_code': ingredient ? 'G' : 'UNIT',
          'inventory_control_mode': 'UNTRACKED',
          if (ingredient) ...{'perishable': false, 'requires_expiry': false},
          'status': 'ACTIVE',
          'version': 1,
          'created_at': DateTime.now().toUtc().toIso8601String(),
          'updated_at': DateTime.now().toUtc().toIso8601String(),
        });
      }
    }
  }
  final String vendor;
  final List<String> branchIds;
  final conversions = <String, List<JsonObject>>{};
  final configs = <String, JsonObject>{};
  final rows = <String, List<JsonObject>>{
    'ingredients': [],
    'packaging-materials': [],
  };
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, int status) => {
    '_message': message,
    '_code': 'PRODUCTION_MASTER_ERROR',
    '_status': status,
  };
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final nested = nestedRequest(request, body);
    if (nested != null) return nested;
    final match = RegExp(
      r'^/api/v1/vendor/(ingredients|packaging-materials)(?:/([^/]+))?(/archive)?$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final kind = match[1]!, id = match[2];
    final collection = rows[kind]!;
    final row = collection.where((r) => r['id'] == id).firstOrNull;
    if (request.method == 'GET') {
      if (id == null) {
        final status = request.url.queryParameters['status'];
        final q = (request.url.queryParameters['q'] ?? '').toLowerCase();
        final limit =
            int.tryParse(request.url.queryParameters['limit'] ?? '') ?? 50;
        return ok(
          collection
              .where(
                (r) =>
                    (status == null ||
                        status.isEmpty ||
                        r['status'] == status) &&
                    '${r['name']} ${r['code']}'.toLowerCase().contains(q),
              )
              .take(limit)
              .toList(),
        );
      }
      return row == null
          ? fail('Production master resource was not found.', 404)
          : ok(row);
    }
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    final fingerprint = jsonEncode([
      request.method,
      request.url.path,
      body,
      request.headers['If-Match'] ?? request.headers['if-match'],
    ]);
    if (key == null || key.isEmpty) {
      return fail('An idempotency key is required.', 400);
    }
    final saved = replay[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : fail('This request key was used for a different change.', 409);
    }
    if (id != null) {
      if (row == null) {
        return fail('Production master resource was not found.', 404);
      }
      final version = int.tryParse(
        request.headers['If-Match'] ?? request.headers['if-match'] ?? '',
      );
      if (version != row['version']) {
        return fail(
          'The resource changed before the operation completed.',
          409,
        );
      }
      if (row['status'] == 'ARCHIVED') {
        return fail('Archived materials cannot be changed.', 409);
      }
    }
    JsonObject result;
    if (match[3] != null && request.method == 'POST') {
      row!['status'] = 'ARCHIVED';
      row['version'] = (row['version'] as int) + 1;
      row['updated_at'] = DateTime.now().toUtc().toIso8601String();
      result = ok(row);
    } else if ((id == null && request.method == 'POST') ||
        (id != null && request.method == 'PUT')) {
      final code = '${body['code'] ?? ''}'.trim().toUpperCase();
      final name = '${body['name'] ?? ''}'.trim();
      final unit = '${body['default_unit_code'] ?? ''}'.trim().toUpperCase();
      if (code.isEmpty ||
          code.length > 80 ||
          name.isEmpty ||
          name.length > 200 ||
          unit.isEmpty ||
          unit.length > 80 ||
          '${body['description'] ?? ''}'.length > 4000 ||
          !['MASS', 'VOLUME', 'COUNT'].contains(body['measurement_family']) ||
          ![
            'UNTRACKED',
            'INVENTORY_CONTROLLED',
          ].contains(body['inventory_control_mode']) ||
          !['DRAFT', 'ACTIVE', 'INACTIVE'].contains(body['status']) ||
          body['requires_expiry'] == true && body['perishable'] != true) {
        return fail('The production material details are invalid.', 400);
      }
      if (collection.any((r) => r['id'] != id && r['code'] == code)) {
        return fail('This material code already exists.', 409);
      }
      final now = DateTime.now().toUtc().toIso8601String();
      final next = <String, dynamic>{
        ...body,
        'id': id ?? 'mock-material-${++sequence}',
        'business_id': vendor,
        'code': code,
        'name': name,
        'default_unit_code': unit,
        'version': row == null ? 1 : (row['version'] as int) + 1,
        'created_at': row?['created_at'] ?? now,
        'updated_at': now,
      };
      if (row == null) {
        collection.add(next);
      } else {
        row.clear();
        row.addAll(next);
      }
      result = ok(next, id == null ? 201 : 200);
    } else {
      return fail('This action is not supported.', 404);
    }
    replay[key] = (fingerprint: fingerprint, result: result);
    return result;
  }

  JsonObject? nestedRequest(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/(ingredients|packaging-materials)/([^/]+)/(conversions|branches)(?:/([^/]+))?$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final kind = match[1]!,
        ownerId = match[2]!,
        area = match[3]!,
        targetId = match[4];
    final owner = rows[kind]!.where((r) => r['id'] == ownerId).firstOrNull;
    if (owner == null) {
      return fail('Production master resource was not found.', 404);
    }
    final ownerKey = '$kind/$ownerId';
    final list = conversions.putIfAbsent(ownerKey, () => []);
    final configKey = '$ownerKey/$targetId';
    if (area == 'branches' &&
        (targetId == null || !branchIds.contains(targetId))) {
      return fail('Branch was not found.', 404);
    }
    if (request.method == 'GET') {
      if (area == 'conversions' && targetId == null) return ok(list);
      if (area == 'branches') {
        final config = configs[configKey];
        if (config == null) {
          return fail('Production master resource was not found.', 404);
        }
        final effective =
            '${config['inventory_control_mode_override'] ?? ''}'.isEmpty
            ? owner['inventory_control_mode']
            : config['inventory_control_mode_override'];
        return ok({
          ...config,
          'effective_inventory_control_mode': effective,
          'inventory_mapping_status': effective == 'UNTRACKED'
              ? 'NOT_REQUIRED'
              : '${config['inventory_item_id'] ?? ''}'.isEmpty
              ? 'NOT_CONFIGURED'
              : 'CONFIGURED',
        });
      }
      return fail('This resource is not available.', 404);
    }
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    if (key == null || key.isEmpty) {
      return fail('An idempotency key is required.', 400);
    }
    final fingerprint = jsonEncode([
      request.method,
      request.url.path,
      body,
      request.headers['If-Match'] ?? request.headers['if-match'],
    ]);
    final saved = replay[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : fail('This request key was used for a different change.', 409);
    }
    if (owner['status'] == 'ARCHIVED') {
      return fail('Archived materials cannot be changed.', 409);
    }
    final now = DateTime.now().toUtc().toIso8601String();
    JsonObject result;
    if (area == 'conversions' &&
        (targetId == null && request.method == 'POST' ||
            targetId != null && request.method == 'PUT')) {
      final existing = list.where((r) => r['id'] == targetId).firstOrNull;
      if (targetId != null) {
        if (existing == null) return fail('Conversion was not found.', 404);
        final version = int.tryParse(
          request.headers['If-Match'] ?? request.headers['if-match'] ?? '',
        );
        if (version != existing['version']) {
          return fail(
            'The conversion changed before the operation completed.',
            409,
          );
        }
      }
      final from = '${body['from_unit_code'] ?? ''}'.trim().toUpperCase(),
          to = '${body['to_unit_code'] ?? ''}'.trim().toUpperCase();
      final numerator = body['numerator'], denominator = body['denominator'];
      final mode = body['conversion_mode'];
      const units = {
        'G',
        'GRAM',
        'GRAMS',
        'KG',
        'KILOGRAM',
        'KILOGRAMS',
        'ML',
        'MILLILITRE',
        'MILLILITRES',
        'MILLILITER',
        'MILLILITERS',
        'L',
        'LITRE',
        'LITRES',
        'LITER',
        'LITERS',
        'EA',
        'EACH',
        'COUNT',
        'UNIT',
        'UNITS',
      };
      if (from.isEmpty ||
          from == to ||
          to != owner['default_unit_code'] ||
          body['measurement_family'] != owner['measurement_family'] ||
          numerator is! int ||
          denominator is! int ||
          numerator <= 0 ||
          denominator <= 0 ||
          numerator > 1000000000000 ||
          denominator > 1000000000000 ||
          !['EXACT_AUTHORITATIVE', 'DISPLAY_ONLY'].contains(mode) ||
          !['ACTIVE', 'INACTIVE', 'ARCHIVED'].contains(body['status']) ||
          '${body['source_context'] ?? ''}'.length > 1000 ||
          mode == 'EXACT_AUTHORITATIVE' &&
              (!units.contains(from) || !units.contains(to)) &&
              '${body['source_context'] ?? ''}'.trim().isEmpty) {
        return fail(
          'The conversion details are invalid or require a material-specific source.',
          400,
        );
      }
      final next = <String, dynamic>{
        ...body,
        'id': targetId ?? 'mock-conversion-${++sequence}',
        'business_id': vendor,
        'owner_id': ownerId,
        'from_unit_code': from,
        'to_unit_code': to,
        'version': existing == null ? 1 : (existing['version'] as int) + 1,
        'created_at': existing?['created_at'] ?? now,
        'updated_at': now,
      };
      if (existing == null) {
        list.add(next);
      } else {
        existing.clear();
        existing.addAll(next);
      }
      result = ok(next, targetId == null ? 201 : 200);
    } else if (area == 'branches' && request.method == 'PUT') {
      final override = '${body['inventory_control_mode_override'] ?? ''}';
      final effective = override.isEmpty
          ? owner['inventory_control_mode']
          : override;
      final item = '${body['inventory_item_id'] ?? ''}';
      final validItem = kind == 'ingredients'
          ? 'mock-ingredient-rice-$targetId'
          : 'mock-packaging-container-$targetId';
      if (body['enabled'] is! bool ||
          body['production_eligible'] is! bool ||
          !['', 'UNTRACKED', 'INVENTORY_CONTROLLED'].contains(override) ||
          effective == 'UNTRACKED' && item.isNotEmpty ||
          effective == 'INVENTORY_CONTROLLED' &&
              body['production_eligible'] == true &&
              item.isEmpty ||
          item.isNotEmpty && item != validItem) {
        return fail(
          'Choose a valid inventory item of the right material type for this branch.',
          400,
        );
      }
      final existing = configs[configKey];
      final next = <String, dynamic>{
        ...body,
        'id': existing?['id'] ?? 'mock-branch-config-${++sequence}',
        'business_id': vendor,
        'branch_id': targetId,
        'owner_id': ownerId,
        'effective_inventory_control_mode': effective,
        'inventory_mapping_status': effective == 'UNTRACKED'
            ? 'NOT_REQUIRED'
            : item.isEmpty
            ? 'NOT_CONFIGURED'
            : 'CONFIGURED',
        'version': existing == null ? 1 : (existing['version'] as int) + 1,
        'created_at': existing?['created_at'] ?? now,
        'updated_at': now,
      };
      configs[configKey] = next;
      result = ok(next);
    } else {
      return fail('This action is not supported.', 404);
    }
    replay[key] = (fingerprint: fingerprint, result: result);
    return result;
  }
}
