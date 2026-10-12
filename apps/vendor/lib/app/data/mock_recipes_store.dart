import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import '../../features/catalogue/state/catalogue_resource.dart';
import 'mock_catalogue_store.dart';
import 'mock_production_materials_store.dart';

class MockRecipesStore {
  MockRecipesStore(
    this.vendor,
    this.catalogue,
    this.materials, {
    required bool empty,
  }) {
    final food = catalogue.advancedRecords(CatalogueKind.items).firstOrNull;
    if (empty || food == null) return;
    final menu = catalogue.advancedRecord(
      CatalogueKind.menus,
      '${food['menu_id']}',
    );
    if (menu == null) return;
    recipes.add(
      base({
        'id': 'mock-recipe-rice',
        'branch_id': menu['branch_id'],
        'target_type': 'FOOD_ITEM',
        'target_ref': food['id'],
        'name': 'Rice preparation',
        'status': 'ACTIVE',
      }),
    );
    final v = base({
      'id': 'mock-recipe-version',
      'recipe_id': 'mock-recipe-rice',
      'version_number': 1,
      'status': 'DRAFT',
      'internal_notes': 'Kitchen preparation standard.',
      'expected_output_numerator': 1,
      'expected_output_denominator': 1,
      'output_unit_code': 'SERVING',
      'serving_count': 1,
      'expected_waste_bps': 0,
    });
    versions.add(v);
    components[v['id']] = [
      base({
        'id': 'mock-recipe-component',
        'recipe_version_id': v['id'],
        'ingredient_id': 'mock-production-rice',
        'quantity_numerator': 250,
        'quantity_denominator': 1,
        'unit_code': 'G',
        'expected_waste_bps': 0,
        'optional': false,
        'sequence': 0,
        'normalization_status': 'UNRESOLVED',
        'normalized_quantity_numerator': 0,
        'normalized_quantity_denominator': 1,
      }),
    ];
  }
  JsonObject Function(JsonObject)? costProjection;
  final String vendor;
  final MockCatalogueStore catalogue;
  final MockProductionMaterialsStore materials;
  final recipes = <JsonObject>[],
      versions = <JsonObject>[],
      snapshots = <JsonObject>[];
  final components = <String, List<JsonObject>>{},
      packaging = <String, List<JsonObject>>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject base(JsonObject r) => {
    'business_id': vendor,
    'version': 1,
    'created_at': now,
    'updated_at': now,
    ...r,
  };
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, int status) => {
    '_message': message,
    '_code': 'RECIPE_ERROR',
    '_status': status,
  };
  JsonObject view(JsonObject v) => {
    'version': v,
    'components': components[v['id']] ?? [],
    'packaging_requirements': packaging[v['id']] ?? [],
  };
  JsonObject? recipe(String? id) =>
      recipes.where((r) => r['id'] == id).firstOrNull;
  JsonObject? version(String? id) =>
      versions.where((r) => r['id'] == id).firstOrNull;
  void bump(JsonObject r) {
    r['version'] = (r['version'] as int) + 1;
    r['updated_at'] = now;
  }

  List<JsonObject> issues(JsonObject r, JsonObject v, {bool persist = false}) {
    final result = <JsonObject>[];
    void issue(String code, String field, String message) =>
        result.add({'code': code, 'field': field, 'message': message});
    final target = catalogue.advancedRecord(
      CatalogueKind.items,
      '${r['target_ref']}',
    );
    if (target == null || target['status'] == 'ARCHIVED') {
      issue(
        'TARGET_INVALID',
        'recipe.target',
        'The catalogue target is no longer valid.',
      );
    }
    if ((components[v['id']] ?? []).isEmpty) {
      issue(
        'COMPONENTS_REQUIRED',
        'components',
        'Add at least one ingredient before activation.',
      );
    }
    for (final isPack in [false, true]) {
      for (final line
          in (isPack ? packaging : components)[v['id']] ?? <JsonObject>[]) {
        final type = isPack ? 'packaging-materials' : 'ingredients';
        final ref = line[isPack ? 'packaging_material_id' : 'ingredient_id'];
        final master = materials.rows[type]!
            .where((m) => m['id'] == ref)
            .firstOrNull;
        if (master == null || master['status'] != 'ACTIVE') {
          issue(
            'MATERIAL_NOT_ACTIVE',
            type,
            'The material must exist and be active.',
          );
          continue;
        }
        final config = materials.configs['$type/$ref/${r['branch_id']}'];
        final control =
            '${config?['inventory_control_mode_override'] ?? ''}'.isEmpty
            ? master['inventory_control_mode']
            : config!['inventory_control_mode_override'];
        if (control == 'INVENTORY_CONTROLLED' &&
            (config == null ||
                config['enabled'] != true ||
                config['production_eligible'] != true ||
                '${config['inventory_item_id'] ?? ''}'.isEmpty)) {
          issue(
            'INVENTORY_MAPPING_REQUIRED',
            type,
            'Configure an enabled production stock mapping for this material.',
          );
          continue;
        }
        int n = line['quantity_numerator'], d = line['quantity_denominator'];
        String normalized = 'UNRESOLVED';
        if (line['unit_code'] == master['default_unit_code']) {
          normalized = 'EXACT';
        } else {
          final conversion = (materials.conversions['$type/$ref'] ?? [])
              .where(
                (c) =>
                    c['from_unit_code'] == line['unit_code'] &&
                    c['to_unit_code'] == master['default_unit_code'] &&
                    c['conversion_mode'] == 'EXACT_AUTHORITATIVE' &&
                    c['status'] == 'ACTIVE',
              )
              .firstOrNull;
          if (conversion != null) {
            final num = BigInt.from(n) * BigInt.from(conversion['numerator']);
            final den = BigInt.from(d) * BigInt.from(conversion['denominator']);
            final gcd = num.gcd(den);
            n = (num ~/ gcd).toInt();
            d = (den ~/ gcd).toInt();
            normalized = 'EXACT';
          }
        }
        if (control == 'INVENTORY_CONTROLLED' && normalized != 'EXACT') {
          issue(
            'EXACT_CONVERSION_REQUIRED',
            type,
            'An exact conversion to the default unit is required.',
          );
          continue;
        }
        if (persist) {
          line['normalization_status'] = normalized;
          line['normalized_quantity_numerator'] = normalized == 'EXACT' ? n : 0;
          line['normalized_quantity_denominator'] = normalized == 'EXACT'
              ? d
              : 1;
          line['normalized_unit_code'] = normalized == 'EXACT'
              ? master['default_unit_code']
              : '';
        }
      }
    }
    return result;
  }

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    if (request.url.path.startsWith('/api/v1/vendor/recipe-cost/')) {
      return cost(request);
    }
    final m = RegExp(
      r'^/api/v1/vendor/recipes(?:/([^/]+))?(?:/(archive|versions))?(?:/([^/]+))?(?:/(components|packaging|validate|activate|retire|scale))?$',
    ).firstMatch(request.url.path);
    if (m == null) return null;
    final id = m[1], area = m[2], vid = m[3], action = m[4];
    final r = recipe(id), v = version(vid);
    if (id != null && r == null) return fail('Recipe was not found.', 404);
    if (vid != null && (v == null || v['recipe_id'] != id)) {
      return fail('Recipe version was not found.', 404);
    }
    if (request.method == 'GET') {
      if (id == null) {
        final q = (request.url.queryParameters['q'] ?? '').toLowerCase(),
            status = request.url.queryParameters['status'];
        final list = recipes
            .where(
              (r) =>
                  (status == null || status.isEmpty || r['status'] == status) &&
                  '${r['name']}'.toLowerCase().contains(q),
            )
            .toList();
        return ok(
          list
              .skip(
                int.tryParse(request.url.queryParameters['offset'] ?? '') ?? 0,
              )
              .take(
                int.tryParse(request.url.queryParameters['limit'] ?? '') ?? 50,
              )
              .toList(),
        );
      }
      if (area == 'versions') {
        return vid == null
            ? ok(versions.where((v) => v['recipe_id'] == id).toList())
            : ok(view(v!));
      }
      return ok(r);
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
    final cached = replay[key];
    if (cached != null) {
      return cached.fingerprint == fingerprint
          ? cached.result
          : fail('This request key was used for a different change.', 409);
    }
    if ((area == 'archive' ||
            vid != null && request.method == 'PUT' ||
            action == 'activate' ||
            action == 'retire') &&
        int.tryParse(
              request.headers['If-Match'] ?? request.headers['if-match'] ?? '',
            ) !=
            (vid == null ? r!['version'] : v!['version'])) {
      return fail('The recipe record changed. Refresh before editing.', 409);
    }
    if (r?['status'] == 'ARCHIVED' &&
        action != 'validate' &&
        action != 'scale') {
      return fail('This recipe is archived.', 409);
    }
    JsonObject result;
    if (id == null && request.method == 'POST') {
      if (body['target_type'] != 'FOOD_ITEM') {
        return fail('This Mock catalogue target is not available.', 404);
      }
      final food = catalogue.advancedRecord(
        CatalogueKind.items,
        '${body['target_ref']}',
      );
      final menu = food == null
          ? null
          : catalogue.advancedRecord(CatalogueKind.menus, '${food['menu_id']}');
      if (food == null || menu == null) {
        return fail('Choose a valid catalogue target.', 400);
      }
      if ('${body['target_variant_ref'] ?? ''}'.isNotEmpty &&
          !catalogue
              .advancedRecords(CatalogueKind.foodVariants)
              .any(
                (v) =>
                    v['id'] == body['target_variant_ref'] &&
                    v['item_id'] == food['id'],
              )) {
        return fail('Choose a valid item variant.', 400);
      }
      if ('${body['name'] ?? ''}'.trim().isEmpty ||
          '${body['name']}'.length > 200 ||
          '${body['internal_description'] ?? ''}'.length > 4000) {
        return fail('Recipe details are invalid.', 400);
      }
      if (recipes.any(
        (r) =>
            r['target_type'] == body['target_type'] &&
            r['target_ref'] == body['target_ref'] &&
            '${r['target_variant_ref'] ?? ''}' ==
                '${body['target_variant_ref'] ?? ''}',
      )) {
        return fail('A recipe already exists for this target.', 409);
      }
      final next = base({
        ...body,
        'id': 'mock-recipe-${++sequence}',
        'branch_id': menu['branch_id'],
        'status': 'ACTIVE',
      });
      recipes.add(next);
      result = ok(next, 201);
    } else if (area == 'archive') {
      if (versions.any(
        (v) => v['recipe_id'] == id && v['status'] == 'ACTIVE',
      )) {
        return fail(
          'Retire the active version before archiving this recipe.',
          409,
        );
      }
      r!['status'] = 'ARCHIVED';
      bump(r);
      result = ok(r);
    } else if (area == 'versions' && vid == null && request.method == 'POST' ||
        vid != null && action == null && request.method == 'PUT') {
      final num = body['expected_output_numerator'],
          den = body['expected_output_denominator'],
          waste = body['expected_waste_bps'];
      if (num is! int ||
          den is! int ||
          num <= 0 ||
          den <= 0 ||
          '${body['output_unit_code'] ?? ''}'.isEmpty ||
          '${body['output_unit_code']}'.length > 80 ||
          (body['serving_count'] ?? 0) < 0 ||
          waste is! int ||
          waste < 0 ||
          waste >= 10000 ||
          '${body['internal_notes'] ?? ''}'.length > 4000) {
        return fail('Draft yield details are invalid.', 400);
      }
      if (v != null && v['status'] != 'DRAFT') {
        return fail('Only draft versions can be edited.', 409);
      }
      if (v == null) {
        final next = base({
          ...body,
          'id': 'mock-version-${++sequence}',
          'recipe_id': id,
          'version_number':
              versions.where((v) => v['recipe_id'] == id).length + 1,
          'status': 'DRAFT',
        });
        versions.add(next);
        result = ok(next, 201);
      } else {
        v.addAll(body);
        bump(v);
        result = ok(v);
      }
    } else if (action == 'components' || action == 'packaging') {
      if (v!['status'] != 'DRAFT') {
        return fail('Only draft requirements can be edited.', 409);
      }
      final isPack = action == 'packaging';
      final input = body[isPack ? 'requirements' : 'components'];
      if (input is! List || input.length > (isPack ? 100 : 500)) {
        return fail('Invalid requirement list.', 400);
      }
      final seen = <String>{};
      final next = <JsonObject>[];
      for (final raw in input) {
        final line = jsonObject(raw);
        final ref = line[isPack ? 'packaging_material_id' : 'ingredient_id'];
        final master = materials
            .rows[isPack ? 'packaging-materials' : 'ingredients']!
            .where((m) => m['id'] == ref)
            .firstOrNull;
        if (master == null ||
            !seen.add('$ref') ||
            line['quantity_numerator'] is! int ||
            line['quantity_denominator'] is! int ||
            line['quantity_numerator'] <= 0 ||
            line['quantity_denominator'] <= 0 ||
            '${line['unit_code'] ?? ''}'.isEmpty ||
            (line['sequence'] ?? 0) < 0 ||
            (line['expected_waste_bps'] ?? 0) < 0 ||
            (line['expected_waste_bps'] ?? 0) >= 10000) {
          return fail(
            'Requirements need valid materials and positive quantities.',
            400,
          );
        }
        next.add(
          base({
            ...line,
            'id': 'mock-requirement-${++sequence}',
            'recipe_version_id': vid,
            'normalization_status': 'UNRESOLVED',
            'normalized_quantity_numerator': 0,
            'normalized_quantity_denominator': 1,
          }),
        );
      }
      (isPack ? packaging : components)[vid!] = next;
      bump(v);
      result = ok(view(v));
    } else if (action == 'validate') {
      final errors = issues(r!, v!);
      result = ok({'valid': errors.isEmpty, 'issues': errors});
    } else if (action == 'activate') {
      if (v!['status'] != 'DRAFT') {
        return fail('Only a draft version can be activated.', 409);
      }
      if (issues(r!, v).isNotEmpty) {
        return fail('Resolve the recipe activation issues first.', 422);
      }
      issues(r, v, persist: true);
      for (final old in versions.where(
        (old) => old['recipe_id'] == id && old['status'] == 'ACTIVE',
      )) {
        old['status'] = 'RETIRED';
        old['retired_at'] = now;
        bump(old);
      }
      v['status'] = 'ACTIVE';
      v['activated_at'] = now;
      bump(v);
      result = ok(v);
    } else if (action == 'retire') {
      if (v!['status'] != 'ACTIVE') {
        return fail('Only an active version can be retired.', 409);
      }
      v['status'] = 'RETIRED';
      v['retired_at'] = now;
      bump(v);
      result = ok(v);
    } else if (action == 'scale') {
      final num = body['output_numerator'], den = body['output_denominator'];
      if (num is! int ||
          den is! int ||
          num <= 0 ||
          den <= 0 ||
          body['output_unit_code'] != v!['output_unit_code']) {
        return fail('Use a positive output in the recipe output unit.', 400);
      }
      final mn =
              BigInt.from(num) * BigInt.from(v['expected_output_denominator']),
          md = BigInt.from(den) * BigInt.from(v['expected_output_numerator']);
      final gcd = mn.gcd(md);
      result = ok({
        'recipe_version_id': vid,
        'multiplier_numerator': (mn ~/ gcd).toInt(),
        'multiplier_denominator': (md ~/ gcd).toInt(),
        'requirements': [
          for (final isPack in [false, true])
            for (final line
                in (isPack ? packaging : components)[vid] ?? <JsonObject>[])
              {
                'kind': isPack ? 'PACKAGING' : 'INGREDIENT',
                'reference_id':
                    line[isPack ? 'packaging_material_id' : 'ingredient_id'],
                'quantity_numerator':
                    (BigInt.from(line['quantity_numerator']) * mn).toInt(),
                'quantity_denominator':
                    (BigInt.from(line['quantity_denominator']) * md).toInt(),
                'unit_code': line['unit_code'],
                'expected_waste_bps': line['expected_waste_bps'] ?? 0,
                'normalization_status': line['normalization_status'],
              },
        ],
      });
    } else {
      return fail('This recipe action is unavailable.', 404);
    }
    replay[key] = (fingerprint: fingerprint, result: result);
    return result;
  }

  JsonObject? cost(http.BaseRequest request) {
    final m = RegExp(
      r'^/api/v1/vendor/recipe-cost/(recipes|versions)/([^/]+)(?:/(history|latest|snapshots))?$',
    ).firstMatch(request.url.path);
    if (m == null) return null;
    final area = m[1], id = m[2], action = m[3];
    JsonObject? v;
    if (area == 'recipes') {
      v = versions
          .where((v) => v['recipe_id'] == id && v['status'] == 'ACTIVE')
          .firstOrNull;
    } else {
      v = version(id);
    }
    if (action == 'history') {
      if (recipe(id) == null) return fail('Recipe was not found.', 404);
      final list = snapshots
          .where(
            (s) =>
                s['recipe_id'] == id &&
                (request.url.queryParameters['recipe_version_id'] == null ||
                    s['recipe_version_id'] ==
                        request.url.queryParameters['recipe_version_id']),
          )
          .toList();
      return ok({
        'snapshots': list
            .skip(
              int.tryParse(request.url.queryParameters['offset'] ?? '') ?? 0,
            )
            .take(
              int.tryParse(request.url.queryParameters['limit'] ?? '') ?? 50,
            )
            .toList(),
        'limit': 50,
        'offset': 0,
      });
    }
    if (v == null) return fail('No matching recipe version is available.', 404);
    final r = recipe(v['recipe_id'])!;
    if (action == 'latest') {
      final latest = snapshots
          .where((s) => s['recipe_version_id'] == id)
          .lastOrNull;
      return latest == null
          ? fail('No cost snapshot is available.', 404)
          : ok(latest);
    }
    if (request.url.queryParameters['branch_id'] != r['branch_id']) {
      return fail('Choose the recipe branch for costing.', 403);
    }
    var result = <String, dynamic>{
      'id': '',
      'branch_id': r['branch_id'],
      'calculation_type': 'RECIPE_PREVIEW',
      'calculation_version': 1,
      'recipe_id': r['id'],
      'recipe_version_id': v['id'],
      'strategy': 'LATEST_ACCEPTED_PURCHASE_COST',
      'currency': 'NGN',
      'completeness': 'INCOMPLETE',
      'output_quantity_numerator': v['expected_output_numerator'],
      'output_quantity_denominator': v['expected_output_denominator'],
      'serving_count': v['serving_count'] ?? 0,
      'expected_waste_bps': v['expected_waste_bps'],
      'calculated_at': now,
      'calculated_by_staff_id': 'mock-staff-1',
      'components': [
        for (final isPack in [false, true])
          for (final entry
              in ((isPack ? packaging : components)[v['id']] ?? <JsonObject>[])
                  .indexed)
            {
              'line_number': entry.$1 + 1,
              'subject_type': isPack ? 'PACKAGING' : 'INGREDIENT',
              'subject_ref':
                  entry.$2[isPack ? 'packaging_material_id' : 'ingredient_id'],
              'quantity_numerator': entry.$2['quantity_numerator'],
              'quantity_denominator': entry.$2['quantity_denominator'],
              'quantity_unit_code': entry.$2['unit_code'],
              'cost_source': 'UNRESOLVED',
              'incomplete_reason': 'NO_PURCHASE_EVIDENCE',
              'expected_waste_bps': entry.$2['expected_waste_bps'] ?? 0,
            },
      ],
    };
    result = costProjection?.call(result) ?? result;
    if (action == 'snapshots' && request.method == 'POST') {
      final key =
          request.headers['Idempotency-Key'] ??
          request.headers['idempotency-key'];
      if (key == null) return fail('An idempotency key is required.', 400);
      final fingerprint = request.url.toString();
      final cached = replay[key];
      if (cached != null) {
        return cached.fingerprint == fingerprint
            ? cached.result
            : fail('This request key was used for a different snapshot.', 409);
      }
      result['id'] = 'mock-recipe-cost-${++sequence}';
      result['calculation_type'] = 'RECIPE_SNAPSHOT';
      snapshots.add(result);
      final response = ok(result, 201);
      replay[key] = (fingerprint: fingerprint, result: response);
      return response;
    }
    return ok(result);
  }
}
