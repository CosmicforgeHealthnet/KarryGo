import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/delivery/state/delivery_policy_controller.dart';
import 'package:vendor/features/delivery/state/hybrid_fallback_controller.dart';
import 'package:vendor/features/delivery/state/rider_workspace_controller.dart';
import 'package:vendor/features/delivery/ui/delivery_policy_screen.dart';
import 'package:vendor/features/delivery/ui/hybrid_fallback_screen.dart';
import 'finance_test.dart' show financeApp;
import 'rider_delivery_test.dart' show RiderTransport;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

void main() {
  test(
    'delivery policy preserves advanced server settings and rejects stale replacements',
    () async {
      final app = await financeApp(RiderTransport());
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      final branch = {'id': 'mock-branch', 'name': 'Main branch'};
      final a = DeliveryPolicyController(runtime, branch),
          b = DeliveryPolicyController(runtime, branch);
      addTearDown(a.dispose);
      addTearDown(b.dispose);
      await a.load();
      await b.load();
      expect(
        await a.save({
          'base_fee_minor': 75000,
          'reason': 'Updated delivery charge',
        }),
        true,
      );
      expect(a.record!['supported_vehicle_classes'], ['MOTORCYCLE']);
      expect(a.record!['hybrid_fallback_funding_sponsor'], 'VENDOR');
      expect(await b.save({'base_fee_minor': 60000}), false);
      expect(b.stale, true);
      await b.load();
      expect(b.record!['base_fee_minor'], 75000);
    },
  );
  test(
    'fallback lost response reconciles by operation without a duplicate request',
    () async {
      final transport = RiderTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      final mission =
          (await app.deliveryApi.getFulfilmentVendorMissionsMissionId(
            missionId: 'demo-vendor-mission-1',
          )).object;
      final c = HybridFallbackController(runtime, mission);
      await c.load();
      transport.lose = true;
      expect(await c.request('PROVIDER_UNAVAILABLE'), false);
      final calls = transport.commands;
      c.dispose();
      final restored = HybridFallbackController(runtime, mission);
      addTearDown(restored.dispose);
      await restored.load();
      expect(restored.record!['State'], 'COMPLETED');
      expect(restored.attempt.pending, isNull);
      expect(transport.commands, calls);
      expect(restored.canRequest, false);
      final rider = RiderWorkspaceController(
        runtime,
        id: 'demo-vendor-mission-1',
      );
      addTearDown(rider.dispose);
      await rider.load();
      expect(rider.record!['stage'], 'FALLBACK_COMPLETED');
      expect(rider.actions, isEmpty);
    },
  );
  testWidgets('policy and fallback screens fit dark phone width', (
    tester,
  ) async {
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
        home: DeliveryPolicyScreen(
          runtime: runtime,
          branch: const {'id': 'mock-branch', 'name': 'Main branch'},
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Base delivery fee'), findsOneWidget);
    expect(tester.takeException(), isNull);
    final mission = (await tester.runAsync(
      () => app.deliveryApi.getFulfilmentVendorMissionsMissionId(
        missionId: 'demo-vendor-mission-1',
      ),
    ))!.object;
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: HybridFallbackScreen(runtime: runtime, mission: mission),
      ),
    );
    await flush(tester);
    expect(find.text('Fallback reason'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
