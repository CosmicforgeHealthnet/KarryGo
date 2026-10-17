import 'dart:convert';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/mode_http_client.dart';
import 'package:dispatch_provider/core/storage/token_storage.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/auth/data/dispatch_auth_api.dart';
import 'package:dispatch_provider/features/missions/data/provider_operations_api.dart';
import 'package:dispatch_provider/features/wallet/data/wallet_api.dart';
import 'package:dispatch_provider/features/wallet/state/wallet_controller.dart';
import 'package:dispatch_provider/features/wallet/ui/change_transaction_pin_screen.dart';
import 'package:dispatch_provider/features/wallet/ui/earning_summary_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.mock);
  });

  test(
    'Mock signup stays offline and Live cannot reuse its transport',
    () async {
      var networkCalls = 0;
      final client = ModeHttpClient(
        inner: MockClient((request) async {
          networkCalls++;
          throw StateError('Unexpected network call');
        }),
      );
      final api = DispatchAuthApi(
        ApiCoreConfig(baseUrl: 'https://dispatch.test'),
        client: client,
      );
      final started = await api.signupStart(
        phoneNumber: '+2348012345678',
        email: 'demo@example.com',
      );
      expect(started, isA<ApiSuccess>());
      final verified = await api.verify(
        identifier: '+2348012345678',
        otpCode: '123456',
        purpose: 'signup',
      );
      expect(verified, isA<ApiSuccess>());
      expect(networkCalls, 0);
      await AppModeConfig.instance.setMode(AppMode.live);
      await expectLater(
        client.get(Uri.parse('https://dispatch.test/api/v1/provider/me')),
        throwsA(isA<http.ClientException>()),
      );
      expect(networkCalls, 0);
      client.close();
    },
  );

  test(
    'Mock and Live credentials are isolated and individually restored',
    () async {
      final mock = TokenStorage();
      await mock.saveTokens(
        accessToken: 'demo-access',
        refreshToken: 'demo-refresh',
        providerId: 'demo',
      );
      await AppModeConfig.instance.setMode(AppMode.live);
      final live = TokenStorage();
      expect((await live.readTokens()).isEmpty, isTrue);
      await live.saveTokens(
        accessToken: 'live-test-access',
        refreshToken: 'live-test-refresh',
        providerId: 'live',
      );
      await AppModeConfig.instance.setMode(AppMode.mock);
      expect((await TokenStorage().readTokens()).providerId, 'demo');
      await AppModeConfig.instance.setMode(AppMode.live);
      expect((await TokenStorage().readTokens()).providerId, 'live');
    },
  );

  test(
    'A failed operational response keeps the same idempotency key on retry',
    () async {
      final keys = <String?>[];
      final client = MockClient((request) async {
        keys.add(request.headers['Idempotency-Key']);
        if (keys.length == 1)
          throw http.ClientException('Connection lost after sending');
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'scanned': true},
          }),
          200,
        );
      });
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        client: client,
        token: () => 'test-token',
      );
      const path = '/api/v1/provider/trips/trip/stops/stop/parcels/scan';
      const body = {
        'parcel_code': 'PARCEL-A',
        'manifest_ref': 'manifest',
        'manifest_version': 1,
      };
      await expectLater(
        api.command(path, body),
        throwsA(isA<http.ClientException>()),
      );
      expect((await api.command(path, body))['scanned'], isTrue);
      expect(keys[0], isNotEmpty);
      expect(keys[1], keys[0]);
    },
  );

  test(
    'Mock withdrawal refreshes history and a replay does not debit twice',
    () async {
      final client = ModeHttpClient();
      final api = WalletApi(
        ApiCoreConfig(baseUrl: 'https://wallet.test'),
        client: client,
      );
      await api.setPin(accessToken: 'mock', pin: '123456');
      Future<dynamic> withdraw() => api.requestWithdrawal(
        accessToken: 'mock',
        bankAccountId: 'demo-bank-1',
        amountKobo: 10000,
        idempotencyKey: 'withdraw-test',
        pin: '123456',
      );
      expect(await withdraw(), isA<ApiSuccess>());
      expect(await withdraw(), isA<ApiSuccess>());
      final earnings = await client.get(
        Uri.parse(
          'https://wallet.test/api/v1/payment-wallet/provider/earnings',
        ),
      );
      expect(jsonDecode(earnings.body)['data']['available_kobo'], 12440000);
      final history = await client.get(
        Uri.parse(
          'https://wallet.test/api/v1/payment-wallet/provider/transactions',
        ),
      );
      final entries = jsonDecode(history.body)['data']['transactions'] as List;
      expect(
        entries.where((entry) => entry['reference'] == 'DEMO-WITHDRAWAL-1'),
        hasLength(1),
      );
      client.close();
    },
  );

  testWidgets('Rejected PIN change shows the error and never reports success', (
    tester,
  ) async {
    final client = MockClient((request) async {
      expect(request.method, 'PUT');
      expect(jsonDecode(request.body), {
        'current_pin': '111111',
        'new_pin': '222222',
      });
      return http.Response(
        jsonEncode({
          'success': false,
          'error': {
            'code': 'invalid_pin',
            'message': 'Your current PIN is incorrect.',
          },
        }),
        403,
      );
    });
    final controller = WalletController(
      api: WalletApi(
        ApiCoreConfig(baseUrl: 'https://wallet.test'),
        client: client,
      ),
      getAccessToken: () => 'test-token',
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.light,
        home: ChangeTransactionPinScreen(walletController: controller),
      ),
    );
    final fields = find.byType(TextField);
    await tester.enterText(fields.at(0), '111111');
    await tester.enterText(fields.at(1), '222222');
    await tester.enterText(fields.at(2), '222222');
    await tester.pump();
    await tester.ensureVisible(find.text('Update PIN'));
    await tester.tap(find.text('Update PIN'));
    await tester.pumpAndSettle();
    expect(find.text('Your current PIN is incorrect.'), findsOneWidget);
    expect(find.text('Done'), findsNothing);
  });

  testWidgets(
    'Live earnings never shows the demo total or demo delivery count',
    (tester) async {
      await AppModeConfig.instance.setMode(AppMode.live);
      final controller = WalletController(
        api: WalletApi(
          ApiCoreConfig(baseUrl: 'https://wallet.test'),
          client: MockClient((_) async => http.Response('{}', 500)),
        ),
        getAccessToken: () => 'test-token',
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          home: EarningSummaryScreen(walletController: controller),
        ),
      );
      expect(find.textContaining('424,350'), findsNothing);
      expect(find.text('48'), findsNothing);
      expect(find.text('Available earnings'), findsOneWidget);
    },
  );
}
