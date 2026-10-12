import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/purchasing/state/purchasing_workspace_controller.dart';
import 'package:vendor/features/purchasing/ui/purchasing_workspace_screen.dart';
import 'package:vendor/features/purchasing/ui/purchase_lines_screen.dart';
import 'package:vendor/features/purchasing/ui/purchase_receiving_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

class PurchaseTransport extends FinanceTransport {
  String? losePath;
  final attempts = <JsonObject>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.method == 'GET' || !request.url.path.contains('/purchasing/')) {
      return super.send(request);
    }
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    attempts.add({
      'path': request.url.path,
      'body': jsonDecode(utf8.decode(bytes)),
      'headers': {
        for (final e in request.headers.entries) e.key.toLowerCase(): e.value,
      },
    });
    final result = await super.send(copy);
    if (losePath == request.url.path) {
      losePath = null;
      await result.stream.drain<void>();
      throw http.ClientException('Accepted response lost');
    }
    return result;
  }
}

Future<String> branchOf(SupportRuntime runtime) async {
  final response = (await runtime.backend.businessApi.request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches',
    pathParameters: {'businessId': runtime.backend.scope.vendorId!},
  ));
  final branches = response.data is List
      ? response.data as List
      : response.object['items'] as List;
  return branches[0]['id'];
}

