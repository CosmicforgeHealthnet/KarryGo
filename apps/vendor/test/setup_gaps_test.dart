import 'support/mock_vendor_runtime.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'support/mock/mock_onboarding_store.dart';
import 'support/mock/mock_vendor_client.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:flutter_test/flutter_test.dart';
import 'support/mock/mock_dashboard_data.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/delivery/state/rider_workspace_controller.dart';
import 'package:vendor/features/settings/state/settings_controller.dart';
import 'dashboard_test.dart' show dashboardRuntime;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'First rider availability starts at zero and uses rider branch discovery',
    () async {
      final runtime = await dashboardRuntime(
        scenario: MockDashboardScenario.riderSetup,
      );
      addTearDown(runtime.dispose);
      final controller = RiderWorkspaceController(runtime);
      addTearDown(controller.dispose);
      await controller.load();
      expect(controller.error, isNull);
      expect(controller.availability, isNull);
      expect(controller.branches, isNotEmpty);
      expect(controller.branches.first['display_name'], isNotEmpty);
      expect(
        await controller.setAvailability('other-business-branch', 'AVAILABLE'),
        isFalse,
      );
      expect(
        await controller.setAvailability(
          '${controller.branches.first['id']}',
          'AVAILABLE',
        ),
        isTrue,
      );
      expect(controller.availability?['version'], 1);
      await controller.load();
      expect(controller.availability?['state'], 'AVAILABLE');
      expect(
        await controller.setAvailability(
          '${controller.branches.first['id']}',
          'OFFLINE',
        ),
        isTrue,
      );
      expect(controller.availability?['version'], 2);
    },
  );
  test(
    'Draft branches activate through existing business readiness rather than resume',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final controller = SettingsController(
        runtime.backend,
        SettingsKind.branches,
      );
      addTearDown(controller.dispose);
      await controller.load();
      final old = controller.rows.first;
      final brands = await controller.all(
        '${SettingsController.business}/brands',
        controller.scope,
      );
      if (brands.isEmpty) {
        final brand = await runtime.backend.businessApi.request(
          'POST',
          '${SettingsController.business}/brands',
          pathParameters: controller.scope,
          command: VendorCommand.create(),
          body: VendorPayload({'display_name': 'Setup test brand'}),
        );
        brands.add(brand.object);
      }
      final created = await runtime.backend.businessApi.request(
        'POST',
        controller.collection,
        pathParameters: controller.scope,
        command: VendorCommand.create(),
        body: VendorPayload({
          for (final key in [
            'display_name',
            'address',
            'latitude',
            'longitude',
            'timezone',
          ])
            key: old[key],
          'brand_id': brands.first['id'],
        }),
      );
      final branch = created.object;
      expect(branch['status'], 'DRAFT');
      expect(
        await controller.mutate(
          'POST',
          '${controller.detail}/resume',
          {'expected_version': branch['version']},
          parameters: controller.params(branch),
        ),
        isFalse,
      );
      await controller.load();
      final ready = await runtime.backend.onboardingApi.request(
        'GET',
        '/api/v1/vendor/onboarding/activation-readiness',
      );
      expect(ready.object['ready'], isFalse);
      expect(
        await controller.mutate(
          'POST',
          '/api/v1/vendor/onboarding/request-activation',
          null,
        ),
        isTrue,
      );
      final active = await controller.readRecord(branch);
      expect(active['status'], 'DRAFT');
      expect(active['version'], 1);
    },
  );
  test('Approved draft branch uses the existing activation endpoint', () async {
    final runtime =
        MockVendorRuntime(
            preferences: VendorPreferences(inMemory: true),
            mockLatency: Duration.zero,
          )
          ..scenario = MockAuthScenario.singleVendor
          ..onboardingScenario = MockOnboardingScenario.ready;
    await runtime.initialize();
    await runtime.start('owner@example.com', email: true);
    await runtime.verify(MockVendorClient.code);
    addTearDown(runtime.dispose);
    final controller = SettingsController(
      runtime.backend,
      SettingsKind.branches,
    );
    addTearDown(controller.dispose);
    await controller.load();
    final branch = controller.rows.first;
    expect(branch['status'], 'DRAFT');
    expect(
      await controller.mutate(
        'POST',
        '/api/v1/vendor/onboarding/request-activation',
        null,
      ),
      isTrue,
      reason:
          controller.error ??
          controller.feedback ??
          'stale=${controller.stale}',
    );
    expect((await controller.readRecord(branch))['status'], 'ACTIVE');
  });
}
