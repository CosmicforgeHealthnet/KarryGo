import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/app/vendor_runtime.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';

void main() {
  test(
    'the shipped runtime ignores saved mock settings and opens the intro',
    () async {
      final preferences = VendorPreferences(inMemory: true);
      await preferences.write('vendor.settings.mode', 'mock');
      await preferences.write(
        'vendor.settings.origin',
        'https://mock.karryam.invalid',
      );
      final runtime = VendorRuntime(
        preferences: preferences,
        liveSessionStore: MemoryVendorSessionStore(),
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      expect(runtime.mode, VendorDataMode.live);
      expect(runtime.useEmail, isTrue);
      expect(runtime.liveOrigin, 'https://vendor-api.staging.karryam.com');
      expect(runtime.page, VendorPage.onboarding);
      expect(runtime.testVerificationCode, isNull);
      expect(runtime.mockUploadTransport, isNull);
    },
  );
  test('returning users see sign-in without repeating the intro', () async {
    final preferences = VendorPreferences(inMemory: true);
    await preferences.write('vendor.intro.completed', 'true');
    final runtime = VendorRuntime(
      preferences: preferences,
      liveSessionStore: MemoryVendorSessionStore(),
    );
    addTearDown(runtime.dispose);
    await runtime.initialize();
    expect(runtime.page, VendorPage.signIn);
  });
  test('invalid service configuration produces a recoverable screen', () async {
    final runtime = VendorRuntime(
      preferences: VendorPreferences(inMemory: true),
      liveSessionStore: MemoryVendorSessionStore(),
    )..liveOrigin = 'invalid';
    addTearDown(runtime.dispose);
    await runtime.initialize();
    expect(runtime.page, VendorPage.connectionError);
    expect(runtime.canRetry, isTrue);
  });
  test('delivery refusal keeps the server explanation on sign-in', () async {
    final runtime = VendorRuntime(
      preferences: VendorPreferences(inMemory: true),
      liveSessionStore: MemoryVendorSessionStore(),
      liveClientFactory: () => MockClient(
        (request) async => http.Response(
          jsonEncode({
            'success': false,
            'error': {
              'code': 'service_unavailable',
              'message':
                  'Phone verification delivery is not configured. Use email.',
            },
          }),
          503,
        ),
      ),
    );
    addTearDown(runtime.dispose);
    await runtime.initialize();
    await runtime.finishIntro(registering: false);
    await runtime.start('+2348012345678', email: false);
    expect(runtime.page, VendorPage.signIn);
    expect(
      runtime.error,
      'Phone verification delivery is not configured. Use email.',
    );
    expect(runtime.canRetry, isTrue);
  });
}
