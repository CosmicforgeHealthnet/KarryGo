import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/app/data/mock_vendor_client.dart';
import 'package:vendor/app/data/mock_onboarding_store.dart';
import 'package:vendor/app/data/mock_dashboard_data.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/features/finance/state/finance_workspace_controller.dart';
import 'package:vendor/features/finance/state/withdrawal_attempt_store.dart';
import 'package:vendor/features/finance/state/vendor_withdrawal_workflow.dart';
import 'package:vendor/features/finance/ui/finance_design.dart';
import 'package:vendor/features/finance/ui/finance_panel.dart';
import 'package:vendor/features/finance/ui/withdrawal_history_screen.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

class FinanceTransport extends http.BaseClient {
  FinanceTransport({
    MockDashboardScenario scenario = MockDashboardScenario.populated,
  }) : inner = MockVendorClient(
         scenario: MockAuthScenario.singleVendor,
         onboardingScenario: MockOnboardingScenario.activated,
         dashboardScenario: scenario,
         latency: Duration.zero,
       );
  final MockVendorClient inner;
  bool loseResponse = false, rejectAuth = false;
  String? deniedResource;
  int posts = 0, refreshes = 0;
  final List<JsonObject> bodies = [];
  final List<String?> keys = [];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    if (request.url.path.endsWith('/auth/refresh')) refreshes++;
    if (request.method == 'GET' &&
        request.url.path.endsWith('/${deniedResource ?? '-'}')) {
      return http.StreamedResponse(
        Stream.value(
          utf8.encode(
            jsonEncode({
              'success': false,
              'error': {
                'code': 'FORBIDDEN',
                'message': 'Finance permission required.',
              },
            }),
          ),
        ),
        403,
      );
    }
    if (request.method == 'POST' &&
        request.url.path.endsWith('/finance/withdrawals')) {
      posts++;
      bodies.add(jsonObject(jsonDecode(utf8.decode(bytes))));
      keys.add(
        request.headers.entries
            .where((e) => e.key.toLowerCase() == 'idempotency-key')
            .firstOrNull
            ?.value,
      );
      if (rejectAuth) {
        return http.StreamedResponse(
          Stream.value(
            utf8.encode(
              jsonEncode({
                'success': false,
                'error': {
                  'code': 'UNAUTHORIZED',
                  'message': 'Session expired.',
                },
              }),
            ),
          ),
          401,
        );
      }
      final response = await inner.send(copy);
      if (loseResponse) {
        loseResponse = false;
        await response.stream.drain<void>();
        throw http.ClientException('Response lost after acceptance');
      }
      return response;
    }
    return inner.send(copy);
  }

  @override
  void close() {
    inner.close();
    super.close();
  }
}

class FailingAttemptStore extends MemoryWithdrawalAttemptStore {
  bool failSave = false, failClear = false;
  @override
  Future<void> save(JsonObject value) async {
    if (failSave) throw StateError('Storage unavailable');
    await super.save(value);
  }

  @override
  Future<void> clear() async {
    if (failClear) throw StateError('Storage unavailable');
    await super.clear();
  }
}

Future<VendorApp> financeApp(FinanceTransport transport) async {
  final app = VendorApp(
    config: VendorAppConfig(origin: 'https://vendor.example'),
    sessions: MemoryVendorSessionStore(),
    httpClient: transport,
  );
  await app.initialize();
  await app.auth.start(email: 'owner@example.com');
  await app.auth.verify(otp: MockVendorClient.code);
  return app;
}

Future<String> grant(
  VendorApp app, {
  String purpose = 'finance.withdrawal',
}) async {
  final challenge = await app.auth.startStepUp(purpose);
  return (await app.auth.verifyStepUp(
    purpose: purpose,
    challengeId: challenge.challengeId,
    otp: MockVendorClient.code,
  )).grantId;
}

Future<bool> withdraw(
  FinanceWorkspaceController c,
  String grantId, {
  int amount = 1000000,
}) => c.submit(
  bankAccountId: 'mock-bank-account',
  amount: amount,
  currency: 'NGN',
  grantId: grantId,
);

