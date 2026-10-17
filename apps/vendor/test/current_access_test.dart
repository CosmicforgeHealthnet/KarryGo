import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'support/mock/mock_settings_store.dart';
import 'support/mock/mock_onboarding_store.dart';
import 'support/mock/mock_dashboard_data.dart';
import 'dashboard_test.dart' show dashboardRuntime;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Current access reads through the authenticated access contract',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final response = await runtime.backend.accessApi.request(
        'GET',
        '/api/v1/vendor/staff/me/access',
      );
      expect(response.object['staff_id'], 'mock-staff-1');
      expect(response.object['grants'], isNotEmpty);
    },
  );
  test('Access is resolved from own unrevoked assignments, not role names', () {
    final store = MockSettingsStore(
      MockOnboardingStore('mock-vendor', MockOnboardingScenario.activated),
      MockDashboardScenario.populated,
    );
    store.assignments.clear();
    store.roles.first['permission_codes'] = ['vendor.orders.read'];
    store.assignments.addAll([
      {
        'staff_id': 'mock-staff-1',
        'role_id': 'role_owner',
        'branch_id': 'branch-one',
      },
      {
        'staff_id': 'mock-staff-1',
        'role_id': 'role_owner',
        'branch_id': 'branch-two',
        'revoked_at': 'now',
      },
      {'staff_id': 'another-staff', 'role_id': 'role_owner', 'branch_id': null},
    ]);
    final request = http.Request(
      'GET',
      Uri.parse('https://mock/api/v1/vendor/staff/me/access'),
    );
    final data = store.handle(request, {})!['_data'] as Map;
    expect(data['grants'], [
      {
        'permission_code': 'vendor.orders.read',
        'branch_id': 'branch-one',
        'is_high_risk': false,
        'description': 'View vendor orders.',
        'category': 'commerce',
      },
    ]);
    store.assignments.clear();
    expect((store.handle(request, {})!['_data'] as Map)['grants'], isEmpty);
  });
}
