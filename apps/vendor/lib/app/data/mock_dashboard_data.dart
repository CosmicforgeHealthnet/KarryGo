import '../../../core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';
import '../../features/orders/state/order_actions.dart';

enum MockDashboardScenario {
  populated('Active business'),
  riderSetup('First rider setup'),
  settingsRestricted('Settings access limited'),
  settingsUnavailable('Settings unavailable'),
  financeUnavailable('Finance unavailable'),
  payoutVerification('Transfer needs verification'),
  empty('Empty workspace'),
  restricted('Restricted finance'),
  unavailable('Orders unavailable');

  const MockDashboardScenario(this.label);
  final String label;
}

class MockDashboardData {
  MockDashboardData(this.store, this.scenario);
  final MockOnboardingStore store;
  final MockDashboardScenario scenario;
  List<JsonObject>? _orders;
  final Map<String, JsonObject> _picking = {};
  final Map<String, JsonObject> _handovers = {};
  final Map<String, ({String fingerprint, JsonObject result})> _commands = {};

  JsonObject command(
    String path,
    JsonObject body,
    Map<String, String> headers, {
    bool Function(String, String)? canReplace,
  }) {
    final parts = path.split('/');
    final id = parts[6];
    final order =
        (response('/api/v1/vendor/commerce/orders', {})!['vendor_orders']
                as List)
            .cast<JsonObject>()
            .where((o) => o['vendor_order_id'] == id)
            .firstOrNull;
    if (order == null) return {'_status': 404, '_message': 'Order not found.'};
    String? header(String name) => headers.entries
        .where((e) => e.key.toLowerCase() == name.toLowerCase())
        .firstOrNull
        ?.value;
    final key = header('Idempotency-Key');
    final fingerprint = '$path|$body|${header('If-Match')}';
    if (key == null) {
      return {'_status': 400, '_message': 'Idempotency-Key is required.'};
    }
    final saved = _commands[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : {
              '_status': 409,
              '_message': 'This retry key belongs to a different command.',
            };
    }
    if (path.contains('/handover/pickup/confirm')) {
      if (parts[8] != order['logistics_mission_ref'] ||
          !['READY', 'HANDED_OVER'].contains(order['status'])) {
        return {'_status': 409, '_message': 'This handover is not available.'};
      }
      final observation = _handover(order);
      if (body['handover_ref'] != observation['handover_ref'] ||
          !['OTP', 'QR'].contains(body['method']) ||
          body['credential'] !=
              (body['method'] == 'OTP' ? '654321' : 'mock-pickup-credential')) {
        return {
          '_status': 409,
          '_message': 'The pickup credential was not accepted.',
        };
      }
      observation['status'] = 'CONSUMED';
      observation['source_version'] =
          (observation['source_version'] as int) + 1;
      return JsonObject.from(observation);
    }
    final version = int.tryParse(header('If-Match') ?? '');
    if (parts.length > 8 && parts[7] == 'lines') {
      final lineId = parts[8];
      final rows = picking(order);
      final row = rows.where((r) => r['OrderLineID'] == lineId).firstOrNull;
      if (row == null) {
        return {'_status': 404, '_message': 'Picking line not found.'};
      }
      if (version !=
          (parts.last == 'substitutions'
              ? order['version']
              : parts.last == 'actual-quantity'
              ? jsonObject(row['Measurement'] ?? {})['version']
              : row['Version'])) {
        return {
          '_status': 409,
          '_message': 'This line changed. Refresh it and try again.',
        };
      }
      switch (parts.last) {
        case 'actual-quantity':
          final measurement = row['Measurement'];
          final quantity = body['actual_quantity'];
          if (measurement is! Map ||
              quantity is! int ||
              quantity < 0 ||
              quantity > (measurement['reserved_upper_quantity'] as int) ||
              quantity > (measurement['hard_max_quantity'] as int) ||
              [
                'FINALIZED',
                'FINANCIAL_PENDING',
              ].contains(measurement['state'])) {
            return {
              '_status': 409,
              '_message':
                  'Review the measured quantity and current settlement state.',
            };
          }
          final denominator = measurement['pricing_denominator'] as int;
          final amount =
              ((measurement['unit_price_minor'] as int) * quantity +
                  denominator ~/ 2) ~/
              denominator;
          measurement['actual_quantity'] = quantity;
          measurement['final_amount_minor'] = amount;
          measurement['state'] =
              amount > (order['product_subtotal_minor'] as int)
              ? 'REAPPROVAL_REQUIRED'
              : 'ACTUAL_RECORDED';
          measurement['version'] = (measurement['version'] as int) + 1;
          final result = JsonObject.from(measurement);
          _commands[key] = (fingerprint: fingerprint, result: result);
          return result;
        case 'availability':
          final picked = body['picked_quantity'];
          final unavailable = body['unavailable_quantity'];
          if (picked is! int ||
              unavailable is! int ||
              picked < 0 ||
              unavailable < 0 ||
              picked + unavailable > (row['OrderedQuantity'] as int)) {
            return {
              '_status': 409,
              '_message': 'Invalid availability quantities.',
            };
          }
          row['QuantityFound'] = picked;
          row['RemainingQuantity'] = (row['OrderedQuantity'] as int) - picked;
          row['State'] = body['outcome'] == 'FULLY_AVAILABLE'
              ? 'COMPLETE'
              : 'BLOCKED';
          break;
        case 'quality':
          if (row['_hasOutcome'] != true) {
            return {
              '_status': 409,
              '_message': 'Record availability before recording quality.',
            };
          }
          if (![
            'MET',
            'PARTIAL',
            'NOT_AVAILABLE',
            'SUBSTITUTE',
          ].contains(body['result'])) {
            return {'_status': 409, '_message': 'Invalid quality result.'};
          }
          row['_quality'] = body['result'];
          row['QualityResult'] = body['result'];
          final old = row['QualityResults'] as List;
          row['QualityResults'] = [
            for (final preference in row['QualityPreferences'] as List)
              {
                'preference_type': preference['type'],
                'preference_value': preference['value'],
                'result': body['result'],
                'observed_value': body['observed_value'] ?? '',
                'reason': body['reason'] ?? '',
                'version': old.isEmpty ? 1 : (old.first['version'] as int) + 1,
                'created_at': old.isEmpty
                    ? DateTime.now().toUtc().toIso8601String()
                    : old.first['created_at'],
              },
          ];
          break;
        case 'substitutions':
          if (row['SubstitutionPolicy'] == 'EXACT_ONLY' ||
              row['SubstitutionPolicy'] == 'AUTO_REFUND') {
            return {
              '_status': 409,
              '_message': 'This line does not allow replacement suggestions.',
            };
          }
          if (!(canReplace?.call(
                body['proposed_catalogue_ref'].toString(),
                order['branch_id'].toString(),
              ) ??
              (body['proposed_catalogue_ref'] == 'demo-listing'))) {
            return {
              '_status': 404,
              '_message': 'Replacement listing not found.',
            };
          }
          order['status'] = 'AWAITING_CUSTOMER';
          order['version'] = (order['version'] as int) + 1;
          final proposal = {
            'ID': 'demo-proposal',
            'Status': 'PENDING',
            'Version': 1,
          };
          _commands[key] = (fingerprint: fingerprint, result: proposal);
          return proposal;
        default:
          return {'_status': 404, '_message': 'Unknown line command.'};
      }
      row['Version'] = row['_hasOutcome'] == true
          ? (row['Version'] as int) + 1
          : 1;
      row['_hasOutcome'] = true;
      row['HasOutcome'] = true;
      row['QualityResult'] = row['_quality'];
      final result = {'Version': row['Version']};
      _commands[key] = (fingerprint: fingerprint, result: result);
      return result;
    }
    if (version != order['version']) {
      return {
        '_status': 409,
        '_message': 'This order changed. Refresh it and try again.',
      };
    }
    final action = availableOrderActions(
      order,
    ).where((a) => a.route == parts.last).firstOrNull;
    if (action == null) {
      return {
        '_status': 409,
        '_message': 'This action is not available for the current order state.',
      };
    }
    if (action.route == 'reject' &&
        body['reason_code']?.toString().trim().isNotEmpty != true) {
      return {'_status': 409, '_message': 'A rejection reason is required.'};
    }
    if (action.route == 'pack' &&
        order['domain'] == 'SHOP' &&
        picking(order).any(
          (r) =>
              r['State'] != 'COMPLETE' ||
              r['_quality'] != 'MET' ||
              (r['Measurement'] is Map &&
                  (r['Measurement'] as Map)['state'] != 'FINALIZED'),
        )) {
      return {
        '_status': 409,
        '_message': 'Finish availability and quality checks before packing.',
      };
    }
    order['status'] = switch (action.route) {
      'accept' => order['status'] == 'CREATED' ? 'PREPARING' : 'ACCEPTED',
      'preparing' => 'PREPARING',
      'picking' => 'PICKING',
      'pack' => 'PACKING',
      'ready' => 'READY',
      'ready-for-pickup' => 'READY_FOR_PICKUP',
      'pickup' => 'PICKED_UP',
      'handover' => 'HANDED_OVER',
      'delivering' => 'DELIVERING',
      'delivered' => 'DELIVERED',
      'complete' => 'COMPLETED',
      'cancel' => 'CANCELLED',
      'reject' => 'REJECTED',
      _ => order['status'],
    };
    order['version'] = (order['version'] as int) + 1;
    if (action.route == 'cancel') {
      order['cancel_reason_code'] = body['reason_code'];
    }
    if (action.route == 'reject') {
      order['rejection_reason'] = body['reason_code'];
    }
    final result = {
      'vendor_order': JsonObject.from(order),
      'parent_order': {'parent_order_id': order['parent_order_id']},
    };
    _commands[key] = (fingerprint: fingerprint, result: result);
    return result;
  }

