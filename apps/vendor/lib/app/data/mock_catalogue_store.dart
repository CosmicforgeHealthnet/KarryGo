import 'dart:convert';
import '../../../core/contracts/vendor_contract.dart';
import '../../features/catalogue/state/catalogue_resource.dart';
import 'mock_onboarding_store.dart';

class MockCatalogueStore {
  MockCatalogueStore(MockOnboardingStore business, {required bool empty}) {
    if (empty) return;
    for (var i = 0; i < business.branches.length; i++) {
      final branch = business.branches[i]['id'].toString();
      final menu = 'demo-menu-$branch';
      final section = i == 0 ? 'demo-section' : 'demo-section-$i';
      _add(CatalogueKind.menus, {
        'id': menu,
        'branch_id': branch,
        'capability_type': 'RESTAURANT',
        'name': 'Everyday favourites',
        'description': 'Meals from the Karryam kitchen.',
        'status': 'ACTIVE',
      });
      _add(CatalogueKind.sections, {
        'id': section,
        'menu_id': menu,
        'name': 'Main meals',
        'description': 'Everyday favourites.',
      });
      _add(CatalogueKind.items, {
        'id': 'demo-food-$i',
        'menu_id': menu,
        'section_id': section,
        'name': 'Smoky jollof rice',
        'description': 'Long-grain rice, slow-cooked with tomatoes and spices.',
        'base_price_minor': 350000,
        'currency': 'NGN',
        'dietary_codes': [],
        'allergen_codes': [],
        'status': 'ACTIVE',
      });
    }
    for (var i = 0; i < business.branches.length; i++) {
      final branch = business.branches[i]['id'].toString();
      final dishes = [
        (
          'burger',
          'Classic beef burger',
          'Beef patty, lettuce, cheese and a toasted bun.',
          450000,
        ),
        (
          'pizza',
          'Margherita pizza',
          'Tomato, mozzarella and fresh basil.',
          650000,
        ),
        (
          'salad',
          'Garden salad',
          'Fresh greens and seasonal vegetables.',
          250000,
        ),
      ];
      for (final dish in dishes) {
        _add(CatalogueKind.items, {
          'id': 'demo-${dish.$1}-$i',
          'menu_id': 'demo-menu-$branch',
          'section_id': i == 0 ? 'demo-section' : 'demo-section-$i',
          'name': dish.$2,
          'description': dish.$3,
          'base_price_minor': dish.$4,
          'currency': 'NGN',
          'dietary_codes': [],
          'allergen_codes': [],
          'status': 'ACTIVE',
        });
      }
    }
    _add(CatalogueKind.groups, {
      'id': 'demo-group',
      'name': 'Choose your side',
      'description': 'A little extra, your way.',
      'min_select': 0,
      'max_select': 1,
    });
    _add(CatalogueKind.modifiers, {
      'id': 'demo-modifier',
      'group_id': 'demo-group',
      'name': 'Plantain',
      'price_delta_minor': 50000,
    });
    _add(CatalogueKind.categories, {
      'id': 'demo-category',
      'name': 'Pantry essentials',
      'slug': 'pantry-essentials',
      'parent_ref': null,
      'depth': 0,
    });
    _add(CatalogueKind.products, {
      'id': 'demo-product',
      'category_ref': 'demo-category',
      'name': 'Everyday grocery pack',
      'description': 'A selection of pantry essentials.',
      'product_class': 'PACKAGED',
      'operating_models': [],
      'media_refs': [],
      'status': 'ACTIVE',
    });
    _add(CatalogueKind.shopVariants, {
      'id': 'demo-variant',
      'product_ref': 'demo-product',
      'name': 'Standard pack',
      'sku': 'DEMO-PACK-01',
      'package_level': 'EACH',
      'pack_quantity': 1,
      'retail_price_minor': 275000,
      'temperature_zone': 'AMBIENT',
      'stackable': true,
      'large_load': false,
      'handling_flags': [],
    });
    for (final branch in business.branches) {
      _add(CatalogueKind.listings, {
        'id': branch == business.branches.first
            ? 'demo-listing'
            : 'demo-listing-${branch['id']}',
        'branch_ref': branch['id'],
        'product_ref': 'demo-product',
        'variant_ref': 'demo-variant',
        'retail_enabled': true,
        'wholesale_enabled': false,
        'minimum_order_quantity': 1,
        'order_multiple': 1,
        'auto_hide_when_unavailable': true,
        'logistics_available': true,
        'inventory_authority': 'NOT_IMPLEMENTED',
      });
    }
  }
  final Map<CatalogueKind, List<JsonObject>> _rows = {};
  final Map<String, ({String fingerprint, JsonObject result})> _commands = {};
  final Map<String, List<String>> _attachments = {};
  int _sequence = 0;
  void _add(CatalogueKind kind, JsonObject row) {
    (_rows[kind] ??= []).add({
      'status': 'ACTIVE',
      'version': 1,
      'position': (_rows[kind] ?? []).length,
      ...row,
    });
  }

