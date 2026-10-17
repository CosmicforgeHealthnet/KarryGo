import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/catalogue/state/catalogue_resource.dart';
import 'mock_catalogue_store.dart';
import 'mock_advanced_catalogue_store.dart';
import 'package:vendor/features/catalogue/state/advanced_resource.dart';

class MockInventoryStore {
  MockInventoryStore(this.catalogue, this.advanced) {
    for (final listing in catalogue.advancedRecords(CatalogueKind.listings)) {
      final id = 'stock-${listing['id']}';
      balances[id] = {
        'stock_basis_ref': id,
        'branch_ref': listing['branch_ref'],
        'listing_ref': listing['id'],
        'variant_ref': listing['variant_ref'],
        'on_hand_quantity': 24,
        'available_quantity': 24,
        'reserved_quantity': 0,
        'quarantined_quantity': 0,
        'in_transit_quantity': 0,
        'backorder_quantity': 0,
        'revision': 1,
        'updated_at': now,
      };
      revisions[listing['branch_ref'].toString()] = 1;
      alerts.add({
        'id': 'alert-${listing['id']}',
        'branch_ref': listing['branch_ref'],
        'stock_basis_ref': id,
        'alert_type': 'LOW_STOCK',
        'severity': 'WARNING',
        'details': {'available_quantity': 24, 'minimum_quantity': 30},
        'status': 'OPEN',
        'created_at': now,
        'updated_at': now,
      });
    }
  }
  final MockCatalogueStore catalogue;
  final MockAdvancedCatalogueStore advanced;
  final balances = <String, JsonObject>{};
  final revisions = <String, int>{};
  final receipts = <String, JsonObject>{}, stocktakes = <String, JsonObject>{};
  final transfers = <String, JsonObject>{},
      quarantines = <String, JsonObject>{};
  final reorderRules = <String, JsonObject>{};
  final quarantineLots = <String, List<({JsonObject lot, int quantity})>>{};
  final movements = <JsonObject>[], alerts = <JsonObject>[];
  final lots = <String, List<JsonObject>>{};
  final commands = <String, ({String fingerprint, JsonObject response})>{};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject ok(Object? value, [int status = 200]) => {
    '_data': value,
    '_status': status,
  };
  JsonObject fail(
    String message, [
    int status = 400,
    String code = 'SHOP_INVENTORY_REQUEST_REFUSED',
  ]) => {'_message': message, '_status': status, '_code': code};
  Object? copy(Object? value) => jsonDecode(jsonEncode(value));
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final path = request.url.path;
    if (!path.contains('/shop/') || !path.contains('/inventory/')) return null;
    if (request.method == 'GET') return read(path, request.url.queryParameters);
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    if (key == null || key.isEmpty) return fail('Idempotency-Key is required.');
    final fingerprint = '${request.method} $path ${jsonEncode(body)}';
    final cached = commands[key];
    if (cached != null) {
      return cached.fingerprint == fingerprint
          ? jsonObject(copy(cached.response))
          : fail('This command key was already used.', 409);
    }
    final response = write(request.method, path, body);
    if (response['_message'] == null) {
      commands[key] = (
        fingerprint: fingerprint,
        response: jsonObject(copy(response)),
      );
    }
    return response;
  }

  JsonObject read(String path, Map<String, String> query) {
    final branch = RegExp(
      r'/branches/([^/]+)/inventory/([^/]+)(?:/([^/]+))?$',
    ).firstMatch(path);
    if (branch != null) {
      final id = branch[1]!, kind = branch[2]!;
      if (kind == 'revision') {
        return ok({
          'revision': revisions[id] ?? 0,
          'domain': 'SHOP_INVENTORY',
          'updated_at': now,
        });
      }
      if (kind == 'balances' && branch[3] != null) {
        final row = balances[branch[3]];
        return row == null || row['branch_ref'] != id
            ? fail('Stock balance not found.', 404)
            : ok(copy(row));
      }
      final rows = switch (kind) {
        'balances' =>
          balances.values
              .map(
                (row) => {
                  'branch_ref': row['branch_ref'],
                  'listing_ref': row['listing_ref'],
                  'variant_ref': row['variant_ref'],
                  'stock_basis_ref': row['stock_basis_ref'],
                  'physical_on_hand': row['on_hand_quantity'],
                  'physical_available': row['available_quantity'],
                  'reserved_quantity': row['reserved_quantity'],
                  'quarantined_quantity': row['quarantined_quantity'],
                  'in_transit_quantity': row['in_transit_quantity'],
                  'backorder_quantity': row['backorder_quantity'],
                  'inventory_revision': revisions[id] ?? 0,
                  'evaluated_at': now,
                },
              )
              .toList(),
        'receipts' => receipts.values.toList(),
        'stocktakes' => stocktakes.values.toList(),
        'transfers' => transfers.values.toList(),
        'quarantines' => quarantines.values.toList(),
        'reorder-rules' =>
          reorderRules.values
              .map(
                (r) => {
                  ...r,
                  'recommended_reorder_quantity': recommendedReorder(r),
                },
              )
              .toList(),
        'movements' => movements,
        'alerts' => alerts,
        _ => <JsonObject>[],
      };
      final filtered = rows
          .where(
            (row) =>
                (kind == 'transfers'
                    ? row['source_branch_ref'] == id ||
                          row['destination_branch_ref'] == id
                    : row['branch_ref'] == id) &&
                (query['status'] == null || row['status'] == query['status']),
          )
          .toList();
      if (kind == 'balances') return ok(copy(filtered));
      final offset = int.tryParse(query['offset'] ?? '') ?? 0,
          limit = int.tryParse(query['limit'] ?? '') ?? 50;
      return ok({
        'items': copy(filtered.skip(offset).take(limit).toList()),
        'has_more': offset + limit < filtered.length,
      });
    }
    final detail = RegExp(
      r'/inventory/(receipts|stocktakes|transfers)/([^/]+)$',
    ).firstMatch(path);
    if (detail != null) {
      final record = (detail[1] == 'receipts'
          ? receipts
          : detail[1] == 'transfers'
          ? transfers
          : stocktakes)[detail[2]];
      return record == null
          ? fail('Inventory record not found.', 404)
          : ok(copy(record));
    }
    return fail('Inventory route not found.', 404);
  }

  JsonObject writeTransfer(String path, JsonObject body) {
    if (path == '/api/v1/vendor/shop/inventory/transfers') {
      final source = body['source_branch_id'],
          destination = body['destination_branch_id'];
      final inputs = body['lines'];
      if (source == destination ||
          inputs is! List ||
          inputs.isEmpty ||
          inputs.length > 200) {
        return fail(
          'Choose different branches and between 1 and 200 transfer items.',
        );
      }
      final variants = <String>{};
      for (final input in inputs) {
        final from = balances[input['source_stock_basis_id']],
            to = balances[input['destination_stock_basis_id']];
        if (from == null ||
            to == null ||
            from['branch_ref'] != source ||
            to['branch_ref'] != destination ||
            from['variant_ref'] != to['variant_ref'] ||
            !quantity(input['quantity'])) {
          return fail(
            'Transfer items must be the same variant in the selected branches.',
          );
        }
        if (!variants.add(from['variant_ref'].toString())) {
          return fail('A variant may appear only once.');
        }
      }
      final id = 'transfer-${++sequence}';
      final row = {
        'id': id,
        'source_branch_ref': source,
        'destination_branch_ref': destination,
        'notes': body['notes'] ?? '',
        'status': 'DRAFT',
        'created_at': now,
        'updated_at': now,
        'version': 1,
        'dispatched_at': null,
        'received_at': null,
        'lines': [
          for (final input in inputs)
            {
              'id': 'transfer-line-${++sequence}',
              'source_stock_basis_ref': input['source_stock_basis_id'],
              'destination_stock_basis_ref':
                  input['destination_stock_basis_id'],
              'variant_ref':
                  balances[input['source_stock_basis_id']]!['variant_ref'],
              'sent_quantity': input['quantity'],
              'received_quantity': 0,
              'damaged_quantity': 0,
              'missing_quantity': 0,
              'status': 'PENDING',
            },
        ],
      };
      transfers[id] = row;
      return ok(copy(row), 201);
    }
    final match = RegExp(
      r'/inventory/transfers/([^/]+)/(dispatch|cancel|receive)$',
    ).firstMatch(path);
    if (match == null || !transfers.containsKey(match[1])) {
      return fail('Transfer not found.', 404);
    }
    final row = transfers[match[1]]!, action = match[2]!;
    final lines = row['lines'] as List;
    if (action == 'cancel') {
      if (row['status'] == 'CANCELLED') return ok(copy(row));
      if (!['DRAFT', 'APPROVED'].contains(row['status'])) {
        return fail(
          'A dispatched transfer must be resolved through receipt and discrepancy evidence.',
          409,
        );
      }
      row['status'] = 'CANCELLED';
      row['version'] += 1;
      row['updated_at'] = now;
      for (final line in lines) {
        line['status'] = 'CANCELLED';
      }
      return ok(copy(row));
    }
    if (action == 'dispatch') {
      if (row['status'] == 'IN_TRANSIT') return ok(copy(row));
      if (!['DRAFT', 'APPROVED'].contains(row['status'])) {
        return fail('Transfer cannot be dispatched from this state.', 409);
      }
      for (final line in lines) {
        final from = balances[line['source_stock_basis_ref']]!;
        if (line['sent_quantity'] > from['available_quantity']) {
          return fail(
            'Transfer exceeds sellable source stock.',
            409,
            'SHOP_INSUFFICIENT_STOCK',
          );
        }
      }
      for (final line in lines) {
        final from = balances[line['source_stock_basis_ref']]!;
        final sent = line['sent_quantity'] as int;
        move(from, -sent, 'TRANSFER_OUT', row['id'], validLotsOnly: true);
        from['in_transit_quantity'] += sent;
        movements.first['in_transit_delta'] = sent;
        movements.first['source_type'] = 'TRANSFER';
        line['status'] = 'IN_TRANSIT';
      }
      row['status'] = 'IN_TRANSIT';
      row['version'] += 1;
      row['updated_at'] = now;
      row['dispatched_at'] = now;
      return ok(copy(row));
    }
    if (!['IN_TRANSIT', 'PARTIALLY_RECEIVED'].contains(row['status'])) {
      return fail('Transfer is not in transit.', 409);
    }
    final inputs = body['lines'];
    if (inputs is! List || inputs.isEmpty) {
      return fail('Receipt lines are required.');
    }
    final totals = <String, int>{};
    for (final input in inputs) {
      final line = lines.where((l) => l['id'] == input['line_id']).firstOrNull;
      if (line == null) return fail('Transfer line not found.', 404);
      for (final key in [
        'received_quantity',
        'damaged_quantity',
        'missing_quantity',
      ]) {
        if (!quantity(input[key], zero: true)) {
          return fail('Arrival quantities must be non-negative.');
        }
      }
      final n =
          (input['received_quantity'] as int) +
          (input['damaged_quantity'] as int) +
          (input['missing_quantity'] as int);
      totals[input['line_id']] = (totals[input['line_id']] ?? 0) + n;
      if (n == 0 ||
          totals[input['line_id']]! +
                  line['received_quantity'] +
                  line['damaged_quantity'] +
                  line['missing_quantity'] >
              line['sent_quantity']) {
        return fail(
          'Resolved quantity exceeds quantity remaining, or is zero.',
        );
      }
    }
    for (final input in inputs) {
      final line = lines.firstWhere((l) => l['id'] == input['line_id']);
      final received = input['received_quantity'] as int,
          loss =
              (input['damaged_quantity'] as int) +
              (input['missing_quantity'] as int);
      final from = balances[line['source_stock_basis_ref']]!,
          to = balances[line['destination_stock_basis_ref']]!;
      if (received > 0) {
        move(to, received, 'TRANSFER_IN', row['id']);
        movements.first['source_type'] = 'TRANSFER';
      }
      from['in_transit_quantity'] -= received + loss;
      from['revision'] += 1;
      from['updated_at'] = now;
      final branch = from['branch_ref'].toString();
      revisions[branch] = (revisions[branch] ?? 0) + 1;
      if (loss > 0) {
        movements.insert(0, {
          'id': 'movement-${++sequence}',
          'branch_ref': branch,
          'stock_basis_ref': from['stock_basis_ref'],
          'variant_ref': from['variant_ref'],
          'movement_type': 'DAMAGE',
          'quantity': loss,
          'on_hand_delta': 0,
          'reserved_delta': 0,
          'quarantined_delta': 0,
          'in_transit_delta': -loss,
          'source_type': 'TRANSFER',
          'source_ref': row['id'],
          'reason_code': 'TRANSFER_DISCREPANCY',
          'occurred_at': now,
          'inventory_revision': revisions[branch],
        });
      }
      for (final key in [
        'received_quantity',
        'damaged_quantity',
        'missing_quantity',
      ]) {
        line[key] += input[key];
      }
      line['status'] =
          line['received_quantity'] +
                  line['damaged_quantity'] +
                  line['missing_quantity'] ==
              line['sent_quantity']
          ? 'RECEIVED'
          : 'PARTIALLY_RECEIVED';
    }
    row['status'] = lines.every((l) => l['status'] == 'RECEIVED')
        ? 'RECEIVED'
        : 'PARTIALLY_RECEIVED';
    row['version'] += 1;
    row['updated_at'] = now;
    if (row['status'] == 'RECEIVED') row['received_at'] = now;
    return ok(copy(row));
  }

  bool quantity(Object? value, {bool zero = false}) =>
      value is int && value >= (zero ? 0 : 1) && value <= 1000000000;
  void move(
    JsonObject row,
    int delta,
    String type,
    String source, {
    String? expiresAt,
    String? lotId,
    bool validLotsOnly = false,
  }) {
    final branch = row['branch_ref'].toString();
    final id = row['stock_basis_ref'].toString();
    final stockLots = lots.putIfAbsent(
      id,
      () => [
        {'id': 'opening-$id', 'quantity': row['on_hand_quantity']},
      ],
    );
    if (delta > 0) {
      stockLots.add({
        'id': lotId ?? 'lot-${++sequence}',
        'quantity': delta,
        'expires_at': expiresAt,
      });
    } else {
      var remaining = -delta;
      for (final lot in stockLots) {
        if (lotId != null && lot['id'] != lotId) continue;
        if (validLotsOnly &&
            lot['expires_at'] != null &&
            !DateTime.parse(lot['expires_at']).isAfter(DateTime.now())) {
          continue;
        }
        final free =
            (lot['quantity'] as int) - (lot['quarantined'] as int? ?? 0);
        final n = free < remaining ? free : remaining;
        lot['quantity'] -= n;
        remaining -= n;
        if (remaining == 0) break;
      }
    }
    row['on_hand_quantity'] += delta;
    row['available_quantity'] += delta;
    row['available_quantity'] =
        stockLots.fold<int>(
          0,
          (sum, lot) =>
              lot['expires_at'] != null &&
                  !DateTime.parse(lot['expires_at']).isAfter(DateTime.now())
              ? sum
              : sum +
                    (lot['quantity'] as int) -
                    (lot['quarantined'] as int? ?? 0),
        ) -
        (row['reserved_quantity'] as int);
    row['revision'] += 1;
    row['updated_at'] = now;
    revisions[branch] = (revisions[branch] ?? 0) + 1;
    movements.insert(0, {
      'id': 'movement-${++sequence}',
      'branch_ref': branch,
      'stock_basis_ref': row['stock_basis_ref'],
      'variant_ref': row['variant_ref'],
      'movement_type': type,
      'quantity': delta.abs(),
      'on_hand_delta': delta,
      'reserved_delta': 0,
      'quarantined_delta': 0,
      'in_transit_delta': 0,
      'source_type': type,
      'source_ref': source,
      'occurred_at': now,
      'inventory_revision': revisions[branch],
    });
  }

  JsonObject writeQuarantine(String path, JsonObject body) {
    if (path == '/api/v1/vendor/shop/inventory/quarantines') {
      final row = balances[body['stock_basis_id']];
      final n = body['quantity'];
      final reason = body['reason_code']?.toString().trim().toUpperCase();
      if (row == null) return fail('Stock item not found.', 404);
      if (!quantity(n) || !quarantineReasons.contains(reason)) {
        return fail(
          'Choose a positive quantity and controlled quarantine reason.',
        );
      }
      final stockLots = lots.putIfAbsent(
        row['stock_basis_ref'],
        () => [
          {
            'id': 'opening-${row['stock_basis_ref']}',
            'quantity': row['on_hand_quantity'],
          },
        ],
      );
      final eligible =
          stockLots
              .where(
                (lot) =>
                    lot['expires_at'] == null ||
                    DateTime.parse(lot['expires_at']).isAfter(DateTime.now()),
              )
              .toList()
            ..sort((a, b) {
              final left = a['expires_at']?.toString(),
                  right = b['expires_at']?.toString();
              return left == null
                  ? (right == null ? 0 : 1)
                  : right == null
                  ? -1
                  : left.compareTo(right);
            });
      final free = eligible.fold<int>(
        0,
        (sum, lot) =>
            sum + (lot['quantity'] as int) - (lot['quarantined'] as int? ?? 0),
      );
      if (n > free - (row['reserved_quantity'] as int)) {
        return fail(
          'Quarantine exceeds sellable physical stock.',
          409,
          'SHOP_INVENTORY_INSUFFICIENT',
        );
      }
      final id = 'quarantine-${++sequence}';
      var remaining = n as int;
      final allocations = <({JsonObject lot, int quantity})>[];
      for (final lot in eligible) {
        final available =
            (lot['quantity'] as int) - (lot['quarantined'] as int? ?? 0);
        final take = available < remaining ? available : remaining;
        if (take <= 0) continue;
        allocations.add((lot: lot, quantity: take));
        lot['quarantined'] = (lot['quarantined'] as int? ?? 0) + take;
        remaining -= take;
        if (remaining == 0) break;
      }
      quarantineLots[id] = allocations;
      quarantines[id] = {
        'id': id,
        'branch_ref': row['branch_ref'],
        'stock_basis_ref': row['stock_basis_ref'],
        'variant_ref': row['variant_ref'],
        'quantity': n,
        'reason_code': reason,
        'status': 'ACTIVE',
        'created_at': now,
        'resolved_at': null,
        'version': 1,
      };
      row['quarantined_quantity'] += n;
      quarantineMovement(row, id, 'QUARANTINE_IN', n, 0, n, reason!);
      return ok(copy(quarantines[id]), 201);
    }
    final match = RegExp(
      r'/inventory/quarantines/([^/]+)/resolve$',
    ).firstMatch(path);
    if (match == null) return fail('Quarantine route not found.', 404);
    final record = quarantines[match[1]];
    if (record == null) {
      return fail('Quarantine not found.', 404, 'SHOP_QUARANTINE_NOT_FOUND');
    }
    final action = body['action']?.toString().trim().toUpperCase();
    if (action != 'RELEASE' && action != 'DISPOSE') {
      return fail('Use RELEASE or DISPOSE.');
    }
    final target = action == 'RELEASE' ? 'RELEASED' : 'DISPOSED';
    if (record['status'] == target) return ok(copy(record));
    if (record['status'] != 'ACTIVE') {
      return fail('Quarantine is already resolved.', 409);
    }
    final n = record['quantity'] as int;
    final row = balances[record['stock_basis_ref']]!;
    for (final allocation in quarantineLots[record['id']]!) {
      allocation.lot['quarantined'] -= allocation.quantity;
      if (action == 'DISPOSE') {
        allocation.lot['quantity'] -= allocation.quantity;
      }
    }
    row['quarantined_quantity'] -= n;
    final delta = action == 'DISPOSE' ? -n : 0;
    row['on_hand_quantity'] += delta;
    quarantineMovement(
      row,
      record['id'],
      action == 'DISPOSE' ? 'WASTE' : 'QUARANTINE_RELEASE',
      n,
      delta,
      -n,
      record['reason_code'],
    );
    record['status'] = target;
    record['resolved_at'] = now;
    record['version'] += 1;
    return ok(copy(record));
  }

  void quarantineMovement(
    JsonObject row,
    String id,
    String type,
    int n,
    int delta,
    int hold,
    String reason,
  ) {
    row['available_quantity'] =
        lots[row['stock_basis_ref']]!.fold<int>(
          0,
          (sum, lot) =>
              lot['expires_at'] != null &&
                  !DateTime.parse(lot['expires_at']).isAfter(DateTime.now())
              ? sum
              : sum +
                    (lot['quantity'] as int) -
                    (lot['quarantined'] as int? ?? 0),
        ) -
        (row['reserved_quantity'] as int);
    row['revision'] += 1;
    row['updated_at'] = now;
    final branch = row['branch_ref'].toString();
    revisions[branch] = (revisions[branch] ?? 0) + 1;
    movements.insert(0, {
      'id': 'movement-${++sequence}',
      'branch_ref': branch,
      'stock_basis_ref': row['stock_basis_ref'],
      'variant_ref': row['variant_ref'],
      'movement_type': type,
      'quantity': n,
      'on_hand_delta': delta,
      'reserved_delta': 0,
      'quarantined_delta': hold,
      'in_transit_delta': 0,
      'source_type': 'QUARANTINE',
      'source_ref': id,
      'reason_code': reason,
      'occurred_at': now,
      'inventory_revision': revisions[branch],
    });
  }

  static const quarantineReasons = {
    'QUALITY_CHECK',
    'DAMAGED',
    'RECALL',
    'TEMPERATURE_EXCURSION',
    'SUSPECTED_SPOILAGE',
    'OTHER_CONTROLLED',
  };

  int recommendedReorder(JsonObject rule) {
    final row = balances[rule['stock_basis_ref']]!;
    final stockLots = lots[row['stock_basis_ref']];
    final available = stockLots == null
        ? row['available_quantity'] as int
        : stockLots.fold<int>(
                0,
                (sum, lot) =>
                    lot['expires_at'] != null &&
                        !DateTime.parse(
                          lot['expires_at'],
                        ).isAfter(DateTime.now())
                    ? sum
                    : sum +
                          (lot['quantity'] as int) -
                          (lot['quarantined'] as int? ?? 0),
              ) -
              (row['reserved_quantity'] as int);
    final amount = (rule['target_quantity'] as int) - available;
    return amount < 0 ? 0 : amount;
  }

  JsonObject write(String method, String path, JsonObject body) {
    final reorder = RegExp(
      r'/inventory/stock-bases/([^/]+)/reorder-rule$',
    ).firstMatch(path);
    if (reorder != null && method == 'PUT') {
      final row = balances[reorder[1]];
      if (row == null) return fail('Stock item not found.', 404);
      final minimum = body['minimum_quantity'],
          target = body['target_quantity'],
          hours = body['expiring_soon_hours'];
      if (minimum is! int ||
          target is! int ||
          minimum < 0 ||
          target <= minimum ||
          target > 1000000000 ||
          hours != null && (hours is! int || hours < 1 || hours > 8760)) {
        return fail(
          'Target must exceed the non-negative minimum. Expiry hours must be between 1 and 8760.',
        );
      }
      final prior = reorderRules[row['stock_basis_ref']];
      final rule = <String, dynamic>{
        'id': prior?['id'] ?? 'reorder-${++sequence}',
        'branch_ref': row['branch_ref'],
        'stock_basis_ref': row['stock_basis_ref'],
        'variant_ref': row['variant_ref'],
        'minimum_quantity': minimum,
        'target_quantity': target,
        'expiring_soon_hours': hours,
        'status': 'ACTIVE',
        'version': (prior?['version'] as int? ?? 0) + 1,
      };
      rule['recommended_reorder_quantity'] = recommendedReorder(rule);
      reorderRules[row['stock_basis_ref']] = rule;
      return ok(copy(rule));
    }
    if (path == '/api/v1/vendor/shop/inventory/quarantines' ||
        path.contains('/inventory/quarantines/')) {
      return writeQuarantine(path, body);
    }
    if (path == '/api/v1/vendor/shop/inventory/transfers' ||
        path.contains('/inventory/transfers/')) {
      return writeTransfer(path, body);
    }
    if (path.endsWith('/adjustments')) {
      final row = balances[body['stock_basis_id']];
      final delta = body['quantity_delta'];
      if (row == null) return fail('Stock item not found.', 404);
      if (body['expected_revision'] != row['revision']) {
        return fail(
          'Inventory changed after it was loaded. Refresh and review your adjustment.',
          409,
          'SHOP_INVENTORY_STALE_REVISION',
        );
      }
      if (delta is! int ||
          delta == 0 ||
          delta.abs() > 1000000000 ||
          !adjustmentReasons.contains(body['reason_code'])) {
        return fail('Choose a valid quantity and adjustment reason.');
      }
      if (row['available_quantity'] + delta < 0) {
        return fail('Insufficient available stock.', 409);
      }
      final id = 'adjustment-${++sequence}';
      move(row, delta, delta > 0 ? 'ADJUSTMENT_IN' : 'ADJUSTMENT_OUT', id);
      return ok({
        'id': id,
        'branch_ref': row['branch_ref'],
        'stock_basis_ref': row['stock_basis_ref'],
        'variant_ref': row['variant_ref'],
        ...body,
        'status': 'POSTED',
        'version': 1,
        'created_at': now,
      }, 201);
    }
    if (path.endsWith('/stock-bases')) {
      final branch = RegExp(r'/branches/([^/]+)/').firstMatch(path)![1]!;
      final listing = catalogue
          .advancedRecords(CatalogueKind.listings)
          .where(
            (r) => r['id'] == body['listing_id'] && r['branch_ref'] == branch,
          )
          .firstOrNull;
      if (listing == null) return fail('Choose a listing from this branch.');
      if (listing['status'] != 'ACTIVE') {
        return fail('Stock tracking requires an active listing.', 409);
      }
      final measure = (advanced.values[AdvancedKind.measures] ?? [])
          .where(
            (m) => m['id'] == body['measure_id'] && m['status'] == 'ACTIVE',
          )
          .firstOrNull;
      if (measure == null) return fail('Choose an active stock measure.');
      if (body['consumption_mode'] == 'EXACT_CONVERSION' &&
          measure['converts_exactly'] != true) {
        return fail(
          'This measure has no exact inventory conversion.',
          409,
          'INVENTORY_CONVERSION_UNRESOLVED',
        );
      }
      if (body['consumption_mode'] == 'SAME_UNIT') {
        final linked = advanced.values[AdvancedKind.listingUnits] ?? [];
        final eligible = (advanced.values[AdvancedKind.sellUnits] ?? []).any(
          (unit) =>
              unit['measure_ref'] == measure['id'] &&
              unit['status'] == 'ACTIVE' &&
              unit['conversion_type'] != 'CALIBRATED_APPROXIMATE' &&
              linked.any(
                (link) =>
                    link['listing_ref'] == listing['id'] &&
                    link['sell_unit_ref'] == unit['id'] &&
                    link['enabled'] == true,
              ),
        );
        if (!eligible) {
          return fail(
            'No enabled sell unit consumes this stock measure exactly.',
            409,
            'INVENTORY_CONVERSION_UNRESOLVED',
          );
        }
      }
      if (![
            'SAME_UNIT',
            'EXACT_CONVERSION',
            'VARIABLE_FULFILMENT',
          ].contains(body['consumption_mode']) ||
          !['FIFO', 'FEFO'].contains(body['allocation_policy'])) {
        return fail('Choose a valid stock policy.');
      }
      final id = 'stock-${listing['id']}';
      balances.putIfAbsent(
        id,
        () => {
          'stock_basis_ref': id,
          'branch_ref': branch,
          'listing_ref': listing['id'],
          'variant_ref': listing['variant_ref'],
          'on_hand_quantity': 0,
          'available_quantity': 0,
          'reserved_quantity': 0,
          'quarantined_quantity': 0,
          'in_transit_quantity': 0,
          'backorder_quantity': 0,
          'revision': 0,
          'updated_at': now,
        },
      );
      return ok({
        'id': id,
        'branch_ref': branch,
        'listing_ref': listing['id'],
        'variant_ref': listing['variant_ref'],
        'product_ref': listing['product_ref'],
        'measure_ref': body['measure_id'],
        'consumption_mode': body['consumption_mode'],
        'allocation_policy': body['allocation_policy'],
        'lot_tracking_enabled': body['lot_tracking'],
        'status': 'ACTIVE',
        'version': 1,
      });
    }
    if (path == '/api/v1/vendor/shop/inventory/receipts') {
      final lines = body['lines'];
      if (lines is! List || lines.isEmpty) {
        return fail('Add at least one receipt line.');
      }
      for (final line in lines) {
        if (!quantity(line['quantity']) ||
            balances[line['stock_basis_id']]?['branch_ref'] !=
                body['branch_id']) {
          return fail('Choose a valid stock item and quantity.');
        }
      }
      final id = 'receipt-${++sequence}';
      receipts[id] = {
        'id': id,
        'branch_ref': body['branch_id'],
        'source_ref': body['source_ref'],
        'received_at': body['received_at'],
        'status': 'DRAFT',
        'version': 1,
        'created_at': now,
        'updated_at': now,
        'lines': [
          for (final line in lines)
            {
              'id': 'line-${++sequence}',
              'stock_basis_ref': line['stock_basis_id'],
              'variant_ref': balances[line['stock_basis_id']]!['variant_ref'],
              'quantity': line['quantity'],
              for (final key in [
                'lot_number',
                'batch_number',
                'manufactured_at',
                'best_before_at',
                'expires_at',
              ])
                if (line[key] != null) key: line[key],
            },
        ],
      };
      return ok(copy(receipts[id]), 201);
    }
    final receipt = RegExp(
      r'/inventory/receipts/([^/]+)/(post|cancel)$',
    ).firstMatch(path);
    if (receipt != null) {
      final row = receipts[receipt[1]];
      if (row == null) return fail('Receipt not found.', 404);
      final target = receipt[2] == 'post' ? 'POSTED' : 'CANCELLED';
      if (row['status'] == target) return ok(copy(row));
      if (row['status'] == 'POSTED' && target == 'CANCELLED') {
        for (final line in row['lines']) {
          final lot = (lots[line['stock_basis_ref']] ?? [])
              .where((l) => l['id'] == line['lot_ref'])
              .firstOrNull;
          if (lot == null ||
              lot['quantity'] != line['quantity'] ||
              (lot['quarantined'] as int? ?? 0) > 0) {
            return fail(
              'Posted receipt stock is no longer intact; use an explicit adjustment.',
              409,
            );
          }
        }
        for (final line in row['lines']) {
          move(
            balances[line['stock_basis_ref']]!,
            -(line['quantity'] as int),
            'ADJUSTMENT_OUT',
            row['id'],
            lotId: line['lot_ref'],
          );
        }
        row['status'] = target;
        row['version'] += 1;
        return ok(copy(row));
      }
      if (row['status'] != 'DRAFT') {
        return fail('Only a draft receipt can perform this action.', 409);
      }
      if (target == 'POSTED') {
        for (final line in row['lines']) {
          line['lot_ref'] = 'lot-${++sequence}';
          move(
            balances[line['stock_basis_ref']]!,
            line['quantity'] as int,
            'RECEIPT',
            row['id'],
            expiresAt: line['expires_at'],
            lotId: line['lot_ref'],
          );
        }
        row['posted_at'] = now;
      }
      row['status'] = target;
      row['version'] += 1;
      return ok(copy(row));
    }
    final create = RegExp(
      r'/branches/([^/]+)/inventory/stocktakes$',
    ).firstMatch(path);
    if (create != null) {
      if (!['FULL_STOCKTAKE', 'CYCLE_COUNT'].contains(body['stocktake_type'])) {
        return fail('Choose a stocktake type.');
      }
      final branch = create[1]!, ids = body['stock_basis_ids'] as List? ?? [];
      final id = 'stocktake-${++sequence}';
      stocktakes[id] = {
        'id': id,
        'branch_ref': branch,
        'stocktake_type': body['stocktake_type'],
        'snapshot_revision': revisions[branch] ?? 0,
        'status': 'COUNTING',
        'started_at': now,
        'version': 1,
        'lines': [
          for (final row in balances.values)
            if (row['branch_ref'] == branch &&
                (ids.isEmpty || ids.contains(row['stock_basis_ref'])))
              {
                'stock_basis_ref': row['stock_basis_ref'],
                'variant_ref': row['variant_ref'],
                'expected_quantity': row['on_hand_quantity'],
                'variance_quantity': 0,
              },
        ],
      };
      return ok(copy(stocktakes[id]), 201);
    }
    final count = RegExp(
      r'/inventory/stocktakes/([^/]+)/lines/([^/]+)$',
    ).firstMatch(path);
    if (count != null) {
      final row = stocktakes[count[1]];
      if (row == null) return fail('Stocktake not found.', 404);
      if (row['status'] != 'COUNTING') {
        return fail('This stocktake no longer accepts counts.', 409);
      }
      if (!quantity(body['counted_quantity'], zero: true)) {
        return fail('Enter a non-negative whole quantity.');
      }
      final line = (row['lines'] as List)
          .where((r) => r['stock_basis_ref'] == count[2])
          .firstOrNull;
      if (line == null) return fail('Count line not found.', 404);
      line['counted_quantity'] = body['counted_quantity'];
      line['variance_quantity'] =
          body['counted_quantity'] - line['expected_quantity'];
      return ok({'counted': true});
    }
    final post = RegExp(
      r'/inventory/stocktakes/([^/]+)/post$',
    ).firstMatch(path);
    if (post != null) {
      final row = stocktakes[post[1]];
      if (row == null) return fail('Stocktake not found.', 404);
      if (row['status'] == 'POSTED') return ok(copy(row));
      if (!['COUNTING', 'READY'].contains(row['status'])) {
        return fail('Stocktake cannot be posted.', 409);
      }
      final lines = row['lines'] as List;
      if (lines.any((l) => l['counted_quantity'] == null)) {
        return fail('Count every stock item before posting.');
      }
      if (revisions[row['branch_ref']] != row['snapshot_revision']) {
        return fail(
          'Inventory moved after this snapshot. Start a new stocktake and recount.',
          409,
          'SHOP_INVENTORY_STALE_REVISION',
        );
      }
      for (final line in lines) {
        final balance = balances[line['stock_basis_ref']]!;
        if (line['counted_quantity'] <
            balance['reserved_quantity'] + balance['quarantined_quantity']) {
          return fail('Count is below reserved or quarantined stock.', 409);
        }
      }
      for (final line in lines) {
        final delta = line['counted_quantity'] - line['expected_quantity'];
        if (delta != 0) {
          move(
            balances[line['stock_basis_ref']]!,
            delta as int,
            'STOCKTAKE_CORRECTION',
            row['id'],
          );
        }
      }
      row['status'] = 'POSTED';
      row['version'] += 1;
      row['posted_at'] = now;
      return ok(copy(row));
    }
    final alert = RegExp(
      r'/inventory/alerts/([^/]+)/(acknowledge|resolve)$',
    ).firstMatch(path);
    if (alert != null) {
      final row = alerts.where((a) => a['id'] == alert[1]).firstOrNull;
      if (row == null) return fail('Alert not found.', 404);
      if (row['status'] != 'RESOLVED') {
        row['status'] = alert[2] == 'resolve' ? 'RESOLVED' : 'ACKNOWLEDGED';
      }
      row['updated_at'] = now;
      return ok(copy(row));
    }
    return fail('Inventory route not found.', 404);
  }
}

const adjustmentReasons = [
  'FOUND_STOCK',
  'DATA_CORRECTION',
  'DAMAGE',
  'WASTE',
  'SPOILAGE',
  'THEFT_LOSS',
  'EXPIRY',
  'MANUAL_CORRECTION',
  'OTHER_CONTROLLED',
];
