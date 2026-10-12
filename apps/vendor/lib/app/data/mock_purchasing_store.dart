import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';

class MockPurchasingStore {
  MockPurchasingStore(this.business, {required bool empty}) {
    for (final branch in business.branches) {
      final id = branch['id'] as String;
      inventory[id] = [
        {
          'id': 'mock-balance-$id',
          'business_id': vendor,
          'branch_id': id,
          'inventory_item_id': 'mock-ingredient-rice-$id',
          'on_hand': 20,
          'reserved': 0,
          'quarantined': 0,
          'available': 20,
          'revision': 1,
        },
        {
          'id': 'mock-packaging-balance-$id',
          'business_id': vendor,
          'branch_id': id,
          'inventory_item_id': 'mock-packaging-container-$id',
          'on_hand': 100,
          'reserved': 0,
          'quarantined': 0,
          'available': 100,
          'revision': 1,
        },
        {
          'id': 'mock-prepared-balance-$id',
          'business_id': vendor,
          'branch_id': id,
          'inventory_item_id': 'mock-prepared-output-$id',
          'on_hand': 0,
          'reserved': 0,
          'quarantined': 0,
          'available': 0,
          'revision': 1,
        },
      ];
    }
    policy = {
      'rules_complete': true,
      'id': 'mock-approval-policy',
      'business_id': vendor,
      'name': 'Purchasing approvals',
      'allow_self_approval': false,
      'status': 'ACTIVE',
      'version': 1,
      'rules': [
        {
          'id': 'mock-approval-rule',
          'business_id': vendor,
          'minimum_amount_minor': 0,
          'required_approval_count': 1,
          'approval_scope': 'BRANCH_OR_BUSINESS',
          'required_permission_code': 'vendor.purchasing.approve',
          'step_up_required': false,
        },
      ],
    };
    if (empty || business.branches.isEmpty) return;
    categories.add(
      base({
        'id': 'mock-supplier-category',
        'code': 'FOODSTUFF',
        'name': 'Foodstuff',
        'description': 'Rice and pantry supplies.',
        'status': 'ACTIVE',
      }),
    );
    suppliers.add(
      base({
        'id': 'mock-supplier',
        'category_id': 'mock-supplier-category',
        'code': 'HARVEST',
        'name': 'Fresh Harvest Supplies',
        'legal_name': 'Fresh Harvest Limited',
        'default_lead_time_hours': 24,
        'status': 'ACTIVE',
        'notes': '',
      }),
    );
    terms['mock-supplier'] = base({
      'supplier_id': 'mock-supplier',
      'payment_terms_days': 7,
      'currency_code': 'NGN',
      'delivery_terms': 'Delivery to branch receiving area.',
    });
    final branch = business.branches.first['id'] as String;
    products.add(
      base({
        'id': 'mock-supplier-product',
        'supplier_id': 'mock-supplier',
        'branch_id': branch,
        'inventory_item_id': 'mock-ingredient-rice-$branch',
        'supplier_sku': 'RICE-25KG',
        'purchase_unit_code': 'BAG',
        'purchase_unit_name': 'Bag',
        'conversion_mode': 'SAME_AS_INVENTORY',
        'inventory_units_per_purchase_unit': 1,
        'minimum_order_quantity': 1,
        'purchase_multiple': 1,
        'typical_cost_minor': 250000,
        'currency_code': 'NGN',
        'lead_time_hours': 24,
        'status': 'ACTIVE',
      }),
    );
    final order = createOrder({
      'branch_id': branch,
      'supplier_id': 'mock-supplier',
      'currency_code': 'NGN',
      'notes': 'Weekly pantry restock',
      'lines': [
        {
          'supplier_product_id': 'mock-supplier-product',
          'purchase_quantity': 4,
        },
      ],
    });
    order['id'] = 'mock-purchase-order';
    order['purchase_order_number'] = 'PO-2026-001';
    order['status'] = 'ORDERED';
    order['created_by_staff_id'] = 'mock-staff-2';
    order['expected_delivery_date'] = DateTime.now()
        .add(const Duration(days: 1))
        .toUtc()
        .toIso8601String();
    orders.add(order);
    alerts.add(
      base({
        'id': 'mock-purchasing-alert',
        'branch_id': branch,
        'supplier_id': 'mock-supplier',
        'purchase_order_id': order['id'],
        'alert_type': 'PO_EXPECTED_SOON',
        'severity': 'INFO',
        'status': 'OPEN',
        'details': {'purchase_order_number': order['purchase_order_number']},
      }),
    );
  }
  final MockOnboardingStore business;
  String get vendor => business.vendorId;
  String get now => DateTime.now().toUtc().toIso8601String();
  final categories = <JsonObject>[],
      suppliers = <JsonObject>[],
      contacts = <JsonObject>[],
      products = <JsonObject>[],
      orders = <JsonObject>[],
      receipts = <JsonObject>[],
      discrepancies = <JsonObject>[],
      costs = <JsonObject>[],
      alerts = <JsonObject>[],
      documents = <JsonObject>[],
      approvals = <JsonObject>[];
  final terms = <String, JsonObject>{},
      receivingPolicies = <String, JsonObject>{},
      inventory = <String, List<JsonObject>>{},
      commands = <String, JsonObject>{};
  late JsonObject policy;
  int sequence = 0;
  JsonObject base(JsonObject fields) => {
    'business_id': vendor,
    'version': 1,
    'created_at': now,
    'updated_at': now,
    ...fields,
  };
  JsonObject ok(Object? data, [int status = 200]) => {
    '_status': status,
    '_data': jsonDecode(jsonEncode(data)),
  };
  JsonObject fail(String message, [int status = 409]) => {
    '_status': status,
    '_message': message,
    '_code': 'PURCHASING_REQUEST_REFUSED',
  };
  JsonObject? find(List<JsonObject> rows, Object? id) =>
      rows.where((r) => r['id'] == id).firstOrNull;
  JsonObject receiving(String branch) => receivingPolicies.putIfAbsent(
    branch,
    () => base({
      'id': 'mock-receiving-$branch',
      'branch_id': branch,
      'over_delivery_mode': 'DENY',
      'over_delivery_tolerance_bps': 0,
      'cost_variance_approval_bps': 0,
      'cost_variance_approval_minor': 0,
      'require_cost_variance_approval': true,
    }),
  );
  List<JsonObject> orderLines(
    JsonObject body,
    String supplier,
    String branch,
    String currency,
    String orderId,
  ) {
    final inputs = (body['lines'] as List).map(jsonObject).toList();
    if (inputs.isEmpty) {
      throw StateError('Add at least one purchase order line.');
    }
    final seen = <String>{};
    return inputs.map((line) {
      final product = find(products, line['supplier_product_id']);
      if (product == null ||
          product['status'] != 'ACTIVE' ||
          product['supplier_id'] != supplier ||
          product['branch_id'] != branch ||
          product['currency_code'] != currency ||
          !seen.add(product['id'])) {
        throw StateError(
          'Choose valid supplier products for this branch and currency.',
        );
      }
      final q = line['purchase_quantity'];
      final cost = line['unit_cost_minor'] ?? product['typical_cost_minor'];
      if (q is! int ||
          q < product['minimum_order_quantity'] ||
          q % (product['purchase_multiple'] as int) != 0 ||
          cost is! int ||
          cost < 0) {
        throw StateError('Invalid purchase quantity or unit cost.');
      }
      return base({
        'id': 'mock-po-line-${++sequence}',
        'purchase_order_id': orderId,
        'supplier_product_id': product['id'],
        'supplier_product_version': product['version'],
        'inventory_item_id': product['inventory_item_id'],
        'supplier_sku_snapshot': product['supplier_sku'],
        'purchase_unit_code_snapshot': product['purchase_unit_code'],
        'purchase_unit_name_snapshot': product['purchase_unit_name'],
        'conversion_mode_snapshot': product['conversion_mode'],
        'inventory_units_per_purchase_unit_snapshot':
            product['inventory_units_per_purchase_unit'],
        'purchase_quantity': q,
        'converted_inventory_quantity':
            q * (product['inventory_units_per_purchase_unit'] as int),
        'received_purchase_quantity': 0,
        'unit_cost_minor': cost,
        'line_total_minor': q * cost,
        'currency_code_snapshot': currency,
      });
    }).toList();
  }