  JsonObject? advancedRecord(CatalogueKind kind, String id) =>
      (_rows[kind] ?? []).where((row) => row['id'] == id).firstOrNull;
  List<JsonObject> advancedRecords(CatalogueKind kind) => _rows[kind] ?? [];
  JsonObject _copy(JsonObject row) => jsonObject(jsonDecode(jsonEncode(row)));
  ({CatalogueResource resource, String? id, String? action})? _match(
    String path,
  ) {
    for (final kind in CatalogueKind.values) {
      final resource = CatalogueResource(kind);
      final names = <String>[];
      final pattern = resource.collection.replaceAllMapped(
        RegExp(r'\{(\w+)\}'),
        (m) {
          names.add(m[1]!);
          return '([^/]+)';
        },
      );
      final match = RegExp(
        '^$pattern(?:/([^/]+))?(?:/([^/]+))?\$',
      ).firstMatch(path);
      if (match != null) {
        final parameters = {
          for (var i = 0; i < names.length; i++) names[i]: match[i + 1]!,
        };
        final tail = match[names.length + 1];
        final action = match[names.length + 2];
        if (action != null &&
            !['activate', 'pause', 'archive', 'restore'].contains(action)) {
          continue;
        }
        return (
          resource: CatalogueResource(kind, parameters: parameters),
          id: tail == 'reorder' ? null : tail,
          action: tail == 'reorder' ? 'reorder' : match[names.length + 2],
        );
      }
    }
    final special = RegExp(
      r'^/api/v1/vendor/(food/items|shop/listings|shop/variants)/([^/]+)(?:/([^/]+))?$',
    ).firstMatch(path);
    if (special != null) {
      return (
        resource: CatalogueResource(
          special[1] == 'food/items'
              ? CatalogueKind.items
              : special[1] == 'shop/listings'
              ? CatalogueKind.listings
              : CatalogueKind.shopVariants,
        ),
        id: special[2],
        action: special[3],
      );
    }
    return null;
  }

  bool handles(String path) =>
      _match(path) != null ||
      path.contains('/modifier-groups/') &&
          path.startsWith('/api/v1/vendor/food/items/');
  List<JsonObject> _collection(
    CatalogueResource resource,
    Map<String, String> query,
  ) {
    final rows = _rows[resource.kind] ?? [];
    return rows.where((row) {
        for (final e in resource.parameters.entries) {
          final field = switch (e.key) {
            'menuId' => 'menu_id',
            'sectionId' => 'section_id',
            'itemId' => 'item_id',
            'groupId' => 'group_id',
            'productId' => 'product_ref',
            'branchId' => 'branch_ref',
            _ => e.key,
          };
          if (row[field] != e.value) return false;
        }
        if (query['branch_id'] != null &&
            row['branch_id'] != query['branch_id']) {
          return false;
        }
        if (query['status'] != null && row['status'] != query['status']) {
          return false;
        }
        if (query['include_archived'] != 'true' &&
            resource.kind != CatalogueKind.menus &&
            row['status'] == 'ARCHIVED') {
          return false;
        }
        return true;
      }).toList()
      ..sort((a, b) => (a['position'] as int).compareTo(b['position'] as int));
  }

