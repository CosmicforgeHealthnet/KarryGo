import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/delivery/state/fulfilment_workspace_controller.dart';
import 'package:vendor/features/delivery/state/rider_workspace_controller.dart';
import 'package:vendor/features/delivery/ui/fulfilment_workspace_screen.dart';
import 'finance_test.dart' show financeApp;
import 'rider_delivery_test.dart' show RiderTransport, toReceiver;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

void main() {
  test(
    'packing recovery creates one stable parcel and updates pickup readiness',
    () async {
      final transport = RiderTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      final c = FulfilmentWorkspaceController(
        runtime,
        'demo-fulfilment-group-1',
      );
      await c.load();
      final unit = c.units.single;
      final order = await c.order(unit['vendor_order_id'] as String);
      final line = order['lines'][0];
      final count = c.parcels.length;
      transport.lose = true;
      expect(
        await c.command(
          method: 'POST',
          path: '/api/v1/vendor/fulfilment/units/{unitId}/parcels',
          params: {'unitId': unit['fulfilment_unit_id'] as String},
          body: {
            'manifest': {
              'package_count': 1,
              'lines': [
                {'order_line_ref': line['id'], 'quantity': 1},
              ],
            },
            'temperature_control': 'HOT',
            'fragile': false,
          },
        ),
        false,
      );
      final key = transport.riderKeys.last;
      c.dispose();
      final restored = FulfilmentWorkspaceController(
        runtime,
        'demo-fulfilment-group-1',
      );
      addTearDown(restored.dispose);
      await restored.load();
      expect(restored.parcels.length, count + 1);
      expect(await restored.retry(), true);
      expect(transport.riderKeys.last, key);
      expect(restored.parcels.length, count + 1);
      expect(
        await restored.command(
          method: 'PUT',
          path: '/api/v1/vendor/fulfilment/units/{unitId}/readiness',
          params: {'unitId': unit['fulfilment_unit_id'] as String},
          version: unit['version'] as int,
          body: {'readiness': 'READY'},
        ),
        true,
      );
      expect(restored.units.single['readiness'], 'READY');
    },
  );
  test(
    'verified pickup seals the manifest and custody survives delivery and return request',
    () async {
      final app = await financeApp(RiderTransport());
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      final rider = RiderWorkspaceController(
        runtime,
        id: 'demo-vendor-mission-1',
      );
      addTearDown(rider.dispose);
      await rider.load();
      await toReceiver(rider);
      final parcel = FulfilmentWorkspaceController(
        runtime,
        'demo-fulfilment-group-1',
        parcelId: 'demo-delivery-parcel-1',
      );
      addTearDown(parcel.dispose);
      await parcel.load();
      expect(parcel.sealed, true);
      expect(parcel.custody.any((e) => e['event_type'] == 'HANDED_OVER'), true);
      expect(
        await parcel.command(
          method: 'PUT',
          path: '/api/v1/vendor/fulfilment/parcels/{parcelId}/manifest',
          params: {'parcelId': 'demo-delivery-parcel-1'},
          version: parcel.parcel!['version'] as int,
          body: parcel.parcel!['manifest'],
        ),
        false,
      );
      await parcel.load();
      expect(
        await parcel.command(
          method: 'POST',
          path: '/api/v1/vendor/fulfilment/parcels/{parcelId}/returns',
          params: {'parcelId': 'demo-delivery-parcel-1'},
          body: {'reason_code': 'Receiver requested a return'},
        ),
        true,
      );
      expect(parcel.parcel!['status'], 'RETURN_REQUIRED');
    },
  );
  testWidgets('packing and manifest screens fit a dark phone', (tester) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final app = (await tester.runAsync(() => financeApp(RiderTransport())))!;
    addTearDown(app.dispose);
    final runtime = SupportRuntime(app);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: FulfilmentWorkspaceScreen(
          runtime: runtime,
          groupId: 'demo-fulfilment-group-1',
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Pack a parcel'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.tap(find.text('Pack a parcel'));
    await flush(tester);
    expect(find.text('Packed contents'), findsOneWidget);
    expect(find.text('Packages inside this parcel'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
