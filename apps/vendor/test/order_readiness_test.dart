import 'dashboard_test.dart' show flush;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/readiness/state/order_readiness_controller.dart';
import 'package:vendor/features/readiness/ui/order_readiness_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

void main() {
  test(
    'readiness starts from actual branch capabilities and uses its own version',
    () async {
      final app = await financeApp(FinanceTransport());
      addTearDown(app.dispose);
      final order = (await app.ordersApi.getCommerceOrdersVendorOrderId(
        vendorOrderId: 'demo-order-1048',
      )).object;
      final c = OrderReadinessController(app, order);
      addTearDown(c.dispose);
      await c.load();
      expect(c.missing, true);
      expect(c.types, contains('RESTAURANT'));
      expect(await c.save('start', type: 'invented'), false);
      expect(await c.save('start', type: 'RESTAURANT'), true);
      expect(c.record!['state'], 'PREPARING');
      expect(c.record!['readiness_version'], 3);
      expect(c.history.map((h) => h['to_state']), ['PREPARING', 'ACCEPTED']);
      expect(await c.save('ready'), true);
      expect(c.record!['state'], 'READY');
      expect(c.actions, isNot(contains('estimate')));
      expect(
        (await app.ordersApi.getCommerceOrdersVendorOrderId(
          vendorOrderId: 'demo-order-1048',
        )).object['status'],
        order['status'],
      );
    },
  );
  test(
    'readiness refuses stale writes and supports an audited ready correction',
    () async {
      final app = await financeApp(FinanceTransport());
      addTearDown(app.dispose);
      final order = (await app.ordersApi.getCommerceOrdersVendorOrderId(
        vendorOrderId: 'demo-order-1048',
      )).object;
      final a = OrderReadinessController(app, order),
          b = OrderReadinessController(app, order);
      addTearDown(a.dispose);
      addTearDown(b.dispose);
      await a.load();
      await a.save('start', type: 'RESTAURANT');
      await b.load();
      await a.save('ready');
      final first = a.record!['first_actual_ready_at'];
      expect(
        await b.save('delay', reason: 'Kitchen equipment unavailable'),
        false,
      );
      expect(b.stale, true);
      expect(b.actions, isEmpty);
      await b.load();
      expect(await b.save('delay'), false);
      expect(await b.save('delay', reason: 'Missing item discovered'), true);
      expect(b.record!['state'], 'DELAYED');
      expect(b.record!['first_actual_ready_at'], first);
      expect(await b.save('estimate', substate: 'WEIGHING'), false);
      expect(await b.save('estimate', substate: 'ASSEMBLING'), true);
    },
  );
  testWidgets(
    'preparation workspace fits a dark phone and opens a focused action sheet',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final app = (await tester.runAsync(
        () => financeApp(FinanceTransport()),
      ))!;
      addTearDown(app.dispose);
      final order = (await tester.runAsync(
        () => app.ordersApi.getCommerceOrdersVendorOrderId(
          vendorOrderId: 'demo-order-1048',
        ),
      ))!.object;
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: OrderReadinessScreen(
            runtime: SupportRuntime(app),
            order: order,
          ),
        ),
      );
      await flush(tester);
      expect(find.text('Not started'), findsOneWidget);
      await tester.tap(find.text('Start preparation'));
      await flush(tester);
      expect(find.text('Business type'), findsOneWidget);
      expect(find.text('Review update'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}
