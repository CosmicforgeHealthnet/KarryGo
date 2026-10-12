import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';
import '../../features/catalogue/state/advanced_resource.dart';
import '../../features/catalogue/state/advanced_vocabulary.dart';
import '../../features/catalogue/state/catalogue_resource.dart';
import 'mock_catalogue_store.dart';
import 'mock_onboarding_store.dart';

class MockAdvancedCatalogueStore {
  MockAdvancedCatalogueStore(this.catalogue, this.business) {
    add(AdvancedKind.measures, {
      'id': 'demo-measure-unit',
      'code': 'UNIT',
      'display_name': 'Unit',
      'measurement_family': 'COUNT',
      'measure_origin': 'STANDARD',
      'scope': 'PLATFORM',
      'base_quantity': 1,
      'base_unit': 'unit',
      'converts_exactly': true,
    });
    add(AdvancedKind.measures, {
      'id': 'demo-measure-bowl',
      'code': 'BOWL',
      'display_name': 'Market bowl',
      'measurement_family': 'LOCAL_CONTAINER',
      'measure_origin': 'LOCAL',
      'scope': 'PLATFORM',
      'base_quantity': null,
      'converts_exactly': false,
    });
    final variant = catalogue.advancedRecord(
      CatalogueKind.shopVariants,
      'demo-variant',
    );
    if (variant != null) {
      final unit = add(AdvancedKind.sellUnits, {
        'id': 'demo-sell-unit',
        'product_ref': variant['product_ref'],
        'variant_ref': variant['id'],
        'measure_ref': 'demo-measure-unit',
        'fraction_numerator': 1,
        'fraction_denominator': 1,
        'display_name': 'Standard pack',
        'measurement_mode': 'FIXED',
        'conversion_type': 'EXACT',
        'exact_base_family': 'COUNT',
        'exact_base_quantity': 1,
        'calibration_ref': null,
        'estimated_base_quantity': null,
        'underfill_tolerance_bps': 0,
        'overfill_tolerance_bps': 0,
        'actual_measurement_required': false,
        'display_precision': 0,
        'price_minor': variant['retail_price_minor'],
        'wholesale_price_minor': null,
        'retail_enabled': true,
        'wholesale_enabled': false,
        'minimum_order_quantity': 1,
        'order_multiple': 1,
        'maximum_order_quantity': null,
      });
      add(AdvancedKind.identifiers, {
        'id': 'demo-identifier',
        'variant_ref': variant['id'],
        'product_ref': variant['product_ref'],
        'identifier_type': 'INTERNAL_SKU',
        'identifier_value': variant['sku'],
        'package_level': variant['package_level'],
        'externally_verified': false,
      });
      for (final listing in catalogue.advancedRecords(CatalogueKind.listings)) {
        linkUnit(listing, unit);
      }
    }
  }
  final MockCatalogueStore catalogue;
  final MockOnboardingStore business;
  final values = <AdvancedKind, List<JsonObject>>{};
  final profiles = <String, JsonObject>{};
  final cache = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject copy(JsonObject row) => jsonObject(jsonDecode(jsonEncode(row)));
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject error(int status, String message) => {
    '_status': status,
    '_message': message,
  };
  JsonObject add(AdvancedKind kind, JsonObject row) {
    final v = {
      'id': 'demo-advanced-${++sequence}',
      'status': 'ACTIVE',
      'version': 1,
      'position': (values[kind] ?? []).length,
      'created_at': now,
      'updated_at': now,
      ...row,
    };
    if (kind == AdvancedKind.aliases) {
      for (final field in ['status', 'version', 'position', 'updated_at']) {
        v.remove(field);
      }
    }
    if (kind == AdvancedKind.foodHistory || kind == AdvancedKind.shopHistory) {
      for (final field in [
        'status',
        'version',
        'position',
        'created_at',
        'updated_at',
      ]) {
        v.remove(field);
      }
    }
    (values[kind] ??= []).add(v);
    return v;
  }

  JsonObject? lookup(AdvancedKind kind, String id) =>
      (values[kind] ?? []).where((r) => r['id'] == id).firstOrNull;
  JsonObject linkUnit(JsonObject listing, JsonObject unit) =>
      add(AdvancedKind.listingUnits, {
        'listing_ref': listing['id'],
        'sell_unit_ref': unit['id'],
        'calibration_ref': null,
        'enabled': true,
        'price_override_minor': null,
        'wholesale_price_override_minor': null,
      });
  ({AdvancedResource resource, String? id, bool archive})? match(String path) {
    for (final kind in AdvancedKind.values) {
      final resource = AdvancedResource(kind);
      final names = <String>[];
      final expression = resource.path.replaceAllMapped(RegExp(r'\{(\w+)\}'), (
        m,
      ) {
        names.add(m[1]!);
        return '([^/]+)';
      });
      final suffix = resource.singleton
          ? '(?:/(archive))?'
          : kind == AdvancedKind.listingUnits
          ? '(?:/sell-units/([^/]+))?'
          : '(?:/([^/]+))?(?:/(archive))?';
      final m = RegExp('^$expression$suffix\$').firstMatch(path);
      if (m == null) continue;
      return (
        resource: AdvancedResource(
          kind,
          parameters: {
            for (var i = 0; i < names.length; i++) names[i]: m[i + 1]!,
          },
        ),
        id: resource.singleton ? null : m[names.length + 1],
        archive: resource.singleton
            ? m[names.length + 1] == 'archive'
            : kind == AdvancedKind.listingUnits
            ? false
            : m[names.length + 2] == 'archive',
      );
    }
    return null;
  }

