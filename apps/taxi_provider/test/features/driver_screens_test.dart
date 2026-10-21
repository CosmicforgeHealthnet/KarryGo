import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taxi_provider/app/taxi_provider_app.dart';
import 'package:taxi_provider/core/config/taxi_provider_config.dart';
import 'package:taxi_provider/core/data/driver_client.dart';
import 'package:taxi_provider/core/theme/karryam_theme.dart';
import 'package:taxi_provider/features/auth/data/provider_auth_api.dart';
import 'package:taxi_provider/features/auth/data/provider_session_store.dart';
import 'package:taxi_provider/features/auth/models/provider_auth_models.dart';
import 'package:taxi_provider/features/auth/state/provider_auth_controller.dart';
import 'package:taxi_provider/features/notifications/ui/notifications_screen.dart';
import 'package:taxi_provider/features/trips/data/trips_api.dart';
import 'package:taxi_provider/features/trips/ui/trips_screen.dart';
import 'package:taxi_provider/features/wallet/data/wallet_api.dart';
import 'package:taxi_provider/features/wallet/ui/withdraw_screen.dart';

const _base = 'http://localhost/api/v1/taxi';

http.Response _ok(Object data, [int status = 200]) =>
    http.Response(jsonEncode({'success': true, 'data': data}), status);

http.Response _unauthorized() => http.Response(
  jsonEncode({
    'success': false,
    'error': {'code': 'unauthorized', 'message': 'Token expired.'},
  }),
  401,
);

Map<String, dynamic> _session(String access, {String onboarding = 'complete'}) => {
  'access_token': access,
  'refresh_token': 'refresh-$access',
  'provider': {
    'id': 'driver-1',
    'phone': '+2348031234567',
    'first_name': 'Ada',
    'last_name': 'Obi',
    'account_status': 'active',
    'onboarding_status': onboarding,
  },
};

/// A signed-in auth controller backed by [handler].
Future<ProviderAuthController> signedIn(
  Future<http.Response> Function(http.Request) handler, {
  String onboarding = 'complete',
}) async {
  final store = MemoryProviderSessionStore();
  await store.write(ProviderSession.fromJson(_session('old', onboarding: onboarding)));
  final auth = ProviderAuthController(
    api: ProviderAuthApi(config: const ApiCoreConfig(baseUrl: _base), client: MockClient(handler)),
    sessionStore: store,
  );
  await auth.initialize();
  return auth;
}

/// Unmounts the screen and disposes [auth], whose session-refresh timer
/// would otherwise outlive the test.
Future<void> finish(WidgetTester tester, ProviderAuthController auth) async {
  await tester.pumpWidget(const SizedBox());
  auth.dispose();
}

