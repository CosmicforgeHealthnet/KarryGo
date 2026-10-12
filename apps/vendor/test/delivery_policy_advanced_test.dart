import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/delivery/state/delivery_policy_controller.dart';
import 'package:vendor/features/delivery/ui/delivery_policy_advanced_screen.dart';
import 'finance_test.dart' show financeApp;
import 'rider_delivery_test.dart' show RiderTransport;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

void main() {
  test(
    'advanced rules survive a lost response and identical retry after reopening',
    () async {
      final transport = RiderTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final branch = {'id': 'mock-branch', 'name': 'Main branch'};
      final c = DeliveryPolicyController(runtime, branch);
      await c.load();
      transport.lose = true;
      expect(
        await c.save({
          'fee_rule': 'ZONE_BASED',
          'operating_timezone': 'Africa/Lagos',
          'service_zones': [
            {
              'id': 'mock-zone-central',
              'name': 'Central',
              'enabled': true,
              'min_distance_meters': 0,
              'max_distance_meters': 5000,
              'fee_minor': 50000,
              'priority': 10,
            },
          ],
          'distance_tiers': [
            {
              'min_distance_meters': 0,
              'max_distance_meters': 0,
              'fee_minor': 75000,
            },
          ],
          'operating_periods': [
            {'weekday': 1, 'start_minute': 1320, 'end_minute': 120},
          ],
          'supported_vehicle_classes': ['MOTORCYCLE', 'VAN'],
          'fallback_triggers': ['TIME_OVERFLOW'],
        }),
        false,
      );
      expect(c.attempt.pending, isNotNull);
      c.dispose();
      final reopened = DeliveryPolicyController(runtime, branch);
      await reopened.load();
      expect(reopened.canWrite, isFalse);
      expect(await reopened.save({}), true);
      expect(transport.riderKeys[0], transport.riderKeys[1]);
      expect(transport.riderBodies[0], transport.riderBodies[1]);
      expect(reopened.record!['version'], 2);
      expect(reopened.record!['service_zones'][0]['id'], 'mock-zone-central');
      expect(reopened.record!['operating_periods'][0]['end_minute'], 120);
      expect(reopened.attempt.pending, isNull);
      reopened.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  test(
    'invalid tier gaps do not submit; advanced restrictions can be cleared explicitly',
    () async {
      final transport = RiderTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final c = DeliveryPolicyController(runtime, {'id': 'mock-branch'});
      await c.load();
      expect(
        await c.save({
          'distance_tiers': [
            {
              'min_distance_meters': 0,
              'max_distance_meters': 1000,
              'fee_minor': 50000,
            },
            {
              'min_distance_meters': 2000,
              'max_distance_meters': 0,
              'fee_minor': 100000,
            },
          ],
        }),
        false,
      );
      expect(transport.commands, 0);
      expect(c.error, contains('consecutive'));
      expect(
        await c.save({
          'distance_tiers': [],
          'service_zones': [],
          'operating_periods': [],
          'supported_vehicle_classes': [],
          'fallback_triggers': [],
          'max_delivery_radius_meters': null,
        }),
        true,
      );
      expect(c.record!['supported_vehicle_classes'], isEmpty);
      expect(c.record!['max_delivery_radius_meters'], isNull);
      c.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  testWidgets(
    'advanced screen and rule sheet fit a dark phone and support no-typing mock setup',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: const DeliveryPolicyAdvancedScreen(
            values: {'operating_timezone': 'Africa/Lagos'},
            currency: 'NGN',
            hybrid: true,
            mock: true,
          ),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('Add zone'));
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      await tester.ensureVisible(find.text('Save rule'));
      await tester.tap(find.text('Save rule'));
      await tester.pumpAndSettle();
      expect(find.text('Central delivery zone'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
    },
  );
}
