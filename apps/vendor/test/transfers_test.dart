import 'package:vendor/app/data/vendor_preferences.dart';
import 'dart:convert';
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
import 'package:vendor/features/inventory/ui/transfer_editor.dart';
import 'package:vendor/features/inventory/ui/transfer_screen.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

const sourceBasis = 'stock-demo-listing';
const destinationBasis = 'stock-demo-listing-mock-branch-east';
JsonObject transferBody({int quantity = 5}) => {
  'source_branch_id': 'mock-branch',
  'destination_branch_id': 'mock-branch-east',
  'lines': [
    {
      'source_stock_basis_id': sourceBasis,
      'destination_stock_basis_id': destinationBasis,
      'quantity': quantity,
    },
  ],
};
Future<JsonObject> draft(StockController c, {int quantity = 5}) async =>
    (await c.mutate(
      'POST',
      '${StockController.root}/transfers',
      body: transferBody(quantity: quantity),
    ))!;
Future<JsonObject?> action(
  StockController c,
  String id,
  String action, {
  JsonObject? body,
}) => c.mutate(
  'POST',
  '${StockController.root}/transfers/{transferId}/$action',
  parameters: {'transferId': id},
  body: body,
);
JsonObject arrival(
  String line, {
  int received = 0,
  int damaged = 0,
  int missing = 0,
}) => {
  'lines': [
    {
      'line_id': line,
      'received_quantity': received,
      'damaged_quantity': damaged,
      'missing_quantity': missing,
    },
  ],
};

