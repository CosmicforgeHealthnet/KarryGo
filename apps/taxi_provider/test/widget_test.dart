import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taxi_provider/app/taxi_provider_app.dart';
import 'package:taxi_provider/features/auth/data/provider_auth_api.dart';
import 'package:taxi_provider/features/auth/data/provider_session_store.dart';
import 'package:taxi_provider/features/auth/state/provider_auth_controller.dart';

void main() {
  ProviderAuthController buildAuth(
    Future<http.Response> Function(http.Request) handler,
  ) {
    return ProviderAuthController(
      api: ProviderAuthApi(
        config: ApiCoreConfig(baseUrl: 'http://localhost:8102/api/v1/taxi'),
        client: MockClient(handler),
      ),
      sessionStore: MemoryProviderSessionStore(),
    );
  }

  testWidgets('a signed-out driver lands on phone entry', (tester) async {
    final auth = buildAuth((_) async => http.Response('{}', 500));
    await auth.initialize();

    await tester.pumpWidget(TaxiProviderApp(authController: auth));
    await tester.pumpAndSettle();

    expect(find.text('Drive with Karry Go'), findsOneWidget);
    expect(find.text('Continue'), findsOneWidget);
  });

  testWidgets('submitting a phone number moves to the OTP screen', (
    tester,
  ) async {
    final auth = buildAuth((request) async {
      if (request.url.path.endsWith('/provider/auth/start')) {
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {
              'challenge_id': 'ch-1',
              'expires_in': 600,
              'debug_otp': '123456',
            },
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response('{}', 404);
    });
    await auth.initialize();

    await tester.pumpWidget(TaxiProviderApp(authController: auth));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField).first, '08099990001');
    await tester.tap(find.text('Continue'));
    await tester.pumpAndSettle();

    expect(find.text('Enter your code'), findsOneWidget);
    // Debug builds surface the code so a local run needs no log tailing.
    expect(find.text('123456'), findsOneWidget);
  });
}