  List<JsonObject> picking(JsonObject order) => [
    for (final raw in order['lines'] as List)
      _picking.putIfAbsent(
        (raw as Map)['id'].toString(),
        () => {
          'ParentOrderID': order['parent_order_id'],
          'VendorOrderID': order['vendor_order_id'],
          'OrderLineID': raw['id'],
          'OrderedQuantity': raw['quantity'],
          'QuantityFound': 0,
          'RemainingQuantity': raw['quantity'],
          'MeasurementMode': order['vendor_order_id'] == 'demo-order-1047'
              ? 'VARIABLE_COUNT'
              : 'FIXED_UNIT',
          'HasOutcome': false,
          'QualityResult': null,
          'QualityResults': <JsonObject>[],
          'Measurement': order['vendor_order_id'] == 'demo-order-1047'
              ? {
                  'mode': 'VARIABLE_COUNT',
                  'estimated_quantity': 24,
                  'unit_price_minor': raw['unit_price_minor'],
                  'estimated_amount_minor': raw['quoted_total_minor'],
                  'pricing_denominator': 12,
                  'tolerance_quantity': 2,
                  'hard_max_quantity': 30,
                  'reserved_upper_quantity': 26,
                  'actual_quantity': 0,
                  'final_amount_minor': 0,
                  'state': 'ESTIMATED',
                  'version': 7,
                }
              : null,
          'SubstitutionPolicy': 'VENDOR_SUGGEST_AND_APPROVE',
          'QualityPreferences': <JsonObject>[
            {
              'type': 'SORTING',
              'value': 'Carefully sorted',
              'source': 'CHECKOUT_EXPLICIT',
              'source_version': 1,
            },
          ],
          '_quality': 'MET',
          'State': 'PENDING',
          'Version': 1,
          'ExpectedVendorOrderVersion': order['version'],
        },
      ),
  ];
  JsonObject _handover(JsonObject order) => _handovers.putIfAbsent(
    order['vendor_order_id'].toString(),
    () => {
      'authority': 'DISPATCH',
      'mission_ref': order['logistics_mission_ref'],
      'fulfilment_ref': order['logistics_mission_ref'],
      'handover_ref': 'demo-pickup-${order['vendor_order_id']}',
      'stage': 'PICKUP',
      'status': 'ACTIVE',
      'source_version': 1,
      'direction': {
        'credential_purpose': 'COMMERCE_PICKUP',
        'holder_role': 'PROVIDER',
        'verifier_role': 'VENDOR_SENDER',
        'execution_authority': 'DISPATCH',
      },
    },
  );
  JsonObject? response(String path, Map<String, String> query) {
    if (path.startsWith('/api/v1/vendor/commerce/orders/')) {
      final orders =
          response('/api/v1/vendor/commerce/orders', {})!['vendor_orders']
              as List;
      for (final order in orders) {
        if (order['vendor_order_id'] == path.split('/')[6]) {
          if (path.contains('/handover/')) {
            if (path.split('/')[8] != order['logistics_mission_ref'] ||
                !['READY', 'HANDED_OVER'].contains(order['status'])) {
              return null;
            }
            return _handover(JsonObject.from(order as Map));
          }
          if (path.endsWith('/picking-queue') || path.endsWith('/picking')) {
            return {
              '_list': picking(JsonObject.from(order as Map))
                  .map(
                    (row) => JsonObject.from(row)
                      ..remove('_quality')
                      ..remove('_hasOutcome'),
                  )
                  .toList(),
            };
          }
          return JsonObject.from(order as Map);
        }
      }
      return null;
    }
    if (path.startsWith('/api/v1/vendor/food/menus/')) {
      final id = path.split('/')[6];
      final matches = store.branches.where(
        (branch) => id == 'demo-menu-${branch['id']}',
      );
      if (matches.isEmpty) return null;
      if (path.endsWith('/sections')) {
        return {
          'sections': [
            {
              'id': 'demo-section',
              'menu_id': id,
              'name': 'Main meals',
              'description': 'Everyday favourites.',
              'status': 'ACTIVE',
              'position': 0,
              'version': 1,
            },
          ],
        };
      }
      return JsonObject.from(
        (response('/api/v1/vendor/food/menus', {
                      'branch_id': matches.first['id'].toString(),
                    })!['menus']
                    as List)
                .first
            as Map,
      );
    }
    if (path.contains('/shop/branches/') && path.endsWith('/listings')) {
      return {
        'items': [
          {
            'id': 'demo-listing',
            'branch_ref': path.split('/')[6],
            'product_ref': 'demo-product',
            'variant_ref': 'demo-variant',
            'status': 'ACTIVE',
            'version': 1,
          },
        ],
        'pagination': {'has_more': false},
      };
    }
    if (path == '/api/v1/vendor/shop/variants/demo-variant') {
      return {
        'id': 'demo-variant',
        'product_ref': 'demo-product',
        'name': 'Standard pack',
        'status': 'ACTIVE',
        'version': 1,
      };
    }
    if (path == '/api/v1/vendor/shop/products/demo-product') {
      return JsonObject.from(
        (response('/api/v1/vendor/shop/products', {})!['items'] as List).first
            as Map,
      );
    }
    if (path == '/api/v1/vendor/shop/products/demo-product/variants') {
      return {
        '_list': [
          {
            'id': 'demo-variant',
            'product_ref': 'demo-product',
            'name': 'Standard pack',
            'sku': 'DEMO-PACK-01',
            'package_level': 'UNIT',
            'pack_quantity': 1,
            'status': 'ACTIVE',
            'version': 1,
          },
        ],
      };
    }
    if (path == '/api/v1/vendor/commerce/orders') {
      final statuses = [
        'SENT_TO_VENDOR',
        'PREPARING',
        'READY',
        'PICKING',
        'COMPLETED',
        'CANCELLED',
      ];
      final orders = _orders ??= [
        for (var i = 0; i < statuses.length; i++)
          {
            'vendor_order_id': 'demo-order-${1048 - i}',
            'parent_order_id': 'demo-parent-${1048 - i}',
            'business_id': store.vendorId,
            'branch_id':
                store.branches[i.isEven ? 0 : store.branches.length - 1]['id'],
            'domain': i == 3 || i == 1 ? 'SHOP' : 'FOOD',
            'currency': 'NGN',
            'status': statuses[i],
            'product_subtotal_minor': 1850000 - i * 100000,
            'customer_payable_minor': 1950000 - i * 100000,
            'vendor_payable_minor': 1750000 - i * 100000,
            'delivery_responsibility': i == 3
                ? 'VENDOR_OWN_DELIVERY'
                : 'PLATFORM_LOGISTICS',
            'fulfilment_mode': 'DELIVERY',
            'inventory_state': 'NOT_REQUIRED',
            'version': 1,
            if (i == 2) 'logistics_mission_ref': 'demo-mission-1046',
            'created_at':
                '2026-10-08T09:${(30 - i * 4).toString().padLeft(2, '0')}:00Z',
            'lines': [
              {
                'id': 'demo-line-$i',
                'item_name': i == 1
                    ? 'Market oranges'
                    : i == 3
                    ? 'Everyday grocery pack'
                    : 'Jollof rice meal',
                'quantity': 2,
                'unit_price_minor': (1850000 - i * 100000) ~/ 2,
                'quoted_total_minor': 1850000 - i * 100000,
              },
            ],
          },
      ];
      return {
        'vendor_orders': scenario == MockDashboardScenario.empty
            ? []
            : orders
                  .where(
                    (o) =>
                        (query['branch_id'] == null ||
                            o['branch_id'] == query['branch_id']) &&
                        (query['status'] == null ||
                            o['status'] == query['status']),
                  )
                  .toList(),
      };
    }
    if (path == '/api/v1/vendor/food/menus') {
      return {
        'menus': scenario == MockDashboardScenario.empty
            ? []
            : [
                {
                  'id':
                      'demo-menu-${query['branch_id'] ?? store.branches.first['id']}',
                  'branch_id': query['branch_id'] ?? store.branches.first['id'],
                  'capability_type': 'RESTAURANT',
                  'name': 'Everyday favourites',
                  'description': 'Meals from the Karryam kitchen.',
                  'status': 'ACTIVE',
                  'version': 1,
                },
              ],
        'pagination': {'limit': 50, 'offset': 0, 'has_more': false},
      };
    }
    if (path == '/api/v1/vendor/shop/products') {
      return {
        'items': scenario == MockDashboardScenario.empty
            ? []
            : [
                {
                  'id': 'demo-product',
                  'name': 'Everyday grocery pack',
                  'description': 'A selection of pantry essentials.',
                  'product_class': 'STANDARD',
                  'status': 'ACTIVE',
                  'version': 1,
                },
              ],
        'has_more': false,
      };
    }
    return null;
  }
}
