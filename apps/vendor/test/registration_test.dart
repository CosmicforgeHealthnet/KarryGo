import 'support/mock_vendor_runtime.dart';
import 'package:flutter_test/flutter_test.dart';
import 'support/mock/mock_vendor_client.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/app/vendor_runtime.dart';
import 'package:vendor/features/onboarding/state/business_onboarding_controller.dart';
import 'package:vendor/features/auth/ui/vendor_auth_screens.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:flutter/material.dart';

void main() {
  test(
    'new owner verifies before entering draft business onboarding',
    () async {
      final runtime = MockVendorRuntime(
        preferences: VendorPreferences(inMemory: true),
        mockLatency: Duration.zero,
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.start(
        'new-owner@example.com',
        email: true,
        registration: {
          'legal_name': 'New vendor kitchen',
          'currency': 'NGN',
          'timezone': 'Africa/Lagos',
        },
      );
      await runtime.verify('000000');
      expect(runtime.auth.session, isNull);
      expect(runtime.page, VendorPage.code);
      await runtime.resend();
      await runtime.verify(MockVendorClient.code);
      expect(runtime.page, VendorPage.signedIn);
      final app = runtime.backend;
      final business = await app.businessApi.getBusinessesBusinessId(
        businessId: app.scope.vendorId!,
      );
      expect(business.object['status'], 'DRAFT');
      expect(business.object['legal_name'], 'New vendor kitchen');
      expect(business.object['currency'], 'NGN');
    },
  );
  test(
    'contact-only registration enters business creation before other setup',
    () async {
      final runtime = MockVendorRuntime(
        preferences: VendorPreferences(inMemory: true),
        mockLatency: Duration.zero,
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.start(
        'new-account@example.com',
        email: true,
        registration: {},
      );
      await runtime.verify(MockVendorClient.code);
      expect(runtime.page, VendorPage.signedIn);
      final controller = BusinessOnboardingController(runtime.backend);
      addTearDown(controller.dispose);
      await controller.load();
      expect(controller.needsBusiness, isTrue);
      final saved = await controller.save('POST', '/api/v1/vendor/businesses', {
        'legal_name': 'Fresh vendor business',
        'currency': 'NGN',
        'timezone': 'Africa/Lagos',
      });
      expect(saved, isTrue, reason: controller.error);
      await controller.load();
      expect(controller.needsBusiness, isFalse);
      expect(controller.business['status'], 'DRAFT');
    },
  );

  test('returning users skip the intro automatically after sign-in', () async {
    final preferences = VendorPreferences(inMemory: true);
    final first = MockVendorRuntime(
      preferences: preferences,
      mockLatency: Duration.zero,
    )..scenario = MockAuthScenario.singleVendor;
    await first.initialize();
    expect(first.page, VendorPage.onboarding);
    await first.start('existing@example.com', email: true);
    await first.verify(MockVendorClient.code);
    expect(first.page, VendorPage.signedIn);
    await Future<void>.delayed(Duration.zero);
    first.dispose();
    final returning = MockVendorRuntime(
      preferences: preferences,
      mockLatency: Duration.zero,
    );
    addTearDown(returning.dispose);
    await returning.initialize();
    expect(returning.page, VendorPage.signIn);
  });

  testWidgets('new user sign-up asks only for contact details', (tester) async {
    final runtime = MockVendorRuntime(
      preferences: VendorPreferences(inMemory: true),
      mockLatency: Duration.zero,
    );
    addTearDown(runtime.dispose);
    await runtime.initialize();
    await runtime.finishIntro();
    await tester.pumpWidget(
      MaterialApp(
        home: VendorSignInScreen(runtime: runtime, onSettings: () {}),
      ),
    );
    expect(find.text('Create account'), findsOneWidget);
    expect(find.text('Legal business name'), findsNothing);
    expect(find.text('Currency'), findsNothing);
    expect(find.text('Timezone'), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
  });
  test('returning Live users keep their selected source and session', () async {
    final preferences = VendorPreferences(inMemory: true);
    final sessions = MemoryVendorSessionStore();
    MockVendorRuntime makeRuntime() => MockVendorRuntime(
      preferences: preferences,
      liveSessionStore: sessions,
      mockLatency: Duration.zero,
      liveClientFactory: () => MockVendorClient(
        scenario: MockAuthScenario.singleVendor,
        latency: Duration.zero,
      ),
    );
    final first = makeRuntime();
    await first.initialize();
    expect(first.mode, VendorDataMode.mock);
    await first.changeMode(
      VendorDataMode.live,
      origin: 'https://api.example.com',
    );
    await first.start('returning@example.com', email: true);
    await first.verify(MockVendorClient.code);
    expect(first.page, VendorPage.signedIn);
    await Future<void>.delayed(Duration.zero);
    first.dispose();
    final returning = makeRuntime();
    addTearDown(returning.dispose);
    await returning.initialize();
    expect(returning.mode, VendorDataMode.live);
    expect(returning.page, VendorPage.signedIn);
  });
}
