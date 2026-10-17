import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/features/inventory/ui/branch_stock_screens.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

/// A tiny in-memory stand-in for the general inventory routes.
class FakeStockServer {
  final requests = <http.Request>[];
  bool offline = false, staleOnPost = false;
  int branchCount = 2;
  final stock = <String, Map<String, int>>{
    'b1': {'flour-1': 10, 'sugar-1': 4},
    'b2': {'flour-2': 0},
  };
  final names = {'flour-1': 'Flour', 'sugar-1': 'Sugar', 'flour-2': 'Flour'};
  final counted = <String, int>{};
  Map<String, dynamic>? transfer;

  http.Response ok(Object data, [int status = 200]) => http.Response(
    jsonEncode({'success': true, 'data': data}),
    status,
    headers: {'content-type': 'application/json'},
  );
  http.Response fail(int status, String code) => http.Response(
    jsonEncode({
      'success': false,
      'error': {'code': code, 'message': 'internal detail'},
    }),
    status,
    headers: {'content-type': 'application/json'},
  );

  Future<http.Response> handle(http.Request r) async {
    requests.add(r);
    if (offline) throw http.ClientException('offline');
    final path = r.url.path;
    final body = r.body.isEmpty ? <String, dynamic>{} : jsonDecode(r.body);
    if (path.endsWith('/branches') && path.contains('/businesses/')) {
      return ok({
        'items': [
          {'id': 'b1', 'display_name': 'Ikeja Kitchen', 'status': 'ACTIVE'},
          if (branchCount > 1)
            {'id': 'b2', 'display_name': 'Lekki Kitchen', 'status': 'ACTIVE'},
        ],
        'pagination': {'has_more': false},
      });
    }
    if (path.endsWith('/vendor/ingredients')) {
      return ok([
        {
          'name': 'Flour',
          'default_unit_code': 'KG',
          'inventory_control_mode': 'INVENTORY_CONTROLLED',
          'status': 'ACTIVE',
        },
      ]);
    }
    if (path.endsWith('/vendor/packaging-materials')) return ok([]);
    final balances = RegExp(
      r'/inventory/branches/([^/]+)/balances$',
    ).firstMatch(path);
    if (balances != null) {
      final b = balances[1]!;
      return ok({
        'items': [
          for (final e in stock[b]!.entries)
            {
              'branch_id': b,
              'inventory_item_id': e.key,
              'name': names[e.key],
              'on_hand': e.value,
            },
        ],
      });
    }
    if (path.endsWith('/inventory/stocktakes')) {
      return ok({
        'id': 'st1',
        'branch_id': body['branch_id'],
        'status': 'COUNTING',
        'lines': [
          for (final e in stock[body['branch_id']]!.entries)
            {
              'id': 'l-${e.key}',
              'inventory_item_id': e.key,
              'expected_quantity': e.value,
            },
        ],
      }, 201);
    }
    final count = RegExp(
      r'/stocktakes/st1/items/([^/]+)/count$',
    ).firstMatch(path);
    if (count != null) {
      counted[count[1]!] = body['counted_quantity'];
      return ok({'counted': true});
    }
    if (path.endsWith('/stocktakes/st1/post')) {
      if (staleOnPost) return fail(409, 'conflict');
      return ok({
        'id': 'st1',
        'status': 'POSTED',
        'lines': [
          for (final e in stock['b1']!.entries)
            {
              'inventory_item_id': e.key,
              'expected_quantity': e.value,
              'counted_quantity': counted[e.key],
              'variance_quantity': counted[e.key]! - e.value,
            },
        ],
      });
    }
    if (path.endsWith('/stocktakes/st1/cancel')) {
      return ok({'id': 'st1', 'status': 'CANCELLED', 'lines': []});
    }
    if (path.endsWith('/inventory/transfers')) {
      transfer = {
        'id': 't1',
        'status': 'DRAFT',
        'source_branch_id': body['source_branch_id'],
        'destination_branch_id': body['destination_branch_id'],
        'lines': [
          for (final l in body['lines'])
            {
              'id': 'tl-${l['source_inventory_item_id']}',
              'source_inventory_item_id': l['source_inventory_item_id'],
              'destination_inventory_item_id':
                  l['destination_inventory_item_id'],
              'sent_quantity': l['quantity'],
              'received_quantity': 0,
              'damaged_quantity': 0,
              'missing_quantity': 0,
            },
        ],
      };
      return ok(transfer!, 201);
    }
    if (path.endsWith('/transfers/t1/dispatch')) {
      transfer!['status'] = 'IN_TRANSIT';
      return ok(transfer!);
    }
    if (path.endsWith('/transfers/t1/receive')) {
      for (final l in body['lines']) {
        final line = (transfer!['lines'] as List).firstWhere(
          (x) => x['id'] == l['line_id'],
        );
        line['received_quantity'] = l['received_quantity'];
        line['missing_quantity'] = l['missing_quantity'];
      }
      transfer!['status'] = 'RECEIVED';
      return ok(transfer!);
    }
    return fail(404, 'not_found');
  }

