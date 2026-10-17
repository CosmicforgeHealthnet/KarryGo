import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'mock_recipes_store.dart';
import 'mock_purchasing_store.dart';

class MockProductionBatchesStore {
  MockProductionBatchesStore(this.recipes, this.stock, {this.jobExists});
  final bool Function(String id, String branch)? jobExists;
  final MockRecipesStore recipes;
  final MockPurchasingStore stock;
  final batches = <JsonObject>[];
  final components = <String, List<JsonObject>>{},
      packaging = <String, List<JsonObject>>{},
      reservations = <String, List<JsonObject>>{};
  final policies = <String, JsonObject>{}, outputs = <String, JsonObject>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String get vendor => recipes.vendor;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, int status) => {
    '_message': message,
    '_code': 'PRODUCTION_BATCH_ERROR',
    '_status': status,
  };
  JsonObject base(JsonObject row) => {
    'business_id': vendor,
    'version': 1,
    'created_at': now,
    'updated_at': now,
    ...row,
  };
  void bump(JsonObject r) {
    r['version'] = (r['version'] as int) + 1;
    r['updated_at'] = now;
  }

  JsonObject detail(JsonObject b) => {
    'batch': b,
    'components': components[b['id']] ?? [],
    'packaging': packaging[b['id']] ?? [],
  };
  String policyKey(String branch, Map body) =>
      '$branch/${body['target_type']}/${body['target_ref']}/${body['target_variant_ref'] ?? ''}';
  JsonObject policy(String branch, Map body) =>
      policies[policyKey(branch, body)] ??
      {
        'id': '',
        'business_id': vendor,
        'branch_id': branch,
        'target_type': body['target_type'],
        'target_ref': body['target_ref'],
        'target_variant_ref': body['target_variant_ref'] ?? '',
        'operating_model': 'ON_DEMAND',
        'output_inventory_numerator': 1,
        'output_inventory_denominator': 1,
        'status': 'ACTIVE',
        'version': 0,
      };
  JsonObject? balance(String branch, String item) =>
      (stock.inventory[branch] ?? [])
          .where((r) => r['inventory_item_id'] == item)
          .firstOrNull;
  ({int n, int d}) fraction(BigInt n, BigInt d) {
    final gcd = n.gcd(d);
    return (n: (n ~/ gcd).toInt(), d: (d ~/ gcd).toInt());
  }

  List<JsonObject> build(JsonObject batch, bool isPack) {
    final versionId = batch['recipe_version_id'];
    final lines =
        (isPack ? recipes.packaging : recipes.components)[versionId] ?? [];
    return [
      for (final line in lines)
        (() {
          final ref = line[isPack ? 'packaging_material_id' : 'ingredient_id'];
          final type = isPack ? 'packaging-materials' : 'ingredients';
          final master = recipes.materials.rows[type]!
              .where((r) => r['id'] == ref)
              .firstOrNull;
          if (master == null) {
            throw const FormatException('Material unavailable');
          }
          final config =
              recipes.materials.configs['$type/$ref/${batch['branch_id']}'];
          final control =
              '${config?['inventory_control_mode_override'] ?? ''}'.isEmpty
              ? master['inventory_control_mode']
              : config!['inventory_control_mode_override'];
          final controlled = control == 'INVENTORY_CONTROLLED';
          final exact = line['normalization_status'] == 'EXACT';
          var n =
              BigInt.from(
                line[exact
                    ? 'normalized_quantity_numerator'
                    : 'quantity_numerator'],
              ) *
              BigInt.from(batch['multiplier_numerator']);
          var d =
              BigInt.from(
                line[exact
                    ? 'normalized_quantity_denominator'
                    : 'quantity_denominator'],
              ) *
              BigInt.from(batch['multiplier_denominator']);
          final waste = isPack
              ? 0
              : (line['expected_waste_bps'] == 0
                    ? batch['recipe_expected_waste_bps']
                    : line['expected_waste_bps']);
          if (waste > 0) {
            n *= BigInt.from(10000);
            d *= BigInt.from(10000 - waste);
          }
          final quantity = fraction(n, d);
          if (controlled &&
              (config == null ||
                  config['enabled'] != true ||
                  config['production_eligible'] != true ||
                  '${config['inventory_item_id'] ?? ''}'.isEmpty ||
                  !exact)) {
            throw const FormatException(
              'Material requires exact production inventory mapping',
            );
          }
          final reserve = controlled && line['optional'] != true
              ? quantity.n
              : 0;
          if (reserve > 0 && quantity.d != 1) {
            throw const FormatException(
              'Inventory quantities must be exact whole units',
            );
          }
          return <String, dynamic>{
            'id': 'mock-batch-line-${++sequence}',
            isPack ? 'packaging_material_id' : 'ingredient_id': ref,
            isPack ? 'recipe_packaging_requirement_id' : 'recipe_component_id':
                line['id'],
            'unit_code': line[exact ? 'normalized_unit_code' : 'unit_code'],
            'planned_quantity_numerator': quantity.n,
            'planned_quantity_denominator': quantity.d,
            'expected_waste_bps': waste,
            'optional': line['optional'] ?? false,
            'sequence': line['sequence'],
            'inventory_controlled': controlled,
            'inventory_item_id': config?['inventory_item_id'] ?? '',
            'reserved_quantity': reserve,
            'actual_used_numerator': 0,
            'actual_used_denominator': 1,
            'actual_waste_numerator': 0,
            'actual_waste_denominator': 1,
            'actual_recorded': false,
            'variance_numerator': -quantity.n,
            'variance_denominator': quantity.d,
            if (isPack) 'production_consumption_point': 'PRODUCTION',
          };
        })(),
    ];
  }

  void reserve(JsonObject b, Map<String, int> quantities, String kind) {
    for (final entry in quantities.entries) {
      final row = balance(b['branch_id'], entry.key);
      if (row == null || row['available'] < entry.value) {
        throw const FormatException('Insufficient inventory');
      }
    }
    if (quantities.isEmpty) return;
    final rid = 'mock-production-reservation-${++sequence}';
    reservations.putIfAbsent(b['id'], () => []).add({
      'id': 'mock-batch-reservation-${++sequence}',
      'kind': kind,
      'sequence': (reservations[b['id']] ?? []).length,
      'inventory_reservation_id': rid,
      'source_ref': 'PRODUCTION_BATCH:${b['id']}',
      'status': 'ACTIVE',
      'created_at': now,
    });
    b['inventory_reservation_id'] ??= rid;
    for (final entry in quantities.entries) {
      final row = balance(b['branch_id'], entry.key)!;
      row['reserved'] += entry.value;
      row['available'] = row['on_hand'] - row['reserved'] - row['quarantined'];
      row['revision'] += 1;
    }
  }

  List<JsonObject> lines(JsonObject b) => [
    ...components[b['id']] ?? [],
    ...packaging[b['id']] ?? [],
  ];
  void release(JsonObject b, {bool consume = false}) {
    final totals = <String, int>{}, used = <String, int>{};
    for (final line in lines(b)) {
      if (line['inventory_controlled'] != true) continue;
      final item = '${line['inventory_item_id']}';
      totals[item] = (totals[item] ?? 0) + (line['reserved_quantity'] as int);
      if (consume && line['actual_recorded'] == true) {
        used[item] =
            (used[item] ?? 0) +
            (line['actual_used_numerator'] as int) +
            (line['actual_waste_numerator'] as int);
      }
    }
    for (final entry in totals.entries) {
      final row = balance(b['branch_id'], entry.key)!;
      row['reserved'] -= entry.value;
      row['on_hand'] -= used[entry.key] ?? 0;
      row['available'] = row['on_hand'] - row['reserved'] - row['quarantined'];
      row['revision'] += 1;
    }
    for (final row in reservations[b['id']] ?? <JsonObject>[]) {
      row['status'] = consume ? 'COMMITTED' : 'RELEASED';
    }
  }

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final policyMatch = RegExp(
      r'^/api/v1/vendor/production-stock-policies/branches/([^/]+)$',
    ).firstMatch(request.url.path);
    final m = RegExp(
      r'^/api/v1/vendor/production-batches(?:/([^/]+))?(?:/([^/]+))?$',
    ).firstMatch(request.url.path);
    if (policyMatch == null && m == null) return null;
    final id = m?[1],
        action = m?[2],
        b = batches.where((b) => b['id'] == id).firstOrNull;
    if (id != null && b == null) return fail('Batch was not found.', 404);
    if (request.method == 'GET') {
      if (policyMatch != null) {
        final branch = policyMatch[1]!;
        if (!stock.inventory.containsKey(branch)) {
          return fail('Branch was not found.', 404);
        }
        return ok(policy(branch, request.url.queryParameters));
      }
      if (id == null) {
        final branch = request.url.queryParameters['branch_id'],
            status = request.url.queryParameters['status'];
        final list = batches
            .where(
              (b) =>
                  (branch == null ||
                      branch.isEmpty ||
                      b['branch_id'] == branch) &&
                  (status == null || status.isEmpty || b['status'] == status),
            )
            .toList();
        final offset =
                int.tryParse(request.url.queryParameters['offset'] ?? '') ?? 0,
            limit =
                int.tryParse(request.url.queryParameters['limit'] ?? '') ?? 50;
        return ok({
          'items': list.skip(offset).take(limit).toList(),
          'pagination': {
            'limit': limit,
            'offset': offset,
            'has_more': list.length > offset + limit,
          },
        });
      }
      if (action == 'inventory') {
        return ok({'reservations': reservations[id] ?? []});
      }
      if (action == 'output') {
        return outputs[id] == null
            ? fail('No prepared output is available.', 404)
            : ok(outputs[id]);
      }
      return ok(detail(b!));
    }
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    if (key == null) return fail('An idempotency key is required.', 400);
    final fingerprint = jsonEncode([
      request.method,
      request.url.toString(),
      body,
      request.headers['If-Match'] ?? request.headers['if-match'],
    ]);
    final old = replay[key];
    if (old != null) {
      return old.fingerprint == fingerprint
          ? old.result
          : fail('This key was used for a different action.', 409);
    }
    final version = int.tryParse(
      request.headers['If-Match'] ?? request.headers['if-match'] ?? '',
    );
    if (b != null && action != 'preview' && version != b['version']) {
      return fail('The batch changed. Refresh before continuing.', 409);
    }
    JsonObject result;
    try {
      if (policyMatch != null) {
        final branch = policyMatch[1]!;
        final r = recipes.recipes
            .where(
              (r) =>
                  r['branch_id'] == branch &&
                  r['target_type'] == body['target_type'] &&
                  r['target_ref'] == body['target_ref'] &&
                  '${r['target_variant_ref'] ?? ''}' ==
                      '${body['target_variant_ref'] ?? ''}',
            )
            .firstOrNull;
        if (r == null) {
          return fail('Choose a recipe target belonging to this branch.', 400);
        }
        final current = policy(branch, body);
        if (version != current['version']) {
          return fail('The stock policy changed. Refresh before editing.', 409);
        }
        final n = body['output_inventory_numerator'],
            d = body['output_inventory_denominator'];
        if (n is! int ||
            d is! int ||
            n <= 0 ||
            d <= 0 ||
            !['ON_DEMAND', 'BATCH_STOCK'].contains(body['operating_model'])) {
          return fail('Invalid output conversion or operating model.', 400);
        }
        if (body['operating_model'] == 'BATCH_STOCK' &&
            body['prepared_inventory_item_id'] !=
                'mock-prepared-output-$branch') {
          return fail(
            'Choose an active production-component inventory item.',
            400,
          );
        }
        final next = base({
          ...body,
          'id': current['id'] == ''
              ? 'mock-stock-policy-${++sequence}'
              : current['id'],
          'branch_id': branch,
          'status': 'ACTIVE',
          'version': current['version'] + 1,
          'prepared_inventory_item_id': body['operating_model'] == 'ON_DEMAND'
              ? ''
              : body['prepared_inventory_item_id'],
        });
        policies[policyKey(branch, body)] = next;
        result = ok(next);
      } else if (id == null && request.method == 'POST') {
        final r = recipes.recipe(body['recipe_id']),
            v = recipes.version(body['recipe_version_id']);
        final n = body['planned_output_numerator'],
            d = body['planned_output_denominator'];
        if (r == null ||
            v == null ||
            r['status'] != 'ACTIVE' ||
            v['status'] != 'ACTIVE' ||
            v['recipe_id'] != r['id'] ||
            body['branch_id'] != r['branch_id'] ||
            n is! int ||
            d is! int ||
            n <= 0 ||
            d <= 0) {
          return fail(
            'Choose an active recipe version and valid planned output.',
            400,
          );
        }
        if ('${body['production_job_id'] ?? ''}'.isNotEmpty &&
            jobExists?.call(
                  '${body['production_job_id']}',
                  '${r['branch_id']}',
                ) !=
                true) {
          return fail('No matching Mock production job is available.', 404);
        }
        final p = policy(r['branch_id'], r);
        final multiplier = fraction(
          BigInt.from(n) * BigInt.from(v['expected_output_denominator']),
          BigInt.from(d) * BigInt.from(v['expected_output_numerator']),
        );
        final next = base({
          ...body,
          'id': 'mock-production-batch-${++sequence}',
          'recipe_version_number': v['version_number'],
          'target_type': r['target_type'],
          'target_ref': r['target_ref'],
          'target_variant_ref': r['target_variant_ref'] ?? '',
          'expected_output_numerator': v['expected_output_numerator'],
          'expected_output_denominator': v['expected_output_denominator'],
          'recipe_expected_waste_bps': v['expected_waste_bps'],
          'multiplier_numerator': multiplier.n,
          'multiplier_denominator': multiplier.d,
          'output_unit_code': v['output_unit_code'],
          'operating_model': p['operating_model'],
          'prepared_inventory_item_id': p['prepared_inventory_item_id'] ?? '',
          'output_inventory_numerator': p['output_inventory_numerator'],
          'output_inventory_denominator': p['output_inventory_denominator'],
          'status': 'DRAFT',
          'actual_consumption_recorded': false,
          'actual_output_numerator': 0,
          'actual_output_denominator': 1,
          'created_by_staff_id': 'mock-staff-1',
        });
        batches.add(next);
        result = ok(next, 201);
      } else if (action == 'preview' || action == 'plan') {
        if (b!['status'] != 'DRAFT') {
          return fail('Only draft batches can be planned.', 409);
        }
        final ingredient = build(b, false), packs = build(b, true);
        if (action == 'preview') {
          result = ok({
            'batch_id': id,
            'requirements': [
              for (final pack in [false, true])
                for (final row in pack ? packs : ingredient)
                  {
                    'kind': pack ? 'PACKAGING' : 'INGREDIENT',
                    'reference_id':
                        row[pack ? 'packaging_material_id' : 'ingredient_id'],
                    'unit_code': row['unit_code'],
                    'quantity_numerator': row['planned_quantity_numerator'],
                    'quantity_denominator': row['planned_quantity_denominator'],
                    'expected_waste_bps': row['expected_waste_bps'],
                    'inventory_controlled': row['inventory_controlled'],
                    'inventory_item_id': row['inventory_item_id'],
                    'reservation_quantity': row['reserved_quantity'],
                    'optional': row['optional'],
                    if (pack)
                      'production_consumption_point':
                          row['production_consumption_point'],
                  },
            ],
          });
        } else {
          final totals = <String, int>{};
          for (final row in [...ingredient, ...packs]) {
            if (row['reserved_quantity'] > 0) {
              totals[row['inventory_item_id']] =
                  (totals[row['inventory_item_id']] ?? 0) +
                  (row['reserved_quantity'] as int);
            }
          }
          reserve(b, totals, 'INITIAL');
          components[id!] = ingredient;
          packaging[id] = packs;
          b['status'] = 'PLANNED';
          b['planned_at'] = now;
          bump(b);
          result = ok(detail(b));
        }
      } else if (action == 'start') {
        if (b!['status'] != 'PLANNED') {
          return fail('Plan the batch before starting production.', 409);
        }
        b['status'] = 'IN_PROGRESS';
        b['started_at'] = now;
        bump(b);
        result = ok(detail(b));
      } else if (action == 'actual-consumption' || action == 'waste') {
        if (b!['status'] != 'IN_PROGRESS') {
          return fail('Start production before recording materials.', 409);
        }
        final list = body['kind'] == 'INGREDIENT'
            ? components[id]
            : body['kind'] == 'PACKAGING'
            ? packaging[id]
            : null;
        final line = list
            ?.where(
              (l) =>
                  l[body['kind'] == 'INGREDIENT'
                      ? 'ingredient_id'
                      : 'packaging_material_id'] ==
                  body['reference_id'],
            )
            .firstOrNull;
        if (line == null) {
          return fail('Choose a material frozen into this batch.', 400);
        }
        final n = body['quantity_numerator'], d = body['quantity_denominator'];
        if (n is! int ||
            d is! int ||
            n < 0 ||
            d <= 0 ||
            action == 'waste' && n <= 0) {
          return fail('Enter a valid material quantity.', 400);
        }
        final quantity = fraction(BigInt.from(n), BigInt.from(d));
        var usedN = line['actual_used_numerator'] as int,
            usedD = line['actual_used_denominator'] as int,
            wasteN = line['actual_waste_numerator'] as int,
            wasteD = line['actual_waste_denominator'] as int;
        if (action == 'actual-consumption') {
          usedN = quantity.n;
          usedD = quantity.d;
        } else {
          if ('${body['reason_code'] ?? ''}'.trim().isEmpty) {
            return fail('Enter the waste reason code.', 400);
          }
          final q = fraction(
            BigInt.from(wasteN) * BigInt.from(quantity.d) +
                BigInt.from(quantity.n) * BigInt.from(wasteD),
            BigInt.from(wasteD) * BigInt.from(quantity.d),
          );
          wasteN = q.n;
          wasteD = q.d;
        }
        if (line['inventory_controlled'] == true &&
            (usedD != 1 ||
                wasteD != 1 ||
                usedN + wasteN > line['reserved_quantity'])) {
          return fail(
            'Reserve sufficient whole inventory units before recording this quantity.',
            409,
          );
        }
        line.addAll({
          'actual_used_numerator': usedN,
          'actual_used_denominator': usedD,
          'actual_waste_numerator': wasteN,
          'actual_waste_denominator': wasteD,
        });
        if (action == 'actual-consumption') {
          line['actual_recorded'] = true;
          b['actual_consumption_recorded'] = true;
        }
        final variance = fraction(
          (BigInt.from(usedN) * BigInt.from(wasteD) +
                      BigInt.from(wasteN) * BigInt.from(usedD)) *
                  BigInt.from(line['planned_quantity_denominator']) -
              BigInt.from(line['planned_quantity_numerator']) *
                  BigInt.from(usedD) *
                  BigInt.from(wasteD),
          BigInt.from(usedD) *
              BigInt.from(wasteD) *
              BigInt.from(line['planned_quantity_denominator']),
        );
        line['variance_numerator'] = variance.n;
        line['variance_denominator'] = variance.d;
        bump(b);
        result = ok(detail(b));
      } else if (action == 'inventory-adjustments') {
        if (!['PLANNED', 'IN_PROGRESS'].contains(b!['status'])) {
          return fail(
            'Additional stock can only be reserved for a planned or running batch.',
            409,
          );
        }
        final input = body['lines'];
        if (input is! List || input.isEmpty) {
          return fail('Choose stock to reserve.', 400);
        }
        final totals = <String, int>{},
            changes = <({JsonObject line, int quantity})>[];
        for (final raw in input) {
          final entry = jsonObject(raw);
          final list = entry['kind'] == 'INGREDIENT'
              ? components[id]
              : entry['kind'] == 'PACKAGING'
              ? packaging[id]
              : null;
          final line = list
              ?.where(
                (l) =>
                    l[entry['kind'] == 'INGREDIENT'
                        ? 'ingredient_id'
                        : 'packaging_material_id'] ==
                    entry['reference_id'],
              )
              .firstOrNull;
          final quantity = entry['quantity'];
          if (line == null ||
              line['inventory_controlled'] != true ||
              quantity is! int ||
              quantity <= 0) {
            return fail(
              'Choose controlled material and additional whole inventory units.',
              400,
            );
          }
          totals[line['inventory_item_id']] =
              (totals[line['inventory_item_id']] ?? 0) + quantity;
          changes.add((line: line, quantity: quantity));
        }
        reserve(b, totals, 'TOP_UP');
        for (final entry in changes) {
          entry.line['reserved_quantity'] += entry.quantity;
        }
        bump(b);
        result = ok(detail(b));
      } else if (action == 'complete') {
        if ('${b!['production_job_id'] ?? ''}'.isNotEmpty &&
            jobExists?.call('${b['production_job_id']}', '${b['branch_id']}') !=
                true) {
          return fail(
            'The linked job is unavailable or no longer active.',
            409,
          );
        }
        if (b['status'] != 'IN_PROGRESS') {
          return fail('Only a running batch can be completed.', 409);
        }
        if (lines(b).any(
          (l) =>
              l['inventory_controlled'] == true &&
              l['optional'] != true &&
              l['actual_recorded'] != true,
        )) {
          return fail(
            'Record controlled material consumption before completing.',
            409,
          );
        }
        final n = body['actual_output_numerator'],
            d = body['actual_output_denominator'];
        if (n is! int || d is! int || n <= 0 || d <= 0) {
          return fail('Enter valid actual output.', 400);
        }
        JsonObject? output;
        if (b['operating_model'] == 'BATCH_STOCK') {
          final q = fraction(
            BigInt.from(n) * BigInt.from(b['output_inventory_numerator']),
            BigInt.from(d) * BigInt.from(b['output_inventory_denominator']),
          );
          if (q.d != 1 ||
              balance(b['branch_id'], b['prepared_inventory_item_id']) ==
                  null) {
            return fail(
              'Prepared output must map to an active inventory item in whole units.',
              400,
            );
          }
          output = {
            'id': 'mock-production-output-${++sequence}',
            'inventory_item_id': b['prepared_inventory_item_id'],
            'quantity': q.n,
            'inventory_source_ref': 'PRODUCTION_BATCH:$id',
            'created_at': now,
          };
        }
        release(b, consume: true);
        if (output != null) {
          final row = balance(b['branch_id'], output['inventory_item_id'])!;
          row['on_hand'] += output['quantity'];
          row['available'] =
              row['on_hand'] - row['reserved'] - row['quarantined'];
          row['revision'] += 1;
          outputs[id!] = output;
        }
        b['status'] = 'COMPLETED';
        b['actual_output_numerator'] = n;
        b['actual_output_denominator'] = d;
        b['completed_at'] = now;
        bump(b);
        result = ok(detail(b));
      } else if (action == 'cancel') {
        if (!['DRAFT', 'PLANNED', 'IN_PROGRESS'].contains(b!['status'])) {
          return fail('This batch cannot be cancelled.', 409);
        }
        final reason = '${body['reason'] ?? ''}'.trim();
        if (reason.isEmpty || reason.length > 1000) {
          return fail(
            'Enter a cancellation reason within 1,000 characters.',
            400,
          );
        }
        release(b);
        b['status'] = 'CANCELLED';
        b['cancel_reason'] = reason;
        b['cancelled_at'] = now;
        bump(b);
        result = ok(b);
      } else {
        return fail('This production action is unavailable.', 404);
      }
    } catch (_) {
      return fail(
        'Stock or exact conversion requirements could not be satisfied.',
        409,
      );
    }
    replay[key] = (fingerprint: fingerprint, result: result);
    return result;
  }
}
