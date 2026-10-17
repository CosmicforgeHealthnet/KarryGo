import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/orders/state/order_actions.dart';
import 'package:vendor/features/orders/state/order_detail_controller.dart';
import 'package:vendor/features/orders/ui/order_detail_screen.dart';
import 'package:vendor/features/orders/ui/order_form_sheet.dart';
import 'package:vendor/features/orders/ui/order_feedback_dialog.dart';
import 'package:vendor/features/orders/ui/order_substitution_screen.dart';
import 'package:vendor/app/vendor_app.dart';
import 'support/mock/mock_vendor_client.dart';
import 'support/mock/mock_onboarding_store.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/features/handover/models/handover_models.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  test(
    'food transitions persist and platform delivery stays with its executor',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = OrderDetailController(r.backend, 'demo-order-1048');
      addTearDown(c.dispose);
      await c.load();
      expect(await c.submit('accept', {}, version: 1), isTrue);
      expect(c.order!['status'], 'ACCEPTED');
      expect(await c.submit('preparing', {}, version: 2), isTrue);
      expect(await c.submit('ready', {}, version: 3), isTrue);
      expect(c.order!['status'], 'READY');
      expect(
        availableOrderActions(c.order!).any((a) => a.executorOnly),
        isFalse,
      );
      expect(await c.submit('pickup', {}, version: 4), isFalse);
      expect(c.stale, isTrue);
      await c.load();
      expect(c.order!['status'], 'READY');
      expect(c.order!['version'], 4);
    },
  );
  test(
    'stale order cannot overwrite a change and recovers with a fresh read',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final a = OrderDetailController(r.backend, 'demo-order-1048');
      final b = OrderDetailController(r.backend, 'demo-order-1048');
      addTearDown(a.dispose);
      addTearDown(b.dispose);
      await a.load();
      await b.load();
      await a.submit('accept', {}, version: 1);
      expect(
        await b.submit('reject', {'reason_code': 'Out of stock'}, version: 1),
        isFalse,
      );
      expect(b.stale, isTrue);
      await b.load();
      expect(b.order!['status'], 'ACCEPTED');
      expect(b.order!['version'], 2);
    },
  );
  test(
    'shop availability and quality gate packing before own delivery completes',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = OrderDetailController(r.backend, 'demo-order-1045');
      addTearDown(c.dispose);
      await c.load();
      expect(c.picking, hasLength(1));
      expect(await c.submit('pack', {}, version: 1), isFalse);
      await c.load();
      final row = c.picking.single;
      await c.line(row['OrderLineID'].toString(), 'availability', {
        'picked_quantity': 2,
        'unavailable_quantity': 0,
        'outcome': 'FULLY_AVAILABLE',
      }, row['Version'] as int);
      expect(c.picking.single['State'], 'COMPLETE');
      await c.line(row['OrderLineID'].toString(), 'quality', {
        'result': 'NOT_AVAILABLE',
      }, c.picking.single['Version'] as int);
      expect(
        await c.submit('pack', {}, version: c.order!['version'] as int),
        isFalse,
      );
      await c.load();

      await c.line(row['OrderLineID'].toString(), 'quality', {
        'result': 'MET',
      }, c.picking.single['Version'] as int);
      for (final action in [
        'pack',
        'ready-for-pickup',
        'pickup',
        'delivering',
        'delivered',
        'complete',
      ]) {
        expect(
          await c.submit(action, {}, version: c.order!['version'] as int),
          isTrue,
          reason: action,
        );
      }
      expect(c.order!['status'], 'COMPLETED');
      expect(availableOrderActions(c.order!), isEmpty);
    },
  );
  test(
    'identical retry keys replay instead of applying a transition twice',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.ordersApi;
      final command = VendorCommand.create(version: 1);
      Future<void> accept() async {
        await api.request(
          'POST',
          '/api/v1/vendor/commerce/orders/{vendorOrderId}/accept',
          pathParameters: {'vendorOrderId': 'demo-order-1048'},
          body: VendorPayload({}),
          command: command,
        );
      }

      await accept();
      await accept();
      final response = await api.getCommerceOrdersVendorOrderId(
        vendorOrderId: 'demo-order-1048',
      );
      expect(response.object['status'], 'ACCEPTED');
      expect(response.object['version'], 2);
    },
  );
  test(
    'a committed write stays saved when its canonical refresh fails',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final transport = OrderProbe()..failReadAfterWrite = true;
      addTearDown(transport.close);
      final app = VendorApp(
        config: VendorAppConfig(origin: 'http://localhost'),
        sessions: MemoryVendorSessionStore(),
        httpClient: transport,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.auth.session!);
      final c = OrderDetailController(app, 'demo-order-1048');
      addTearDown(c.dispose);
      await c.load();
      expect(await c.submit('accept', {}, version: 1), isTrue);
      expect(c.changed, isTrue);
      expect(c.stale, isTrue);
      expect(c.feedback, contains('Saved. Refresh'));
      expect(
        c.can('/api/v1/vendor/commerce/orders/{vendorOrderId}/accept'),
        isFalse,
      );
      await c.load();
      expect(c.order!['status'], 'ACCEPTED');
      expect(transport.writes, 1);
    },
  );
  test(
    'a denied mutation disables that action without fabricating role privileges',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final transport = OrderProbe()..denyWrites = true;
      addTearDown(transport.close);
      final app = VendorApp(
        config: VendorAppConfig(origin: 'http://localhost'),
        sessions: MemoryVendorSessionStore(),
        httpClient: transport,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.auth.session!);
      final c = OrderDetailController(app, 'demo-order-1048');
      addTearDown(c.dispose);
      await c.load();
      expect(await c.submit('accept', {}, version: 1), isFalse);
      expect(c.restricted, isTrue);
      expect(c.order!['status'], 'SENT_TO_VENDOR');
      expect(
        c.can('/api/v1/vendor/commerce/orders/{vendorOrderId}/accept'),
        isFalse,
      );
      expect(await c.submit('accept', {}, version: 1), isFalse);
      expect(transport.writes, 1);
    },
  );
  test(
    'pickup credentials update the handover observation, not platform order delivery',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.handoverApi;
      final observation = await api
          .getCommerceOrdersVendorOrderIdMissionsMissionIdHandoverStage(
            vendorOrderId: 'demo-order-1046',
            missionId: 'demo-mission-1046',
            stage: 'PICKUP',
          );
      expect(observation.object['status'], 'ACTIVE');
      final result = await api
          .postCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirm(
            vendorOrderId: 'demo-order-1046',
            missionId: 'demo-mission-1046',
            command: VendorCommand.create(),
            body:
                PostCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirmRequest.fromJson(
                  {
                    'handover_ref': observation.object['handover_ref'],
                    'method': 'OTP',
                    'credential': '654321',
                  },
                ),
          );
      expect(result.object['status'], 'CONSUMED');
      expect(result.object.containsKey('credential'), isFalse);
      final order = await r.backend.ordersApi.getCommerceOrdersVendorOrderId(
        vendorOrderId: 'demo-order-1046',
      );
      expect(order.object['status'], 'READY');
    },
  );
  test('replacement suggestions remain pending customer approval', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = OrderDetailController(r.backend, 'demo-order-1045');
    addTearDown(c.dispose);
    await c.load();
    expect(
      await c.line(
        c.picking.single['OrderLineID'].toString(),
        'substitutions',
        {
          'proposed_catalogue_ref': 'demo-listing-mock-branch-east',
          'reason': 'Original item unavailable',
        },
        c.order!['version'] as int,
      ),
      isTrue,
    );
    expect(c.order!['status'], 'AWAITING_CUSTOMER');
    expect(c.order!['status'], isNot('PACKING'));
  });
  testWidgets('order confirmation advances the same screen on a phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    var changed = false;
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: Scaffold(
          body: OrderDetailScreen(
            runtime: r,
            id: 'demo-order-1048',
            onChanged: () => changed = true,
          ),
        ),
      ),
    );
    await flush(tester);
    await tester.tap(find.text('Accept order'));
    await tester.pumpAndSettle();
    expect(find.byType(OrderFormSheet), findsOneWidget);
    await tester.runAsync(() async {
      await tester.tap(find.widgetWithText(FilledButton, 'Accept order').last);
      await Future<void>.delayed(const Duration(milliseconds: 60));
    });
    await flush(tester);
    expect(find.byType(OrderFeedbackDialog), findsOneWidget);
    expect(find.text('Order accepted'), findsOneWidget);
    await tester.tap(find.text('Continue'));
    await tester.pumpAndSettle();
    expect(find.text('Cancel order'), findsOneWidget);
    expect(find.text('More actions'), findsNothing);
    expect(find.text('Accepted'), findsOneWidget);
    expect(find.text('Start preparing'), findsOneWidget);
    expect(changed, isTrue);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets(
    'replacement browsing opens a screen and returns to the same order',
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
          home: OrderDetailScreen(
            runtime: r,
            id: 'demo-order-1045',
            onChanged: () {},
          ),
        ),
      );
      await flush(tester);
      await tester.scrollUntilVisible(
        find.text('Suggest replacement'),
        220,
        scrollable: find
            .descendant(
              of: find.byType(OrderDetailScreen),
              matching: find.byType(Scrollable),
            )
            .first,
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('Suggest replacement'));
      await flush(tester);
      expect(find.byType(OrderSubstitutionScreen), findsOneWidget);
      expect(find.byType(BottomSheet), findsNothing);
      await tester.tap(find.text('Standard pack'));
      await tester.pumpAndSettle();
      expect(find.byType(OrderFormSheet), findsOneWidget);
      await tester.enterText(
        find.byType(TextFormField),
        'Original item unavailable',
      );
      await tester.pumpAndSettle();
      await tester.runAsync(() async {
        await tester.tap(find.widgetWithText(FilledButton, 'Send suggestion'));
        await Future<void>.delayed(const Duration(milliseconds: 80));
      });
      await flush(tester);
      expect(find.text('Replacement suggested'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await tester.pumpAndSettle();
      expect(find.byType(OrderSubstitutionScreen), findsNothing);
      await tester.scrollUntilVisible(
        find.text('Awaiting Customer'),
        -240,
        scrollable: find
            .descendant(
              of: find.byType(OrderDetailScreen),
              matching: find.byType(Scrollable),
            )
            .first,
      );
      await tester.pumpAndSettle();
      expect(find.text('Awaiting Customer'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets('order screen and feedback fit large text on a phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(context).copyWith(
            textScaler: const TextScaler.linear(1.6),
            disableAnimations: true,
          ),
          child: child!,
        ),
        home: OrderDetailScreen(
          runtime: r,
          id: 'demo-order-1048',
          onChanged: () {},
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Cancel order'), findsOneWidget);
    expect(tester.takeException(), isNull);
    final context = tester.element(find.text('Order details'));
    showOrderFeedback(
      context,
      title: 'Refresh before continuing',
      message: 'This order changed. Refresh it and try again.',
      warning: true,
      action: 'Refresh order',
    );
    await tester.pumpAndSettle();
    expect(find.text('Refresh before continuing'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.tap(find.widgetWithText(FilledButton, 'Refresh order'));
    await tester.pumpAndSettle();
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets(
    'destructive action requires a reason and never sends an empty form',
    (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: OrderFormSheet(
              title: 'Reject order',
              message: 'This ends this order.',
              fields: const [
                OrderInput('reason_code', 'Reason', required: true),
              ],
              destructive: true,
            ),
          ),
        ),
      );
      final button = tester.widget<FilledButton>(
        find.widgetWithText(FilledButton, 'Reject order'),
      );
      expect(button.onPressed, isNull);
      await tester.enterText(find.byType(TextFormField), 'Item unavailable');
      await tester.pump();
      expect(
        tester
            .widget<FilledButton>(
              find.widgetWithText(FilledButton, 'Reject order'),
            )
            .onPressed,
        isNotNull,
      );
      expect(find.byType(OrderFormSheet), findsOneWidget);
    },
  );
}

class OrderProbe extends http.BaseClient {
  final inner = MockVendorClient(
    onboardingScenario: MockOnboardingScenario.activated,
    latency: Duration.zero,
  );
  bool failReadAfterWrite = false, denyWrites = false, _failRead = false;
  int writes = 0;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (_failRead && request.method == 'GET') {
      _failRead = false;
      throw http.ClientException('Read interrupted');
    }
    if (request.method == 'POST' &&
        request.url.path.contains('/commerce/orders/')) {
      writes++;
      if (denyWrites) {
        return http.StreamedResponse(
          Stream.value(
            utf8.encode(
              jsonEncode({
                'success': false,
                'error': {
                  'code': 'FORBIDDEN',
                  'message': 'You do not have access to manage orders.',
                },
              }),
            ),
          ),
          403,
          headers: {'content-type': 'application/json'},
        );
      }
      final response = await inner.send(request);
      if (failReadAfterWrite) _failRead = true;
      return response;
    }
    return inner.send(request);
  }

  @override
  void close() {
    inner.close();
    super.close();
  }
}