  Iterable<http.Request> calls(String method, String suffix) =>
      requests.where((r) => r.method == method && r.url.path.endsWith(suffix));
}

Future<(VendorApp, FakeStockServer)> stockApp() async {
  final r = await dashboardRuntime();
  addTearDown(r.dispose);
  final server = FakeStockServer();
  final app = VendorApp(
    config: VendorAppConfig(origin: 'https://server.invalid'),
    sessions: MemoryVendorSessionStore(),
    httpClient: MockClient(server.handle),
  );
  addTearDown(app.dispose);
  await app.client.acceptSession(r.backend.auth.session!);
  return (app, server);
}

Future<void> pumpScreen(WidgetTester tester, Widget screen) async {
  tester.view.physicalSize = const Size(900, 5000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(MaterialApp(theme: vendorTheme(), home: screen));
  await flush(tester);
}

Future<void> choose(WidgetTester tester, String label, String option) async {
  await tester.tap(find.widgetWithText(DropdownButtonFormField<String>, label));
  await tester.pumpAndSettle();
  await tester.tap(find.text(option).last);
  await flush(tester);
}

Future<void> confirm(WidgetTester tester, String title) async {
  await tester.tap(find.text(title).last);
  await flush(tester);
}

void main() {
  testWidgets('stock count shows differences by name and posts counts', (
    tester,
  ) async {
    final (app, server) = (await tester.runAsync(stockApp))!;
    await pumpScreen(tester, BranchStocktakeScreen(app: app));
    await choose(tester, 'Branch', 'Ikeja Kitchen');
    expect(find.text('Flour'), findsOneWidget);
    expect(find.text('10 kg in stock'), findsOneWidget);
    expect(find.textContaining('flour-1'), findsNothing);
    await tester.tap(find.text('Start counting'));
    await flush(tester);
    expect(
      server.calls('POST', '/inventory/stocktakes').single.body,
      contains('FULL_STOCKTAKE'),
    );
    await tester.enterText(find.byKey(const ValueKey('count-flour-1')), '8');
    await tester.enterText(find.byKey(const ValueKey('count-sugar-1')), '4');
    await tester.pump();
    expect(find.text('Difference: -2'), findsOneWidget);
    expect(find.text('Recorded in stock: 10 kg'), findsOneWidget);
    expect(find.text('Recorded in stock: 4'), findsOneWidget);
    expect(find.text('Matches stock'), findsOneWidget);
    await tester.ensureVisible(find.text('Save count'));
    await tester.tap(find.text('Save count'));
    await flush(tester);
    await confirm(tester, 'Update stock to your count?');
    expect(server.counted, {'flour-1': 8, 'sugar-1': 4});
    expect(jsonDecode(server.calls('PUT', '/count').first.body), {
      'counted_quantity': 8,
    });
    expect(
      server.calls('PUT', '/count').first.headers['Idempotency-Key'] ??
          server.calls('PUT', '/count').first.headers['idempotency-key'],
      isNotNull,
    );
    expect(find.text('Stock count saved'), findsOneWidget);
    expect(find.textContaining('Flour: -2'), findsOneWidget);
  });

  testWidgets('stale stock count asks for a recount in plain English', (
    tester,
  ) async {
    final (app, server) = (await tester.runAsync(stockApp))!;
    server.staleOnPost = true;
    await pumpScreen(tester, BranchStocktakeScreen(app: app));
    await choose(tester, 'Branch', 'Ikeja Kitchen');
    await tester.tap(find.text('Start counting'));
    await flush(tester);
    await tester.enterText(find.byKey(const ValueKey('count-flour-1')), '8');
    await tester.enterText(find.byKey(const ValueKey('count-sugar-1')), '4');
    await tester.pump();
    await tester.ensureVisible(find.text('Save count'));
    await tester.tap(find.text('Save count'));
    await flush(tester);
    await confirm(tester, 'Update stock to your count?');
    expect(find.textContaining('Start a new count.'), findsOneWidget);
    expect(find.text('Start a new count'), findsOneWidget);
    expect(find.textContaining('internal detail'), findsNothing);
  });

  testWidgets('offline stock count shows a network message', (tester) async {
    final (app, server) = (await tester.runAsync(stockApp))!;
    server.offline = true;
    await pumpScreen(tester, BranchStocktakeScreen(app: app));
    expect(find.textContaining('internet connection'), findsOneWidget);
  });

  testWidgets('one branch cannot move stock and says why', (tester) async {
    final (app, server) = (await tester.runAsync(stockApp))!;
    server.branchCount = 1;
    await pumpScreen(tester, BranchTransferScreen(app: app));
    expect(find.text('You need two branches'), findsOneWidget);
  });

  testWidgets('transfer sends matched items by name and receives them', (
    tester,
  ) async {
    final (app, server) = (await tester.runAsync(stockApp))!;
    await pumpScreen(tester, BranchTransferScreen(app: app));
    await choose(tester, 'From branch', 'Ikeja Kitchen');
    await choose(tester, 'To branch', 'Lekki Kitchen');
    await choose(tester, 'Item', 'Flour (10 kg in stock)');
    await tester.enterText(find.byKey(const ValueKey('quantity-0')), '12');
    await tester.pump();
    expect(
      find.textContaining('Only 10 kg of Flour is in stock'),
      findsOneWidget,
    );
    await tester.enterText(find.byKey(const ValueKey('quantity-0')), '6');
    await tester.pump();
    await tester.ensureVisible(find.text('Send stock'));
    await tester.tap(find.text('Send stock'));
    await flush(tester);
    await confirm(tester, 'Send stock to Lekki Kitchen?');
    final created = jsonDecode(
      server.calls('POST', '/inventory/transfers').single.body,
    );
    expect(created['source_branch_id'], 'b1');
    expect(created['destination_branch_id'], 'b2');
    expect(created['lines'], [
      {
        'source_inventory_item_id': 'flour-1',
        'destination_inventory_item_id': 'flour-2',
        'quantity': 6,
      },
    ]);
    expect(server.calls('POST', '/transfers/t1/dispatch'), hasLength(1));
    expect(find.text('Stock is on its way'), findsOneWidget);
    await tester.enterText(
      find.byKey(const ValueKey('received-tl-flour-1')),
      '5',
    );
    await tester.ensureVisible(find.text('Confirm received'));
    await tester.tap(find.text('Confirm received'));
    await flush(tester);
    final receipt = jsonDecode(
      server.calls('POST', '/transfers/t1/receive').single.body,
    );
    expect(receipt['source_ref'], isNotEmpty);
    expect(receipt['lines'], [
      {
        'line_id': 'tl-flour-1',
        'received_quantity': 5,
        'damaged_quantity': 0,
        'missing_quantity': 1,
      },
    ]);
    expect(find.text('Stock received'), findsOneWidget);
    expect(find.textContaining('1 missing'), findsOneWidget);
  });

  testWidgets('item missing at destination blocks the move', (tester) async {
    final (app, server) = (await tester.runAsync(stockApp))!;
    await pumpScreen(tester, BranchTransferScreen(app: app));
    await choose(tester, 'From branch', 'Ikeja Kitchen');
    await choose(tester, 'To branch', 'Lekki Kitchen');
    await choose(tester, 'Item', 'Sugar (4 in stock)');
    await tester.enterText(find.byKey(const ValueKey('quantity-0')), '2');
    await tester.pump();
    expect(
      find.textContaining('Sugar isn’t set up at Lekki Kitchen'),
      findsOneWidget,
    );
    expect(server.calls('POST', '/inventory/transfers'), isEmpty);
  });
}
