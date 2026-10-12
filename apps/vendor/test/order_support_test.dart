import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/app/vendor_runtime.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/support/state/order_support_controller.dart';
import 'package:vendor/features/support/ui/order_support_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'dashboard_test.dart' show dashboardRuntime, flush;

class SupportTransport extends FinanceTransport {
  bool lose = false, hideReports = false, denyRead = false, denyWrite = false;
  int supportPosts = 0;
  final supportKeys = <String?>[];
  final supportBodies = <JsonObject>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (!request.url.path.endsWith('/support-cases')) {
      return super.send(request);
    }
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    if (request.method == 'POST') {
      supportPosts++;
      supportKeys.add(request.headers['Idempotency-Key']);
      supportBodies.add(jsonObject(jsonDecode(utf8.decode(bytes))));
    }
    if (request.method == 'GET' && (hideReports || denyRead) ||
        request.method == 'POST' && denyWrite) {
      final denied = denyRead || denyWrite;
      return http.StreamedResponse(
        Stream.value(
          utf8.encode(
            jsonEncode(
              denied
                  ? {
                      'success': false,
                      'error': {
                        'code': 'FORBIDDEN',
                        'message': 'Support access required.',
                      },
                    }
                  : {
                      'success': true,
                      'data': {'cases': []},
                    },
            ),
          ),
        ),
        denied ? 403 : 200,
        headers: {'content-type': 'application/json'},
      );
    }
    final response = await super.send(copy);
    if (request.method == 'POST' && lose) {
      lose = false;
      throw http.ClientException('Lost accepted report response');
    }
    return response;
  }
}

class SupportRuntime extends VendorRuntime {
  SupportRuntime(this.value, {VendorPreferences? store})
    : super(preferences: store ?? VendorPreferences(inMemory: true)) {
    mode = VendorDataMode.live;
  }
  final VendorApp value;
  @override
  VendorApp get backend => value;
}

class FailingSupportStorage extends VendorPreferences {
  FailingSupportStorage() : super(inMemory: true);
  @override
  Future<void> write(String key, String value) async {
    throw StateError('Storage unavailable');
  }
}

const report = {
  'category': 'MISSING_ITEM',
  'description': 'An item was missing from the package.',
  'line_refs': ['demo-line-0'],
};

void main() {
  test(
    'support transport uses string category, actual item refs and required retry key',
    () async {
      final transport = SupportTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      addTearDown(runtime.dispose);
      final c = OrderSupportController(runtime, 'demo-order-1048');
      addTearDown(c.dispose);
      await c.initialize();
      expect(c.categories.length, 11);
      final result = await c.create({
        ...report,
        'description': 'Verification 123456 should not be retained.',
      });
      expect(result?['status'], 'open');
      expect(transport.supportKeys.single, isNotEmpty);
      expect(transport.supportBodies.single['category'], 'MISSING_ITEM');
      expect(
        transport.supportBodies.single['description'],
        contains('[REDACTED]'),
      );
      expect(transport.supportBodies.single.keys.toSet(), {
        'category',
        'description',
        'line_refs',
      });
      expect(c.pending, isNull);
    },
  );
  test(
    'an uncertain report restores and retries the original key and body',
    () async {
      final transport = SupportTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      addTearDown(runtime.dispose);
      final first = OrderSupportController(runtime, 'demo-order-1048');
      await first.initialize();
      transport.lose = true;
      expect(await first.create(report), isNull);
      expect(first.pending, isNotNull);
      expect(first.canCreate, isFalse);
      first.dispose();
      transport.hideReports = true;
      final restored = OrderSupportController(runtime, 'demo-order-1048');
      addTearDown(restored.dispose);
      await restored.initialize();
      expect(restored.pending, isNotNull);
      expect(
        await restored.create({...report, 'category': 'QUALITY_ISSUE'}),
        isNotNull,
      );
      expect(transport.supportPosts, 2);
      expect(transport.supportKeys[0], transport.supportKeys[1]);
      expect(transport.supportBodies[0], transport.supportBodies[1]);
      transport.hideReports = false;
      await restored.load();
      expect(restored.cases.length, 1);
    },
  );
  test('refresh reconciles an accepted report without another write', () async {
    final transport = SupportTransport();
    final app = await financeApp(transport);
    addTearDown(app.dispose);
    final runtime = SupportRuntime(app);
    addTearDown(runtime.dispose);
    final c = OrderSupportController(runtime, 'demo-order-1048');
    addTearDown(c.dispose);
    await c.initialize();
    transport.lose = true;
    await c.create(report);
    await c.load();
    expect(c.pending, isNull);
    expect(c.cases.length, 1);
    expect(transport.supportPosts, 1);
    expect(c.canCreate, isTrue);
  });
  test('failed recovery storage prevents sending a new report', () async {
    final transport = SupportTransport();
    final app = await financeApp(transport);
    addTearDown(app.dispose);
    final runtime = SupportRuntime(app, store: FailingSupportStorage());
    addTearDown(runtime.dispose);
    final c = OrderSupportController(runtime, 'demo-order-1048');
    addTearDown(c.dispose);
    await c.initialize();
    expect(await c.create(report), isNull);
    expect(transport.supportPosts, 0);
    expect(c.commandError, contains('Nothing was submitted'));
  });
  test('support read and write denials remain independent', () async {
    final transport = SupportTransport()..denyRead = true;
    final app = await financeApp(transport);
    addTearDown(app.dispose);
    final runtime = SupportRuntime(app);
    addTearDown(runtime.dispose);
    final c = OrderSupportController(runtime, 'demo-order-1048');
    addTearDown(c.dispose);
    await c.initialize();
    expect(c.readDenied, isTrue);
    expect(c.canCreate, isTrue);
    transport.denyRead = false;
    transport.denyWrite = true;
    expect(await c.create(report), isNull);
    expect(c.writeDenied, isTrue);
    expect(c.canCreate, isFalse);
  });
  testWidgets(
    'mock issue review, submit, acknowledgement and case details fit a dark phone',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final runtime = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(runtime.dispose);
      final order = (await tester.runAsync(
        () => runtime.backend.ordersApi.getCommerceOrdersVendorOrderId(
          vendorOrderId: 'demo-order-1048',
        ),
      ))!;
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: OrderSupportScreen(runtime: runtime, order: order.object),
        ),
      );
      await flush(tester);
      expect(find.text('No issues reported'), findsOneWidget);
      await tester.tap(find.text('Report an issue'));
      await flush(tester);
      expect(find.text('Quality Issue'), findsOneWidget);
      expect(
        tester
            .widget<TextFormField>(find.byType(TextFormField))
            .controller!
            .text,
        isNotEmpty,
      );
      await tester.tap(find.byType(CheckboxListTile));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Review report'));
      await tester.pumpAndSettle();
      expect(find.text('Review your report'), findsOneWidget);
      await tester.tap(find.text('Submit report'));
      await flush(tester);
      expect(find.text('Issue reported'), findsOneWidget);
      await tester.tap(find.text('View reports'));
      await flush(tester);
      expect(find.text('Report an issue'), findsOneWidget);
      await tester.tap(find.text('Quality Issue'));
      await tester.pumpAndSettle();
      expect(find.text('Case reference'), findsOneWidget);
      expect(find.text('Open'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}
