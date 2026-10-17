import 'support/mock_vendor_runtime.dart';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'support/mock/mock_vendor_client.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/app/vendor_mobile_app.dart';
import 'package:vendor/app/vendor_runtime.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/features/auth/models/vendor_session.dart';

MockVendorRuntime mockRuntime({
  MockAuthScenario scenario = MockAuthScenario.multipleVendors,
}) => MockVendorRuntime(
  preferences: VendorPreferences(inMemory: true),
  mockLatency: Duration.zero,
)..scenario = scenario;
http.Response ok(Object? data) =>
    http.Response(jsonEncode({'success': true, 'data': data}), 200);

void main() {
  test(
    'mock is the default and its session never reaches live storage',
    () async {
      final store = MemoryVendorSessionStore();
      final runtime = MockVendorRuntime(
        preferences: VendorPreferences(inMemory: true),
        liveSessionStore: store,
        mockLatency: Duration.zero,
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      expect(runtime.mode, VendorDataMode.mock);
      expect(runtime.page, VendorPage.onboarding);
      await runtime.finishIntro();
      await runtime.start('+2348012345678', email: false);
      await runtime.verify(MockVendorClient.code);
      expect(runtime.page, VendorPage.vendorChoice);
      runtime.selectVendor(MockVendorClient.vendorIds.last);
      await runtime.chooseVendor();
      expect(runtime.auth.session!.vendorId, MockVendorClient.vendorIds.last);
      expect(runtime.page, VendorPage.signedIn);
      expect(await store.read(), isNull);
    },
  );

  test('single-vendor response goes directly to the signed-in state', () async {
    final runtime = mockRuntime(scenario: MockAuthScenario.singleVendor);
    addTearDown(runtime.dispose);
    await runtime.initialize();
    await runtime.start('owner@example.com', email: true);
    await runtime.verify(MockVendorClient.code);
    expect(runtime.page, VendorPage.signedIn);
    expect(runtime.auth.availableVendorIds, isEmpty);
  });

  test('wrong code is rejected; resend replaces the challenge', () async {
    final runtime = mockRuntime();
    addTearDown(runtime.dispose);
    await runtime.initialize();
    await runtime.start('owner@example.com', email: true);
    final first = runtime.auth.challenge!.challengeId;
    await runtime.verify('000000');
    expect(runtime.page, VendorPage.code);
    expect(runtime.error, contains('Invalid or expired'));
    expect(runtime.auth.session, isNull);
    await runtime.resend();
    expect(runtime.auth.challenge!.challengeId, isNot(first));
    expect(runtime.error, isNull);
    expect(runtime.remainingSeconds, greaterThan(290));
  });

  test(
    'mock expiry disables verification and can request another code',
    () async {
      final runtime = mockRuntime(scenario: MockAuthScenario.expiredCode);
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.start('owner@example.com', email: true);
      await Future<void>.delayed(const Duration(milliseconds: 1100));
      expect(runtime.codeExpired, isTrue);
      await runtime.verify(MockVendorClient.code);
      expect(runtime.auth.session, isNull);
      await runtime.resend();
      expect(runtime.codeExpired, isFalse);
    },
  );

  for (final scenario in [
    MockAuthScenario.accessUnavailable,
    MockAuthScenario.accessSuspended,
  ]) {
    test('${scenario.label} uses the backend access failure', () async {
      final runtime = mockRuntime(scenario: scenario);
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.start('owner@example.com', email: true);
      await runtime.verify(MockVendorClient.code);
      expect(runtime.page, VendorPage.accessUnavailable);
      expect(runtime.auth.session, isNull);
      expect(runtime.suspended, scenario == MockAuthScenario.accessSuspended);
    });
  }

  test(
    'mock connection failure preserves contact and never falls back to live',
    () async {
      final runtime = mockRuntime(scenario: MockAuthScenario.connectionFailure);
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.finishIntro();
      await runtime.start('owner@example.com', email: true);
      expect(runtime.page, VendorPage.connectionError);
      expect(runtime.identifier, 'owner@example.com');
      expect(runtime.canRetry, isTrue);
      expect(runtime.error, contains('connection'));
    },
  );

  test(
    'live requests use the selected origin and exact auth payloads',
    () async {
      final requests = <http.Request>[];
      final store = MemoryVendorSessionStore();
      final runtime = MockVendorRuntime(
        preferences: VendorPreferences(inMemory: true),
        liveSessionStore: store,
        mockLatency: Duration.zero,
        liveClientFactory: () => MockClient((request) async {
          requests.add(request);
          if (request.url.path.endsWith('/start')) {
            return ok({'challenge_id': 'server-challenge', 'expires_in': 120});
          }
          return ok({
            'access_token': 'server-access',
            'refresh_token': 'server-refresh',
            'expires_in': 900,
            'staff_id': 'server-staff',
            'vendor_id': 'server-vendor',
          });
        }),
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.start('mock@example.com', email: true);
      expect(requests, isEmpty);
      await runtime.changeMode(
        VendorDataMode.live,
        origin: 'https://vendor.example',
      );
      expect(runtime.auth.challenge, isNull);
      await runtime.start('owner@example.com', email: true);
      await runtime.verify('654321');
      expect(
        requests.map((r) => r.url.origin),
        everyElement('https://vendor.example'),
      );
      expect(jsonDecode(requests[0].body), {'email': 'owner@example.com'});
      expect(jsonDecode(requests[1].body), {
        'email': 'owner@example.com',
        'challenge_id': 'server-challenge',
        'otp': '654321',
      });
      expect((await store.read())!.accessToken, 'server-access');
      await runtime.changeMode(VendorDataMode.mock);
      expect(runtime.auth.session, isNull);
      expect((await store.read())!.accessToken, 'server-access');
    },
  );

  test(
    'live origin validation fails before changing the mock transport',
    () async {
      final runtime = mockRuntime();
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await expectLater(
        runtime.changeMode(
          VendorDataMode.live,
          origin: 'https://vendor.example/api',
        ),
        throwsArgumentError,
      );
      expect(runtime.mode, VendorDataMode.mock);
      expect(runtime.busy, isFalse);
    },
  );

  test(
    'live restoration validates its saved session through refresh',
    () async {
      final store = MemoryVendorSessionStore();
      await store.save(
        const VendorSession(
          accessToken: 'old',
          refreshToken: 'saved',
          expiresIn: 900,
          staffId: 'staff',
          vendorId: 'vendor',
        ),
      );
      final requests = <http.Request>[];
      final runtime = MockVendorRuntime(
        preferences: VendorPreferences(inMemory: true),
        liveSessionStore: store,
        liveClientFactory: () => MockClient((request) async {
          requests.add(request);
          return ok({
            'access_token': 'new',
            'refresh_token': 'rotated',
            'expires_in': 900,
            'staff_id': 'staff',
            'vendor_id': 'vendor',
          });
        }),
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.changeMode(
        VendorDataMode.live,
        origin: 'https://vendor.example',
      );
      expect(requests.single.url.path, '/api/v1/vendor/auth/refresh');
      expect(jsonDecode(requests.single.body), {'refresh_token': 'saved'});
      expect(runtime.page, VendorPage.signedIn);
      expect((await store.read())!.refreshToken, 'rotated');
    },
  );

  test('rate limits follow Retry-After and allow an explicit retry', () async {
    final runtime = mockRuntime(scenario: MockAuthScenario.rateLimited);
    addTearDown(runtime.dispose);
    await runtime.initialize();
    await runtime.finishIntro();
    await runtime.start('owner@example.com', email: true);
    expect(runtime.canRequest, isFalse);
    expect(runtime.retrySeconds, inInclusiveRange(1, 3));
    await Future<void>.delayed(const Duration(milliseconds: 3100));
    expect(runtime.canRetry, isTrue);
    await runtime.retry();
    expect(runtime.page, VendorPage.code);
    expect(runtime.error, isNull);
  });

  test('secure live sessions are isolated by server origin', () async {
    FlutterSecureStorage.setMockInitialValues({});
    final requests = <http.Request>[];
    final runtime = MockVendorRuntime(
      preferences: VendorPreferences(inMemory: true),
      mockLatency: Duration.zero,
      liveClientFactory: () => MockClient((request) async {
        requests.add(request);
        if (request.url.path.endsWith('/start')) {
          return ok({'challenge_id': 'server-challenge', 'expires_in': 120});
        }
        return ok({
          'access_token': 'server-access',
          'refresh_token': 'server-refresh',
          'expires_in': 900,
          'staff_id': 'staff',
          'vendor_id': 'vendor',
        });
      }),
    );
    addTearDown(runtime.dispose);
    await runtime.initialize();
    await runtime.changeMode(
      VendorDataMode.live,
      origin: 'https://one.vendor.example',
    );
    await runtime.start('owner@example.com', email: true);
    await runtime.verify('654321');
    requests.clear();
    await runtime.changeMode(
      VendorDataMode.live,
      origin: 'https://two.vendor.example',
    );
    expect(requests, isEmpty);
    expect(runtime.auth.session, isNull);
    await runtime.changeMode(
      VendorDataMode.live,
      origin: 'https://one.vendor.example',
    );
    expect(requests.single.url.origin, 'https://one.vendor.example');
    expect(requests.single.url.path, '/api/v1/vendor/auth/refresh');
    expect(runtime.page, VendorPage.signedIn);
  });

  testWidgets('onboarding and auth fit a small phone and keyboard', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final runtime = mockRuntime();
    await tester.pumpWidget(VendorMobileApp(runtime: runtime));
    await tester.pumpAndSettle();
    expect(find.text('Explore Karryam'), findsOneWidget);
    expect(tester.takeException(), isNull);
    final background = tester
        .widget<Scaffold>(find.byType(Scaffold))
        .backgroundColor;
    await tester.tap(find.text('Explore Karryam'));
    await tester.pumpAndSettle();
    expect(find.text('Continue'), findsOneWidget);
    expect(
      tester.widget<Scaffold>(find.byType(Scaffold)).backgroundColor,
      background,
    );
    expect(tester.takeException(), isNull);
    // Onboarding now ends in account creation; existing vendors use the
    // "Sign in" link, which is on every intro page.
    await tester.tap(find.text('Sign in'));
    await tester.pumpAndSettle();
    expect(
      find.text('Use your registered phone number or email.'),
      findsOneWidget,
    );
    expect(
      tester.widget<Scaffold>(find.byType(Scaffold)).backgroundColor,
      background,
    );
    expect(tester.takeException(), isNull);
    await tester.tap(find.text('Email address').first);
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextFormField), 'owner@example.com');
    await tester.tap(find.byKey(const Key('send-code')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('otp-input')), findsOneWidget);
    await tester.enterText(
      find.byKey(const Key('otp-input')),
      MockVendorClient.code,
    );
    tester.view.viewInsets = const FakeViewPadding(bottom: 300);
    await tester.pump();
    expect(tester.takeException(), isNull);
    tester.view.resetViewInsets();
    await tester.tap(find.byKey(const Key('verify-code')));
    await tester.pumpAndSettle();
    expect(find.text('Choose your\nvendor account.'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    runtime.dispose();
  });
}
