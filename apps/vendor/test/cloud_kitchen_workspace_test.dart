import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/cloud_kitchen/state/cloud_kitchen_workspace_controller.dart';
import 'package:vendor/features/cloud_kitchen/ui/cloud_kitchen_workspace_screen.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'Kitchen settings, package versions, atomic bands and duplicate-safe creation',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final c = CloudKitchenWorkspaceController(
        runtime,
        '$cloudKitchenBase/bulk-packages',
      );
      await c.load();
      expect(c.error, isNull);
      expect(c.rows, isNotEmpty);
      final package = c.rows.first;
      final branch = package['branch_id'] as String,
          p = {'packageId': package['id'] as String};
      final settings = CloudKitchenWorkspaceController(
        runtime,
        '$cloudKitchenBase/settings/{branchId}',
        parameters: {'branchId': branch},
      );
      await settings.load();
      expect(settings.record?['version'], 0);
      final savedSettings = await settings.save(
        method: 'PUT',
        route: '$cloudKitchenBase/settings/{branchId}',
        params: {'branchId': branch},
        body: {
          ...settings.record!,
          'default_capacity_pool_ref': package['capacity_pool_id'],
        },
      );
      expect(savedSettings?['version'], 1, reason: settings.error);
      final added = await c.save(
        method: 'POST',
        route: '$cloudKitchenBase/bulk-packages/{packageId}/bands',
        params: p,
        body: {
          'min_quantity': 201,
          'max_quantity': 300,
          'unit_price_minor': 200000,
          'package_price_minor': 0,
          'capacity_units_per_item': 1,
          'expected_version': 1,
        },
      );
      expect(added, isNotNull, reason: c.error);
      final overlap = await c.save(
        method: 'POST',
        route: '$cloudKitchenBase/bulk-packages/{packageId}/bands',
        params: p,
        body: {
          'min_quantity': 30,
          'max_quantity': 250,
          'unit_price_minor': 200000,
          'package_price_minor': 0,
          'capacity_units_per_item': 1,
          'expected_version': 2,
        },
      );
      expect(overlap, isNull);
      expect(c.pending, isNull);
      await c.load();
      final actual = (await app.cloudKitchenApi.request(
        'GET',
        '$cloudKitchenBase/bulk-packages/{packageId}',
        pathParameters: p,
      )).object;
      expect(actual['version'], 2);
      final bands = (await app.cloudKitchenApi.request(
        'GET',
        '$cloudKitchenBase/bulk-packages/{packageId}/bands',
        pathParameters: p,
      )).list;
      expect(bands.length, 2);
      final band = jsonObject(bands.last);
      final updated = await c.save(
        method: 'PATCH',
        route: '$cloudKitchenBase/bulk-packages/{packageId}/bands/{bandId}',
        params: {...p, 'bandId': band['id'] as String},
        body: {
          'min_quantity': 201,
          'max_quantity': 400,
          'unit_price_minor': 190000,
          'package_price_minor': 0,
          'capacity_units_per_item': 1,
          'expected_package_version': 2,
          'expected_band_version': 1,
        },
      );
      expect(updated?['package_version'], 3);
      final check = (await app.cloudKitchenApi.request(
        'GET',
        '$cloudKitchenBase/bulk-packages/{packageId}/capacity-preview',
        pathParameters: p,
        query: {
          'quantity': '25',
          'start_at': DateTime.now()
              .add(const Duration(days: 1))
              .toUtc()
              .toIso8601String(),
          'end_at': DateTime.now()
              .add(const Duration(days: 1, hours: 1))
              .toUtc()
              .toIso8601String(),
        },
      )).object;
      expect(check['available'], true);
      expect(check['matched_band']['version'], 1);
      transport.lose = true;
      transport.suffix = '/bulk-packages';
      expect(
        await c.save(
          method: 'POST',
          route: '$cloudKitchenBase/bulk-packages',
          body: {
            'branch_id': branch,
            'name': 'New team package',
            'status': 'DRAFT',
            'minimum_quantity': 20,
            'setup_buffer_minutes': 0,
            'cleanup_buffer_minutes': 0,
            'logistics': {},
          },
        ),
        isNull,
      );
      final key = c.pending!['key'];
      c.dispose();
      final reopened = CloudKitchenWorkspaceController(
        runtime,
        '$cloudKitchenBase/bulk-packages',
      );
      await reopened.load();
      expect(reopened.pending!['key'], key);
      expect(await reopened.save(), isNotNull);
      await reopened.load();
      expect(
        reopened.rows.where((r) => r['name'] == 'New team package').length,
        1,
      );
      settings.dispose();
      reopened.dispose();
      app.client.close();
    },
  );
  test(
    'Corporate plan transitions, success-only exceptions and occurrence recovery',
    () async {
      final app = await financeApp(CrmTransport()),
          runtime = SupportRuntime(app);
      final c = CloudKitchenWorkspaceController(
        runtime,
        '$cloudKitchenBase/corporate-plans',
      );
      await c.load();
      final plan = c.rows.first, p = {'planId': plan['id'] as String};
      final suspended = await c.save(
        method: 'POST',
        route: '$cloudKitchenBase/corporate-plans/{planId}/suspend',
        params: p,
        body: {'expected_version': 1},
      );
      expect(suspended?['status'], 'SUSPENDED');
      final updated = await c.save(
        method: 'PATCH',
        route: '$cloudKitchenBase/corporate-plans/{planId}',
        params: p,
        body: {...plan, 'name': 'Updated office plan', 'version': 2},
      );
      expect(updated?['version'], 3);
      final exception = await c.save(
        method: 'POST',
        route: '$cloudKitchenBase/corporate-plans/{planId}/exceptions',
        params: p,
        body: {'occurrence_date': '2026-10-12', 'action': 'SKIP'},
      );
      expect(exception, isNotNull, reason: c.error);
      expect(c.pending, isNull);
      expect(
        (await c.save(
          method: 'POST',
          route: '$cloudKitchenBase/corporate-plans/{planId}/resume',
          params: p,
          body: {'expected_version': 3},
        ))?['status'],
        'ACTIVE',
      );
      final occurrences = CloudKitchenWorkspaceController(
        runtime,
        '$cloudKitchenBase/corporate-plans/{planId}/occurrences',
        parameters: p,
      );
      await occurrences.load();
      expect(occurrences.rows.length, 3);
      final conflict = occurrences.rows
          .where(
            (r) =>
                r['state'] == 'CAPACITY_CONFLICT' &&
                r['dead_lettered_at'] == null,
          )
          .first;
      expect(
        (await occurrences.save(
          method: 'POST',
          route:
              '$cloudKitchenBase/corporate-plans/{planId}/occurrences/{occurrenceId}/retry',
          params: {...p, 'occurrenceId': conflict['id'] as String},
          body: {'expected_version': 1, 'reason': 'Capacity adjusted'},
        ))?['state'],
        'PLANNED',
      );
      final dead = occurrences.rows
          .where((r) => r['dead_lettered_at'] != null)
          .first;
      expect(
        (await occurrences.save(
          method: 'POST',
          route:
              '$cloudKitchenBase/corporate-plans/{planId}/occurrences/{occurrenceId}/replay',
          params: {...p, 'occurrenceId': dead['id'] as String},
          body: {'expected_version': 1, 'reason': 'Configuration corrected'},
        ))?['replayed'],
        true,
      );
      final first = occurrences.rows.first,
          op = {...p, 'occurrenceId': occurrences.rows.first['id'] as String};
      expect(
        (await occurrences.save(
          method: 'POST',
          route:
              '$cloudKitchenBase/corporate-plans/{planId}/occurrences/{occurrenceId}/reschedule',
          params: op,
          body: {
            'expected_version': 1,
            'starts_at': DateTime.now()
                .add(const Duration(days: 2))
                .toUtc()
                .toIso8601String(),
            'ends_at': DateTime.now()
                .add(const Duration(days: 2, hours: 1))
                .toUtc()
                .toIso8601String(),
          },
        ))?['version'],
        2,
      );
      expect(
        (await occurrences.save(
          method: 'POST',
          route:
              '$cloudKitchenBase/corporate-plans/{planId}/occurrences/{occurrenceId}/cancel',
          params: op,
          body: {'expected_version': 2, 'reason': 'Customer rescheduled'},
        ))?['state'],
        'CANCELLED',
      );
      final invalid = await occurrences.save(
        method: 'POST',
        route:
            '$cloudKitchenBase/corporate-plans/{planId}/occurrences/{occurrenceId}/reschedule',
        params: op,
        body: {
          'expected_version': first['version'],
          'starts_at': first['starts_at'],
          'ends_at': first['ends_at'],
        },
      );
      expect(invalid, isNull);
      c.dispose();
      occurrences.dispose();
      app.client.close();
    },
  );
  testWidgets('Kitchen detail fits phone width in light and dark modes', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final app = (await tester.runAsync(() => financeApp(CrmTransport())))!,
        runtime = SupportRuntime(app);
    for (final brightness in [Brightness.light, Brightness.dark]) {
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: brightness),
          home: CloudKitchenWorkspaceScreen(
            key: ValueKey(brightness),
            runtime: runtime,
            area: KitchenArea.packages,
            id: 'mock-cloud-kitchen-package',
          ),
        ),
      );
      for (var i = 0; i < 20; i++) {
        await tester.pump(const Duration(milliseconds: 100));
        await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 10)),
        );
      }
      await tester.pump();
      expect(find.text('Team lunch package'), findsOneWidget);
      await tester.scrollUntilVisible(
        find.text('Smoky jollof rice'),
        200,
        maxScrolls: 10,
        scrollable: find.descendant(of: find.byType(ListView).first, matching: find.byType(Scrollable)).first,
      );
      expect(find.text('Smoky jollof rice'), findsOneWidget);
      expect(tester.takeException(), isNull);
    }
    await tester.pumpWidget(const SizedBox.shrink());
    app.client.close();
  });
}
