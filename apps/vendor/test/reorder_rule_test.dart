import 'dart:convert';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/features/inventory/state/stock_controller.dart';
import 'package:vendor/features/inventory/ui/reorder_rule.dart';
import 'package:vendor/features/inventory/ui/stock_screen.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

const basis = 'stock-demo-listing';
Future<JsonObject?> save(
  StockController c, {
  int minimum = 10,
  int target = 30,
  int? hours = 72,
}) => c.mutate(
  'PUT',
  reorderPath,
  parameters: {'stockBasisId': basis},
  body: {
    'minimum_quantity': minimum,
    'target_quantity': target,
    'expiring_soon_hours': hours,
  },
);
void main() {
  test(
    'rule creation and updates leave stock unchanged and explicitly clear expiry hours',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      expect(await c.reorderRule(basis), isNull);
      final before = await c.balance(basis);
      final first = (await save(c))!;
      expect(first['recommended_reorder_quantity'], 6);
      expect(first['status'], 'ACTIVE');
      final second = (await save(c, minimum: 0, target: 40, hours: null))!;
      expect(second['id'], first['id']);
      expect(second['version'], first['version'] + 1);
      expect(second['expiring_soon_hours'], isNull);
      expect((await c.reorderRule(basis))!['target_quantity'], 40);
      expect(await c.balance(basis), before);
      await c.load(selected: StockSection.movements);
      expect(c.rows, isEmpty);
    },
  );
  test(
    'invalid quantities and expiry windows cannot overwrite the existing rule',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      await save(c);
      for (final hours in [0, -1, 8761]) {
        expect(await save(c, hours: hours), isNull);
      }
      expect(await save(c, minimum: -1), isNull);
      expect(await save(c, target: 10), isNull);
      expect(await save(c, target: 1000000001), isNull);
      expect((await c.reorderRule(basis))!['version'], 1);
      expect(await save(c, minimum: 0, target: 1, hours: 8760), isNotNull);
    },
  );
  test(
    'recommended reorder follows quarantine and receipts, independently of minimum alerts',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      await save(c, minimum: 0);
      expect((await c.reorderRule(basis))!['recommended_reorder_quantity'], 6);
      await c.mutate(
        'POST',
        '${StockController.root}/quarantines',
        body: {
          'stock_basis_id': basis,
          'quantity': 3,
          'reason_code': 'QUALITY_CHECK',
          'note': '',
        },
      );
      expect((await c.reorderRule(basis))!['recommended_reorder_quantity'], 9);
      final receipt = (await c.mutate(
        'POST',
        '${StockController.root}/receipts',
        body: {
          'branch_id': c.branchId,
          'received_at': DateTime.now().toUtc().toIso8601String(),
          'lines': [
            {'stock_basis_id': basis, 'quantity': 10},
          ],
        },
      ))!;
      await c.mutate(
        'POST',
        '${StockController.root}/receipts/{receiptId}/post',
        parameters: {'receiptId': receipt['id']},
      );
      expect((await c.reorderRule(basis))!['recommended_reorder_quantity'], 0);
    },
  );
  test(
    'expired receipts do not reduce the recommended replenishment',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      final receipt = (await c.mutate(
        'POST',
        '${StockController.root}/receipts',
        body: {
          'branch_id': c.branchId,
          'received_at': DateTime.now().toUtc().toIso8601String(),
          'lines': [
            {
              'stock_basis_id': basis,
              'quantity': 10,
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
      expect((await save(c))!['recommended_reorder_quantity'], 6);
    },
  );
  test(
    'rule list is branch scoped and command replay does not increment the version',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      final command = VendorCommand.create();
      final body = VendorPayload({
        'minimum_quantity': 10,
        'target_quantity': 30,
        'expiring_soon_hours': null,
      });
      for (var i = 0; i < 2; i++) {
        await c.app.shopCatalogueApi.request(
          'PUT',
          reorderPath,
          pathParameters: {'stockBasisId': basis},
          body: body,
          command: command,
        );
      }
      expect((await c.reorderRule(basis))!['version'], 1);
      final east = StockController(r.backend, 'mock-branch-east', preferences: r.mockSupportPreferences);
      addTearDown(east.dispose);
      expect(
        await east.reorderRule('stock-demo-listing-mock-branch-east'),
        isNull,
      );
    },
  );
  test(
    'Live reads remain errors on denial and uncertain updates retry exact nullable payload and key',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final probe = ReorderProbe();
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://server.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: probe,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = StockController(app, 'mock-branch', preferences: VendorPreferences(inMemory: true));
      addTearDown(c.dispose);
      probe.status = 403;
      await expectLater(c.reorderRule(basis), throwsA(anything));
      probe.status = 503;
      expect(await save(c, hours: null), isNull);
      expect(c.uncertainOutcome, true);
      probe.status = 200;
      expect(await save(c, hours: null), isNotNull);
      final writes = probe.calls.where((r) => r.method == 'PUT').toList();
      expect(
        writes[0].headers['Idempotency-Key'],
        writes[1].headers['Idempotency-Key'],
      );
      expect(jsonDecode(writes[1].body), {
        'minimum_quantity': 10,
        'target_quantity': 30,
        'expiring_soon_hours': null,
      });
      expect(c.uncertainOutcome, false);
    },
  );
  testWidgets(
    'stock details open a prefilled rule sheet and save refreshes the returned recommendation',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      await tester.runAsync(c.prepare);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: StockBalanceScreen(
            controller: c,
            row: c.rows.single,
            mock: true,
            onAdjust: (_) async {},
          ),
        ),
      );
      await flush(tester);
      await tester.scrollUntilVisible(
        find.text('Set reorder rule'),
        160,
        scrollable: find.byType(Scrollable).first,
      );
      await flush(tester);
      await tester.tap(find.text('Set reorder rule'));
      await flush(tester);
      expect(find.byType(ReorderRuleEditor), findsOneWidget);
      expect(
        tester
            .widget<TextFormField>(
              find.widgetWithText(TextFormField, 'Target stock'),
            )
            .controller!
            .text,
        '30',
      );
      await tester.runAsync(() async {
        await tester.tap(find.text('Save reorder rule'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Reorder rule saved'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await flush(tester);
      await tester.scrollUntilVisible(
        find.text('Edit reorder rule'),
        150,
        scrollable: find.byType(Scrollable).first,
      );
      await flush(tester);
      expect(find.text('6 stock units'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets('editing sheet fits larger text and keyboard, validates target', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    tester.view.viewInsets = const FakeViewPadding(bottom: 300);
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetViewInsets);
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
    addTearDown(c.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(
            context,
          ).copyWith(textScaler: const TextScaler.linear(1.4)),
          child: child!,
        ),
        home: Scaffold(
          body: Align(
            alignment: Alignment.bottomCenter,
            child: ReorderRuleEditor(controller: c, basis: basis, mock: true),
          ),
        ),
      ),
    );
    await flush(tester);
    final target = find.widgetWithText(TextFormField, 'Target stock');
    await tester.ensureVisible(target);
    await tester.enterText(target, '10');
    await tester.tap(find.text('Save reorder rule'));
    await flush(tester);
    expect(
      find.text('Target must be greater than minimum stock.'),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });
}

class ReorderProbe extends http.BaseClient {
  int status = 503;
  final calls = <http.Request>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    calls.add(request as http.Request);
    return http.StreamedResponse(
      Stream.value(
        utf8.encode(
          jsonEncode(
            status == 200
                ? {
                    'success': true,
                    'data': {
                      'id': 'rule',
                      'branch_ref': 'mock-branch',
                      'stock_basis_ref': basis,
                      'variant_ref': 'variant',
                      'minimum_quantity': 10,
                      'target_quantity': 30,
                      'recommended_reorder_quantity': 6,
                      'expiring_soon_hours': null,
                      'status': 'ACTIVE',
                      'version': 1,
                    },
                  }
                : {
                    'success': false,
                    'error': {
                      'code': 'REORDER_UNAVAILABLE',
                      'message': 'Reorder unavailable',
                    },
                  },
          ),
        ),
      ),
      status,
      headers: {'content-type': 'application/json'},
    );
  }
}