  bool acceptsReplacement(String id, String branchId) {
    final listing = (_rows[CatalogueKind.listings] ?? [])
        .where(
          (r) =>
              r['id'] == id &&
              r['branch_ref'] == branchId &&
              r['status'] == 'ACTIVE',
        )
        .firstOrNull;
    if (listing == null) return false;
    return (_rows[CatalogueKind.products] ?? []).any(
          (r) => r['id'] == listing['product_ref'] && r['status'] == 'ACTIVE',
        ) &&
        (_rows[CatalogueKind.shopVariants] ?? []).any(
          (r) => r['id'] == listing['variant_ref'] && r['status'] == 'ACTIVE',
        );
  }

  JsonObject? read(String path, Map<String, String> query) {
    final mediaMatch = RegExp(
      r'^/api/v1/vendor/food/items/([^/]+)/media$',
    ).firstMatch(path);
    if (mediaMatch != null) {
      final row = (_rows[CatalogueKind.items] ?? [])
          .where((r) => r['id'] == mediaMatch[1])
          .firstOrNull;
      return row == null
          ? {'_status': 404, '_message': 'Item not found.'}
          : {'media': row['media'] ?? []};
    }

    final attached = RegExp(
      r'^/api/v1/vendor/food/items/([^/]+)/modifier-groups$',
    ).firstMatch(path);
    if (attached != null) {
      final ids = _attachments[attached[1]] ?? [];
      return {
        'modifier_groups': [
          for (var i = 0; i < ids.length; i++)
            for (final group in _rows[CatalogueKind.groups] ?? [])
              if (group['id'] == ids[i])
                {
                  'modifier_group_id': group['id'],
                  'position': i,
                  'version': 1,
                  'group': _copy(group),
                },
        ],
      };
    }
    final match = _match(path);
    if (match == null) return null;
    if (match.id != null) {
      final row = (_rows[match.resource.kind] ?? [])
          .where((r) => r['id'] == match.id)
          .firstOrNull;
      if (row == null) {
        return {'_status': 404, '_message': 'Catalogue entry not found.'};
      }
      return match.resource.kind == CatalogueKind.listings
          ? {'listing': _copy(row), 'sell_units': []}
          : _copy(row);
    }
    final rows = _collection(match.resource, query);
    final offset = int.tryParse(query['offset'] ?? '') ?? 0;
    final limit = int.tryParse(query['limit'] ?? '') ?? 50;
    final page = rows
        .skip(offset)
        .take(match.resource.paginated ? limit : rows.length)
        .map(_copy)
        .toList();
    if (match.resource.kind == CatalogueKind.shopVariants) {
      return {'_list': page};
    }
    return {
      match.resource.rowsKey: page,
      if (match.resource.food)
        'pagination': {
          'has_more':
              match.resource.paginated && offset + page.length < rows.length,
          'offset': offset,
          'limit': limit,
        }
      else
        'has_more':
            match.resource.paginated && offset + page.length < rows.length,
    };
  }

  JsonObject write(
    String method,
    String path,
    JsonObject body,
    Map<String, String> headers,
  ) {
    final key = headers.entries
        .where((e) => e.key.toLowerCase() == 'idempotency-key')
        .firstOrNull
        ?.value;
    if (key == null) {
      return {'_status': 400, '_message': 'Idempotency-Key is required.'};
    }
    final fingerprint = jsonEncode([method, path, body]);
    final saved = _commands[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? _copy(saved.result)
          : {
              '_status': 409,
              '_message': 'This retry key belongs to a different command.',
            };
    }
    final result = _write(method, path, body);
    if (result['_status'] == null) {
      _commands[key] = (fingerprint: fingerprint, result: _copy(result));
    }
    return result;
  }

