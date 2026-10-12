import 'package:http/http.dart' as http;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/scheduled_commerce/state/scheduling_workspace_controller.dart';
import 'package:vendor/features/scheduled_commerce/ui/scheduling_workspace_screen.dart';
import 'package:vendor/features/scheduled_commerce/ui/recurring_schedule_editor.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;
import 'crm_workspace_test.dart' show CrmTransport;
import 'package:vendor/features/scheduled_commerce/state/recurring_creation_controller.dart';

class LostScheduleReadTransport extends FinanceTransport {
  bool failRead = false;
  int writes = 0;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final response = await super.send(request);
    if (request.url.path.contains('/commerce/section21/')) {
      if (request.method == 'POST') {
        writes++;
        failRead = true;
      } else if (request.method == 'GET' && failRead) {
        failRead = false;
        throw http.ClientException('Lost refresh');
      }
    }
    return response;
  }
}

void main() {
  test(
    'Recurring creation freezes corporate context and safely recovers a lost response',
    () async {
      final transport = CrmTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final list = SchedulingWorkspaceController(runtime, recurring: true);
      await list.load();
      final source = list.rows.first;
      final creation = RecurringCreationController(runtime);
      await creation.restore();
      transport.lose = true;
      transport.suffix = '/recurring-schedules';
      final now = DateTime.now().toUtc().add(const Duration(days: 2));
      final changes = {
        'StartsOn': now.toIso8601String(),
        'EndsOn': null,
        'Headcount': 120,
      };
      expect(await creation.create(source: source, changes: changes), isNull);
      expect(creation.pending, isNotNull);
      final savedKey = creation.pending!['body']['IdempotencyKey'];
      source['Headcount'] = 999;
      creation.dispose();
      final restored = RecurringCreationController(runtime);
      await restored.restore();
      expect(restored.pending!['body']['IdempotencyKey'], savedKey);
      expect(restored.pending!['body']['Headcount'], 120);
      final result = await restored.create();
      expect(result, isNotNull, reason: restored.error);
      expect(result!['CorporateRef'], source['CorporateRef']);
      expect(result['DestinationSnapshot'], source['DestinationSnapshot']);
      expect(result['MenuRotation'], source['MenuRotation']);
      expect(result['ProductionRequirement'], source['ProductionRequirement']);
      expect(result['Status'], 'ACTIVE');
      await list.load();
      expect(list.rows.where((r) => r['ID'] == result['ID']).length, 1);
      expect(list.rows.length, 3);
      expect(restored.pending, isNull);
      restored.dispose();
      list.dispose();
      app.client.close();
    },
  );

  test(
    'scheduled reads paginate and cancellation reads back the stored status',
    () async {
      final app = await financeApp(FinanceTransport());
      final runtime = SupportRuntime(app);
      final list = SchedulingWorkspaceController(runtime);
      await list.load();
      expect(list.error, isNull);
      expect(list.rows, isNotEmpty);
      final page = await app.scheduledCommerceApi.request(
        'GET',
        '$schedulingBase/scheduled-orders',
        query: {'limit': '1'},
      );
      expect(page.object['items'], hasLength(1));
      expect((page.object['pagination'] as Map)['has_more'], true);
      final c = SchedulingWorkspaceController(
        runtime,
        id: list.rows.first['ID'] as String,
      );
      await c.load();
      expect(c.writable, true);
      expect(await c.changeStatus('CANCELLED'), true);
      expect(c.status, 'CANCELLED');
      expect(c.writable, false);
      list.dispose();
      c.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test(
    'recurring actions preserve opaque metadata and reject stale writes',
    () async {
      final app = await financeApp(FinanceTransport());
      final runtime = SupportRuntime(app);
      final c = SchedulingWorkspaceController(
        runtime,
        recurring: true,
        id: 'demo-recurring-0',
      );
      final stale = SchedulingWorkspaceController(
        runtime,
        recurring: true,
        id: 'demo-recurring-0',
      );
      await c.load();
      await stale.load();
      expect(c.error, isNull);
      expect(c.occurrenceError, isNull);
      expect(c.occurrences, hasLength(3));
      final destination = jsonObject(c.record!['DestinationSnapshot']);
      expect(await c.changeStatus('PAUSED'), true);
      expect(c.record!['Version'], 2);
      expect(await stale.changeStatus('PAUSED'), false);
      expect(stale.stale, true);
      await c.load();
      expect(await c.edit({'Headcount': 95, 'MealTime': '13:00'}), true);
      expect(c.record!['Headcount'], 95);
      expect(c.record!['DestinationSnapshot'], destination);
      expect(await c.changeStatus('ENDED'), true);
      expect(c.writable, false);
      c.dispose();
      stale.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test('accepted write with lost refresh requires reconciliation', () async {
    final transport = LostScheduleReadTransport();
    final app = await financeApp(transport);
    final runtime = SupportRuntime(app);
    final c = SchedulingWorkspaceController(
      runtime,
      recurring: true,
      id: 'demo-recurring-0',
    );
    await c.load();
    expect(await c.changeStatus('PAUSED'), false);
    expect(c.stale, true);
    expect(c.writable, false);
    expect(c.error, contains('accepted'));
    await c.load();
    expect(c.status, 'PAUSED');
    expect(transport.writes, 1);
    c.dispose();
    runtime.dispose();
    app.dispose();
  });

  testWidgets('scheduling list and recurring editor fit a small dark phone', (
    tester,
  ) async {
    late SupportRuntime runtime;
    await tester.runAsync(() async {
      runtime = SupportRuntime(await financeApp(FinanceTransport()));
    });
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: SchedulingWorkspaceScreen(runtime: runtime),
      ),
    );
    await flush(tester);
    expect(tester.takeException(), isNull);
    expect(find.text('Plan ahead'), findsOneWidget);
    final c = SchedulingWorkspaceController(
      runtime,
      recurring: true,
      id: 'demo-recurring-0',
    );
    await tester.runAsync(c.load);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: RecurringScheduleEditor(record: c.record!),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    c.dispose();
    runtime.value.dispose();
    runtime.dispose();
  });
}