  JsonObject? parent(AdvancedResource r) => switch (r.kind) {
    AdvancedKind.schedules => catalogue.advancedRecord(
      CatalogueKind.menus,
      r.parameters['menuId']!,
    ),
    AdvancedKind.availability ||
    AdvancedKind.source ||
    AdvancedKind.override ||
    AdvancedKind.prep ||
    AdvancedKind.logistics ||
    AdvancedKind.effective ||
    AdvancedKind.foodHistory => catalogue.advancedRecord(
      CatalogueKind.items,
      r.parameters['itemId']!,
    ),
    AdvancedKind.aliases => lookup(
      AdvancedKind.measures,
      r.parameters['measureId']!,
    ),
    AdvancedKind.calibrations ||
    AdvancedKind.sellUnits ||
    AdvancedKind.packaging ||
    AdvancedKind.fresh => catalogue.advancedRecord(
      CatalogueKind.products,
      r.parameters['productId']!,
    ),
    AdvancedKind.identifiers => catalogue.advancedRecord(
      CatalogueKind.shopVariants,
      r.parameters['variantId']!,
    ),
    AdvancedKind.quantityBreaks ||
    AdvancedKind.listingUnits => catalogue.advancedRecord(
      CatalogueKind.listings,
      r.parameters['listingId']!,
    ),
    AdvancedKind.models =>
      business.branches
          .where((b) => b['id'] == r.parameters['branchId'])
          .firstOrNull,
    _ => {},
  };
  String key(AdvancedResource r) =>
      '${r.kind.name}:${r.parameters.values.join(':')}';
  List<JsonObject> collection(AdvancedResource r, {bool archived = false}) =>
      (values[r.kind] ?? []).where((row) {
        if (!archived && row['status'] == 'ARCHIVED') return false;
        return r.parameters.entries.every(
          (e) =>
              row[switch (e.key) {
                'menuId' => 'menu_id',
                'productId' => 'product_ref',
                'variantId' => 'variant_ref',
                'listingId' => 'listing_ref',
                'measureId' => 'measure_ref',
                'itemId' => 'item_ref',
                _ => e.key,
              }] ==
              e.value,
        );
      }).toList();
  bool version(JsonObject row, JsonObject body) =>
      (row['version'] ?? 0) == body['expected_version'];
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final tool = readTool(request, body);
    if (tool != null) return tool;
    final m = match(request.url.path);
    if (m == null) return null;
    final r = m.resource, p = parent(r);
    if (p == null) return error(404, 'Catalogue parent not found.');
    if (r.kind == AdvancedKind.listingUnits &&
        request.method != 'GET' &&
        m.id == null &&
        !body.containsKey('sell_unit_ids')) {
      return null;
    }
    if (request.method == 'GET') {
      return read(r, m.id, request.url.queryParameters, p);
    }
    if (p['status'] == 'ARCHIVED') {
      return error(409, 'This catalogue parent is archived.');
    }
    final token = request.headers.entries
        .where((h) => h.key.toLowerCase() == 'idempotency-key')
        .firstOrNull
        ?.value;
    if (token == null) return error(400, 'Idempotency-Key is required.');
    final fingerprint = jsonEncode([request.method, request.url.path, body]),
        replay = cache[token];
    if (replay != null) {
      return replay.fingerprint == fingerprint
          ? copy(replay.result)
          : error(409, 'The idempotency key has different details.');
    }
    final result = write(r, m.id, m.archive, request.method, body, p);
    if (result['_message'] == null) {
      cache[token] = (fingerprint: fingerprint, result: copy(result));
    }
    return result;
  }

  JsonObject? readTool(http.BaseRequest request, JsonObject body) {
    final path = request.url.path;
    final food = RegExp(
      r'^/api/v1/vendor/food/items/([^/]+)/pricing/resolve$',
    ).firstMatch(path);
    if (food != null && request.method == 'POST') {
      return foodPreview(food[1]!, body);
    }
    final shop = RegExp(
      r'^/api/v1/vendor/shop/listings/([^/]+)/resolved-price$',
    ).firstMatch(path);
    if (shop != null && request.method == 'GET') {
      return shopPreview(shop[1]!, request.url.queryParameters);
    }
    if (path == '/api/v1/vendor/shop/identifier-lookup' &&
        request.method == 'GET') {
      final value = request.url.queryParameters['value']?.trim() ?? '',
          type = request.url.queryParameters['type']?.trim().toUpperCase();
      if (value.isEmpty ||
          type != null &&
              !catalogueVocabularyList('IdentifierTypes').contains(type)) {
        return error(422, 'Provide a code and supported identifier type.');
      }
      final identifiers =
          (values[AdvancedKind.identifiers] ?? [])
              .where(
                (i) =>
                    i['status'] == 'ACTIVE' &&
                    i['identifier_value'].toString().toUpperCase() ==
                        value.toUpperCase() &&
                    (type == null || i['identifier_type'] == type),
              )
              .toList()
            ..sort(
              (a, b) => a['identifier_type'].toString().compareTo(
                b['identifier_type'].toString(),
              ),
            );
      final identifier = identifiers.firstOrNull;
      if (identifier == null) {
        return {
          ...error(404, 'Identifier not found.'),
          '_code': 'SHOP_IDENTIFIER_NOT_FOUND',
        };
      }
      final variant = catalogue.advancedRecord(
        CatalogueKind.shopVariants,
        identifier['variant_ref'].toString(),
      );
      final product = catalogue.advancedRecord(
        CatalogueKind.products,
        identifier['product_ref'].toString(),
      );
      if (variant == null ||
          product == null ||
          variant['status'] == 'ARCHIVED' ||
          product['status'] == 'ARCHIVED') {
        return error(404, 'Product or variant not found.');
      }
      return ok({
        'identifier': identifier,
        'variant': variant,
        'product': product,
      });
    }
    return null;
  }

  JsonObject foodPreview(String id, JsonObject body) {
    final item = catalogue.advancedRecord(CatalogueKind.items, id);
    if (item == null || item['status'] == 'ARCHIVED') {
      return error(404, 'Food item not found.');
    }
    if (body.keys.any((k) => !['variant_ref', 'modifier_refs'].contains(k))) {
      return error(422, 'Provide only a variant and modifier references.');
    }
    final override = profile(
      AdvancedResource(AdvancedKind.override, parameters: {'itemId': id}),
      item,
    );
    var price =
        override['base_price_override_minor'] as int? ??
        item['base_price_minor'] as int;
    var source = override['base_price_override_minor'] == null
        ? 'ITEM'
        : 'BRANCH_OVERRIDE';
    JsonObject? variant;
    final selectedVariant = body['variant_ref']?.toString().trim() ?? '';
    if (selectedVariant.isNotEmpty) {
      variant = catalogue.advancedRecord(
        CatalogueKind.foodVariants,
        selectedVariant,
      );
      if (variant == null ||
          variant['item_id'] != id ||
          variant['status'] == 'ARCHIVED') {
        return error(404, 'Variant not found for this item.');
      }
      if (variant['price_override_minor'] != null) {
        price = variant['price_override_minor'];
        source = 'VARIANT_OVERRIDE';
      }
    }
    final refs = (body['modifier_refs'] as List? ?? []).cast<String>();
    if (refs.length > 50 ||
        refs.toSet().length != refs.length ||
        refs.any((r) => r.trim().isEmpty)) {
      return error(422, 'Choose up to 50 unique modifiers.');
    }
    final attached =
        (catalogue.read(
                  '/api/v1/vendor/food/items/$id/modifier-groups',
                  {},
                )!['modifier_groups']
                as List)
            .map(jsonObject)
            .map((link) => jsonObject(link['group']))
            .where((group) => group['status'] != 'ARCHIVED')
            .toList();
    final components = <JsonObject>[];
    for (final group in attached) {
      final selected = catalogue
          .advancedRecords(CatalogueKind.modifiers)
          .where(
            (m) =>
                m['group_id'] == group['id'] &&
                m['status'] != 'ARCHIVED' &&
                refs.contains(m['id']),
          )
          .toList();
      if (selected.length < group['min_select'] ||
          selected.length > group['max_select']) {
        return error(422, 'Complete the selection rules for ${group['name']}.');
      }
      for (final m in selected) {
        components.add({
          'modifier_ref': m['id'],
          'modifier_group_ref': group['id'],
          'name': m['name'],
          'price_delta_minor': m['price_delta_minor'],
          'modifier_version': m['version'],
        });
      }
    }
    if (components.length != refs.length) {
      return error(422, 'One or more modifiers are unavailable for this item.');
    }
    final extras = components.fold<int>(
      0,
      (sum, m) => sum + (m['price_delta_minor'] as int),
    );
    return ok({
      'currency': business.business['currency'],
      'item_ref': id,
      'item_version': item['version'],
      if (variant != null) 'variant_ref': variant['id'],
      if (variant != null) 'variant_version': variant['version'],
      'base_source': source,
      'base_price_minor': price,
      'modifiers': components,
      'modifier_total_minor': extras,
      'effective_unit_price_minor': price + extras,
      'catalogue_revision': 1,
      'resolved_at': now,
    });
  }

  JsonObject shopPreview(String id, Map<String, String> query) {
    final listing = catalogue.advancedRecord(CatalogueKind.listings, id);
    if (listing == null || listing['status'] == 'ARCHIVED') {
      return error(404, 'Branch listing not found.');
    }
    final variant = catalogue.advancedRecord(
      CatalogueKind.shopVariants,
      listing['variant_ref'].toString(),
    );
    if (variant == null || variant['status'] == 'ARCHIVED') {
      return error(404, 'Variant not found.');
    }
    final quantity = int.tryParse(query['quantity'] ?? '1');
    if (quantity == null || quantity < 1 || quantity > 1000000000) {
      return error(422, 'Use a quantity between 1 and 1,000,000,000.');
    }
    final wholesale = query['channel']?.toLowerCase() == 'wholesale',
        unitId = query['sell_unit_id']?.trim() ?? '';
    final result = <String, dynamic>{
      'listing_ref': id,
      'branch_ref': listing['branch_ref'],
      'variant_ref': listing['variant_ref'],
      'sell_unit_ref': unitId.isEmpty ? null : unitId,
      'currency': business.business['currency'],
      'quantity': quantity,
      'mode': wholesale ? 'WHOLESALE' : 'RETAIL',
      'unit_price_minor': null,
      'total_price_minor': null,
      'price_source': 'UNPRICED',
      'quantity_break_ref': null,
      'minimum_order_quantity': listing['minimum_order_quantity'],
      'order_multiple': listing['order_multiple'],
      'maximum_order_quantity': listing['maximum_order_quantity'],
      'order_rules_satisfied': false,
      'availability': 'UNAVAILABLE',
    };
    if (listing[wholesale ? 'wholesale_enabled' : 'retail_enabled'] != true) {
      return ok({
        ...result,
        'reason':
            'This listing is not enabled for ${wholesale ? 'wholesale' : 'retail'}.',
      });
    }
    JsonObject? unit, association;
    if (unitId.isNotEmpty) {
      unit = lookup(AdvancedKind.sellUnits, unitId);
      if (unit == null || unit['variant_ref'] != variant['id']) {
        return error(404, 'Sell unit not found for this listing.');
      }
      if (unit['status'] != 'ACTIVE' ||
          unit[wholesale ? 'wholesale_enabled' : 'retail_enabled'] != true) {
        return ok({
          ...result,
          'reason':
              'This sell unit is not enabled for the requested sales channel.',
        });
      }
      association =
          collection(
                AdvancedResource(
                  AdvancedKind.listingUnits,
                  parameters: {'listingId': id},
                ),
                archived: true,
              )
              .where(
                (u) => u['sell_unit_ref'] == unitId && u['enabled'] == true,
              )
              .firstOrNull;
      if (association == null) {
        return ok({
          ...result,
          'reason': 'This sell unit is not enabled for the branch listing.',
        });
      }
      if (unit['minimum_order_quantity'] > result['minimum_order_quantity']) {
        result['minimum_order_quantity'] = unit['minimum_order_quantity'];
      }
      if (unit['order_multiple'] > result['order_multiple']) {
        result['order_multiple'] = unit['order_multiple'];
      }
      if (unit['maximum_order_quantity'] != null &&
          (result['maximum_order_quantity'] == null ||
              unit['maximum_order_quantity'] <
                  result['maximum_order_quantity'])) {
        result['maximum_order_quantity'] = unit['maximum_order_quantity'];
      }
    }
    final bands =
        collection(
          AdvancedResource(
            AdvancedKind.quantityBreaks,
            parameters: {'listingId': id},
          ),
        ).where(
          (band) =>
              (band['sell_unit_ref'] ?? '') == unitId &&
              band['min_quantity'] <= quantity &&
              (band['max_quantity'] == null ||
                  band['max_quantity'] >= quantity),
        );
    final band = wholesale ? bands.firstOrNull : null;
    if (band != null) {
      result['unit_price_minor'] = band['unit_price_minor'];
      result['price_source'] = 'QUANTITY_BREAK';
      result['quantity_break_ref'] = band['id'];
    } else {
      final prices = <String, Object?>{
        'LISTING_SELL_UNIT_OVERRIDE':
            association?[wholesale
                ? 'wholesale_price_override_minor'
                : 'price_override_minor'],
        'LISTING_OVERRIDE':
            listing[wholesale
                ? 'wholesale_price_override_minor'
                : 'retail_price_override_minor'],
        'SELL_UNIT_PRICE':
            unit?[wholesale ? 'wholesale_price_minor' : 'price_minor'],
        'VARIANT_BASE':
            variant[wholesale ? 'wholesale_price_minor' : 'retail_price_minor'],
      };
      final chosen = prices.entries.where((e) => e.value != null).firstOrNull;
      if (chosen != null) {
        result['unit_price_minor'] = chosen.value;
        result['price_source'] = chosen.key;
      }
    }
    final price = result['unit_price_minor'] as int?;
    if (price == null) {
      return ok({
        ...result,
        'reason': 'No price is configured for this listing and unit.',
      });
    }
    if (price != 0 && quantity > 99999999999 ~/ price) {
      return ok({
        ...result,
        'reason': 'The requested quantity exceeds the supported total.',
      });
    }
    result['total_price_minor'] = price * quantity;
    result['order_rules_satisfied'] =
        quantity >= result['minimum_order_quantity'] &&
        quantity % result['order_multiple'] == 0 &&
        (result['maximum_order_quantity'] == null ||
            quantity <= result['maximum_order_quantity']);
    if (result['order_rules_satisfied'] != true) {
      result['reason'] =
          'The requested quantity does not satisfy the configured order rules.';
    }
    return ok(result);
  }

  JsonObject profile(AdvancedResource r, JsonObject p) {
    final saved = profiles[key(r)];
    if (saved != null) return copy(saved);
    return switch (r.kind) {
      AdvancedKind.availability => {
        'item_id': p['id'],
        'branch_id': branch(p),
        'status': 'AVAILABLE',
        'effective_status': 'AVAILABLE',
        'reason': '',
        'effective_until': null,
        'expired': false,
        'version': 0,
      },
      AdvancedKind.source => {
        'item_id': p['id'],
        'mode': 'MANUAL',
        'inventory_recipe_ref': null,
        'inventory_availability_support': 'NOT_SUPPORTED',
        'version': 0,
      },
      AdvancedKind.override => {
        'item_id': p['id'],
        'branch_id': branch(p),
        'base_price_override_minor': null,
        'inherits_base_price': true,
        'prep_min_minutes': null,
        'prep_max_minutes': null,
        'inherits_prep_profile': true,
        'version': 0,
      },
      AdvancedKind.models => {
        'id': '',
        'branch_ref': p['id'],
        'primary_model': null,
        'models': [],
        'status': '',
        'version': 0,
      },
      _ => {'version': 0},
    };
  }

  String branch(JsonObject item) =>
      catalogue
          .advancedRecord(
            CatalogueKind.menus,
            item['menu_id'].toString(),
          )?['branch_id']
          .toString() ??
      '';
  JsonObject prep(JsonObject item) {
    final params = {'itemId': item['id'].toString()};
    final override = profile(
      AdvancedResource(AdvancedKind.override, parameters: params),
      item,
    );
    if (override['prep_min_minutes'] != null) {
      return {
        'configured': true,
        'prep_min_minutes': override['prep_min_minutes'],
        'prep_max_minutes': override['prep_max_minutes'],
        'source': 'BRANCH_OVERRIDE',
      };
    }
    final data =
        profiles[key(AdvancedResource(AdvancedKind.prep, parameters: params))];
    return data == null
        ? {
            'configured': false,
            'prep_min_minutes': 0,
            'prep_max_minutes': 0,
            'source': 'NOT_CONFIGURED',
          }
        : {
            'configured': true,
            'prep_min_minutes': data['prep_min_minutes'],
            'prep_max_minutes': data['prep_max_minutes'],
            'source': 'ITEM_DEFAULT',
          };
  }

  Map<String, String> eligibleModels(JsonObject branch) {
    final active = <String>{};
    for (final link in (branch['capabilities'] as List? ?? []).map(
      jsonObject,
    )) {
      final cap = business.capabilities
          .where(
            (c) =>
                c['id'] ==
                (link['vendor_capability_id'] ?? link['capability_id']),
          )
          .firstOrNull;
      if (link['status'] == 'ACTIVE' && cap?['status'] == 'ACTIVE') {
        active.add(cap!['capability_type'].toString());
      }
    }
    const admissions = {
      'SUPERMARKET': ['SHOP_GROCERY'],
      'GROCERY_SHOP': ['SHOP_GROCERY'],
      'FOODSTUFF_DEALER': ['SHOP_GROCERY', 'WHOLESALE'],
      'MARKET_TRADER': ['MARKET'],
      'WHOLESALER': ['WHOLESALE'],
    };
    return {
      for (final e in admissions.entries)
        if (e.value.any(active.contains))
          e.key: e.value.firstWhere(active.contains),
    };
  }

  JsonObject read(
    AdvancedResource r,
    String? id,
    Map<String, String> query,
    JsonObject p,
  ) {
    if (r.singleton) {
      var data = profile(r, p);
      if (r.kind == AdvancedKind.availability) {
        final expires = DateTime.tryParse(
          data['effective_until']?.toString() ?? '',
        );
        data['expired'] = expires != null && !expires.isAfter(DateTime.now());
        data['effective_status'] = data['expired'] == true
            ? 'AVAILABLE'
            : data['status'];
      }
      if (r.kind == AdvancedKind.prep || r.kind == AdvancedKind.logistics) {
        return ok({
          'configured': profiles.containsKey(key(r)),
          if (profiles.containsKey(key(r))) 'profile': data,
          if (r.kind == AdvancedKind.prep) 'effective': prep(p),
        });
      }
      if (r.kind == AdvancedKind.models) {
        return ok({
          'configuration': data,
          'eligible_capabilities': eligibleModels(p).keys.toList(),
        });
      }
      if (r.kind == AdvancedKind.effective) {
        return effective(p, at: DateTime.tryParse(query['at'] ?? ''));
      }
      return ok(data);
    }
    if (r.kind == AdvancedKind.listingUnits && id == null) {
      return ok({
        'listing': copy(p),
        'sell_units': collection(r, archived: true),
      });
    }
    if (id != null) {
      final row = lookup(r.kind, id);
      if (row == null || !collection(r, archived: true).contains(row)) {
        return error(404, 'Catalogue record not found.');
      }
      return r.kind == AdvancedKind.measures
          ? ok({
              'measurement': row,
              'aliases': collection(
                AdvancedResource(
                  AdvancedKind.aliases,
                  parameters: {'measureId': id},
                ),
                archived: true,
              ),
            })
          : ok(row);
    }
    var rows = collection(r, archived: query['include_archived'] == 'true');
    if (r.kind == AdvancedKind.shopHistory && query['product_ref'] != null) {
      rows = rows
          .where((r) => r['product_ref'] == query['product_ref'])
          .toList();
    }
    if (r.paginated) {
      final offset = int.tryParse(query['offset'] ?? '0') ?? 0,
          limit = int.tryParse(query['limit'] ?? '50') ?? 50;
      return ok({
        r.kind == AdvancedKind.foodHistory ? 'price_versions' : 'items': rows
            .skip(offset)
            .take(limit)
            .toList(),
        'has_more': rows.length > offset + limit,
        'pagination': {
          'limit': limit,
          'offset': offset,
          'has_more': rows.length > offset + limit,
        },
      });
    }
    if (r.kind == AdvancedKind.schedules) {
      final b = business.branches
          .where((b) => b['id'] == p['branch_id'])
          .firstOrNull;
      return ok({
        'schedules': rows,
        'timezone': b?['timezone'] ?? 'Africa/Lagos',
      });
    }
    return ok(rows);
  }

  bool scheduleMatches(JsonObject schedule, DateTime local) {
    final start = schedule['start_local_time'].toString(),
        end = schedule['end_local_time'].toString();
    final clock =
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
    bool dayMatches(DateTime day) {
      final date = day.toIso8601String().substring(0, 10);
      return (schedule['days_of_week'] as List).contains(day.weekday) &&
          (schedule['effective_start_date'] == null ||
              date.compareTo(schedule['effective_start_date']) >= 0) &&
          (schedule['effective_end_date'] == null ||
              date.compareTo(schedule['effective_end_date']) <= 0);
    }

    if (start.compareTo(end) < 0) {
      return dayMatches(local) &&
          clock.compareTo(start) >= 0 &&
          clock.compareTo(end) < 0;
    }
    return clock.compareTo(start) >= 0 && dayMatches(local) ||
        clock.compareTo(end) < 0 &&
            dayMatches(local.subtract(const Duration(days: 1)));
  }

  JsonObject effective(JsonObject item, {DateTime? at}) {
    final availability = profile(
          AdvancedResource(
            AdvancedKind.availability,
            parameters: {'itemId': item['id'].toString()},
          ),
          item,
        ),
        source = profile(
          AdvancedResource(
            AdvancedKind.source,
            parameters: {'itemId': item['id'].toString()},
          ),
          item,
        );
    final menu = catalogue.advancedRecord(
          CatalogueKind.menus,
          item['menu_id'].toString(),
        ),
        section = catalogue.advancedRecord(
          CatalogueKind.sections,
          item['section_id'].toString(),
        );
    final b = business.branches
        .where((b) => b['id'] == menu?['branch_id'])
        .firstOrNull;
    final evaluated = (at ?? DateTime.now()).toUtc();
    final schedules = collection(
      AdvancedResource(
        AdvancedKind.schedules,
        parameters: {'menuId': menu?['id'].toString() ?? ''},
      ),
    );
    final local = b?['timezone'] == 'Africa/Lagos'
        ? evaluated.add(const Duration(hours: 1))
        : evaluated;
    final matched = schedules
        .where((window) => scheduleMatches(window, local))
        .firstOrNull;
    final expiry = DateTime.tryParse(
      availability['effective_until']?.toString() ?? '',
    );
    final operational = expiry != null && !expiry.isAfter(evaluated)
        ? 'AVAILABLE'
        : availability['status'];
    final reason = business.business['status'] != 'ACTIVE'
        ? 'BUSINESS_INACTIVE'
        : b?['status'] != 'ACTIVE'
        ? 'BRANCH_INACTIVE'
        : menu?['status'] != 'ACTIVE'
        ? 'MENU_NOT_ACTIVE'
        : schedules.isNotEmpty && matched == null
        ? 'OUTSIDE_MENU_SCHEDULE'
        : section?['status'] != 'ACTIVE'
        ? 'SECTION_INACTIVE'
        : item['status'] != 'ACTIVE'
        ? 'ITEM_NOT_ACTIVE'
        : operational == 'SOLD_OUT'
        ? 'ITEM_SOLD_OUT'
        : operational == 'PAUSED'
        ? 'ITEM_PAUSED'
        : source['mode'] == 'INVENTORY_LINKED'
        ? 'INVENTORY_SOURCE_UNAVAILABLE'
        : 'SELLABLE';
    return ok({
      'sellability': {
        'sellable': reason == 'SELLABLE',
        'reason_code': reason,
        'evaluated_at': evaluated.toIso8601String(),
        if (matched != null) 'matched_schedule_ref': matched['id'],
        'branch_id': b?['id'],
        'timezone': b?['timezone'],
        'menu_ref': menu?['id'],
        'section_ref': section?['id'],
        'item_ref': item['id'],
        'operational_status': operational,
        'availability_version': availability['version'],
        'availability_mode': source['mode'],
        'inventory_availability_support': 'NOT_SUPPORTED',
        'catalogue_revision': 1,
      },
      'prep': prep(item),
      if (profiles.containsKey('logistics:${item['id']}'))
        'logistics': profiles['logistics:${item['id']}'],
    });
  }

  JsonObject write(
    AdvancedResource r,
    String? id,
    bool archive,
    String method,
    JsonObject body,
    JsonObject p,
  ) {
    if (r.singleton) {
      final current = profile(r, p);
      if (!version(current, body)) {
        return error(409, 'This configuration changed. Refresh before saving.');
      }
      if (method == 'DELETE' || archive) {
        profiles.remove(key(r));
        if (r.kind == AdvancedKind.models) {
          for (final listing
              in catalogue
                  .advancedRecords(CatalogueKind.listings)
                  .where(
                    (l) =>
                        l['branch_ref'] == p['id'] &&
                        l['operating_model'] != null &&
                        l['status'] == 'ACTIVE',
                  )) {
            listing['status'] = 'INACTIVE';
            listing['version'] = (listing['version'] as int) + 1;
          }
          profiles[key(r)] = {
            ...current,
            'status': 'ARCHIVED',
            'version': (current['version'] as int) + 1,
          };
        }
        return ok({'cleared': true});
      }
      final clean = Map<String, dynamic>.of(body)..remove('expected_version');
      if (r.kind == AdvancedKind.availability) {
        if (!['AVAILABLE', 'SOLD_OUT', 'PAUSED'].contains(clean['status'])) {
          return error(422, 'Choose a valid availability status.');
        }
        final until = DateTime.tryParse(
          clean['effective_until']?.toString() ?? '',
        );
        if (until != null && !until.isAfter(DateTime.now())) {
          return error(422, 'The restore time must be in the future.');
        }
        clean.addAll({
          'item_id': p['id'],
          'branch_id': branch(p),
          'effective_status': clean['status'],
          'effective_until': clean['effective_until'],
          'expired': false,
          'reason': clean['reason'] ?? '',
        });
      }
      if (r.kind == AdvancedKind.prep || r.kind == AdvancedKind.override) {
        final min = clean['prep_min_minutes'], max = clean['prep_max_minutes'];
        if ((min == null) != (max == null) ||
            min != null && (min < 0 || max > 1440 || min > max)) {
          return error(422, 'Use a valid preparation range.');
        }
        if (r.kind == AdvancedKind.override) {
          if (clean.remove('clear_base_price_override') == true) {
            clean['base_price_override_minor'] = null;
          }
          if (clean.remove('clear_prep_override') == true) {
            clean['prep_min_minutes'] = null;
            clean['prep_max_minutes'] = null;
          }
          clean.addAll({'item_id': p['id'], 'branch_id': branch(p)});
        } else {
          clean['item_id'] = p['id'];
        }
      }
      if (r.kind == AdvancedKind.source) {
        if (!['MANUAL', 'INVENTORY_LINKED'].contains(clean['mode'])) {
          return error(422, 'Choose a valid availability source.');
        }
        if (clean['mode'] == 'INVENTORY_LINKED' &&
            (clean['inventory_recipe_ref']?.toString().isEmpty ?? true)) {
          return error(422, 'The inventory recipe reference is required.');
        }
        clean['inventory_availability_support'] = 'NOT_SUPPORTED';
        clean['item_id'] = p['id'];
      }
      if (r.kind == AdvancedKind.logistics) {
        if (![
              'AMBIENT',
              'HOT',
              'COLD',
              'FROZEN',
            ].contains(clean['temperature_class']) ||
            ![
              'SMALL',
              'MEDIUM',
              'LARGE',
            ].contains(clean['package_size_class'])) {
          return error(422, 'Choose supported handling classes.');
        }
        clean['item_id'] = p['id'];
      }
      if (r.kind == AdvancedKind.models) {
        final models = (clean['models'] as List? ?? []).cast<String>();
        if (models.isEmpty ||
            models.any((m) => !eligibleModels(p).containsKey(m))) {
          return error(
            409,
            'This selling model is not admitted by the branch capabilities.',
          );
        }
        clean['models'] = models
            .map(
              (m) => {'code': m, 'admitted_capability': eligibleModels(p)[m]},
            )
            .toList();
        clean['branch_ref'] = p['id'];
        clean['status'] = 'ACTIVE';
      }
      final saved = {
        ...current,
        ...clean,
        'version': (current['version'] as int? ?? 0) + 1,
        'updated_at': now,
      };
      if (r.kind == AdvancedKind.override) {
        saved['inherits_base_price'] =
            saved['base_price_override_minor'] == null;
        saved['inherits_prep_profile'] = saved['prep_min_minutes'] == null;
      }
      if (r.kind == AdvancedKind.override &&
          saved['base_price_override_minor'] !=
              current['base_price_override_minor']) {
        add(AdvancedKind.foodHistory, {
          'subject_type': 'BRANCH_OVERRIDE',
          'subject_ref': p['id'],
          'item_ref': p['id'],
          'branch_id': branch(p),
          'currency': business.business['currency'],
          'previous_amount_minor': current['base_price_override_minor'],
          'amount_minor': saved['base_price_override_minor'],
          'previous_inherited': current['base_price_override_minor'] == null,
          'inherited': saved['base_price_override_minor'] == null,
          'change_reason': saved['base_price_override_minor'] == null
              ? 'OVERRIDE_CLEARED'
              : 'OVERRIDE_SET',
          'subject_version': saved['version'],
          'revision': sequence,
          'actor_type': 'STAFF',
          'recorded_at': now,
          'effective_at': now,
        });
      }
      profiles[key(r)] = saved;
      return ok(saved);
    }
    if (r.kind == AdvancedKind.listingUnits && id == null) {
      if (!version(p, body)) {
        return error(409, 'This listing changed. Refresh before saving.');
      }
      final ids = (body['sell_unit_ids'] as List).cast<String>();
      for (final v in ids) {
        final unit = lookup(AdvancedKind.sellUnits, v);
        if (unit == null ||
            unit['variant_ref'] != p['variant_ref'] ||
            unit['status'] != 'ACTIVE') {
          return error(
            409,
            'Only active units for this variant can be selected.',
          );
        }
        if (!(unit['retail_enabled'] == true && p['retail_enabled'] == true ||
            unit['wholesale_enabled'] == true &&
                p['wholesale_enabled'] == true)) {
          return error(409, 'The sell unit has no matching sales channel.');
        }
      }
      for (final current in collection(r, archived: true)) {
        current['enabled'] = false;
        current['version'] = (current['version'] as int) + 1;
      }
      for (final v in ids) {
        final current = collection(
          r,
          archived: true,
        ).where((u) => u['sell_unit_ref'] == v).firstOrNull;
        if (current == null) {
          linkUnit(p, lookup(AdvancedKind.sellUnits, v)!);
        } else {
          current['enabled'] = true;
          current['version'] = (current['version'] as int) + 1;
        }
      }
      p['version'] = (p['version'] as int) + 1;
      return ok(p);
    }
    final current = id == null ? null : lookup(r.kind, id);
    if (id != null &&
        (current == null || !collection(r, archived: true).contains(current))) {
      return error(404, 'Catalogue record not found.');
    }
    if (current != null &&
        r.kind != AdvancedKind.aliases &&
        !version(current, body)) {
      return error(409, 'This record changed. Refresh before saving.');
    }
    if ((r.kind == AdvancedKind.measures || r.kind == AdvancedKind.aliases) &&
        current?['scope'] == 'PLATFORM') {
      return error(403, 'Platform definitions cannot be changed.');
    }
    if (archive) {
      current!['status'] = 'ARCHIVED';
      current['version'] = (current['version'] as int) + 1;
      if (r.kind == AdvancedKind.sellUnits) {
        for (final unit
            in values[AdvancedKind.listingUnits] ?? <JsonObject>[]) {
          if (unit['sell_unit_ref'] == current['id']) {
            unit['enabled'] = false;
            unit['version'] = (unit['version'] as int) + 1;
          }
        }
      }
      return ok(current);
    }
    if (method == 'DELETE' && r.kind == AdvancedKind.aliases) {
      values[r.kind]?.remove(current);
      return ok({'removed': true});
    }
    final clean = Map<String, dynamic>.of(body)..remove('expected_version');
    final aliases = {
      'family': 'measurement_family',
      'measure_id': 'measure_ref',
      'branch_id': 'branch_ref',
      'variant_id': 'variant_ref',
      'weight_grams': 'approximate_weight_grams',
      'volume_ml': 'approximate_volume_ml',
      'source': 'calibration_source',
      'calibration_id': 'calibration_ref',
      'estimated_base_qty': 'estimated_base_quantity',
      'underfill_bps': 'underfill_tolerance_bps',
      'overfill_bps': 'overfill_tolerance_bps',
      'measurement_required': 'actual_measurement_required',
      'minimum_order_qty': 'minimum_order_quantity',
      'maximum_order_qty': 'maximum_order_quantity',
      'parent_variant_id': 'parent_variant_ref',
      'child_variant_id': 'child_variant_ref',
      'sell_unit_id': 'sell_unit_ref',
      'price_override': 'price_override_minor',
      'wholesale_override': 'wholesale_price_override_minor',
      'required': 'selection_required',
    };
    for (final e in aliases.entries) {
      if (clean.containsKey(e.key)) clean[e.value] = clean.remove(e.key);
    }
    for (final e in r.parameters.entries) {
      clean[switch (e.key) {
            'menuId' => 'menu_id',
            'productId' => 'product_ref',
            'variantId' => 'variant_ref',
            'listingId' => 'listing_ref',
            'measureId' => 'measure_ref',
            _ => e.key,
          }] =
          e.value;
    }
    if (r.kind == AdvancedKind.schedules) {
      if (clean.remove('clear_effective_start_date') == true) {
        clean['effective_start_date'] = null;
      }
      if (clean.remove('clear_effective_end_date') == true) {
        clean['effective_end_date'] = null;
      }
      final candidate = {...?current, ...clean};
      if ((candidate['days_of_week'] as List? ?? []).isEmpty ||
          candidate['start_local_time'] == candidate['end_local_time']) {
        return error(422, 'Choose days and a non-zero time window.');
      }
      final days = (candidate['days_of_week'] as List).cast<int>();
      final clockPattern = RegExp(r'^(?:[01][0-9]|2[0-3]):[0-5][0-9]$');
      if (days.any((day) => day < 1 || day > 7) ||
          days.toSet().length != days.length ||
          !clockPattern.hasMatch(candidate['start_local_time'].toString()) ||
          !clockPattern.hasMatch(candidate['end_local_time'].toString())) {
        return error(422, 'Choose valid weekdays and local times.');
      }
      final from = candidate['effective_start_date'] as String?,
          to = candidate['effective_end_date'] as String?;
      if (from != null && to != null && from.compareTo(to) > 0) {
        return error(422, 'The first date must precede the last date.');
      }
      clean['overnight'] =
          candidate['start_local_time'].toString().compareTo(
            candidate['end_local_time'].toString(),
          ) >
          0;
    }
    if (r.kind == AdvancedKind.measures && current == null) {
      if (collection(
        r,
        archived: true,
      ).any((m) => m['code'] == clean['code'])) {
        return error(409, 'This measure code is already used.');
      }
      final exact = [
        'MASS',
        'VOLUME',
        'COUNT',
      ].contains(clean['measurement_family']);
      if (exact != (clean['base_quantity'] != null)) {
        return error(422, 'Only exact families use a base quantity.');
      }
      clean.addAll({
        'scope': 'BUSINESS',
        'measure_origin': 'VENDOR_DEFINED',
        'converts_exactly': exact,
        'base_unit': switch (clean['measurement_family']) {
          'MASS' => 'g',
          'VOLUME' => 'ml',
          'COUNT' => 'unit',
          _ => null,
        },
      });
    }
    if (r.kind == AdvancedKind.aliases) clean['scope'] = 'BUSINESS';
    if ({AdvancedKind.sellUnits, AdvancedKind.calibrations}.contains(r.kind)) {
      final measure = lookup(
        AdvancedKind.measures,
        (clean['measure_ref'] ?? current?['measure_ref']).toString(),
      );
      if (measure == null || measure['status'] == 'ARCHIVED') {
        return error(404, 'Measure not found.');
      }
      final variant = clean['variant_ref'] ?? current?['variant_ref'];
      if (variant != null) {
        final row = catalogue.advancedRecord(
          CatalogueKind.shopVariants,
          variant.toString(),
        );
        if (row == null || row['product_ref'] != p['id']) {
          return error(404, 'Variant not found for this product.');
        }
      }
      if (r.kind == AdvancedKind.calibrations) {
        if (clean['approximate_weight_grams'] == null &&
            clean['approximate_volume_ml'] == null &&
            current == null) {
          return error(422, 'Provide an approximate weight or volume.');
        }
        clean['approximate'] = true;
      } else {
        final candidate = {...?current, ...clean};
        final numerator = candidate['fraction_numerator'] as int? ?? 1,
            denominator = candidate['fraction_denominator'] as int? ?? 1;
        if (numerator < 1 ||
            numerator > 1000000 ||
            denominator < 1 ||
            denominator > 1000) {
          return error(422, 'Use a valid positive fraction.');
        }
        const families = {
          'FIXED': [
            'MASS',
            'VOLUME',
            'COUNT',
            'LOCAL_CONTAINER',
            'PACKAGE',
            'GROUPED_COUNT',
            'LOAD',
            'OTHER_CONTROLLED',
          ],
          'VARIABLE_WEIGHT': ['MASS', 'VARIABLE_WEIGHT'],
          'VARIABLE_COUNT': ['COUNT', 'GROUPED_COUNT', 'VARIABLE_COUNT'],
          'LOCAL_CONTAINER': ['LOCAL_CONTAINER', 'LOAD'],
          'PACKAGE_COUNT': ['PACKAGE'],
          'PRICE_PORTION': ['PRICE_PORTION'],
        };
        if (!(families[candidate['measurement_mode']] ?? []).contains(
          measure['measurement_family'],
        )) {
          return error(
            422,
            'This measurement mode is incompatible with the selected measure.',
          );
        }
        if (candidate['measurement_mode'] == 'PRICE_PORTION' &&
            candidate['price_minor'] == null) {
          return error(422, 'A price portion requires a price.');
        }
        if (measure['base_quantity'] != null) {
          final total = (measure['base_quantity'] as int) * numerator;
          if (total % denominator != 0) {
            return error(
              422,
              'This fraction must divide into whole base units.',
            );
          }
          clean['exact_base_quantity'] = total ~/ denominator;
          clean['exact_base_family'] = measure['measurement_family'];
        }
        clean.putIfAbsent(
          'conversion_type',
          () => measure['converts_exactly'] == true
              ? 'EXACT'
              : (clean['calibration_ref'] ?? current?['calibration_ref']) !=
                        null &&
                    (clean['calibration_ref'] ?? current?['calibration_ref']) !=
                        ''
              ? 'CALIBRATED_APPROXIMATE'
              : 'NONE',
        );
        for (final e in {
          'fraction_numerator': 1,
          'fraction_denominator': 1,
          'underfill_tolerance_bps': 0,
          'overfill_tolerance_bps': 0,
          'actual_measurement_required': false,
          'display_precision': 0,
          'retail_enabled': true,
          'wholesale_enabled': false,
          'minimum_order_quantity': 1,
          'order_multiple': 1,
        }.entries) {
          if (current == null) clean.putIfAbsent(e.key, () => e.value);
        }
        if (clean['calibration_ref'] != null &&
            clean['calibration_ref'] != '') {
          final cal = lookup(
            AdvancedKind.calibrations,
            clean['calibration_ref'].toString(),
          );
          if (cal == null ||
              cal['product_ref'] != p['id'] ||
              cal['status'] != 'ACTIVE' ||
              measure['converts_exactly'] == true ||
              cal['measure_ref'] != measure['id'] ||
              cal['variant_ref'] != null && cal['variant_ref'] != variant ||
              cal['branch_ref'] != null) {
            return error(
              409,
              'Use a business calibration for product sell units.',
            );
          }
        }
      }
    }
    if (r.kind == AdvancedKind.packaging) {
      final outer = catalogue.advancedRecord(
            CatalogueKind.shopVariants,
            clean['parent_variant_ref'].toString(),
          ),
          inner = catalogue.advancedRecord(
            CatalogueKind.shopVariants,
            clean['child_variant_ref'].toString(),
          );
      if (outer == null ||
          inner == null ||
          outer['id'] == inner['id'] ||
          outer['product_ref'] != p['id'] ||
          inner['product_ref'] != p['id']) {
        return error(
          409,
          'Choose distinct variants belonging to this product.',
        );
      }
    }
    if (r.kind == AdvancedKind.packaging) {
      final target = clean['parent_variant_ref'];
      final visited = <Object?>{};
      bool reaches(Object? node) {
        if (node == target) return true;
        if (!visited.add(node)) return false;
        return collection(r)
            .where((relation) => relation['parent_variant_ref'] == node)
            .any((relation) => reaches(relation['child_variant_ref']));
      }

      if (reaches(clean['child_variant_ref'])) {
        return error(409, 'Packaging relationships cannot form a cycle.');
      }
    }
    if (r.kind == AdvancedKind.listingUnits &&
        clean['calibration_ref'] != null &&
        clean['calibration_ref'] != '') {
      final cal = lookup(
        AdvancedKind.calibrations,
        clean['calibration_ref'].toString(),
      );
      final unit = lookup(
        AdvancedKind.sellUnits,
        current!['sell_unit_ref'].toString(),
      );
      if (cal == null ||
          cal['status'] != 'ACTIVE' ||
          cal['product_ref'] != p['product_ref'] ||
          cal['measure_ref'] != unit?['measure_ref'] ||
          cal['variant_ref'] != null &&
              cal['variant_ref'] != p['variant_ref'] ||
          cal['branch_ref'] != null && cal['branch_ref'] != p['branch_ref']) {
        return error(
          409,
          'Choose a calibration compatible with this branch, product and measure.',
        );
      }
    }
    if (r.kind == AdvancedKind.identifiers) {
      if (!catalogueVocabularyList(
        'IdentifierTypes',
      ).contains(clean['identifier_type'])) {
        return error(422, 'Choose a supported identifier type.');
      }
      clean['product_ref'] = p['product_ref'];
      clean['externally_verified'] = false;
    }
    if (r.kind == AdvancedKind.fresh) {
      final type = clean['attribute_type'];
      if (!(catalogueVocabularyMap('classAttributes')[p['product_class']] ?? [])
          .contains(type)) {
        return error(
          409,
          'This attribute does not apply to this product class.',
        );
      }
      final found = collection(
        r,
        archived: true,
      ).where((f) => f['attribute_type'] == type).firstOrNull;
      if (found != null && !version(found, body)) {
        return error(
          409,
          'This fresh attribute changed. Refresh before saving.',
        );
      }
      final options = (clean['options'] as List? ?? [])
          .map(jsonObject)
          .toList();
      final codes = catalogueVocabularyMap('FreshAttributeOptions')[type] ?? [];
      if (options.isEmpty ||
          options.any((o) => !codes.contains(o['value'])) ||
          options.map((o) => o['value']).toSet().length != options.length ||
          options.where((o) => o['is_default'] == true).length > 1) {
        return error(
          422,
          'Choose supported, unique fresh options and one default at most.',
        );
      }
      clean['options'] = options
          .map(
            (o) => {
              'id': 'demo-option-${++sequence}',
              'option_value': o['value'],
              'display_name': o['display_name'],
              'is_default': o['is_default'] == true,
              'status': 'ACTIVE',
              'position': options.indexOf(o),
            },
          )
          .toList();
      clean['status'] = 'ACTIVE';
      if (found != null) {
        found.addAll(clean);
        found['version'] = (found['version'] as int) + 1;
        return ok(found);
      }
    }
    if (r.kind == AdvancedKind.quantityBreaks) {
      final candidate = {...?current, ...clean};
      final min = candidate['min_quantity'] as int,
          max = candidate['max_quantity'] as int?;
      if (min < 1 || max != null && max < min) {
        return error(422, 'Use a valid quantity range.');
      }
      final selected = candidate['sell_unit_ref'];
      if (selected != null &&
          selected != '' &&
          !collection(
            AdvancedResource(
              AdvancedKind.listingUnits,
              parameters: r.parameters,
            ),
            archived: true,
          ).any(
            (u) => u['sell_unit_ref'] == selected && u['enabled'] == true,
          )) {
        return error(409, 'Enable this sell unit before adding a price tier.');
      }
      if (collection(r).any(
        (b) =>
            b['id'] != id &&
            b['sell_unit_ref'] == selected &&
            (max == null || b['min_quantity'] <= max) &&
            (b['max_quantity'] == null || b['max_quantity'] >= min),
      )) {
        return error(409, 'Quantity ranges cannot overlap.');
      }
    }
    if (r.kind == AdvancedKind.listingUnits &&
        current != null &&
        clean['enabled'] == true) {
      final unit = lookup(
        AdvancedKind.sellUnits,
        current['sell_unit_ref'].toString(),
      );
      if (unit?['status'] != 'ACTIVE') {
        return error(409, 'Only active sell units can be enabled.');
      }
    }
    final previous = current == null
        ? <String, dynamic>{}
        : Map<String, dynamic>.of(current);
    final saved = current ?? add(r.kind, clean);
    if (current != null) {
      saved.addAll(clean);
      saved['version'] = (saved['version'] as int) + 1;
      saved['updated_at'] = now;
    }
    if ({
      AdvancedKind.sellUnits,
      AdvancedKind.quantityBreaks,
      AdvancedKind.listingUnits,
    }.contains(r.kind)) {
      for (final field in [
        'price_minor',
        'wholesale_price_minor',
        'unit_price_minor',
        'price_override_minor',
        'wholesale_price_override_minor',
      ]) {
        if (clean[field] != null && clean[field] != previous[field]) {
          add(AdvancedKind.shopHistory, {
            'subject_type': r.kind == AdvancedKind.quantityBreaks
                ? 'QUANTITY_BREAK'
                : r.kind == AdvancedKind.listingUnits
                ? (field.contains('wholesale')
                      ? 'LISTING_SELL_UNIT_WHOLESALE'
                      : 'LISTING_SELL_UNIT_RETAIL')
                : (field.contains('wholesale')
                      ? 'SELL_UNIT_WHOLESALE'
                      : 'SELL_UNIT_RETAIL'),
            'subject_ref': saved['id'],
            'product_ref': saved['product_ref'] ?? p['product_ref'],
            'variant_ref': saved['variant_ref'] ?? p['variant_ref'],
            'currency': business.business['currency'],
            'amount_minor': clean[field],
            'previous_amount_minor': previous[field],
            'inherited': false,
            'change_reason': current == null ? 'CREATED' : 'PRICE_CHANGED',
            'subject_version': saved['version'],
            'revision': sequence,
            'actor_type': 'STAFF',
            'recorded_at': now,
            'effective_at': now,
          });
        }
      }
    }
    return ok(saved, current == null ? 201 : 200);
  }
}
