import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/app/vendor_runtime.dart';
import 'mock/mock_vendor_client.dart';
import 'mock/mock_dashboard_data.dart';
import 'mock/mock_onboarding_store.dart';

class MockVendorRuntime extends VendorRuntime {
  MockVendorRuntime({
    super.preferences,
    super.liveSessionStore,
    super.liveClientFactory,
    this.mockLatency = Duration.zero,
  });
  final Duration mockLatency;
  VendorDataMode _mode = VendorDataMode.mock;
  MockAuthScenario scenario = MockAuthScenario.multipleVendors;
  MockDashboardScenario dashboardScenario = MockDashboardScenario.populated;
  MockOnboardingScenario onboardingScenario = MockOnboardingScenario.draft;
  @override
  VendorDataMode get mode => _mode;
  @override
  String? get testVerificationCode =>
      mode == VendorDataMode.mock ? MockVendorClient.code : null;
  @override
  http.Client? createTransport() => mode == VendorDataMode.mock
      ? MockVendorClient(
          scenario: scenario,
          onboardingScenario: onboardingScenario,
          dashboardScenario: dashboardScenario,
          latency: mockLatency,
        )
      : super.createTransport();
  @override
  Future<void> initialize() async {
    final saved = await preferences.read('test.vendor.mode');
    if (saved != null) _mode = VendorDataMode.values.byName(saved);
    liveOrigin = await preferences.read('test.vendor.origin') ?? liveOrigin;
    await super.initialize();
  }

  Future<void> changeMode(VendorDataMode next, {String? origin}) async {
    if (next == VendorDataMode.live) {
      VendorAppConfig(origin: origin ?? liveOrigin);
    }
    _mode = next;
    if (origin != null) liveOrigin = origin;
    await preferences.write('test.vendor.mode', next.name);
    await preferences.write('test.vendor.origin', liveOrigin);
    await initialize();
  }

  Future<void> changeScenario(MockAuthScenario next) async {
    scenario = next;
    await initialize();
  }
}
