import 'package:flutter/material.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/scheduled_commerce/ui/recurring_first_setup_screen.dart';
import 'dashboard_test.dart' show flush;
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/scheduled_commerce/state/recurring_creation_controller.dart';
import 'package:vendor/features/scheduled_commerce/state/scheduling_workspace_controller.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'First corporate setup keeps approved context and recovers one schedule',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final page = (await app.scheduledCommerceApi.request(
        'GET',
        '$schedulingBase/recurring-setup-contexts',
      )).object;
      final context = jsonObject((page['items'] as List).single);
      final start = DateTime.now().toUtc().add(const Duration(days: 1));
      final source = <String, dynamic>{
        ...context,
        'Timezone': 'Africa/Lagos',
        'Weekdays': [1, 2, 3, 4, 5],
        'MealTime': '12:30',
        'Headcount': 20,
        'AllowanceMinor': 0,
        'AdvanceHorizonDays': 14,
        'StartsOn': start.toIso8601String(),
        'ProductionRequirement': {'required': false},
      };
      final changes = <String, dynamic>{
        'StartsOn': start.toIso8601String(),
        'Headcount': 20,
      };
      final c = RecurringCreationController(runtime);
      await c.restore();
      transport.suffix = '/recurring-schedules';
      transport.lose = true;
      expect(await c.create(source: source, changes: changes), isNull);
      expect(c.pending, isNotNull);
      final key = c.pending!['body']['IdempotencyKey'];
      c.dispose();
      final restored = RecurringCreationController(runtime);
      await restored.restore();
      expect(restored.pending!['body']['IdempotencyKey'], key);
      final created = await restored.create();
      expect(created, isNotNull, reason: restored.error);
      expect(created!['DestinationSnapshot'], context['DestinationSnapshot']);
      expect(created['MenuRotation'], context['MenuRotation']);
      expect(created['RequesterID'], context['RequesterID']);
      final list = SchedulingWorkspaceController(runtime, recurring: true);
      await list.load();
      expect(list.rows.where((r) => r['ID'] == created['ID']), hasLength(1));
      final bad = {
        ...source,
        'DestinationSnapshot': {'address': 'Changed address'},
      };
      expect(await restored.create(source: bad, changes: changes), isNull);
      expect(restored.pending, isNull);
      final altered = {
        ...source,
        'MenuRotation': {
          ...jsonObject(source['MenuRotation']),
          'mode': 'ROTATING',
        },
      };
      expect(await restored.create(source: altered, changes: changes), isNull);
      expect(restored.pending, isNull);
      list.dispose();
      restored.dispose();
      runtime.dispose();
      app.dispose();
    },
  );

  testWidgets('Corporate setup fits a small dark phone', (tester) async {
    late SupportRuntime runtime;
    await tester.runAsync(() async {
      runtime = SupportRuntime(await financeApp(CrmTransport()));
    });
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: RecurringFirstSetupScreen(runtime: runtime),
      ),
    );
    await flush(tester);
    expect(tester.takeException(), isNull);
    expect(find.text('Marina Business Group'), findsOneWidget);
    await tester.tap(find.text('Marina Business Group'));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    runtime.value.dispose();
    runtime.dispose();
  });
}