void main() {
  group('service addresses', () {
    test('behind the gateway every service shares the origin', () {
      expect(
        TaxiProviderConfig.siblingBaseUrl(
          'https://api-gateway.staging.karryam.com/api/v1/taxi',
          '/api/v1/payment-wallet',
          8105,
        ),
        'https://api-gateway.staging.karryam.com/api/v1/payment-wallet',
      );
    });

    test('against taxi on its dev port, each service uses its own port', () {
      expect(
        TaxiProviderConfig.siblingBaseUrl('http://192.168.1.112:8102/api/v1/taxi', '/api/v1/support-disputes', 8107),
        'http://192.168.1.112:8107/api/v1/support-disputes',
      );
    });
  });

  group('DriverClient', () {
    test('refreshes an expired session once and retries the request', () async {
      // Startup refresh keeps the old token; the later one issues a new one.
      var refreshes = 0;
      final seen = <String>[];
      final auth = await signedIn((request) async {
        if (request.url.path.endsWith('/auth/refresh')) {
          return _ok(_session(++refreshes == 1 ? 'old' : 'new'));
        }
        return _ok({});
      });
      addTearDown(auth.dispose);
      final client = DriverClient(
        baseUrl: _base,
        auth: auth,
        client: MockClient((request) async {
          final token = request.headers['Authorization']!;
          seen.add(token);
          return token == 'Bearer new' ? _ok({'ok': true}) : _unauthorized();
        }),
      );
      final data = await client.getMap('/provider/profile');

      expect(data['ok'], isTrue);
      expect(seen, ['Bearer old', 'Bearer new']);
      expect(auth.state.status, ProviderAuthStatus.authenticated);
    });

    test('signs the driver out only when the refresh is refused', () async {
      var refreshes = 0;
      final auth = await signedIn((request) async {
        if (request.url.path.endsWith('/auth/refresh')) {
          return ++refreshes == 1 ? _ok(_session('old')) : _unauthorized();
        }
        return _ok({});
      });
      addTearDown(auth.dispose);
      final client = DriverClient(
        baseUrl: _base,
        auth: auth,
        client: MockClient((_) async => _unauthorized()),
      );

      await expectLater(client.get('/provider/profile'), throwsA(isA<ApiException>()));
      expect(auth.state.status, ProviderAuthStatus.phoneEntry);
    });
  });

  testWidgets('a new driver is taken to account setup, not home', (tester) async {
    final auth = await signedIn((request) async {
      if (request.url.path.endsWith('/auth/refresh')) {
        return _ok(_session('new', onboarding: 'profile_required'));
      }
      if (request.url.path.endsWith('/vehicle-classes')) return _ok({'classes': []});
      return _ok({});
    }, onboarding: 'profile_required');

    await tester.pumpWidget(TaxiProviderApp(authController: auth));
    await tester.pumpAndSettle();

    expect(find.text('Set up your account'), findsOneWidget);
    expect(find.text('About you'), findsOneWidget);
    // Continue stays off until the step is complete.
    final button = tester.widget<FilledButton>(find.widgetWithText(FilledButton, 'Continue'));
    expect(button.onPressed, isNull);
    expect(find.text('Enter the state and city you drive in.'), findsOneWidget);
    await finish(tester, auth);
  });

  testWidgets('trip history lists finished rides with their route and fare', (tester) async {
    final auth = await signedIn((r) async => r.url.path.endsWith('/auth/refresh') ? _ok(_session('new')) : _ok({}));
    final api = TripsApi(
      DriverClient(
        baseUrl: _base,
        auth: auth,
        client: MockClient(
          (_) async => _ok({
            'bookings': [
              {
                'id': 'b1',
                'status': 'completed',
                'fare_final_kobo': 250000,
                'created_at': '2026-10-18T10:00:00Z',
                'stops': [
                  {'sequence': 0, 'type': 'PICKUP', 'address': '12 Allen Ave, Ikeja'},
                  {'sequence': 1, 'type': 'DROPOFF', 'address': 'Lekki Phase 1'},
                ],
              },
            ],
          }),
        ),
      ),
    );

    await tester.pumpWidget(
      MaterialApp(theme: karryamTheme(Brightness.light), home: TripsScreen(api: api)),
    );
    await tester.pumpAndSettle();

    expect(find.text('12 Allen Ave, Ikeja'), findsOneWidget);
    expect(find.text('Lekki Phase 1'), findsOneWidget);
    expect(find.text('₦2,500.00'), findsOneWidget);
    await finish(tester, auth);
  });

  testWidgets('a withdrawal above the balance is refused before sending', (tester) async {
    final auth = await signedIn((r) async => r.url.path.endsWith('/auth/refresh') ? _ok(_session('new')) : _ok({}));
    var withdrawals = 0;
    final api = WalletApi(
      DriverClient(
        baseUrl: 'http://localhost/api/v1/payment-wallet',
        auth: auth,
        client: MockClient((r) async {
          if (r.url.path.endsWith('/withdrawals')) withdrawals++;
          return _ok({});
        }),
      ),
    );
    const account = BankAccount(id: 'a1', bankName: 'GTBank', accountNumber: '0123456789', accountName: 'ADA OBI', isDefault: true);

    await tester.pumpWidget(
      MaterialApp(
        theme: karryamTheme(Brightness.light),
        home: WithdrawScreen(api: api, availableKobo: 100000, accounts: const [account]),
      ),
    );
    await tester.enterText(find.byType(TextField).first, '5000');
    await tester.pump();

    expect(find.text('That is more than your available balance.'), findsOneWidget);
    final button = tester.widget<FilledButton>(find.widgetWithText(FilledButton, 'Withdraw'));
    expect(button.onPressed, isNull);
    expect(withdrawals, 0);
    await finish(tester, auth);
  });

  testWidgets('a first withdrawal creates the PIN, then sends it once', (tester) async {
    final auth = await signedIn((r) async => r.url.path.endsWith('/auth/refresh') ? _ok(_session('new')) : _ok({}));
    final calls = <String>[];
    final api = WalletApi(
      DriverClient(
        baseUrl: 'http://localhost/api/v1/payment-wallet',
        auth: auth,
        client: MockClient((r) async {
          calls.add('${r.method} ${r.url.path} ${r.body}');
          if (r.url.path.endsWith('/pin/status')) return _ok({'pin_set': false});
          if (r.url.path.endsWith('/withdrawals')) return _ok({'status': 'pending'}, 201);
          return _ok({});
        }),
      ),
    );
    const account = BankAccount(id: 'a1', bankName: 'GTBank', accountNumber: '0123456789', accountName: 'ADA OBI', isDefault: true);

    await tester.pumpWidget(
      MaterialApp(
        theme: karryamTheme(Brightness.light),
        home: Navigator(
          onGenerateRoute: (_) => MaterialPageRoute(
            builder: (_) => WithdrawScreen(api: api, availableKobo: 100000, accounts: const [account]),
          ),
        ),
      ),
    );
    await tester.enterText(find.byType(TextField).first, '500');
    await tester.pump();
    // The button's spinner animates while the PIN dialogs are open, so
    // advance time rather than waiting for everything to settle.
    Future<void> settle() async {
      for (var i = 0; i < 10; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
    }

    await tester.tap(find.widgetWithText(FilledButton, 'Withdraw'));
    await settle();
    for (var i = 0; i < 2; i++) {
      await tester.enterText(find.byType(TextField).last, '4321');
      await tester.tap(find.text('Continue'));
      await settle();
    }

    expect(calls.where((c) => c.startsWith('POST') && c.contains('/provider/pin ')).length, 1);
    final withdrawal = calls.singleWhere((c) => c.contains('/withdrawals'));
    expect(withdrawal, contains('"amount_kobo":50000'));
    expect(withdrawal, contains('"pin":"4321"'));
    expect(find.text('Withdrawal requested'), findsOneWidget);
    await finish(tester, auth);
  });

  testWidgets('the notification feed reads the server\'s field names', (tester) async {
    final auth = await signedIn((r) async => r.url.path.endsWith('/auth/refresh') ? _ok(_session('new')) : _ok({}));
    // notification-service sends Go field names (Title, Body, CreatedAt).
    final taxi = DriverClient(
      baseUrl: _base,
      auth: auth,
      client: MockClient((_) async => _ok([
        {'ID': 'n1', 'Title': 'Ride completed', 'Body': 'Your ride to Eko Hotel is complete.', 'CreatedAt': '2026-10-20T10:00:00Z'},
      ])),
    );

    await tester.pumpWidget(MaterialApp(theme: karryamTheme(Brightness.light), home: NotificationsScreen(taxi: taxi)));
    await tester.pumpAndSettle();

    expect(find.text('Ride completed'), findsOneWidget);
    expect(find.text('Your ride to Eko Hotel is complete.'), findsOneWidget);
    await finish(tester, auth);
  });
}