  JsonObject _write(String method, String path, JsonObject body) {
    final reorderAttachment = RegExp(
      r'^/api/v1/vendor/food/items/([^/]+)/modifier-groups/reorder$',
    ).firstMatch(path);
    if (reorderAttachment != null && method == 'POST') {
      final list = _attachments[reorderAttachment[1]] ?? [],
          ids = body['ordered_ids'];
      if (ids is! List ||
          ids.length != list.length ||
          ids.toSet().length != list.length ||
          list.any((id) => !ids.contains(id))) {
        return {
          '_status': 409,
          '_message': 'Refresh the full list before changing its order.',
        };
      }
      _attachments[reorderAttachment[1]!] = List<String>.from(ids);
      return {'ordered_modifier_group_ids': ids};
    }
    final attachment = RegExp(
      r'^/api/v1/vendor/food/items/([^/]+)/modifier-groups/([^/]+)$',
    ).firstMatch(path);
    if (attachment != null && ['PUT', 'DELETE'].contains(method)) {
      final item = (_rows[CatalogueKind.items] ?? [])
          .where((r) => r['id'] == attachment[1])
          .firstOrNull;
      final group = (_rows[CatalogueKind.groups] ?? [])
          .where((r) => r['id'] == attachment[2])
          .firstOrNull;
      if (item == null || group == null) {
        return {'_status': 404, '_message': 'Item or group not found.'};
      }
      if (item['status'] == 'ARCHIVED' || group['status'] == 'ARCHIVED') {
        return {
          '_status': 409,
          '_message': 'Archived entries cannot be attached.',
        };
      }
      final list = _attachments[attachment[1]!] ??= [];
      if (method == 'DELETE') {
        list.remove(group['id']);
      } else if (!list.contains(group['id'])) {
        list.add(group['id'].toString());
      }
      return {'attached': method == 'PUT'};
    }
    final match = _match(path);
    if (match == null) {
      return {'_status': 404, '_message': 'Catalogue route not found.'};
    }
    final resource = match.resource;
    final kind = resource.kind;
    if (match.action == 'reorder') {
      final list = _collection(resource, {}), ids = body['ordered_ids'];
      if (ids is! List ||
          ids.length != list.length ||
          ids.toSet().length != list.length ||
          list.any((r) => !ids.contains(r['id']))) {
        return {
          '_status': 409,
          '_message': 'Refresh the full list before changing its order.',
        };
      }
      for (final row in list) {
        row['position'] = ids.indexOf(row['id']);
        row['version'] = (row['version'] as int) + 1;
      }
      return {'ordered_ids': ids};
    }
    if (match.id == null && method == 'POST') {
      if (kind == CatalogueKind.products &&
          !(_rows[CatalogueKind.categories] ?? []).any(
            (r) => r['id'] == body['category_id'] && r['status'] != 'ARCHIVED',
          )) {
        return {'_status': 422, '_message': 'Choose an active category.'};
      }
      if (kind == CatalogueKind.menus &&
          !['RESTAURANT', 'FAST_FOOD'].contains(body['capability_type'])) {
        return {
          '_status': 422,
          '_message': 'This catalogue supports Restaurant and Fast Food only.',
        };
      }
      final row = <String, dynamic>{
        'id': 'mock-${kind.name}-${++_sequence}',
        ...body,
        'status':
            [
              CatalogueKind.menus,
              CatalogueKind.products,
              CatalogueKind.items,
            ].contains(kind)
            ? 'DRAFT'
            : 'ACTIVE',
      };
      for (final e in resource.parameters.entries) {
        row[switch (e.key) {
              'menuId' => 'menu_id',
              'sectionId' => 'section_id',
              'itemId' => 'item_id',
              'groupId' => 'group_id',
              'productId' => 'product_ref',
              'branchId' => 'branch_ref',
              _ => e.key,
            }] =
            e.value;
      }
      if (kind == CatalogueKind.items) {
        row.addAll({
          'currency': 'NGN',
          'dietary_codes': body['dietary_codes'] ?? [],
          'allergen_codes': body['allergen_codes'] ?? [],
        });
      }
      if (kind == CatalogueKind.products) {
        row.addAll({
          'operating_models': body['operating_models'] ?? [],
          'media_refs': [],
        });
      }
      if (kind == CatalogueKind.listings) {
        final variant = (_rows[CatalogueKind.shopVariants] ?? [])
            .where((r) => r['id'] == body['variant_id'])
            .firstOrNull;
        if (variant == null || variant['status'] == 'ARCHIVED') {
          return {
            '_status': 422,
            '_message': 'Choose an available product variant.',
          };
        }
        row.addAll({
          'product_ref': variant['product_ref'],
          'variant_ref': variant['id'],
          'retail_enabled': body['retail_enabled'] ?? true,
          'wholesale_enabled': body['wholesale_enabled'] ?? false,
          'inventory_authority': 'NOT_IMPLEMENTED',
        });
      }
      _transport(kind, row);
      _add(kind, row);
      return _copy((_rows[kind] ?? []).last);
    }
    final row = (_rows[kind] ?? [])
        .where((r) => r['id'] == match.id)
        .firstOrNull;
    if (row == null) {
      return {'_status': 404, '_message': 'Catalogue entry not found.'};
    }
    if (body['expected_version'] != row['version']) {
      return {
        '_status': 409,
        '_message': 'This entry changed. Refresh before saving again.',
      };
    }
    if (row['status'] == 'ARCHIVED' && match.action != 'restore') {
      return {'_status': 409, '_message': 'This entry has been archived.'};
    }
    final target = switch (match.action) {
      'activate' => 'ACTIVE',
      'pause' => 'PAUSED',
      'archive' => 'ARCHIVED',
      'restore' => 'ACTIVE',
      _ => body['status'],
    };
    if (target == 'ACTIVE' &&
        !['DRAFT', 'PAUSED', 'INACTIVE', 'ARCHIVED'].contains(row['status'])) {
      return {'_status': 409, '_message': 'This entry is already active.'};
    }
    if (target == 'PAUSED' && row['status'] != 'ACTIVE') {
      return {
        '_status': 409,
        '_message': 'Only an active entry can be paused.',
      };
    }
    if (kind == CatalogueKind.items && match.action == 'media') {
      final media = body['media'];
      if (media is! List ||
          media.any((e) => e is! Map || e['media_ref'] is! String) ||
          media.where((e) => e['is_primary'] == true).length > 1) {
        return {'_status': 400, '_message': 'Invalid media selection.'};
      }
      row['media'] = [
        for (var i = 0; i < media.length; i++)
          {...jsonObject(media[i]), 'position': i},
      ];
      row['version'] = (row['version'] as int) + 1;
      return {
        'item_id': row['id'],
        'version': row['version'],
        'media': row['media'],
      };
    }
    final update = JsonObject.from(body)..remove('expected_version');
    _transport(kind, update);
    row.addAll(update);
    if (target != null) row['status'] = target;
    if (update['clear_price_override'] == true) {
      row['price_override_minor'] = null;
    }
    row.remove('clear_price_override');
    row['version'] = (row['version'] as int) + 1;
    return _copy(row);
  }

  void _transport(CatalogueKind kind, JsonObject row) {
    final mapping = switch (kind) {
      CatalogueKind.products => {'category_id': 'category_ref'},
      CatalogueKind.categories => {'parent_id': 'parent_ref'},
      CatalogueKind.listings => {
        'retail_override': 'retail_price_override_minor',
        'wholesale_override': 'wholesale_price_override_minor',
        'minimum_order_qty': 'minimum_order_quantity',
        'maximum_order_qty': 'maximum_order_quantity',
        'auto_hide_unavailable': 'auto_hide_when_unavailable',
      },
      _ => const <String, String>{},
    };
    for (final e in mapping.entries) {
      if (row.containsKey(e.key)) row[e.value] = row.remove(e.key);
    }
  }
}