  JsonObject createOrder(JsonObject body) {
    final supplier = find(suppliers, body['supplier_id']);
    if (supplier == null ||
        supplier['status'] != 'ACTIVE' ||
        !inventory.containsKey(body['branch_id'])) {
      throw StateError('Choose a supplier and branch.');
    }
    final id = 'mock-po-${++sequence}';
    final lines = orderLines(
      body,
      supplier['id'],
      body['branch_id'],
      body['currency_code'],
      id,
    );
    final total = lines.fold<int>(
      0,
      (v, l) => v + (l['line_total_minor'] as int),
    );
    return base({
      'id': id,
      'branch_id': body['branch_id'],
      'supplier_id': supplier['id'],
      'supplier_name_snapshot': supplier['name'],
      'supplier_legal_name_snapshot': supplier['legal_name'],
      'supplier_code_snapshot': supplier['code'],
      'payment_terms_days_snapshot':
          terms[supplier['id']]?['payment_terms_days'] ?? 0,
      'delivery_terms_snapshot': terms[supplier['id']]?['delivery_terms'] ?? '',
      'purchase_order_number': 'PO-${sequence.toString().padLeft(4, '0')}',
      'currency_code': body['currency_code'],
      'subtotal_minor': total,
      'total_amount_minor': total,
      'status': 'DRAFT',
      'notes': body['notes'] ?? '',
      'expected_delivery_date': body['expected_delivery_date'],
      'created_by_staff_id': 'mock-staff-1',
      'approval_count': 0,
      'required_approval_count': 1,
      'approval_permission_code': 'vendor.purchasing.approve',
      'approval_scope': 'BRANCH_OR_BUSINESS',
      'self_approval_allowed': policy['allow_self_approval'],
      'approval_step_up_required': false,
      'lines': lines,
    });
  }

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final path = request.url.path;
    final inv = RegExp(
      r'^/api/v1/vendor/inventory/branches/([^/]+)/balances$',
    ).firstMatch(path);
    if (inv != null) {
      if (!inventory.containsKey(inv[1])) return fail('Branch not found.', 404);
      return ok({'items': inventory[inv[1]], 'has_more': false});
    }
    if (!path.startsWith('/api/v1/vendor/purchasing/')) return null;
    final segments = path
        .substring('/api/v1/vendor/purchasing/'.length)
        .split('/');
    final kind = segments[0];
    final branch = request.url.queryParameters['branch_id'];
    if (branch != null && !inventory.containsKey(branch)) {
      return fail('Branch not found.', 404);
    }
    if (request.method == 'GET') {
      return read(segments, request.url.queryParameters);
    }
    final key = request.headers.entries
        .where((e) => e.key.toLowerCase() == 'idempotency-key')
        .firstOrNull
        ?.value;
    final fingerprint = jsonEncode({
      'method': request.method,
      'path': path,
      'body': body,
      'version': request.headers.entries
          .where((e) => e.key.toLowerCase() == 'if-match')
          .firstOrNull
          ?.value,
    });
    if (kind != 'alerts') {
      if (key == null || key.isEmpty) {
        return fail('Idempotency-Key is required.', 400);
      }
      if (commands.containsKey(key)) {
        return commands[key]!['fingerprint'] == fingerprint
            ? jsonObject(jsonDecode(jsonEncode(commands[key]!['response'])))
            : fail('Request key conflicts with an earlier change.');
      }
    }
    try {
      final out = write(segments, request.method, body, request.headers);
      if (kind != 'alerts' && out['_message'] == null) {
        commands[key!] = {
          'fingerprint': fingerprint,
          'response': jsonObject(jsonDecode(jsonEncode(out))),
        };
      }
      return out;
    } on StateError catch (e) {
      return fail(e.message);
    } catch (_) {
      return fail('Invalid purchasing request.', 400);
    }
  }

  JsonObject read(List<String> p, Map<String, String> query) {
    final kind = p[0];
    final branch = query['branch_id'];
    if (kind == 'approval-policy') return ok(policy);
    if (kind == 'receiving-policies') {
      return inventory.containsKey(p[2])
          ? ok(receiving(p[2]))
          : fail('Branch not found.', 404);
    }
    if (kind == 'dashboard') {
      return ok({
        'dashboard': {
          'business_id': vendor,
          'branch_ids': branch == null ? inventory.keys.toList() : [branch],
          'scope': 'BRANCH',
          'purchase_orders': {
            for (final state in [
              'DRAFT',
              'SUBMITTED',
              'APPROVED',
              'ORDERED',
              'PARTIALLY_RECEIVED',
              'RECEIVED',
              'CLOSED',
              'REJECTED',
              'CANCELLED',
            ])
              state.toLowerCase(): orders
                  .where(
                    (o) =>
                        (branch == null || o['branch_id'] == branch) &&
                        o['status'] == state,
                  )
                  .length,
          },
          'receiving': {
            'awaiting_inspection': receipts
                .where(
                  (r) =>
                      r['status'] == 'DRAFT' &&
                      (branch == null || r['branch_id'] == branch),
                )
                .length,
          },
          'discrepancies': {
            'unresolved': discrepancies
                .where(
                  (d) =>
                      !['RESOLVED', 'VOIDED'].contains(d['status']) &&
                      (branch == null || d['branch_id'] == branch),
                )
                .length,
          },
        },
      });
    }
    if (kind == 'outstanding-deliveries') {
      return ok({
        'deliveries': orders
            .where(
              (o) =>
                  ['ORDERED', 'PARTIALLY_RECEIVED'].contains(o['status']) &&
                  (branch == null || o['branch_id'] == branch),
            )
            .map(
              (o) => {
                'purchase_order_id': o['id'],
                'purchase_order_number': o['purchase_order_number'],
                'branch_id': o['branch_id'],
                'supplier_name': o['supplier_name_snapshot'],
                'status': o['status'],
                'expected_delivery_date': o['expected_delivery_date'],
                'total_amount_minor': o['total_amount_minor'],
                'currency_code': o['currency_code'],
              },
            )
            .toList(),
      });
    }
    if (kind == 'alerts') {
      return ok({
        'alerts': alerts
            .where((a) => branch == null || a['branch_id'] == branch)
            .toList(),
      });
    }
    if (kind == 'suppliers' && p.length >= 3) {
      final supplier = find(suppliers, p[1]);
      if (supplier == null) return fail('Supplier not found.', 404);
      if (p[2] == 'terms') {
        return terms[p[1]] == null
            ? fail('Supplier terms not found.', 404)
            : ok(terms[p[1]]);
      }
      if (p[2] == 'performance') {
        return ok({
          'performance': {
            'business_id': vendor,
            'supplier_id': p[1],
            'total_purchase_orders': orders
                .where((o) => o['supplier_id'] == p[1])
                .length,
            'total_goods_receipts': receipts
                .where((r) => r['supplier_id'] == p[1])
                .length,
            'on_time_rate': {
              'value': null,
              'metric_status': 'INSUFFICIENT_HISTORY',
              'sample_count': 0,
            },
            'acceptance_rate': {
              'value': null,
              'metric_status': 'INSUFFICIENT_HISTORY',
              'sample_count': 0,
            },
          },
        });
      }
      final rows = p[2] == 'contacts'
          ? contacts
          : p[2] == 'products'
          ? products
          : null;
      if (rows == null) return fail('Not found.', 404);
      final subset = rows
          .where(
            (r) =>
                r['supplier_id'] == p[1] &&
                (p[2] != 'products' ||
                    branch == null ||
                    r['branch_id'] == branch),
          )
          .toList();
      if (p.length == 4) {
        final row = find(subset, p[3]);
        return row == null ? fail('Record not found.', 404) : ok(row);
      }
      return ok(paginate(subset, query));
    }
    if (kind == 'purchase-orders' && p.length == 3 && p[2] == 'approvals') {
      return ok(
        approvals.where((a) => a['purchase_order_id'] == p[1]).toList(),
      );
    }
    if (kind == 'goods-receipts' && p.length == 3 && p[2] == 'documents') {
      return ok(documents.where((d) => d['goods_receipt_id'] == p[1]).toList());
    }
    final rows = switch (kind) {
      'suppliers' => suppliers,
      'supplier-categories' => categories,
      'purchase-orders' => orders,
      'goods-receipts' => receipts,
      'supplier-discrepancies' => discrepancies,
      'purchase-cost-history' => costs,
      _ => null,
    };
    if (rows == null) return fail('Not found.', 404);
    if (p.length == 2) {
      final row = find(rows, p[1]);
      return row == null ||
              branch != null &&
                  row['branch_id'] != null &&
                  row['branch_id'] != branch
          ? fail('Record not found.', 404)
          : ok(row);
    }
    final subset = rows
        .where(
          (r) =>
              (branch == null ||
                  r['branch_id'] == null ||
                  r['branch_id'] == branch) &&
              (query['status'] == null ||
                  query['status'] == '' ||
                  r['status'] == query['status']),
        )
        .toList();
    return ok(paginate(subset, query));
  }

  List<JsonObject> paginate(List<JsonObject> rows, Map<String, String> query) =>
      rows
          .skip(int.tryParse(query['offset'] ?? '0') ?? 0)
          .take(int.tryParse(query['limit'] ?? '100') ?? 100)
          .toList();
  JsonObject write(
    List<String> p,
    String method,
    JsonObject body,
    Map<String, String> headers,
  ) {
    final kind = p[0];
    if (kind == 'approval-policy') {
      if ((body['expected_version'] ?? 0) != policy['version']) {
        return fail('Approval policy changed.');
      }
      policy = {...policy, ...body, 'version': (policy['version'] as int) + 1};
      policy.remove('expected_version');
      return ok(policy);
    }
    if (kind == 'receiving-policies') {
      final old = receiving(p[2]);
      receivingPolicies[p[2]] = {
        ...old,
        ...body,
        'version': (old['version'] as int) + 1,
      };
      return ok(receivingPolicies[p[2]]);
    }
    if (kind == 'alerts') {
      final row = find(alerts, p[1]);
      if (row == null) return fail('Alert not found.', 404);
      row['status'] = p[2] == 'acknowledge' ? 'ACKNOWLEDGED' : 'RESOLVED';
      return ok({'alert': row});
    }
    if (kind == 'purchase-orders') return writeOrder(p, method, body);
    if (kind == 'goods-receipts') return writeReceipt(p, method, body, headers);
    if (kind == 'supplier-discrepancies') {
      final row = find(discrepancies, p[1]);
      if (row == null) return fail('Discrepancy not found.', 404);
      if (headerVersion(headers) != row['version']) {
        return fail('Discrepancy changed.');
      }
      final permitted = switch (row['status']) {
        'OPEN' => ['approve', 'reject', 'resolve'],
        'APPROVAL_REQUIRED' => ['approve', 'reject'],
        'APPROVED' || 'REJECTED' => ['resolve'],
        _ => <String>[],
      };
      if (!permitted.contains(p[2])) {
        return fail('This discrepancy no longer permits that decision.');
      }
      row['status'] = {
        'approve': 'APPROVED',
        'reject': 'REJECTED',
        'resolve': 'RESOLVED',
      }[p[2]];
      row['notes'] = body['notes'] ?? row['notes'];
      row['version'] = (row['version'] as int) + 1;
      return ok(row);
    }
    String resource = kind;
    List<JsonObject>? rows;
    String? supplier;
    if (kind == 'suppliers' && p.length >= 3) {
      supplier = p[1];
      if (find(suppliers, supplier) == null) {
        return fail('Supplier not found.', 404);
      }
      resource = p[2];
      rows = resource == 'contacts'
          ? contacts
          : resource == 'products'
          ? products
          : null;
      if (resource == 'terms') {
        final old = terms[supplier];
        if ((body['expected_version'] ?? 0) != (old?['version'] ?? 0)) {
          return fail('Supplier terms changed.');
        }
        terms[supplier] = base({
          ...body,
          'supplier_id': supplier,
          'version': (old?['version'] ?? 0) + 1,
        });
        terms[supplier]!.remove('expected_version');
        return ok(terms[supplier]);
      }
    } else {
      rows = kind == 'suppliers'
          ? suppliers
          : kind == 'supplier-categories'
          ? categories
          : null;
    }
    if (rows == null) return fail('Unsupported purchasing command.', 404);
    final idIndex = supplier == null ? 1 : 3;
    final id = p.length > idIndex ? p[idIndex] : null;
    if (id == null) {
      if (resource == 'products') {
        final branch = body['branch_id'];
        if (!inventory.containsKey(branch) ||
            !inventory[branch]!.any(
              (i) => i['inventory_item_id'] == body['inventory_item_id'],
            )) {
          return fail('Inventory item is not available for this branch.', 404);
        }
        if (body['conversion_mode'] == 'SAME_AS_INVENTORY' &&
            body['inventory_units_per_purchase_unit'] != 1) {
          return fail('Same inventory units require a conversion of one.');
        }
      }
      final row = base({
        'id': 'mock-$resource-${++sequence}',
        ...body,
        'supplier_id': ?supplier,
        'status': body['status'] ?? 'ACTIVE',
      });
      rows.add(row);
      return ok(row, 201);
    }
    final row = find(rows, id);
    if (row == null || supplier != null && row['supplier_id'] != supplier) {
      return fail('Record not found.', 404);
    }
    if (body['expected_version'] != row['version']) {
      return fail('This record changed.');
    }
    if (p.last == 'archive') {
      row['status'] = 'ARCHIVED';
    } else {
      row.addAll(body);
      row.remove('expected_version');
    }
    row['version'] = (row['version'] as int) + 1;
    return ok(row);
  }

  int? headerVersion(Map<String, String> headers) => int.tryParse(
    headers.entries
            .where((e) => e.key.toLowerCase() == 'if-match')
            .firstOrNull
            ?.value ??
        '',
  );
  JsonObject writeOrder(List<String> p, String method, JsonObject body) {
    if (p.length == 1) {
      final order = createOrder(body);
      orders.add(order);
      return ok(order, 201);
    }
    final row = find(orders, p[1]);
    if (row == null) return fail('Purchase order not found.', 404);
    if (body['expected_version'] != row['version']) {
      return fail('Purchase order changed.');
    }
    if (p.length == 2 || p.last == 'lines') {
      if (row['status'] != 'DRAFT') return fail('Only a draft can be edited.');
      if (p.last == 'lines') {
        final lines = orderLines(
          body,
          row['supplier_id'],
          row['branch_id'],
          row['currency_code'],
          row['id'],
        );
        row['lines'] = lines;
        row['total_amount_minor'] = lines.fold<int>(
          0,
          (a, l) => a + (l['line_total_minor'] as int),
        );
        row['subtotal_minor'] = row['total_amount_minor'];
      } else {
        row['notes'] = body['notes'];
        row['expected_delivery_date'] = body['expected_delivery_date'];
      }
    } else {
      final action = p[2];
      final allowed = switch (row['status']) {
        'DRAFT' => ['submit', 'cancel'],
        'SUBMITTED' => ['approve', 'reject', 'cancel'],
        'APPROVED' => ['order', 'cancel'],
        'ORDERED' => ['cancel'],
        'RECEIVED' => ['close'],
        _ => <String>[],
      };
      if (!allowed.contains(action)) {
        return fail('This purchase order no longer permits that action.');
      }
      if (action == 'cancel' &&
          (row['lines'] as List).any(
            (l) => l['received_purchase_quantity'] > 0,
          )) {
        return fail('A received order cannot be cancelled.');
      }
      if (['approve', 'reject'].contains(action)) {
        if (row['self_approval_allowed'] != true &&
            row['created_by_staff_id'] == 'mock-staff-1') {
          return fail(
            'Another staff member must approve this purchase order.',
            403,
          );
        }
        if (row['approval_step_up_required'] == true &&
            (body['step_up_grant_id'] ?? '') == '') {
          return fail('Verify this approval first.', 403);
        }
        if (approvals.any(
          (a) =>
              a['purchase_order_id'] == row['id'] &&
              a['approver_staff_id'] == 'mock-staff-1',
        )) {
          return fail(
            'You have already recorded an approval decision for this order.',
          );
        }
        approvals.add(
          base({
            'id': 'mock-approval-${++sequence}',
            'purchase_order_id': row['id'],
            'approver_staff_id': 'mock-staff-1',
            'decision': action == 'approve' ? 'APPROVED' : 'REJECTED',
            'note': body['note'] ?? '',
          }),
        );
      }
      if (action == 'submit') {
        row['self_approval_allowed'] = policy['allow_self_approval'];
        row['required_approval_count'] =
            policy['rules'][0]['required_approval_count'];
        row['approval_step_up_required'] =
            policy['rules'][0]['step_up_required'];
        row['approval_permission_code'] =
            policy['rules'][0]['required_permission_code'];
      }
      if (action == 'approve') {
        row['approval_count'] = (row['approval_count'] as int) + 1;
        row['status'] = row['approval_count'] >= row['required_approval_count']
            ? 'APPROVED'
            : 'SUBMITTED';
      } else {
        row['status'] = {
          'submit': 'SUBMITTED',
          'reject': 'REJECTED',
          'order': 'ORDERED',
          'cancel': 'CANCELLED',
          'close': 'CLOSED',
        }[action];
      }
    }
    row['version'] = (row['version'] as int) + 1;
    return ok(row);
  }

  JsonObject writeReceipt(
    List<String> p,
    String method,
    JsonObject body,
    Map<String, String> headers,
  ) {
    if (p.length == 1) {
      final order = find(orders, body['purchase_order_id']);
      if (order == null ||
          !['ORDERED', 'PARTIALLY_RECEIVED'].contains(order['status'])) {
        return fail('Purchase order is not open for receiving.');
      }
      final receipt = base({
        'id': 'mock-goods-receipt-${++sequence}',
        'purchase_order_id': order['id'],
        'supplier_id': order['supplier_id'],
        'branch_id': order['branch_id'],
        'currency': order['currency_code'],
        'status': 'DRAFT',
        'received_by_staff_id': 'mock-staff-1',
        'lines': <JsonObject>[],
        ...body,
      });
      receipts.add(receipt);
      return ok(receipt, 201);
    }
    final row = find(receipts, p[1]);
    if (row == null) return fail('Receipt not found.', 404);
    if (p.last == 'documents') {
      final doc = base({
        'id': 'mock-receipt-document-${++sequence}',
        'goods_receipt_id': row['id'],
        ...body,
      });
      documents.add(doc);
      return ok(doc, 201);
    }
    if (headerVersion(headers) != row['version']) {
      return fail('Receipt changed.');
    }
    final order = find(orders, row['purchase_order_id'])!;
    if (p.last == 'inspection') {
      if (!['DRAFT', 'INSPECTED'].contains(row['status'])) {
        return fail('This receipt cannot be inspected.');
      }
      final inputs = (body['lines'] as List).map(jsonObject).toList();
      if (inputs.isEmpty) return fail('Inspect at least one line.');
      final seen = <String>{};
      final lines = <JsonObject>[];
      for (final input in inputs) {
        final delivered = input['delivered_quantity'] as int,
            accepted = input['accepted_quantity'] as int,
            damaged = input['damaged_quantity'] as int,
            rejected = input['rejected_quantity'] as int;
        if ([
              delivered,
              accepted,
              damaged,
              rejected,
              input['short_quantity'] as int,
            ].any((q) => q < 0) ||
            delivered != accepted + damaged + rejected) {
          return fail(
            'Delivered must equal accepted + damaged + rejected.',
            400,
          );
        }
        final poLine = (order['lines'] as List)
            .map(jsonObject)
            .where((l) => l['id'] == input['purchase_order_line_id'])
            .firstOrNull;
        if (input['wrong_item'] == true) {
          if (poLine != null ||
              accepted != 0 ||
              (input['wrong_item_reference'] ?? '') == '' ||
              (input['purchase_unit_code'] ?? '') == '' ||
              delivered == 0) {
            return fail('Wrong items cannot create accepted inventory.', 400);
          }
        } else if (poLine == null || !seen.add(poLine['id'])) {
          return fail('Choose each order line only once.', 400);
        }
        lines.add({
          'id': 'mock-gr-line-${++sequence}',
          'goods_receipt_id': row['id'],
          'purchase_order_id': order['id'],
          ...input,
          if (poLine != null) ...{
            'supplier_product_id': poLine['supplier_product_id'],
            'inventory_item_id': poLine['inventory_item_id'],
            'purchase_unit_code': poLine['purchase_unit_code_snapshot'],
            'inventory_units_per_purchase_unit':
                poLine['inventory_units_per_purchase_unit_snapshot'],
            'expected_unit_cost_minor': poLine['unit_cost_minor'],
            'ordered_quantity_snapshot': poLine['purchase_quantity'],
            'received_before_snapshot': poLine['received_purchase_quantity'],
            'inventory_quantity':
                accepted *
                (poLine['inventory_units_per_purchase_unit_snapshot'] as int),
          },
        });
      }
      for (final line in lines) {
        if (line['wrong_item'] != true) {
          final poLine = (order['lines'] as List).firstWhere(
            (l) => l['id'] == line['purchase_order_line_id'],
          );
          final policy = receiving(row['branch_id']);
          final maximum =
              (poLine['purchase_quantity'] as int) +
              (policy['over_delivery_mode'] == 'ALLOW_WITH_TOLERANCE'
                  ? (poLine['purchase_quantity'] as int) *
                        (policy['over_delivery_tolerance_bps'] as int) ~/
                        10000
                  : 0);
          if ((poLine['received_purchase_quantity'] as int) +
                  (line['delivered_quantity'] as int) >
              maximum) {
            return fail('Delivered quantity exceeds the receiving tolerance.');
          }
        }
      }
      for (final issue in discrepancies.where(
        (d) => d['goods_receipt_id'] == row['id'],
      )) {
        issue['status'] = 'VOIDED';
      }
      for (final line in lines) {
        void issue(String type, {int? quantity, int? expected, int? actual}) {
          final diff = expected == null || actual == null
              ? null
              : (actual - expected).abs();
          final bps = expected == null || actual == null
              ? null
              : expected == 0
              ? (diff == 0 ? 0 : 10000)
              : diff! * 10000 ~/ expected;
          final policy = receiving(row['branch_id']);
          final approval =
              type == 'COST_VARIANCE' &&
              policy['require_cost_variance_approval'] == true &&
              diff! > 0 &&
              (policy['cost_variance_approval_bps'] == 0 &&
                      policy['cost_variance_approval_minor'] == 0 ||
                  policy['cost_variance_approval_bps'] > 0 &&
                      bps! >= policy['cost_variance_approval_bps'] ||
                  policy['cost_variance_approval_minor'] > 0 &&
                      diff >= policy['cost_variance_approval_minor']);
          discrepancies.add(
            base({
              'id': 'mock-discrepancy-${++sequence}',
              'supplier_id': row['supplier_id'],
              'branch_id': row['branch_id'],
              'purchase_order_id': order['id'],
              'goods_receipt_id': row['id'],
              'type': type,
              'status': approval ? 'APPROVAL_REQUIRED' : 'OPEN',
              'quantity': quantity,
              'expected_cost_minor': expected,
              'actual_cost_minor': actual,
              'variance_minor': diff,
              'variance_bps': bps,
              'currency': row['currency'],
              'notes': line['quality_notes'] ?? '',
            }),
          );
        }

        if (line['wrong_item'] == true) {
          issue('WRONG_ITEM', quantity: line['delivered_quantity']);
          continue;
        }
        for (final entry in [
          ('damaged_quantity', 'DAMAGED'),
          ('rejected_quantity', 'REJECTED'),
          ('short_quantity', 'SHORT_DELIVERY'),
        ]) {
          if (line[entry.$1] > 0) issue(entry.$2, quantity: line[entry.$1]);
        }
        if (line['quality_issue'] == true) {
          issue('QUALITY_ISSUE', quantity: line['delivered_quantity']);
        }
        final expected = line['expected_unit_cost_minor'] as int;
        final actual = (line['actual_unit_cost_minor'] ?? expected) as int;
        if (expected != actual) {
          issue(
            'COST_VARIANCE',
            expected: expected * (line['delivered_quantity'] as int),
            actual: actual * (line['delivered_quantity'] as int),
          );
        }
        final excess =
            (line['received_before_snapshot'] as int) +
            (line['delivered_quantity'] as int) -
            (line['ordered_quantity_snapshot'] as int);
        if (excess > 0) issue('OVER_DELIVERY', quantity: excess);
      }
      row['lines'] = lines;
      row['status'] = 'INSPECTED';
      row['version'] = (row['version'] as int) + 1;
      return ok(row);
    }
    if (p.last == 'confirm') {
      if (row['status'] != 'INSPECTED') {
        return fail('Inspect the receipt before posting.');
      }
      final receivingPolicy = receiving(row['branch_id']);
      for (final line in row['lines']) {
        if (line['wrong_item'] == true) continue;
        final poLine = (order['lines'] as List).firstWhere(
          (l) => l['id'] == line['purchase_order_line_id'],
        );
        final total =
            (poLine['received_purchase_quantity'] as int) +
            (line['accepted_quantity'] as int);
        final allowed =
            poLine['purchase_quantity'] +
            (receivingPolicy['over_delivery_mode'] == 'ALLOW_WITH_TOLERANCE'
                ? (poLine['purchase_quantity'] as int) *
                      (receivingPolicy['over_delivery_tolerance_bps'] as int) ~/
                      10000
                : 0);
        if (total > allowed) {
          return fail(
            'Accepted quantities exceed the permitted order quantity.',
          );
        }
      }
      var accepted = 0, inventoryQuantity = 0;
      final issues = discrepancies
          .where(
            (d) =>
                d['goods_receipt_id'] == row['id'] && d['status'] != 'VOIDED',
          )
          .length;
      for (final line in row['lines']) {
        final poLine = (order['lines'] as List)
            .where((l) => l['id'] == line['purchase_order_line_id'])
            .firstOrNull;
        if (poLine != null) {
          poLine['received_purchase_quantity'] +=
              (line['accepted_quantity'] as int);
          accepted += line['accepted_quantity'] as int;
          inventoryQuantity += line['inventory_quantity'] as int;
          final balance = inventory[row['branch_id']]!.firstWhere(
            (i) => i['inventory_item_id'] == line['inventory_item_id'],
          );
          balance['on_hand'] += line['inventory_quantity'];
          balance['available'] = balance['on_hand'];
          balance['revision'] += 1;
          costs.add(
            base({
              'id': 'mock-cost-${++sequence}',
              'branch_id': row['branch_id'],
              'supplier_id': row['supplier_id'],
              'supplier_product_id': line['supplier_product_id'],
              'inventory_item_id': line['inventory_item_id'],
              'goods_receipt_id': row['id'],
              'purchase_order_id': order['id'],
              'evidence_type': 'OBSERVED',
              'purchase_quantity': line['accepted_quantity'],
              'inventory_quantity': line['inventory_quantity'],
              'purchase_unit_cost_minor':
                  line['actual_unit_cost_minor'] ??
                  line['expected_unit_cost_minor'],
              'inventory_unit_cost_minor':
                  ((line['actual_unit_cost_minor'] ??
                                  line['expected_unit_cost_minor'])
                              as int) %
                          (line['inventory_units_per_purchase_unit'] as int) ==
                      0
                  ? ((line['actual_unit_cost_minor'] ??
                                line['expected_unit_cost_minor'])
                            as int) ~/
                        (line['inventory_units_per_purchase_unit'] as int)
                  : null,
              'total_cost_minor':
                  (line['accepted_quantity'] as int) *
                  ((line['actual_unit_cost_minor'] ??
                          line['expected_unit_cost_minor'])
                      as int),
              'currency': row['currency'],
              'observed_at': now,
            }),
          );
        }
      }
      order['status'] =
          (order['lines'] as List).every(
            (l) => l['received_purchase_quantity'] >= l['purchase_quantity'],
          )
          ? 'RECEIVED'
          : 'PARTIALLY_RECEIVED';
      order['version'] += 1;
      row['status'] = 'POSTED';
      row['inventory_receipt_id'] = 'mock-inventory-receipt-${row['id']}';
      row['version'] += 1;
      return ok({
        'receipt': row,
        'inventory_receipt_id': row['inventory_receipt_id'],
        'accepted_purchase_quantity': accepted,
        'accepted_inventory_quantity': inventoryQuantity,
        'discrepancy_count': issues,
        'cost_history_count': costs
            .where((c) => c['goods_receipt_id'] == row['id'])
            .length,
        'replayed': false,
      });
    }
    if (p.last == 'reverse') {
      if (row['status'] != 'POSTED' || (body['reason'] ?? '').trim().isEmpty) {
        return fail('A posted receipt and reversal reason are required.');
      }
      for (final line in row['lines']) {
        if (line['inventory_item_id'] != null) {
          final balance = inventory[row['branch_id']]!.firstWhere(
            (i) => i['inventory_item_id'] == line['inventory_item_id'],
          );
          if (balance['available'] < line['inventory_quantity']) {
            return fail('Received stock is no longer intact.');
          }
        }
      }
      for (final line in row['lines']) {
        if (line['inventory_item_id'] != null) {
          final balance = inventory[row['branch_id']]!.firstWhere(
            (i) => i['inventory_item_id'] == line['inventory_item_id'],
          );
          balance['on_hand'] -= line['inventory_quantity'];
          balance['available'] = balance['on_hand'];
          final poLine = (order['lines'] as List).firstWhere(
            (l) => l['id'] == line['purchase_order_line_id'],
          );
          poLine['received_purchase_quantity'] -= line['accepted_quantity'];
        }
      }
      order['status'] =
          (order['lines'] as List).any(
            (l) => l['received_purchase_quantity'] > 0,
          )
          ? 'PARTIALLY_RECEIVED'
          : 'ORDERED';
      order['version'] += 1;
      row['status'] = 'REVERSED';
      row['reversal_reason'] = body['reason'];
      row['version'] += 1;
      return ok(row);
    }
    return fail('Unsupported receipt action.', 404);
  }
}