void main() {
  test(
    'draft transfers are stock-neutral, visible at both branches and cancellable',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(
        r.backend,
        'mock-branch',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(c.dispose);
      final created = await draft(c);
      expect(created['status'], 'DRAFT');
      expect((await c.balance(sourceBasis))['on_hand_quantity'], 24);
      await c.load(selected: StockSection.transfers);
      expect(c.rows, hasLength(1));
      expect(c.name('mock-branch-east'), isNot('mock-branch-east'));
      final east = StockController(
        r.backend,
        'mock-branch-east',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(east.dispose);
      await east.load(selected: StockSection.transfers);
      expect(east.rows, hasLength(1));
      final cancelled = await action(c, created['id'], 'cancel');
      expect(cancelled!['status'], 'CANCELLED');
      expect(await action(c, created['id'], 'dispatch'), isNull);
      expect((await c.balance(sourceBasis))['on_hand_quantity'], 24);
      expect((await east.balance(destinationBasis))['on_hand_quantity'], 24);
    },
  );
  test(
    'dispatch and incremental arrivals account for transit, usable goods and discrepancies',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(
        r.backend,
        'mock-branch',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(c.dispose);
      final east = StockController(
        r.backend,
        'mock-branch-east',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(east.dispose);
      final created = await draft(c);
      final id = created['id'].toString(),
          line = created['lines'][0]['id'].toString();
      expect((await action(c, id, 'dispatch'))!['status'], 'IN_TRANSIT');
      await action(c, id, 'dispatch');
      expect((await c.balance(sourceBasis))['on_hand_quantity'], 19);
      expect((await c.balance(sourceBasis))['in_transit_quantity'], 5);
      expect(await action(c, id, 'cancel'), isNull);
      final first = await action(
        c,
        id,
        'receive',
        body: arrival(line, received: 2, damaged: 1),
      );
      expect(first!['status'], 'PARTIALLY_RECEIVED');
      expect(transferRemaining(jsonObject(first['lines'][0])), 2);
      expect((await c.balance(sourceBasis))['in_transit_quantity'], 2);
      expect((await east.balance(destinationBasis))['on_hand_quantity'], 26);
      final second = await action(
        c,
        id,
        'receive',
        body: arrival(line, received: 1, missing: 1),
      );
      expect(second!['status'], 'RECEIVED');
      expect(second['lines'][0]['received_quantity'], 3);
      expect(second['lines'][0]['damaged_quantity'], 1);
      expect(second['lines'][0]['missing_quantity'], 1);
      expect((await c.balance(sourceBasis))['in_transit_quantity'], 0);
      expect((await east.balance(destinationBasis))['on_hand_quantity'], 27);
      await c.load(selected: StockSection.movements);
      expect(
        c.rows.where((row) => row['movement_type'] == 'DAMAGE'),
        hasLength(2),
      );
      expect(
        await action(c, id, 'receive', body: arrival(line, received: 1)),
        isNull,
      );
    },
  );
  test(
    'dispatch validates current stock and invalid arrivals make no changes',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(
        r.backend,
        'mock-branch',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(c.dispose);
      final excessive = await draft(c, quantity: 25);
      expect(await action(c, excessive['id'], 'dispatch'), isNull);
      expect(c.errorCode, 'SHOP_INSUFFICIENT_STOCK');
      expect((await c.transfer(excessive['id']))['status'], 'DRAFT');
      expect((await c.balance(sourceBasis))['in_transit_quantity'], 0);
      final created = await draft(c);
      final id = created['id'].toString(),
          line = created['lines'][0]['id'].toString();
      await action(c, id, 'dispatch');
      expect(
        await action(c, id, 'receive', body: arrival(line, received: 6)),
        isNull,
      );
      expect(await action(c, id, 'receive', body: arrival(line)), isNull);
      expect(
        await action(c, id, 'receive', body: arrival(line, received: -1)),
        isNull,
      );
      expect((await c.balance(sourceBasis))['in_transit_quantity'], 5);
      expect((await c.transfer(id))['status'], 'IN_TRANSIT');
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/transfers',
          body: {...transferBody(), 'destination_branch_id': 'mock-branch'},
        ),
        isNull,
      );
      expect(
        await c.mutate(
          'POST',
          '${StockController.root}/transfers',
          body: {
            ...transferBody(),
            'lines': [...transferBody()['lines'], ...transferBody()['lines']],
          },
        ),
        isNull,
      );
    },
  );
  test(
    'arrival command replay returns its original result without receiving twice',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = StockController(
        r.backend,
        'mock-branch',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(c.dispose);
      final created = await draft(c);
      await action(c, created['id'], 'dispatch');
      final command = VendorCommand.create(),
          body = VendorPayload(arrival(created['lines'][0]['id'], received: 2));
      final api = r.backend.shopCatalogueApi;
      Future<JsonObject> send() async => (await api.request(
        'POST',
        '${StockController.root}/transfers/{transferId}/receive',
        pathParameters: {'transferId': created['id']},
        command: command,
        body: body,
      )).object;
      final first = await send(), second = await send();
      expect(first, second);
      expect(
        (await c.transfer(created['id']))['lines'][0]['received_quantity'],
        2,
      );
      expect((await c.balance(sourceBasis))['in_transit_quantity'], 3);
    },
  );
  test(
    'live transfer permission errors are preserved and retry keeps the exact arrival payload',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final probe = TransferProbe();
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://server.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: probe,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = StockController(
        app,
        'branch',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(c.dispose);
      probe.status = 403;
      await c.load(selected: StockSection.transfers);
      expect(c.rows, isEmpty);
      expect(c.errors[StockSection.transfers], isNotNull);
      probe.status = 503;
      final body = arrival('line', received: 2, damaged: 1);
      expect(await action(c, 'transfer', 'receive', body: body), isNull);
      expect(c.uncertainOutcome, true);
      probe.status = 200;
      expect(await action(c, 'transfer', 'receive', body: body), isNotNull);
      final writes = probe.calls
          .where((request) => request.method == 'POST')
          .toList();
      expect(
        writes[0].headers['Idempotency-Key'],
        writes[1].headers['Idempotency-Key'],
      );
      expect(jsonDecode(writes[1].body), body);
      expect(c.uncertainOutcome, false);
    },
  );
  testWidgets(
    'Mock transfer creation reviews real branches and opens the draft detail',
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
      await tester.ensureVisible(find.text('Transfers'));
      await tester.tap(find.text('Transfers'));
      await flush(tester);
      await tester.tap(find.text('New transfer'));
      await flush(tester);
      expect(find.byType(TransferEditor), findsOneWidget);
      await tester.tap(find.text('Review transfer'));
      await flush(tester);
      await tester.runAsync(() async {
        await tester.tap(find.text('Create draft transfer'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Transfer draft created'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await flush(tester);
      expect(find.byType(TransferScreen), findsOneWidget);
      expect(find.text('Dispatch transfer'), findsOneWidget);
      expect(find.text('Cancel transfer'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'arrival form rejects over-receipt and records only the current partial arrival',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      final c = StockController(
        r.backend,
        'mock-branch',
        preferences: VendorPreferences(inMemory: true),
      );
      addTearDown(c.dispose);
      await tester.runAsync(c.prepare);
      final created = (await tester.runAsync(() => draft(c)))!;
      await tester.runAsync(() => action(c, created['id'], 'dispatch'));
      final record = (await tester.runAsync(() => c.transfer(created['id'])))!;
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: TransferEditor(runtime: r, controller: c, record: record),
        ),
      );
      await flush(tester);
      final received = find.widgetWithText(
        TextFormField,
        'Received in usable condition',
      );
      await tester.enterText(received, '6');
      await tester.tap(find.text('Review arrival'));
      await flush(tester);
      expect(
        find.textContaining('no more than the 5 remaining'),
        findsOneWidget,
      );
      await tester.enterText(received, '2');
      await tester.tap(find.text('Review arrival'));
      await flush(tester);
      await tester.runAsync(() async {
        await tester.tap(find.text('Confirm arrival'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Arrival recorded'), findsOneWidget);
      final fresh = (await tester.runAsync(() => c.transfer(created['id'])))!;
      expect(fresh['status'], 'PARTIALLY_RECEIVED');
      expect(fresh['lines'][0]['received_quantity'], 2);
      expect(tester.takeException(), isNull);
    },
  );
}

class TransferProbe extends http.BaseClient {
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
                    'data': {'id': 'transfer', 'status': 'PARTIALLY_RECEIVED'},
                  }
                : {
                    'success': false,
                    'error': {
                      'code': status == 403
                          ? 'FORBIDDEN'
                          : 'SERVICE_UNAVAILABLE',
                      'message': 'Transfer unavailable',
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