void main() {
  test(
    'suppliers use body versions and preserve immutable retry after reopening',
    () async {
      final transport = PurchaseTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final c = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/suppliers',
      );
      await c.load();
      expect(c.error, isNull);
      final body = <String, dynamic>{
        'code': 'TEST',
        'name': 'Test Supplier',
        'legal_name': 'Test Supplier Limited',
        'default_lead_time_hours': 24,
        'status': 'ACTIVE',
        'notes': '',
      };
      transport.losePath = '/api/v1/vendor/purchasing/suppliers';
      expect(
        await c.save(
          method: 'POST',
          route: '$purchasingBase/suppliers',
          body: body,
        ),
        isNull,
      );
      body['name'] = 'Mutated';
      final restored = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/suppliers',
      );
      await restored.load();
      final saved = await restored.save();
      expect(saved, isNotNull);
      expect(saved!['name'], 'Test Supplier');
      expect(transport.attempts[0], transport.attempts[1]);
      expect(
        await restored.save(
          method: 'PATCH',
          route: '$purchasingBase/suppliers/{supplierId}',
          params: {'supplierId': saved['id']},
          body: {
            'code': 'TEST',
            'name': 'Updated',
            'legal_name': 'Test Supplier Limited',
            'default_lead_time_hours': 24,
            'status': 'ACTIVE',
            'notes': '',
            'expected_version': saved['version'],
          },
        ),
        isNotNull,
      );
      await restored.load();
      expect(
        await restored.save(
          method: 'POST',
          route: '$purchasingBase/suppliers/{supplierId}/archive',
          params: {'supplierId': saved['id']},
          body: {'expected_version': saved['version']},
        ),
        isNull,
      );
      expect(restored.pending, isNull);
      expect(restored.stale, isTrue);
      c.dispose();
      restored.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test(
    'receipt inspection generates issues and confirmation replay posts accepted stock once',
    () async {
      final transport = PurchaseTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final branch = await branchOf(runtime);
      final c = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/goods-receipts',
        branchId: branch,
      );
      await c.load();
      final receipt = await c.save(
        method: 'POST',
        route: '$purchasingBase/goods-receipts',
        body: {
          'purchase_order_id': 'mock-purchase-order',
          'received_at': DateTime.now().toUtc().toIso8601String(),
        },
      );
      expect(receipt, isNotNull);
      final order = (await app.purchasingApi.request(
        'GET',
        '$purchasingBase/purchase-orders/{purchaseOrderId}',
        pathParameters: {'purchaseOrderId': 'mock-purchase-order'},
      )).object;
      final body = {
        'lines': [
          {
            'purchase_order_line_id': order['lines'][0]['id'],
            'delivered_quantity': 4,
            'accepted_quantity': 3,
            'damaged_quantity': 1,
            'rejected_quantity': 0,
            'short_quantity': 0,
            'wrong_item': false,
            'quality_issue': false,
            'actual_unit_cost_minor': 260000,
          },
        ],
      };
      final inspected = await c.save(
        method: 'PUT',
        route: '$purchasingBase/goods-receipts/{receiptId}/inspection',
        params: {'receiptId': receipt!['id']},
        headerVersion: receipt['version'],
        body: body,
      );
      expect(inspected?['status'], 'INSPECTED');
      final issues = (await app.purchasingApi.request(
        'GET',
        '$purchasingBase/supplier-discrepancies',
        query: {'branch_id': branch},
      )).list.map(jsonObject).toList();
      expect(
        issues.map((i) => i['type']),
        containsAll(['DAMAGED', 'COST_VARIANCE']),
      );
      expect(
        issues.firstWhere((i) => i['type'] == 'COST_VARIANCE')['status'],
        'APPROVAL_REQUIRED',
      );
      transport.losePath =
          '/api/v1/vendor/purchasing/goods-receipts/${receipt['id']}/confirm';
      expect(
        await c.save(
          method: 'POST',
          route: '$purchasingBase/goods-receipts/{receiptId}/confirm',
          params: {'receiptId': receipt['id']},
          headerVersion: inspected!['version'],
          body: {},
        ),
        isNull,
      );
      final restored = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/goods-receipts',
        branchId: branch,
      );
      await restored.load();
      final posted = await restored.save();
      expect(posted?['receipt']['status'], 'POSTED');
      final confirms = transport.attempts
          .where((a) => (a['path'] as String).endsWith('/confirm'))
          .toList();
      expect(confirms, hasLength(2));
      expect(confirms[0], confirms[1]);
      expect(confirms.first['body'], isEmpty);
      expect(confirms.first['headers']['if-match'], '2');
      final balances = (await app.inventoryApi.request(
        'GET',
        '/api/v1/vendor/inventory/branches/{branchId}/balances',
        pathParameters: {'branchId': branch},
      )).object;
      expect(balances['items'][0]['on_hand'], 23);
      final latest = (await app.purchasingApi.request(
        'GET',
        '$purchasingBase/purchase-orders/{purchaseOrderId}',
        pathParameters: {'purchaseOrderId': 'mock-purchase-order'},
      )).object;
      expect(latest['status'], 'PARTIALLY_RECEIVED');
      expect(latest['lines'][0]['received_purchase_quantity'], 3);
      c.dispose();
      restored.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test(
    'self approval is refused and invalid inspection cannot enter stock',
    () async {
      final transport = PurchaseTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final branch = await branchOf(runtime);
      final c = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/purchase-orders',
        branchId: branch,
      );
      await c.load();
      final draft = await c.save(
        method: 'POST',
        route: '$purchasingBase/purchase-orders',
        body: {
          'branch_id': branch,
          'supplier_id': 'mock-supplier',
          'currency_code': 'NGN',
          'lines': [
            {
              'supplier_product_id': 'mock-supplier-product',
              'purchase_quantity': 1,
            },
          ],
        },
      );
      expect(draft?['status'], 'DRAFT');
      final submitted = await c.save(
        method: 'POST',
        route: '$purchasingBase/purchase-orders/{purchaseOrderId}/submit',
        params: {'purchaseOrderId': draft!['id']},
        body: {'expected_version': draft['version']},
      );
      expect(submitted?['status'], 'SUBMITTED');
      expect(
        await c.save(
          method: 'POST',
          route: '$purchasingBase/purchase-orders/{purchaseOrderId}/approve',
          params: {'purchaseOrderId': draft['id']},
          body: {'expected_version': submitted!['version']},
        ),
        isNull,
      );
      expect(c.error, contains('Another staff'));
      expect(c.pending, isNull);
      expect(
        validatePurchaseInspection({
          'delivered_quantity': 4,
          'accepted_quantity': 4,
          'damaged_quantity': 1,
          'rejected_quantity': 0,
          'short_quantity': 0,
          'wrong_item': false,
        }),
        isNotNull,
      );
      expect(
        validatePurchaseInspection({
          'delivered_quantity': 1,
          'accepted_quantity': 1,
          'damaged_quantity': 0,
          'rejected_quantity': 0,
          'short_quantity': 0,
          'wrong_item': true,
          'wrong_item_reference': 'Other',
          'purchase_unit_code': 'BAG',
        }),
        isNotNull,
      );
      c.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test(
    'lost alert action is read back without a second non-idempotent POST',
    () async {
      final transport = PurchaseTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final branch = await branchOf(runtime);
      final c = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/alerts',
        branchId: branch,
        wrapper: 'alerts',
      );
      await c.load();
      expect(c.rows, hasLength(1));
      transport.losePath =
          '/api/v1/vendor/purchasing/alerts/mock-purchasing-alert/acknowledge';
      expect(
        await c.save(
          method: 'POST',
          route: '$purchasingBase/alerts/{alertId}/acknowledge',
          params: {'alertId': 'mock-purchasing-alert'},
          body: {},
        ),
        isNull,
      );
      final restored = PurchasingWorkspaceController(
        runtime,
        '$purchasingBase/alerts',
        branchId: branch,
        wrapper: 'alerts',
      );
      await restored.load();
      expect(await restored.save(), isNotNull);
      expect(transport.attempts, hasLength(1));
      expect(
        transport.attempts.first['headers'].containsKey('idempotency-key'),
        isFalse,
      );
      c.dispose();
      restored.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  testWidgets('purchasing hub and receiving details fit a small dark phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final app = (await tester.runAsync(() => financeApp(PurchaseTransport())))!;
    final runtime = SupportRuntime(app);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: PurchasingHub(runtime: runtime),
      ),
    );
    await flush(tester);
    expect(find.text('Supply overview'), findsOneWidget);
    expect(tester.takeException(), isNull);
    final branch = (await tester.runAsync(() => branchOf(runtime)))!;
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: PurchaseLinesScreen(
          runtime: runtime,
          branchId: branch,
          supplierId: 'mock-supplier',
          currency: 'NGN',
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Add order line'), findsOneWidget);
    await tester.tap(find.text('Add order line'));
    await tester.pumpAndSettle();
    expect(find.text('Purchase quantity'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    runtime.dispose();
    app.dispose();
  });
}