void main() {
  test(
    'amount parsing preserves exact kobo and rejects overflow or malformed input',
    () {
      expect(financeMinor('10000.01'), 1000001);
      expect(financeMinor('0.1'), 10);
      expect(financeMinor('0001.09'), 109);
      for (final value in [
        '1.001',
        '-1',
        '1e3',
        '1,000',
        '.',
        '92233720368547758.08',
      ]) {
        expect(financeMinor(value), isNull);
      }
      expect(financeMinor('92233720368547758.07'), 9223372036854775807);
    },
  );
  test(
    'request reserves available funds without changing unsettled earnings',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final c = FinanceWorkspaceController(app);
      addTearDown(c.dispose);
      await c.initialize();
      expect(c.activeBanks.single['status'], 'active');
      expect(c.canWithdraw, isTrue);
      expect(await withdraw(c, await grant(app)), isTrue);
      expect(c.summary['available_kobo'], 27465000);
      expect(c.summary['pending_kobo'], 4275000);
      expect(c.lastWithdrawal!['status'], 'processing');
      expect(c.pending, isNull);
      expect(c.transactions.first['transaction_type'], 'withdrawal_reserved');
      expect(c.transactions.first['side'], 'DEBIT');
      expect(
        (await c.readWithdrawal(
          c.lastWithdrawal!['reference'] as String,
        ))['amount_kobo'],
        1000000,
      );
      expect(transport.bodies.single.keys.toSet(), {
        'bank_account_id',
        'amount_kobo',
        'currency',
        'step_up_grant_id',
      });
    },
  );
  test(
    'lost acceptance survives controller restoration and replays without a second debit',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final store = MemoryWithdrawalAttemptStore();
      final c = FinanceWorkspaceController(app, attempts: store);
      await c.initialize();
      transport.loseResponse = true;
      expect(await withdraw(c, await grant(app)), isFalse);
      expect(c.unknown, isTrue);
      expect(c.canWithdraw, isFalse);
      expect(c.summary['available_kobo'], 27465000);
      c.dispose();
      final restored = FinanceWorkspaceController(app, attempts: store);
      addTearDown(restored.dispose);
      await restored.initialize();
      expect(restored.canWithdraw, isFalse);
      expect(await restored.retryOriginal(), isTrue);
      expect(transport.keys[0], transport.keys[1]);
      expect(transport.bodies[0], transport.bodies[1]);
      expect(restored.summary['available_kobo'], 27465000);
      expect(
        restored.withdrawals.where((w) => w['reference'] == 'mock-wd-1'),
        hasLength(1),
      );
      expect(await store.read(), isNull);
      expect(restored.canWithdraw, isTrue);
    },
  );
  test(
    'an unresolved request stays unresolved if its original grant can no longer be used',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final store = MemoryWithdrawalAttemptStore();
      await store.save({
        'vendor_id': app.scope.vendorId,
        'key': 'original-request',
        'body': {
          'bank_account_id': 'mock-bank-account',
          'amount_kobo': 1000000,
          'currency': 'NGN',
          'step_up_grant_id': 'expired-grant',
        },
      });
      final c = FinanceWorkspaceController(app, attempts: store);
      addTearDown(c.dispose);
      await c.initialize();
      expect(await c.retryOriginal(), isFalse);
      expect(c.unknown, isTrue);
      expect(c.canWithdraw, isFalse);
      expect((await store.read())!['key'], 'original-request');
    },
  );
  test(
    'wrong-purpose and refreshed-session grants cannot withdraw money',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final c = FinanceWorkspaceController(app);
      addTearDown(c.dispose);
      await c.initialize();
      expect(
        await withdraw(c, await grant(app, purpose: 'bank.change')),
        isFalse,
      );
      expect(c.summary['available_kobo'], 28465000);
      expect(c.canWithdraw, isTrue);
      final old = await grant(app);
      await app.client.refreshSession();
      expect(await withdraw(c, old), isFalse);
      expect(c.summary['available_kobo'], 28465000);
      expect(c.pending, isNull);
    },
  );
  test(
    'server minimum is authoritative and invalid amounts never debit the wallet',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final c = FinanceWorkspaceController(app);
      addTearDown(c.dispose);
      await c.initialize();
      expect(await withdraw(c, await grant(app), amount: 1), isFalse);
      expect(c.commandError, contains('minimum'));
      expect(await withdraw(c, 'unused', amount: 28465001), isFalse);
      expect(await withdraw(c, 'unused', amount: 0), isFalse);
      expect(c.summary['available_kobo'], 28465000);
      expect(transport.posts, 1);
      expect(c.pending, isNull);
    },
  );
  test(
    'idempotency rejects changed details and replays original accepted status',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final flow = VendorWithdrawalWorkflow(app.financeApi),
          key = VendorCommand.create(),
          grantId = await grant(app);
      Future<dynamic> send(int amount) => flow.request(
        bankAccountId: 'mock-bank-account',
        amountKobo: amount,
        stepUpGrantId: grantId,
        currency: 'NGN',
        command: key,
      );
      final first = await send(1000000), replay = await send(1000000);
      expect(first.object, replay.object);
      expect(first.statusCode, 202);
      await expectLater(
        send(2000000),
        throwsA(isA<ApiException>().having((e) => e.statusCode, 'status', 409)),
      );
      expect(
        (await app.financeApi.getFinanceOverview()).object['available_kobo'],
        27465000,
      );
    },
  );
  test(
    'financial POST never refreshes or repeats automatically on 401',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final grantId = await grant(app);
      final before = transport.refreshes;
      transport.rejectAuth = true;
      await expectLater(
        VendorWithdrawalWorkflow(app.financeApi).request(
          bankAccountId: 'mock-bank-account',
          amountKobo: 1000000,
          stepUpGrantId: grantId,
          command: VendorCommand.create(),
        ),
        throwsA(isA<ApiException>().having((e) => e.statusCode, 'status', 401)),
      );
      expect(transport.posts, 1);
      expect(transport.refreshes, before);
    },
  );
  test(
    'recovery storage must succeed before sending and confirmed acceptance is not reported as failed',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final store = FailingAttemptStore()..failSave = true;
      final c = FinanceWorkspaceController(app, attempts: store);
      addTearDown(c.dispose);
      await c.initialize();
      expect(await withdraw(c, await grant(app)), isFalse);
      expect(transport.posts, 0);
      expect(c.busy, isFalse);
      store.failSave = false;
      store.failClear = true;
      expect(await withdraw(c, await grant(app)), isTrue);
      expect(c.journalBlocked, isTrue);
      expect(c.canWithdraw, isFalse);
      store.failClear = false;
      await c.clearConfirmedRecovery();
      expect(c.pending, isNull);
      expect(c.canWithdraw, isTrue);
      expect(transport.posts, 1);
    },
  );
  test(
    'corrupt recovery blocks new commands and never replays malformed details',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final store = MemoryWithdrawalAttemptStore();
      await store.save({
        'vendor_id': app.scope.vendorId,
        'key': '',
        'body': {},
      });
      final c = FinanceWorkspaceController(app, attempts: store);
      addTearDown(c.dispose);
      await c.initialize();
      expect(c.journalBlocked, isTrue);
      expect(c.canWithdraw, isFalse);
      expect(await c.retryOriginal(), isFalse);
      expect(transport.posts, 0);
    },
  );
  test(
    'a denied transaction read leaves permitted balance and payout reads usable',
    () async {
      final transport = FinanceTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final c = FinanceWorkspaceController(app);
      addTearDown(c.dispose);
      await c.initialize();
      transport.deniedResource = 'transactions';
      await c.load();
      expect(c.restricted, {'transactions'});
      expect(c.transactions, isEmpty);
      expect(c.withdrawals, isNotEmpty);
      expect(c.canWithdraw, isTrue);
    },
  );
  test(
    'transfer verification status remains visible and is not treated as paid',
    () async {
      final transport = FinanceTransport(
        scenario: MockDashboardScenario.payoutVerification,
      );
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final c = FinanceWorkspaceController(app);
      addTearDown(c.dispose);
      await c.initialize();
      expect(await withdraw(c, await grant(app)), isTrue);
      expect(c.lastWithdrawal!['status'], 'requires_otp');
      expect(financeStatusCopy('requires_otp'), contains('cannot complete'));
      expect(financeStatusColor('requires_otp'), isNot(KarryamColors.tealInk));
    },
  );
  testWidgets(
    'phone withdrawal moves from review to verification and a real processing record',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final runtime = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(runtime.dispose);
      final c = FinanceWorkspaceController(runtime.backend);
      addTearDown(c.dispose);
      await tester.runAsync(c.initialize);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: Scaffold(
            body: FinancePanel(
              runtime: runtime,
              controller: c,
              onSetup: () async {},
              onChanged: () async {},
            ),
          ),
        ),
      );
      await flush(tester);
      await tester.ensureVisible(find.text('Withdraw funds'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Withdraw funds'));
      await flush(tester);
      await tester.ensureVisible(find.text('Review withdrawal'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Review withdrawal'));
      await tester.pumpAndSettle();
      expect(find.byType(BottomSheet), findsOneWidget);
      expect(find.text('Check your transfer.'), findsOneWidget);
      await tester.runAsync(() async {
        await tester.tap(find.text('Verify and withdraw'));
        await Future<void>.delayed(const Duration(milliseconds: 60));
      });
      await flush(tester);
      expect(find.text('Authorise withdrawal'), findsWidgets);
      expect(find.byType(BottomSheet), findsNothing);
      await tester.ensureVisible(
        find.widgetWithText(FilledButton, 'Authorise withdrawal'),
      );
      await tester.pumpAndSettle();
      await tester.runAsync(() async {
        await tester.tap(
          find.widgetWithText(FilledButton, 'Authorise withdrawal'),
        );
        await Future<void>.delayed(const Duration(milliseconds: 80));
      });
      await flush(tester);
      expect(find.text('Withdrawal requested'), findsOneWidget);
      expect(c.lastWithdrawal!['status'], 'processing');
      await tester.tap(find.text('Continue'));
      await flush(tester);
      expect(find.text('Withdrawal details'), findsOneWidget);
      expect(find.text('Processing'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );
  testWidgets(
    'Finance and transfer details fit a narrow phone with large text',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final runtime = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(runtime.dispose);
      final c = FinanceWorkspaceController(runtime.backend);
      addTearDown(c.dispose);
      await tester.runAsync(c.initialize);
      Widget frame(Widget screen) => MaterialApp(
        theme: vendorTheme(),
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(
            context,
          ).copyWith(textScaler: const TextScaler.linear(1.5)),
          child: child!,
        ),
        home: screen,
      );
      await tester.pumpWidget(
        frame(
          Scaffold(
            body: FinancePanel(
              runtime: runtime,
              controller: c,
              onSetup: () async {},
              onChanged: () async {},
            ),
          ),
        ),
      );
      await flush(tester);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(
        frame(WithdrawalDetailScreen(controller: c, reference: 'mock-wd-paid')),
      );
      await flush(tester);
      expect(find.text('Paid'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );
}
