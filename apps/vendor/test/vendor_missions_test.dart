import 'dashboard_test.dart' show flush;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/delivery/state/vendor_missions_controller.dart';
import 'package:vendor/features/delivery/ui/vendor_missions_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

void main() {
  test(
    'vendor delivery exposes staff updates and keeps secure handovers protected',
    () async {
      final app = await financeApp(FinanceTransport());
      addTearDown(app.dispose);
      final list = VendorMissionsController(app);
      addTearDown(list.dispose);
      await list.load();
      expect(list.missions.length, 4);
      final c = VendorMissionsController(
        app,
        id: list.missions.first['vendor_mission_id'] as String,
      );
      addTearDown(c.dispose);
      await c.load();
      expect(c.actions, ['READY_FOR_PICKUP', 'FAILED']);
      expect(await c.update('DELIVERED'), false);
      expect(await c.update('READY_FOR_PICKUP'), true);
      expect(c.record!['version'], 2);
      expect(c.actions, isNot(contains('CUSTODY_TRANSFERRED')));
      expect(await c.update('FAILED'), false);
      expect(
        await c.update('FAILED', reason: 'Delivery vehicle unavailable'),
        true,
      );
      expect(c.actions, isEmpty);
    },
  );
  test('vendor delivery stops stale staff updates until refresh', () async {
    final app = await financeApp(FinanceTransport());
    addTearDown(app.dispose);
    final a = VendorMissionsController(app, id: 'demo-vendor-mission-1'),
        b = VendorMissionsController(app, id: 'demo-vendor-mission-1');
    addTearDown(a.dispose);
    addTearDown(b.dispose);
    await a.load();
    await b.load();
    await a.update('READY_FOR_PICKUP');
    expect(await b.update('FAILED', reason: 'Late rider'), false);
    expect(b.stale, true);
    expect(b.actions, isEmpty);
    await b.load();
    expect(b.record!['status'], 'READY_FOR_PICKUP');
  });
  testWidgets(
    'vendor deliveries fit a dark phone and open a separate detail screen',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final app = (await tester.runAsync(
        () => financeApp(FinanceTransport()),
      ))!;
      addTearDown(app.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: VendorMissionsScreen(runtime: SupportRuntime(app)),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('View delivery').first);
      await flush(tester);
      expect(find.text('Delivery details'), findsOneWidget);
      expect(find.text('Delivery stops'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}
