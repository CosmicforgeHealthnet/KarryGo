import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import 'mock_production_batches_store.dart';

class MockCostsStore {
  MockCostsStore(this.batches) {
    batches.recipes.costProjection = recipeCost;
  }
  final MockProductionBatchesStore batches;
  final policies = <JsonObject>[];
  final snapshots = <JsonObject>[];
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, [int status = 422]) => {
    '_message': message,
    '_code': 'COST_ERROR',
    '_status': status,
  };
  int rounded(BigInt numerator, BigInt denominator) =>
      ((numerator + denominator ~/ BigInt.two) ~/ denominator).toInt();
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final p = request.url.path.replaceFirst('/api/v1/vendor/', '').split('/');
    if (!['cost', 'production-cost'].contains(p.first)) return null;
    if (request.method == 'GET') return read(p, request.url.queryParameters);
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    if (key == null || key.isEmpty) {
      return fail('A retry key is required.', 400);
    }
    final fingerprint = jsonEncode([
      request.method,
      request.url.toString(),
      body,
      request.headers['If-Match'] ?? request.headers['if-match'],
    ]);
    final saved = replay[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : fail('This key belongs to another cost change.', 409);
    }
    final result = write(p, request, body);
    if ((result['_status'] as int) < 300) {
      replay[key] = (fingerprint: fingerprint, result: result);
    }
    return result;
  }

  List<JsonObject> page(List<JsonObject> rows, Map<String, String> q) => rows
      .skip(int.tryParse(q['offset'] ?? '') ?? 0)
      .take((int.tryParse(q['limit'] ?? '') ?? 50).clamp(1, 100))
      .toList();
  JsonObject? latest(String id, String type) => snapshots
      .where((s) => s['batch_id'] == id && s['calculation_type'] == type)
      .lastOrNull;
  JsonObject read(List<String> p, Map<String, String> q) {
    if (p.first == 'cost' && p.length >= 2 && p[1] == 'policies') {
      if (p.length == 2) {
        final rows = policies
            .where(
              (r) =>
                  q['subject_type'] == null ||
                  q['subject_type'] == r['subject_type'],
            )
            .toList();
        return ok({
          'policies': page(rows, q),
          'limit': int.tryParse(q['limit'] ?? '') ?? 50,
          'offset': int.tryParse(q['offset'] ?? '') ?? 0,
        });
      }
      final r = policies.where((r) => r['id'] == p[2]).firstOrNull;
      return r == null ? fail('Policy not found.', 404) : ok(r);
    }
    if (p.first == 'cost' &&
        p.length == 4 &&
        p[1] == 'snapshots' &&
        p.last == 'components') {
      final r = [
        ...snapshots,
        ...batches.recipes.snapshots,
      ].where((r) => r['id'] == p[2]).firstOrNull;
      return r == null || (r['components'] as List).isEmpty
          ? fail('Component evidence not found.', 404)
          : ok({'components': r['components']});
    }
    if (p.first == 'cost' && p.last == 'highest-cost-subjects') {
      final grouped = <String, JsonObject>{};
      final window = [...snapshots, ...batches.recipes.snapshots]
          .where(
            (s) => q['branch_id'] == null || s['branch_id'] == q['branch_id'],
          )
          .toList()
          .reversed
          .take(500);
      for (final s in window) {
        for (final raw in s['components'] as List) {
          final line = jsonObject(raw);
          if (line['component_cost_minor'] == null) continue;
          final key =
              '${line['subject_type']}/${line['subject_ref']}/${line['currency']}';
          final r = grouped.putIfAbsent(
            key,
            () => {
              'subject_type': line['subject_type'],
              'subject_ref': line['subject_ref'],
              'currency': line['currency'],
              'total_cost_minor': 0,
              'observations': 0,
            },
          );
          r['total_cost_minor'] += line['component_cost_minor'];
          r['observations'] += 1;
        }
      }
      final rows = grouped.values.toList()
        ..sort(
          (a, b) => (b['total_cost_minor'] as int).compareTo(
            a['total_cost_minor'] as int,
          ),
        );
      return ok({
        'subjects': rows.take(int.tryParse(q['limit'] ?? '') ?? 50).toList(),
      });
    }
    if (p.first == 'production-cost' && p.last == 'history') {
      return ok({
        'snapshots': page(
          snapshots
              .where(
                (r) =>
                    q['branch_id'] == null || r['branch_id'] == q['branch_id'],
              )
              .toList()
              .reversed
              .toList(),
          q,
        ),
        'limit': int.tryParse(q['limit'] ?? '') ?? 50,
        'offset': int.tryParse(q['offset'] ?? '') ?? 0,
      });
    }
    if (p.first == 'production-cost' && p.length >= 3 && p[1] == 'batches') {
      final id = p[2],
          expected = latest(p[2], 'PRODUCTION_EXPECTED'),
          actual = latest(p[2], 'PRODUCTION_ACTUAL');
      if (p.last == 'variance') return ok(variance(id, expected, actual));
      if (expected == null && actual == null) {
        return fail('No batch cost snapshots are available.', 404);
      }
      return ok({'expected': ?expected, 'actual': ?actual});
    }
    return fail('Cost route not found.', 404);
  }

  JsonObject write(List<String> p, http.BaseRequest request, JsonObject body) {
    if (p.first == 'cost' && p.length >= 2 && p[1] == 'policies') {
      final create = p.length == 2 && request.method == 'POST';
      final r = create
          ? null
          : policies.where((r) => r['id'] == p[2]).firstOrNull;
      if (!create && r == null) return fail('Policy not found.', 404);
      if (!create &&
          int.tryParse(
                (request.headers['If-Match'] ??
                        request.headers['if-match'] ??
                        '')
                    .replaceAll('"', ''),
              ) !=
              r!['version']) {
        return fail('Policy changed. Refresh before editing.', 409);
      }
      if (p.last == 'archive') {
        r!['status'] = 'ARCHIVED';
        r['version'] += 1;
        r['updated_at'] = now;
        return ok(r);
      }
      if (!['INGREDIENT', 'PACKAGING'].contains(body['subject_type']) ||
          body['subject_ref'] is! String ||
          body['subject_ref'] == '' ||
          ![
            'LATEST_ACCEPTED_PURCHASE_COST',
            'WEIGHTED_AVERAGE_PURCHASE_COST',
            'MANUAL_CONFIGURED_COST',
          ].contains(body['strategy']) ||
          '${body['currency']}'.length != 3) {
        return fail('Check the policy values.');
      }
      final branch = body['branch_id'] ?? '';
      if (branch != '' &&
          !batches.recipes.materials.branchIds.contains(branch)) {
        return fail('Branch not available.', 403);
      }
      final limit = body['weighted_average_receipt_limit'] ?? 0;
      if (limit is! int || limit < 0 || limit > 200) {
        return fail('Receipt limit must be between 0 and 200.');
      }
      if (body['strategy'] == 'MANUAL_CONFIGURED_COST' &&
          (body['manual_unit_cost_minor'] is! int ||
              body['manual_unit_cost_minor'] < 0 ||
              body['manual_unit_code'] == null ||
              body['manual_unit_code'] == '')) {
        return fail('Enter a valid manual amount and unit.');
      }
      if (create &&
          policies.any(
            (v) =>
                v['status'] == 'ACTIVE' &&
                (v['branch_id'] ?? '') == branch &&
                v['subject_type'] == body['subject_type'] &&
                v['subject_ref'] == body['subject_ref'],
          )) {
        return fail(
          'An active policy already exists for this scope and material.',
          409,
        );
      }
      final record =
          r ??
          {
            'id': 'mock-cost-policy-${++sequence}',
            'branch_id': branch,
            'subject_type': body['subject_type'],
            'subject_ref': body['subject_ref'],
            'status': 'ACTIVE',
            'version': 1,
            'created_at': now,
          };
      for (final key in [
        'strategy',
        'currency',
        'manual_unit_cost_minor',
        'manual_unit_code',
        'manual_cost_reason',
      ]) {
        record[key] = body[key];
      }
      record['weighted_average_receipt_limit'] = limit == 0 ? 10 : limit;
      record['updated_at'] = now;
      if (create) {
        policies.add(record);
      } else {
        record['version'] += 1;
      }
      return ok(record, create ? 201 : 200);
    }
    if (p.first == 'production-cost' &&
        p.length == 4 &&
        p[1] == 'batches' &&
        ['expected', 'actual'].contains(p.last) &&
        request.method == 'POST') {
      final batch = batches.batches.where((b) => b['id'] == p[2]).firstOrNull;
      if (batch == null) return fail('Production batch not found.', 404);
      final result = calculate(batch, p.last == 'actual');
      snapshots.add(result);
      return ok(result, 201);
    }
    return fail('Cost route not found.', 404);
  }

  JsonObject calculate(
    JsonObject batch,
    bool actual, {
    List<JsonObject>? ingredients,
    List<JsonObject>? packages,
    bool applyWaste = false,
  }) {
    final lines = <JsonObject>[];
    for (final pack in [false, true]) {
      for (final row
          in (pack ? packages : ingredients) ??
              (pack ? batches.packaging : batches.components)[batch['id']] ??
              <JsonObject>[]) {
        final type = pack ? 'PACKAGING' : 'INGREDIENT',
            ref = row[pack ? 'packaging_material_id' : 'ingredient_id'];
        final branch = batch['branch_id'];
        final eligible = policies
            .where(
              (p) =>
                  p['status'] == 'ACTIVE' &&
                  p['subject_type'] == type &&
                  p['subject_ref'] == ref &&
                  ((p['branch_id'] ?? '') == '' || p['branch_id'] == branch),
            )
            .toList();
        final policy =
            eligible.where((p) => p['branch_id'] == branch).firstOrNull ??
            eligible.firstOrNull;
        var n = BigInt.from(
              row[actual
                      ? 'actual_used_numerator'
                      : 'planned_quantity_numerator'] ??
                  0,
            ),
            d = BigInt.from(
              row[actual
                      ? 'actual_used_denominator'
                      : 'planned_quantity_denominator'] ??
                  1,
            );
        if (actual && !pack) {
          final wn = BigInt.from(row['actual_waste_numerator'] ?? 0),
              wd = BigInt.from(row['actual_waste_denominator'] ?? 1);
          n = n * wd + wn * d;
          d = d * wd;
        }
        final netN = n, netD = d;
        final waste = row['expected_waste_bps'] ?? 0;
        if (applyWaste && waste > 0) {
          n *= BigInt.from(10000);
          d *= BigInt.from(10000 - waste);
        }
        final gcd = n.gcd(d);
        n = n ~/ gcd;
        d = d ~/ gcd;
        final line = <String, dynamic>{
          'line_number': lines.length + 1,
          'subject_type': type,
          'subject_ref': ref,
          'quantity_numerator': n.toInt(),
          'quantity_denominator': d.toInt(),
          'quantity_unit_code': row['unit_code'],
          'expected_waste_bps': row['expected_waste_bps'] ?? 0,
          'cost_source': 'UNRESOLVED',
          if (row['inventory_item_id'] != null)
            'inventory_item_id': row['inventory_item_id'],
        };
        int? unit;
        String? evidence, reason;
        int count = 0;
        if (policy == null) {
          reason = 'NO_POLICY';
        } else if (policy['strategy'] == 'MANUAL_CONFIGURED_COST') {
          unit = policy['manual_unit_cost_minor'];
          evidence = policy['id'];
          if (unit == null) reason = 'NO_MANUAL_COST';
        } else if (row['inventory_item_id'] == null ||
            row['inventory_item_id'] == '') {
          reason = 'NO_INVENTORY_MAPPING';
        } else {
          final observations = batches.stock.costs
              .where(
                (c) =>
                    c['branch_id'] == branch &&
                    c['inventory_item_id'] == row['inventory_item_id'] &&
                    c['evidence_type'] == 'OBSERVED' &&
                    c['inventory_unit_cost_minor'] is int &&
                    !batches.stock.receipts.any(
                      (r) =>
                          r['id'] == c['goods_receipt_id'] &&
                          r['status'] == 'REVERSED',
                    ),
              )
              .toList()
              .reversed
              .take(
                policy['strategy'] == 'LATEST_ACCEPTED_PURCHASE_COST'
                    ? 1
                    : policy['weighted_average_receipt_limit'] as int,
              )
              .toList();
          if (observations.isEmpty) {
            reason = 'NO_PURCHASE_EVIDENCE';
          } else if (observations.any(
            (c) => c['currency'] != policy['currency'],
          )) {
            reason = 'CURRENCY_MISMATCH';
          } else {
            var total = BigInt.zero, weight = BigInt.zero;
            for (final c in observations) {
              final q = BigInt.from(c['inventory_quantity']);
              total += q * BigInt.from(c['inventory_unit_cost_minor']);
              weight += q;
            }
            if (weight > BigInt.zero) {
              unit = rounded(total, weight);
              evidence = observations.first['id'];
              count = observations.length;
            } else {
              reason = 'NO_PURCHASE_EVIDENCE';
            }
          }
        }
        if (reason != null) {
          line['incomplete_reason'] = reason;
        } else {
          line.addAll({
            'cost_source': policy!['strategy'],
            'source_evidence_ref': evidence,
            'source_receipt_count': count,
            'unit_cost_minor': unit,
            'currency': policy['currency'],
            'component_cost_minor': rounded(n * BigInt.from(unit!), d),
            if (applyWaste && waste > 0)
              'waste_cost_minor':
                  rounded(n * BigInt.from(unit), d) -
                  rounded(netN * BigInt.from(unit), netD),
          });
        }
        lines.add(line);
      }
    }
    final resolved = lines
        .where((l) => l['component_cost_minor'] != null)
        .toList();
    final currencies = resolved.map((l) => l['currency']).toSet(),
        strategies = resolved.map((l) => l['cost_source']).toSet();
    final total = resolved.fold<int>(
      0,
      (t, l) => t + l['component_cost_minor'] as int,
    );
    final single = currencies.length == 1;
    final result = <String, dynamic>{
      'id': 'mock-production-cost-${++sequence}',
      'branch_id': batch['branch_id'],
      'batch_id': batch['id'],
      'recipe_id': batch['recipe_id'],
      'recipe_version_id': batch['recipe_version_id'],
      'calculation_type': actual ? 'PRODUCTION_ACTUAL' : 'PRODUCTION_EXPECTED',
      'calculation_version': 1,
      'completeness': resolved.isEmpty
          ? 'UNKNOWN'
          : resolved.length == lines.length && single
          ? 'COMPLETE'
          : 'INCOMPLETE',
      'strategy': strategies.length == 1 ? strategies.first : 'MIXED',
      'currency': single ? currencies.first : '',
      'calculated_at': now,
      'calculated_by_staff_id': 'mock-staff-1',
      'expected_waste_bps': 0,
      'components': lines,
    };
    if (resolved.isNotEmpty && single) {
      result.addAll({
        'total_cost_minor': total,
        'ingredient_cost_minor': resolved
            .where((l) => l['subject_type'] == 'INGREDIENT')
            .fold<int>(0, (t, l) => t + l['component_cost_minor'] as int),
        'packaging_cost_minor': resolved
            .where((l) => l['subject_type'] == 'PACKAGING')
            .fold<int>(0, (t, l) => t + l['component_cost_minor'] as int),
        'waste_cost_minor': resolved.fold<int>(
          0,
          (t, l) => t + (l['waste_cost_minor'] as int? ?? 0),
        ),
      });
    }
    final on = batch['actual_output_numerator'],
        od = batch['actual_output_denominator'];
    if (on is int && on > 0 && od is int && od > 0) {
      result.addAll({
        'output_quantity_numerator': on,
        'output_quantity_denominator': od,
      });
      if (result['total_cost_minor'] != null) {
        result['cost_per_unit_minor'] = rounded(
          BigInt.from(total) * BigInt.from(od),
          BigInt.from(on),
        );
      }
    }
    return result;
  }

  JsonObject recipeCost(JsonObject source) {
    final ingredients = <JsonObject>[], packages = <JsonObject>[];
    for (final raw in source['components'] as List) {
      final line = jsonObject(raw), pack = line['subject_type'] == 'PACKAGING';
      final kind = pack ? 'packaging-materials' : 'ingredients';
      final config = batches
          .recipes
          .materials
          .configs['$kind/${line['subject_ref']}/${source['branch_id']}'];
      (pack ? packages : ingredients).add({
        pack ? 'packaging_material_id' : 'ingredient_id': line['subject_ref'],
        'planned_quantity_numerator': line['quantity_numerator'],
        'planned_quantity_denominator': line['quantity_denominator'],
        'unit_code': line['quantity_unit_code'],
        'expected_waste_bps': line['expected_waste_bps'],
        'inventory_item_id': config?['inventory_item_id'],
      });
    }
    final calculated = calculate(
      {
        'id': 'recipe-preview',
        'branch_id': source['branch_id'],
        'recipe_id': source['recipe_id'],
        'recipe_version_id': source['recipe_version_id'],
      },
      false,
      ingredients: ingredients,
      packages: packages,
      applyWaste: true,
    );
    final servings = source['serving_count'];
    if (servings is int &&
        servings > 0 &&
        calculated['total_cost_minor'] != null) {
      calculated['cost_per_serving_minor'] = rounded(
        BigInt.from(calculated['total_cost_minor']),
        BigInt.from(servings),
      );
    }
    return {
      ...source,
      ...calculated,
      'id': '',
      'calculation_type': 'RECIPE_PREVIEW',
    }..remove('batch_id');
  }

  JsonObject variance(String id, JsonObject? expected, JsonObject? actual) {
    final comparable =
        expected != null &&
        actual != null &&
        expected['currency'] == actual['currency'];
    final rows = <String, JsonObject>{};
    for (final pair in [('expected', expected), ('actual', actual)]) {
      for (final raw in pair.$2?['components'] as List? ?? []) {
        final c = jsonObject(raw),
            key = '${c['subject_type']}/${c['subject_ref']}';
        final r = rows.putIfAbsent(
          key,
          () => {
            'subject_type': c['subject_type'],
            'subject_ref': c['subject_ref'],
            'expected_quantity_numerator': 0,
            'expected_quantity_denominator': 0,
            'actual_quantity_numerator': 0,
            'actual_quantity_denominator': 0,
          },
        );
        r['${pair.$1}_quantity_numerator'] = c['quantity_numerator'];
        r['${pair.$1}_quantity_denominator'] = c['quantity_denominator'];
        if (c['component_cost_minor'] != null) {
          r['${pair.$1}_cost_minor'] = c['component_cost_minor'];
        }
      }
    }
    if (comparable) {
      for (final r in rows.values) {
        if (r['expected_cost_minor'] != null &&
            r['actual_cost_minor'] != null) {
          r['variance_minor'] =
              r['actual_cost_minor'] - r['expected_cost_minor'];
        }
      }
    }
    return {
      'batch_id': id,
      'currency': expected?['currency'] ?? actual?['currency'] ?? '',
      'completeness':
          comparable &&
              expected['completeness'] == 'COMPLETE' &&
              actual['completeness'] == 'COMPLETE'
          ? 'COMPLETE'
          : 'INCOMPLETE',
      if (expected?['total_cost_minor'] != null)
        'expected_total_minor': expected!['total_cost_minor'],
      if (actual?['total_cost_minor'] != null)
        'actual_total_minor': actual!['total_cost_minor'],
      if (comparable &&
          expected['total_cost_minor'] != null &&
          actual['total_cost_minor'] != null)
        'variance_minor':
            actual['total_cost_minor'] - expected['total_cost_minor'],
      'components': comparable ? rows.values.toList() : [],
    };
  }
}
