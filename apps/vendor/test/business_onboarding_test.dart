import 'support/mock_vendor_runtime.dart';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/onboarding/ui/business_onboarding_screen.dart';
import 'package:vendor/features/onboarding/ui/branch_setup_screen.dart';
import 'package:vendor/features/onboarding/ui/setup_design.dart';
import 'package:flutter_test/flutter_test.dart';
import 'support/mock/mock_vendor_client.dart';
import 'support/mock/mock_onboarding_store.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/app/vendor_runtime.dart';
import 'package:vendor/features/onboarding/state/business_onboarding_controller.dart';
import 'package:vendor/features/onboarding/state/setup_progress.dart';
import 'package:vendor/features/onboarding/data/document_upload_client.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

Future<VendorRuntime> signedIn({
  MockOnboardingScenario scenario = MockOnboardingScenario.draft,
}) async {
  final r =
      MockVendorRuntime(
          preferences: VendorPreferences(inMemory: true),
          mockLatency: Duration.zero,
        )
        ..scenario = MockAuthScenario.singleVendor
        ..onboardingScenario = scenario;
  await r.initialize();
  await r.start('owner@example.com', email: true);
  await r.verify(MockVendorClient.code);
  return r;
}

Future<void> flushRequests(WidgetTester tester) async {
  for (var i = 0; i < 20; i++) {
    await tester.pump(const Duration(milliseconds: 10));
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 1));
    });
  }
}

