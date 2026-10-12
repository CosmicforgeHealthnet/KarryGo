import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/inventory/state/stock_controller.dart';
import 'package:vendor/features/inventory/ui/stock_screen.dart';
import 'package:vendor/features/inventory/ui/stock_editor.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  testWidgets(
    'already tracked branch listings explain receiving instead of an empty selector',
    (tester) async {
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      await tester.runAsync(c.prepare);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: StockEditor(runtime: r, controller: c, kind: StockEdit.basis),
        ),
      );
      await flush(tester);
      expect(
        find.textContaining('No active untracked listing'),
        findsOneWidget,
      );
      expect(find.text('Branch listing'), findsNothing);
      expect(find.text('Stock measure'), findsNothing);
      final enable = tester.widget<FilledButton>(
        find.ancestor(
          of: find.text('Enable stock tracking'),
          matching: find.byType(FilledButton),
        ),
      );
      expect(enable.onPressed, isNull);
      await tester.tap(find.text('Receive goods'));
      await flush(tester);
      expect(find.text('Save draft receipt'), findsOneWidget);
      expect(find.text('Stock item'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

  test(
    'receiving is draft first, posting is repeat safe, and branch stock stays isolated',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      await c.prepare();
      expect(c.error, isNull);
      expect(c.rows, hasLength(1));
      final basis = c.rows.single['stock_basis_ref'].toString();
      final receipt = await c.mutate(
        'POST',
        '${StockController.root}/receipts',
        body: {
          'branch_id': c.branchId,
          'source_ref': 'DELIVERY-TEST',
          'received_at': DateTime.now().toUtc().toIso8601String(),
          'lines': [
            {'stock_basis_id': basis, 'quantity': 10},
          ],
        },
      );
      expect(receipt!['status'], 'DRAFT');
      expect((await c.balance(basis))['on_hand_quantity'], 24);
      final parameters = {'receiptId': receipt['id'].toString()};
      expect(
        (await c.mutate(
          'POST',
          '${StockController.root}/receipts/{receiptId}/post',
          parameters: parameters,
        ))!['status'],
        'POSTED',
      );
      expect((await c.balance(basis))['on_hand_quantity'], 34);
      await c.mutate(
        'POST',
        '${StockController.root}/receipts/{receiptId}/post',
        parameters: parameters,
      );
      expect((await c.balance(basis))['on_hand_quantity'], 34);
      final east = StockController(r.backend, 'mock-branch-east');
      addTearDown(east.dispose);
      await east.load();
      expect(east.rows.single['physical_on_hand'], 24);
      await c.load(selected: StockSection.movements);
      expect(c.rows, hasLength(1));
    },
  );
  test('adjustments enforce balance revision and insufficient stock', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = StockController(r.backend, 'mock-branch');
    addTearDown(c.dispose);
    await c.load();
    final basis = c.rows.single['stock_basis_ref'].toString();
    final before = await c.balance(basis);
    final body = {
      'stock_basis_id': basis,
      'expected_revision': before['revision'],
      'quantity_delta': -5,
      'reason_code': 'DAMAGE',
    };
    expect(
      await c.mutate('POST', '${StockController.root}/adjustments', body: body),
      isNotNull,
    );
    expect((await c.balance(basis))['on_hand_quantity'], 19);
    expect(
      await c.mutate('POST', '${StockController.root}/adjustments', body: body),
      isNull,
    );
    expect(c.errorCode, 'SHOP_INVENTORY_STALE_REVISION');
    expect(
      await c.mutate(
        'POST',
        '${StockController.root}/adjustments',
        body: {...body, 'expected_revision': 2, 'quantity_delta': -30},
      ),
      isNull,
    );
    expect((await c.balance(basis))['on_hand_quantity'], 19);
  });
  test(
    'stocktake requires every count, preserves zero, and rejects a moved snapshot',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      await c.load();
      final basis = c.rows.single['stock_basis_ref'].toString();
      Future<JsonObject> start() async => (await c.mutate(
        'POST',
        '${StockController.branchPath}/stocktakes',
        parameters: {'branchId': c.branchId},
        body: {'stocktake_type': 'FULL_STOCKTAKE'},
      ))!;
      final take = await start();
      final parameters = {'stocktakeId': take['id'].toString()};
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/stocktakes/{stocktakeId}/post',
          parameters: parameters,
        ),
        isNull,
      );
      expect(
        await c.mutate(
          'PUT',
          '${StockController.root}/stocktakes/{stocktakeId}/lines/{stockBasisId}',
          parameters: {...parameters, 'stockBasisId': basis},
          body: {'counted_quantity': 0},
        ),
        isNotNull,
      );
      expect(
        (await c.detail(
          'stocktakes',
          take['id'],
        ))['lines'][0]['counted_quantity'],
        0,
      );
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/stocktakes/{stocktakeId}/post',
          parameters: parameters,
        ),
        isNotNull,
      );
      expect((await c.balance(basis))['on_hand_quantity'], 0);
      final second = await start();
      await c.mutate(
        'PUT',
        '${StockController.root}/stocktakes/{stocktakeId}/lines/{stockBasisId}',
        parameters: {'stocktakeId': second['id'], 'stockBasisId': basis},
        body: {'counted_quantity': 0},
      );
      final balance = await c.balance(basis);
      await c.mutate(
        'POST',
        '${StockController.root}/adjustments',
        body: {
          'stock_basis_id': basis,
          'expected_revision': balance['revision'],
          'quantity_delta': 1,
          'reason_code': 'FOUND_STOCK',
        },
      );
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/stocktakes/{stocktakeId}/post',
          parameters: {'stocktakeId': second['id']},
        ),
        isNull,
      );
      expect(c.errorCode, 'SHOP_INVENTORY_STALE_REVISION');
      expect((await c.balance(basis))['on_hand_quantity'], 1);
    },
  );
  test(
    'draft cancellation leaves stock untouched and alerts never mutate stock',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      await c.load();
      final basis = c.rows.single['stock_basis_ref'].toString();
      final draft = (await c.mutate(
        'POST',
        '${StockController.root}/receipts',
        body: {
          'branch_id': c.branchId,
          'lines': [
            {'stock_basis_id': basis, 'quantity': 2},
          ],
        },
      ))!;
      await c.mutate(
        'POST',
        '${StockController.root}/receipts/{receiptId}/cancel',
        parameters: {'receiptId': draft['id']},
      );
      expect((await c.detail('receipts', draft['id']))['status'], 'CANCELLED');
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/receipts/{receiptId}/post',
          parameters: {'receiptId': draft['id']},
        ),
        isNull,
      );
      await c.load(selected: StockSection.alerts);
      final id = c.rows.single['id'].toString();
      await c.mutate(
        'POST',
        '${StockController.root}/alerts/{alertId}/acknowledge',
        parameters: {'alertId': id},
      );
      await c.load();
      expect(c.rows.single['status'], 'ACKNOWLEDGED');
      await c.mutate(
        'POST',
        '${StockController.root}/alerts/{alertId}/resolve',
        parameters: {'alertId': id},
      );
      await c.load();
      expect(c.rows.single['status'], 'RESOLVED');
      expect((await c.balance(basis))['on_hand_quantity'], 24);
    },
  );
  test(
    'the transport returns the original response for the same adjustment command',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.shopCatalogueApi;
      final command = VendorCommand.create();
      final body = VendorPayload({
        'stock_basis_id': 'stock-demo-listing',
        'expected_revision': 1,
        'quantity_delta': 3,
        'reason_code': 'FOUND_STOCK',
      });
      final first = await api.request(
        'POST',
        '${StockController.root}/adjustments',
        body: body,
        command: command,
      );
      final second = await api.request(
        'POST',
        '${StockController.root}/adjustments',
        body: body,
        command: command,
      );
      expect(second.object, first.object);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      expect((await c.balance('stock-demo-listing'))['on_hand_quantity'], 27);
    },
  );
  test(
    'posted receipt reversal is allowed only while its lot is intact',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      Future<JsonObject> receive() async {
        final draft = (await c.mutate(
          'POST',
          '${StockController.root}/receipts',
          body: {
            'branch_id': c.branchId,
            'lines': [
              {'stock_basis_id': 'stock-demo-listing', 'quantity': 10},
            ],
          },
        ))!;
        await c.mutate(
          'POST',
          '${StockController.root}/receipts/{receiptId}/post',
          parameters: {'receiptId': draft['id']},
        );
        return draft;
      }

      final first = await receive();
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/receipts/{receiptId}/cancel',
          parameters: {'receiptId': first['id']},
        ),
        isNotNull,
      );
      expect((await c.balance('stock-demo-listing'))['on_hand_quantity'], 24);
      final second = await receive();
      final balance = await c.balance('stock-demo-listing');
      await c.mutate(
        'POST',
        '${StockController.root}/adjustments',
        body: {
          'stock_basis_id': 'stock-demo-listing',
          'expected_revision': balance['revision'],
          'quantity_delta': -30,
          'reason_code': 'DAMAGE',
        },
      );
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/receipts/{receiptId}/cancel',
          parameters: {'receiptId': second['id']},
        ),
        isNull,
      );
      expect((await c.balance('stock-demo-listing'))['on_hand_quantity'], 4);
    },
  );
  test(
    'expired received lots increase on-hand but not physical availability',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      final receipt = (await c.mutate(
        'POST',
        '${StockController.root}/receipts',
        body: {
          'branch_id': c.branchId,
          'lines': [
            {
              'stock_basis_id': 'stock-demo-listing',
              'quantity': 7,
              'expires_at': DateTime.now()
                  .subtract(const Duration(days: 1))
                  .toUtc()
                  .toIso8601String(),
            },
          ],
        },
      ))!;
      await c.mutate(
        'POST',
        '${StockController.root}/receipts/{receiptId}/post',
        parameters: {'receiptId': receipt['id']},
      );
      await c.load();
      expect(c.rows.single['physical_on_hand'], 31);
      expect(c.rows.single['physical_available'], 24);
    },
  );
  test(
    'stock tracking refuses approximate measures for exact consumption',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      expect(
        await c.mutate(
          'PUT',
          '${StockController.branchPath}/stock-bases',
          parameters: {'branchId': c.branchId},
          body: {
            'listing_id': 'demo-listing',
            'measure_id': 'demo-measure-bowl',
            'consumption_mode': 'EXACT_CONVERSION',
            'allocation_policy': 'FIFO',
            'lot_tracking': true,
            'expected_version': 0,
          },
        ),
        isNull,
      );
      expect(c.errorCode, 'INVENTORY_CONVERSION_UNRESOLVED');
    },
  );
  test(
    'live read errors stay errors and an uncertain adjustment retries with the same key',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final probe = InventoryProbe();
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://server.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: probe,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = StockController(app, 'branch');
      addTearDown(c.dispose);
      await c.load();
      expect(c.rows, isEmpty);
      expect(c.errors[StockSection.balances], isNotNull);
      final body = {
        'stock_basis_id': 'basis',
        'expected_revision': 9,
        'quantity_delta': 2,
        'reason_code': 'FOUND_STOCK',
      };
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/adjustments',
          body: body,
        ),
        isNull,
      );
      expect(c.commands, hasLength(1));
      probe.fail = false;
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/adjustments',
          body: body,
        ),
        isNotNull,
      );
      final calls = probe.calls.where((r) => r.method == 'POST').toList();
      expect(calls[0].headers['Idempotency-Key'], isNotEmpty);
      expect(
        calls[0].headers['Idempotency-Key'],
        calls[1].headers['Idempotency-Key'],
      );
      expect(jsonDecode(calls[0].body), body);
      expect(c.commands, isEmpty);
    },
  );
  testWidgets(
    'inventory fits a narrow phone and stock detail opens as a full screen',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: StockScreen(
            runtime: r,
            branchId: 'mock-branch',
            branchName: 'Main branch',
          ),
        ),
      );
      await flush(tester);
      expect(find.text('24 available'), findsOneWidget);
      await tester.tap(find.text('24 available'));
      await flush(tester);
      expect(find.text('Stock detail'), findsOneWidget);
      expect(find.text('Adjust stock'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'receiving form uses existing stock selectors and mock defaults',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      await tester.runAsync(c.prepare);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: StockEditor(runtime: r, controller: c, kind: StockEdit.receipt),
        ),
      );
      await flush(tester);
      expect(find.text('DELIVERY-001'), findsOneWidget);
      expect(find.text('10'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'saving a receipt opens its review screen before stock is posted',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: StockScreen(
            runtime: r,
            branchId: 'mock-branch',
            branchName: 'Main branch',
          ),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('Receiving'));
      await flush(tester);
      await tester.tap(find.text('Receive goods'));
      await flush(tester);
      await tester.runAsync(() async {
        await tester.tap(find.text('Save draft receipt'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Receipt saved'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await flush(tester);
      expect(find.text('Goods receipt'), findsOneWidget);
      expect(find.text('Draft'), findsOneWidget);
      final c = StockController(r.backend, 'mock-branch');
      addTearDown(c.dispose);
      final balance = await tester.runAsync(
        () => c.balance('stock-demo-listing'),
      );
      expect(balance!['on_hand_quantity'], 24);
      expect(tester.takeException(), isNull);
    },
  );
}

class InventoryProbe extends http.BaseClient {
  bool fail = true;
  final calls = <http.Request>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    calls.add(request as http.Request);
    return http.StreamedResponse(
      Stream.value(
        utf8.encode(
          jsonEncode(
            fail
                ? {
                    'success': false,
                    'error': {
                      'code': 'SERVICE_UNAVAILABLE',
                      'message': 'Inventory unavailable',
                    },
                  }
                : {
                    'success': true,
                    'data': {'id': 'adjustment', 'status': 'POSTED'},
                  },
          ),
        ),
      ),
      fail ? 503 : 201,
      headers: {'content-type': 'application/json'},
    );
  }
}
