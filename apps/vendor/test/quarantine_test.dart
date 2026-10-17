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
import 'package:vendor/features/inventory/ui/stock_screen.dart';
import 'package:vendor/features/inventory/ui/quarantine_screen.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

const basis = 'stock-demo-listing';
JsonObject holdBody({int quantity = 3, String reason = 'QUALITY_CHECK'}) => {
  'stock_basis_id': basis,
  'quantity': quantity,
  'reason_code': reason,
  'note': 'Quality review',
};
Future<JsonObject?> hold(
  StockController c, {
  int quantity = 3,
  String reason = 'QUALITY_CHECK',
}) => c.mutate(
  'POST',
  '${StockController.root}/quarantines',
  body: holdBody(quantity: quantity, reason: reason),
);
Future<JsonObject?> resolve(StockController c, String id, String action) =>
    c.mutate(
      'POST',
      '${StockController.root}/quarantines/{quarantineId}/resolve',
      parameters: {'quarantineId': id},
      body: {'action': action},
    );

void main() {
  test(
    'quarantine holds sellable stock without reducing on-hand and release is repeat-safe',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      await c.prepare();
      final before = await c.balance(basis);
      final record = (await hold(c))!;
      expect(record['status'], 'ACTIVE');
      expect(record.containsKey('note'), false);
      final held = await c.balance(basis);
      expect(held['on_hand_quantity'], 24);
      expect(held['available_quantity'], 21);
      expect(held['quarantined_quantity'], 3);
      expect(held['revision'], before['revision'] + 1);
      await c.load(selected: StockSection.quarantines);
      expect(c.rows.single['id'], record['id']);
      expect((await c.quarantine(record['id']))['quantity'], 3);
      expect(
        (await resolve(c, record['id'], 'RELEASE'))!['status'],
        'RELEASED',
      );
      final after = await c.balance(basis);
      expect(after['on_hand_quantity'], 24);
      expect(after['available_quantity'], 24);
      expect(after['quarantined_quantity'], 0);
      await resolve(c, record['id'], 'RELEASE');
      expect((await c.balance(basis))['revision'], after['revision']);
      expect(await resolve(c, record['id'], 'DISPOSE'), isNull);
      await c.load(selected: StockSection.movements);
      expect(c.rows.map((r) => r['movement_type']), [
        'QUARANTINE_RELEASE',
        'QUARANTINE_IN',
      ]);
    },
  );
  test(
    'disposal removes the hold and on-hand without increasing sellable stock',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      final record = (await hold(c, quantity: 5, reason: 'DAMAGED'))!;
      expect(
        (await resolve(c, record['id'], 'DISPOSE'))!['status'],
        'DISPOSED',
      );
      final after = await c.balance(basis);
      expect(after['on_hand_quantity'], 19);
      expect(after['available_quantity'], 19);
      expect(after['quarantined_quantity'], 0);
      await resolve(c, record['id'], 'DISPOSE');
      expect((await c.balance(basis))['revision'], after['revision']);
      expect(await resolve(c, record['id'], 'RELEASE'), isNull);
      await c.load(selected: StockSection.movements);
      expect(c.rows.first['movement_type'], 'WASTE');
      expect(c.rows.first['on_hand_delta'], -5);
      expect(c.rows.first['quarantined_delta'], -5);
    },
  );
  test(
    'invalid reasons, quantities and actions leave stock unchanged and branches stay isolated',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      for (final n in [0, -1, 25, 1000000001]) {
        expect(await hold(c, quantity: n), isNull);
      }
      expect(await hold(c, reason: 'NEW_REASON'), isNull);
      expect((await c.balance(basis))['on_hand_quantity'], 24);
      final record = (await hold(c))!;
      expect(await resolve(c, record['id'], 'PARTIAL_RELEASE'), isNull);
      expect((await c.balance(basis))['quarantined_quantity'], 3);
      final east = StockController(r.backend, 'mock-branch-east', preferences: r.mockSupportPreferences);
      addTearDown(east.dispose);
      await east.load(selected: StockSection.quarantines);
      expect(east.rows, isEmpty);
      expect(
        (await east.balance(
          'stock-demo-listing-mock-branch-east',
        ))['on_hand_quantity'],
        24,
      );
      final response = await c.app.shopCatalogueApi.request(
        'GET',
        '${StockController.branchPath}/quarantines',
        pathParameters: {'branchId': c.branchId},
        query: {'status': 'RELEASED', 'limit': '1', 'offset': '0'},
      );
      expect(StockController.collection(response.data), isEmpty);
    },
  );
  test(
    'quarantined lots cannot be consumed by adjustments or receipt reversal',
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
              'quantity': 4,
              'expires_at': DateTime.now()
                  .add(const Duration(days: 1))
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
      final record = (await hold(c, quantity: 4))!;
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/receipts/{receiptId}/cancel',
          parameters: {'receiptId': receipt['id']},
        ),
        isNull,
      );
      final b = await c.balance(basis);
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/adjustments',
          body: {
            'stock_basis_id': basis,
            'quantity_delta': -24,
            'expected_revision': b['revision'],
            'reason_code': 'MANUAL_CORRECTION',
          },
        ),
        isNotNull,
      );
      expect((await c.balance(basis))['on_hand_quantity'], 4);
      expect((await c.balance(basis))['available_quantity'], 0);
      await resolve(c, record['id'], 'RELEASE');
      expect((await c.balance(basis))['available_quantity'], 4);
    },
  );
  test(
    'exact command replay creates one quarantine and pagination retains resolved cases',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      final key = VendorCommand.create();
      final body = VendorPayload(holdBody());
      final first = await c.app.shopCatalogueApi.request(
        'POST',
        '${StockController.root}/quarantines',
        body: body,
        command: key,
      );
      final second = await c.app.shopCatalogueApi.request(
        'POST',
        '${StockController.root}/quarantines',
        body: body,
        command: key,
      );
      expect(second.object['id'], first.object['id']);
      expect((await c.balance(basis))['quarantined_quantity'], 3);
      await resolve(c, first.object['id'], 'RELEASE');
      await hold(c, reason: 'RECALL');
      final page = await c.app.shopCatalogueApi.request(
        'GET',
        '${StockController.branchPath}/quarantines',
        pathParameters: {'branchId': c.branchId},
        query: {'limit': '1', 'offset': '0'},
      );
      expect(page.object['has_more'], true);
      expect(StockController.collection(page.data), hasLength(1));
      await c.load(selected: StockSection.quarantines);
      expect(c.rows, hasLength(2));
    },
  );
  test(
    'Live permission and network failures remain errors and retry preserves action and key',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final probe = QuarantineProbe();
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://server.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: probe,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = StockController(app, 'branch', preferences: VendorPreferences(inMemory: true));
      addTearDown(c.dispose);
      probe.status = 403;
      await c.load(selected: StockSection.quarantines);
      expect(c.rows, isEmpty);
      expect(c.errors[StockSection.quarantines], isNotNull);
      probe.status = 503;
      expect(await resolve(c, 'case', 'DISPOSE'), isNull);
      expect(c.uncertainOutcome, true);
      probe.status = 200;
      expect(await resolve(c, 'case', 'DISPOSE'), isNotNull);
      final writes = probe.calls.where((r) => r.method == 'POST').toList();
      expect(
        writes[0].headers['Idempotency-Key'],
        writes[1].headers['Idempotency-Key'],
      );
      expect(jsonDecode(writes[1].body), {'action': 'DISPOSE'});
      expect(c.uncertainOutcome, false);
    },
  );
  test(
    'expired stock cannot be quarantined and releasing a hold does not make expired units sellable',
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
              'quantity': 4,
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
      expect(await hold(c, quantity: 25), isNull);
      final record = (await hold(c, quantity: 24))!;
      expect((await c.balance(basis))['available_quantity'], 0);
      await resolve(c, record['id'], 'RELEASE');
      final balance = await c.balance(basis);
      expect(balance['on_hand_quantity'], 28);
      expect(balance['available_quantity'], 24);
    },
  );
  testWidgets(
    'phone quarantine creation reviews stock, confirms and opens detail with direct actions',
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
      await tester.ensureVisible(find.text('Quarantine'));
      await tester.tap(find.text('Quarantine'));
      await flush(tester);
      await tester.tap(find.text('Quarantine stock'));
      await flush(tester);
      expect(find.byType(QuarantineEditor), findsOneWidget);
      await tester.tap(find.text('Review quarantine'));
      await flush(tester);
      expect(find.text('Confirm quarantine'), findsOneWidget);
      await tester.runAsync(() async {
        await tester.tap(find.text('Confirm quarantine'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Stock quarantined'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await flush(tester);
      expect(find.byType(QuarantineScreen), findsOneWidget);
      expect(find.text('Release stock'), findsOneWidget);
      expect(find.text('Dispose stock'), findsOneWidget);
      expect(find.textContaining('On hold'), findsOneWidget);
      await tester.tap(find.text('Release stock'));
      await flush(tester);
      await tester.runAsync(() async {
        await tester.tap(find.text('Release stock').last);
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Stock released'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await flush(tester);
      expect(find.text('Hold removed'), findsOneWidget);
      expect(find.text('Dispose stock'), findsNothing);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'disposal requires confirmation and cancelling the sheet leaves the hold intact',
    (tester) async {
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      final c = StockController(r.backend, 'mock-branch', preferences: r.mockSupportPreferences);
      addTearDown(c.dispose);
      await tester.runAsync(c.prepare);
      final record = (await tester.runAsync(() => hold(c)))!;
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: QuarantineScreen(controller: c, id: record['id']),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('Dispose stock'));
      await flush(tester);
      expect(find.textContaining('Permanently remove all 3'), findsOneWidget);
      Navigator.pop(tester.element(find.text('Dispose stock').last));
      await flush(tester);
      expect(
        (await tester.runAsync(() => c.quarantine(record['id'])))!['status'],
        'ACTIVE',
      );
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'quarantine form fits large text and keyboard and validates unavailable quantities',
    (tester) async {
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
      await tester.runAsync(c.prepare);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          builder: (context, child) => MediaQuery(
            data: MediaQuery.of(
              context,
            ).copyWith(textScaler: const TextScaler.linear(1.4)),
            child: child!,
          ),
          home: QuarantineEditor(runtime: r, controller: c),
        ),
      );
      await flush(tester);
      final input = find.widgetWithText(TextFormField, 'Quantity to hold');
      await tester.scrollUntilVisible(
        input,
        180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.enterText(input, '25');
      await tester.tap(find.text('Review quarantine'));
      await flush(tester);
      await tester.ensureVisible(
        find.text('Quantity exceeds currently available stock.'),
      );
      expect(
        find.text('Quantity exceeds currently available stock.'),
        findsOneWidget,
      );
      expect(find.text('Confirm quarantine'), findsNothing);
      expect(tester.takeException(), isNull);
    },
  );
}

class QuarantineProbe extends http.BaseClient {
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
                      'id': 'case',
                      'status': 'DISPOSED',
                      'branch_ref': 'branch',
                      'stock_basis_ref': 'basis',
                      'variant_ref': 'variant',
                      'quantity': 3,
                      'reason_code': 'QUALITY_CHECK',
                    },
                  }
                : {
                    'success': false,
                    'error': {
                      'code': status == 403
                          ? 'FORBIDDEN'
                          : 'SERVICE_UNAVAILABLE',
                      'message': 'Quarantine unavailable',
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