void main() {
  test(
    'step states and final action follow readiness across review outcomes',
    () async {
      for (final scenario in [
        MockOnboardingScenario.draft,
        MockOnboardingScenario.pendingReview,
        MockOnboardingScenario.ready,
        MockOnboardingScenario.actionRequired,
        MockOnboardingScenario.expired,
        MockOnboardingScenario.suspended,
      ]) {
        final r = await signedIn(scenario: scenario);
        final c = BusinessOnboardingController(r.backend);
        await c.load();
        expect(
          canFinishSetup(c),
          [
            MockOnboardingScenario.pendingReview,
            MockOnboardingScenario.ready,
          ].contains(scenario),
        );
        expect(stepProgress(c, 'representative').state, switch (scenario) {
          MockOnboardingScenario.ready => SetupState.complete,
          MockOnboardingScenario.pendingReview => SetupState.underReview,
          MockOnboardingScenario.draft => SetupState.notStarted,
          _ => SetupState.attention,
        });
        expect(
          c.requirements.map(requirementStep).toSet().difference({
            'business',
            'representative',
            'capabilities',
            'locations',
            'bank',
            'verification',
          }),
          isEmpty,
        );
        c.dispose();
        r.dispose();
      }
    },
  );

  test(
    'a rotated session cannot redeem its previous security challenge',
    () async {
      final r = await signedIn();
      addTearDown(r.dispose);
      final challenge = await r.auth.startStepUp('finance.bank_change');
      await r.backend.client.refreshSession();
      await expectLater(
        r.auth.verifyStepUp(
          purpose: 'finance.bank_change',
          challengeId: challenge.challengeId,
          otp: '123456',
        ),
        throwsA(isA<Exception>()),
      );
      expect(r.auth.session, isNotNull);
      final fresh = await r.auth.startStepUp('finance.bank_change');
      final grant = await r.auth.verifyStepUp(
        purpose: 'finance.bank_change',
        challengeId: fresh.challengeId,
        otp: '123456',
      );
      expect(grant.grantId, isNotEmpty);
    },
  );

  test(
    'wrong security code preserves the signed-in session and challenge',
    () async {
      final r = await signedIn();
      addTearDown(r.dispose);
      final challenge = await r.auth.startStepUp('finance.bank_change');
      final accessToken = r.auth.session!.accessToken;
      await expectLater(
        r.auth.verifyStepUp(
          purpose: 'finance.bank_change',
          challengeId: challenge.challengeId,
          otp: '000000',
        ),
        throwsA(isA<Exception>()),
      );
      expect(r.auth.session, isNotNull);
      expect(r.auth.session!.accessToken, accessToken);
      final grant = await r.auth.verifyStepUp(
        purpose: 'finance.bank_change',
        challengeId: challenge.challengeId,
        otp: '123456',
      );
      expect(grant.grantId, isNotEmpty);
    },
  );

  testWidgets(
    'business setup and representative form fit a small phone and keyboard',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.view.resetViewInsets);
      final r = (await tester.runAsync(() => signedIn()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: BusinessOnboardingScreen(runtime: r, onSettings: () {}),
        ),
      );
      await flushRequests(tester);
      await tester.pumpAndSettle();
      expect(find.text('Your next chapter starts here.'), findsOneWidget);
      expect(tester.takeException(), isNull);
      expect(find.text('Your activation checklist'), findsNothing);
      final representative = find.text('Authorized representative');
      await tester.scrollUntilVisible(
        representative,
        180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.tap(representative);
      await flushRequests(tester);
      await tester.pumpAndSettle();
      expect(find.text('The person behind the business.'), findsOneWidget);
      expect(
        tester
            .widget<SetupStepIndicator>(find.byType(SetupStepIndicator))
            .current,
        1,
      );
      tester.view.viewInsets = const FakeViewPadding(bottom: 300);
      await tester.pump();
      final first = find.widgetWithText(TextFormField, 'First name');
      await tester.scrollUntilVisible(
        first,
        120,
        scrollable: find.byType(Scrollable).last,
      );
      await tester.enterText(first, 'Ada');
      for (final entry in {
        'Last name': 'Okafor',
        'Email': 'ada@example.com',
        'Phone': '+2348012345678',
      }.entries) {
        final field = find.widgetWithText(TextFormField, entry.key);
        await tester.scrollUntilVisible(
          field,
          120,
          scrollable: find.byType(Scrollable).last,
        );
        await tester.enterText(field, entry.value);
      }
      final authorized = find.text('Authorized to represent this business');
      await tester.scrollUntilVisible(
        authorized,
        120,
        scrollable: find.byType(Scrollable).last,
      );
      tester.view.resetViewInsets();
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.byType(Switch));
      expect(tester.widget<Switch>(find.byType(Switch)).value, isTrue);
      final save = find.text('Save & continue');
      await tester.scrollUntilVisible(
        save,
        120,
        scrollable: find.byType(Scrollable).last,
      );
      await tester.tap(save);
      await flushRequests(tester);
      await tester.pumpAndSettle();
      expect(find.text('What does your business do?'), findsOneWidget);
      expect(
        tester
            .widget<SetupStepIndicator>(find.byType(SetupStepIndicator))
            .current,
        2,
      );
      final saved = (await tester.runAsync(
        () =>
            r.backend.onboardingApi.request('GET', '/api/v1/vendor/onboarding'),
      ))!;
      expect((saved.object['representative'] as Map)['authorized'], isTrue);
      expect(find.text('Your next chapter starts here.'), findsNothing);

      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await flushRequests(tester);
      await tester.pumpAndSettle();
    },
  );
  testWidgets(
    'brands and branches continues directly to document verification',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(
        () => signedIn(scenario: MockOnboardingScenario.pendingReview),
      ))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: BusinessOnboardingScreen(runtime: r, onSettings: () {}),
        ),
      );
      await flushRequests(tester);
      await tester.pumpAndSettle();
      final locations = find.text('Brands & branches');
      await tester.scrollUntilVisible(
        locations,
        180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      await tester.ensureVisible(locations);
      await tester.pumpAndSettle();
      await tester.tap(locations);
      await tester.pumpAndSettle();
      final next = find.text('Continue to documents');
      expect(next.hitTestable(), findsOneWidget);
      await tester.tap(next);
      await flushRequests(tester);
      await tester.pumpAndSettle();
      expect(find.text('Trust starts here.'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await flushRequests(tester);
      await tester.pumpAndSettle();
    },
  );

  testWidgets('mock onboarding can be completed by clicking without typing', (
    tester,
  ) async {
    final r = (await tester.runAsync(() => signedIn()))!;
    addTearDown(r.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: BusinessOnboardingScreen(runtime: r, onSettings: () {}),
      ),
    );
    await flushRequests(tester);
    await tester.pumpAndSettle();
    Future<void> tapLabel(String label) async {
      final target = find.text(label);
      await tester.scrollUntilVisible(
        target,
        180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      await tester.ensureVisible(target);
      await tester.pumpAndSettle();
      await tester.runAsync(() async {
        await tester.tap(target);
        await Future<void>.delayed(const Duration(milliseconds: 50));
      });
      await flushRequests(tester);
      await tester.pumpAndSettle();
    }

    await tapLabel('Business details');
    await tapLabel('Save & continue');
    expect(find.text('The person behind the business.'), findsOneWidget);
    await tapLabel('Save & continue');
    await tapLabel('Save & continue');
    expect(find.text('Built for your business.'), findsOneWidget);
    await tapLabel('Add branch');
    await tapLabel('Save & continue');
    await tapLabel('Continue to documents');
    expect(find.text('Trust starts here.'), findsOneWidget);
    await tapLabel('Upload document');
    await tapLabel('Submit selected documents');
    final c = BusinessOnboardingController(r.backend);
    await tester.runAsync(c.load);
    expect(c.representative['authorized'], isTrue);
    expect(c.branches, hasLength(1));
    expect(c.capabilities, hasLength(1));
    expect(c.bank, isEmpty);
    expect(c.documents.single['status'], 'SUBMITTED');
    expect(c.verifications.single['status'], 'PENDING_REVIEW');
    c.dispose();
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await flushRequests(tester);
    await tester.pumpAndSettle();
  });

  testWidgets('branch setup reads exact operations contracts on a small phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final r = (await tester.runAsync(
      () => signedIn(scenario: MockOnboardingScenario.pendingReview),
    ))!;
    addTearDown(r.dispose);
    final c = BusinessOnboardingController(r.backend);
    addTearDown(c.dispose);
    await tester.runAsync(c.load);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: BranchSetupScreen(
          controller: c,
          runtime: r,
          branch: c.branches.single,
          onSettings: () {},
        ),
      ),
    );
    await flushRequests(tester);
    await tester.pumpAndSettle();
    expect(find.text('Operating hours'), findsOneWidget);
    expect(
      find.text(
        'Branch settings could not be loaded. Check your access and try again.',
      ),
      findsNothing,
    );
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await flushRequests(tester);
    await tester.pumpAndSettle();
  });
  test(
    'branch operations use separate versions and return linked subject IDs',
    () async {
      final r = await signedIn(scenario: MockOnboardingScenario.pendingReview);
      addTearDown(r.dispose);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      await c.load();
      final params = {
        'businessId': c.businessId,
        'branchId': c.branches.single['id'] as String,
      };
      await c.request(
        'PUT',
        '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/hours',
        params: params,
        body: {
          'hours': [
            {
              'weekday': 1,
              'is_closed': false,
              'opens_at': '09:00',
              'closes_at': '17:00',
            },
          ],
          'expected_version': 1,
        },
      );
      await c.request(
        'PUT',
        '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/service-area',
        params: params,
        body: {
          'mode': 'RADIUS',
          'center_latitude': 6.5,
          'center_longitude': 3.4,
          'radius_meters': 5000,
          'expected_version': 0,
        },
      );
      await c.request(
        'PUT',
        '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/operations-settings',
        params: params,
        body: {
          'schema_version': 1,
          'accepting_new_orders': false,
          'pickup_enabled': true,
          'expected_version': 0,
        },
      );
      await c.request(
        'PUT',
        '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}',
        params: {...params, 'capabilityId': c.capabilities.single['id']},
        body: {'expected_version': 0},
      );
      await c.load();
      expect(c.branches.single['version'], 2);
      expect(
        (c.branches.single['capabilities'] as List).single['branch_id'],
        params['branchId'],
      );
    },
  );

  test(
    'draft setup uses server checklist and all nine capability types',
    () async {
      final r = await signedIn();
      addTearDown(r.dispose);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      await c.load();
      expect(c.error, isNull);
      expect(c.loaded, isTrue);
      expect(c.catalogue.length, 9);
      expect(c.readiness['ready'], false);
      expect(c.activated, false);
      expect(
        c.requirements.any((v) => v['code'] == 'AUTHORIZED_REPRESENTATIVE'),
        true,
      );
    },
  );
  test('pending review cannot activate itself', () async {
    final r = await signedIn(scenario: MockOnboardingScenario.pendingReview);
    addTearDown(r.dispose);
    final c = BusinessOnboardingController(r.backend);
    addTearDown(c.dispose);
    await c.load();
    await c.request(
      'POST',
      '/api/v1/vendor/onboarding/request-activation',
      body: {},
    );
    await c.load();
    expect(c.activated, false);
    expect(c.verifications.every((v) => v['status'] == 'PENDING_REVIEW'), true);
  });
  test(
    'server-ready fixture activates only through request activation',
    () async {
      final r = await signedIn(scenario: MockOnboardingScenario.ready);
      addTearDown(r.dispose);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      await c.load();
      expect(c.activated, false);
      await c.request(
        'POST',
        '/api/v1/vendor/onboarding/request-activation',
        body: {},
      );
      await c.load();
      expect(c.activated, true);
    },
  );
  test(
    'bank requires a purpose-bound single-use security grant and returns masked data',
    () async {
      final r = await signedIn();
      addTearDown(r.dispose);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      expect(
        await c.save('PUT', '/api/v1/vendor/onboarding/bank-profile', {
          'bank_code': '044',
          'account_number': '1234567890',
          'expected_version': 0,
        }),
        false,
      );
      final ch = await r.auth.startStepUp('finance.bank_change');
      final grant = await r.auth.verifyStepUp(
        purpose: 'finance.bank_change',
        challengeId: ch.challengeId,
        otp: '123456',
      );
      expect(
        await c.save('PUT', '/api/v1/vendor/onboarding/bank-profile', {
          'bank_code': '044',
          'account_number': '1234567890',
          'expected_version': 0,
          'step_up_grant_id': grant.grantId,
        }),
        true,
      );
      await c.load();
      expect(c.bank.containsKey('account_number'), false);
      expect(c.bank['masked_account'], '******7890');
      expect(
        await c.save('PUT', '/api/v1/vendor/onboarding/bank-profile', {
          'bank_code': '044',
          'account_number': '1234567890',
          'expected_version': 1,
          'step_up_grant_id': grant.grantId,
        }),
        false,
      );
    },
  );
  test(
    'private upload precedes document attachment and review stays pending',
    () async {
      final r = await signedIn();
      addTearDown(r.dispose);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      final bytes = Uint8List.fromList('%PDF-1.7 test'.codeUnits);
      final grant = await c.request(
        'POST',
        '/api/v1/vendor/onboarding/document-uploads',
        body: {
          'dimension': 'BUSINESS',
          'subject_type': 'BUSINESS',
          'subject_ref': c.businessId,
          'mime_type': 'application/pdf',
          'byte_size': bytes.length,
        },
      );
      expect(
        await c.save('POST', '/api/v1/vendor/onboarding/documents', {
          'dimension': 'BUSINESS',
          'document_type': 'registration',
          'media_ref': grant.object['media_ref'],
          'expected_version': 0,
        }),
        false,
      );
      await DocumentUploadClient().upload(
        grant.object,
        bytes,
        'application/pdf',
        transport: r.mockUploadTransport,
      );
      expect(
        await c.save('POST', '/api/v1/vendor/onboarding/documents', {
          'dimension': 'BUSINESS',
          'document_type': 'registration',
          'media_ref': grant.object['media_ref'],
          'expected_version': 0,
        }),
        true,
      );
      await c.load();
      final doc = c.documents.single;
      await c.request(
        'POST',
        '/api/v1/vendor/onboarding/verifications/{dimension}/submit',
        params: {'dimension': 'BUSINESS'},
        body: {
          'subject_type': 'BUSINESS',
          'subject_ref': c.businessId,
          'document_refs': [doc['id']],
        },
      );
      await c.load();
      expect(c.verifications.single['status'], 'PENDING_REVIEW');
      expect(c.documents.single['status'], 'SUBMITTED');
      expect(c.readiness['ready'], false);
      await expectLater(
        DocumentUploadClient().upload(
          grant.object,
          bytes,
          'application/pdf',
          transport: r.mockUploadTransport,
        ),
        throwsFormatException,
      );
    },
  );
  test('upload omits vendor credentials and refuses redirects', () async {
    final transport = MockClient((request) async {
      expect(
        request.headers.keys.any((v) => v.toLowerCase() == 'authorization'),
        false,
      );
      expect(request.followRedirects, false);
      expect(request.headers['X-Upload-Token'], 'grant-token');
      return http.Response(
        '',
        302,
        headers: {'location': 'https://another.example.com'},
      );
    });
    await expectLater(
      DocumentUploadClient().upload(
        {
          'upload_url': 'https://media.example.com/content',
          'upload_token': 'grant-token',
          'upload_session_id': 'session-a',
          'media_ref': 'object-a',
          'expires_at': DateTime.now()
              .add(const Duration(minutes: 1))
              .toIso8601String(),
          'max_bytes': 5,
          'allowed_mime_types': ['application/pdf'],
        },
        Uint8List.fromList([1, 2, 3]),
        'application/pdf',
        transport: transport,
      ),
      throwsFormatException,
    );
    transport.close();
  });
  test(
    'replayed command does not duplicate a branch and stale versions fail',
    () async {
      final r = await signedIn();
      addTearDown(r.dispose);
      final api = r.backend.businessApi;
      final command = VendorCommand.create();
      final body = VendorPayload({
        'display_name': 'Main',
        'address': {
          'line1': '1 Road',
          'city': 'Lagos',
          'region': 'Lagos',
          'country_code': 'NG',
        },
        'latitude': 6.5,
        'longitude': 3.4,
        'timezone': 'Africa/Lagos',
      });
      final params = {'businessId': r.backend.scope.vendorId!};
      final first = await api.request(
        'POST',
        '/api/v1/vendor/businesses/{businessId}/branches',
        body: body,
        pathParameters: params,
        command: command,
      );
      final second = await api.request(
        'POST',
        '/api/v1/vendor/businesses/{businessId}/branches',
        body: body,
        pathParameters: params,
        command: command,
      );
      expect(second.object['id'], first.object['id']);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      await c.load();
      expect(c.branches.length, 1);
      expect(
        await c.save('PATCH', '/api/v1/vendor/businesses/{businessId}', {
          'legal_name': 'Updated',
          'expected_version': 0,
        }, params: params),
        false,
      );
    },
  );
  for (final scenario in [
    MockOnboardingScenario.actionRequired,
    MockOnboardingScenario.expired,
    MockOnboardingScenario.suspended,
  ]) {
    test('${scenario.name} remains blocked', () async {
      final r = await signedIn(scenario: scenario);
      addTearDown(r.dispose);
      final c = BusinessOnboardingController(r.backend);
      addTearDown(c.dispose);
      await c.load();
      expect(c.readiness['ready'], false);
      expect(c.activated, false);
    });
  }
}
