import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/app/customer_app.dart';
import 'package:customer/features/auth/data/customer_auth_api.dart';
import 'package:customer/features/auth/data/customer_session_store.dart';
import 'package:customer/features/auth/models/customer_auth_models.dart';
import 'package:customer/features/auth/state/customer_auth_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  testWidgets('customer can complete Figma-style OTP and profile setup flow', (
    tester,
  ) async {
    final api = CustomerAuthApi(
      config: const ApiCoreConfig(
        baseUrl: 'http://localhost:8101/api/v1/customer',
      ),
      client: MockClient((request) async {
        if (request.url.path.endsWith('/auth/start')) {
          return http.Response(
            jsonEncode({
              'success': true,
              'data': {
                'challenge_id': 'challenge-123',
                'expires_in': 300,
                'debug_otp': '123456',
              },
            }),
            200,
          );
        }

        if (request.url.path.endsWith('/auth/verify')) {
          return http.Response(
            jsonEncode({
              'success': true,
              'data': {
                'access_token': 'access-token',
                'refresh_token': 'refresh-token',
                'expires_in': 900,
                'customer': {
                  'id': 'customer-1',
                  'phone': '+2348012345678',
                  'first_name': null,
                  'last_name': null,
                  'onboarding_status': 'profile_required',
                  'status': 'active',
                },
              },
            }),
            200,
          );
        }

        return http.Response(
          jsonEncode({
            'success': false,
            'error': {'code': 'not_found', 'message': 'Endpoint not found.'},
          }),
          404,
        );
      }),
    );

    final controller = CustomerAuthController(
      api: api,
      sessionStore: MemoryCustomerSessionStore(),
    );

    await tester.pumpWidget(CustomerApp(controller: controller));
    await tester.pumpAndSettle();

    expect(find.text('Enable Location'), findsOneWidget);

    await tester.tap(find.text('Allow Location'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Enable Location'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Open Settings'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Enable Notification'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('I want to receive updates'));
    await tester.pumpAndSettle();

    expect(find.text('Welcome to Cosmicforge Logistics!'), findsOneWidget);

    await tester.enterText(find.byType(TextField).first, '8067735987');
    await tester.pumpAndSettle();
    await tester.ensureVisible(
      find.widgetWithText(FilledButton, 'Continue').first,
    );
    await tester.tap(find.widgetWithText(FilledButton, 'Continue').first);
    await tester.pumpAndSettle();

    expect(find.text('OTP Confirmation!'), findsOneWidget);
    expect(find.textContaining('Local test code: 123456'), findsOneWidget);

    await tester.enterText(find.byType(TextField).first, '123456');
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Continue').first);
    await tester.pumpAndSettle();

    expect(
      find.text('How do you want to use Cosmicforge Logistics?'),
      findsOneWidget,
    );

    await tester.tap(find.text('Send, Ride or Request'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Continue').first);
    await tester.pumpAndSettle();

    expect(find.text('Tell us about you'), findsOneWidget);

    await tester.enterText(find.byType(TextField).at(0), 'Ada Okafor');
    await tester.enterText(find.byType(TextField).at(2), 'ada@example.com');
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Continue').first);
    await tester.pumpAndSettle();

    expect(find.text('Upload a Photo of yourself'), findsOneWidget);

    await tester.tap(find.text('Upload Photo'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Final Step'));
    await tester.pumpAndSettle();

    expect(find.text("You're all set!"), findsOneWidget);

    await tester.tap(find.text('Continue to dashboard'));
    await tester.pumpAndSettle();

    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Book a ride'), findsOneWidget);
  });

  testWidgets('phone continue is disabled until a number is entered', (
    tester,
  ) async {
    final controller = CustomerAuthController(
      api: CustomerAuthApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8101/api/v1/customer',
        ),
        client: MockClient((request) async => http.Response('{}', 500)),
      ),
      sessionStore: MemoryCustomerSessionStore(),
    );

    await tester.pumpWidget(
      CustomerApp(controller: controller, autoInitialize: false),
    );
    controller.completeOnboarding();
    await tester.pumpAndSettle();

    final button = tester.widget<FilledButton>(
      find.widgetWithText(FilledButton, 'Continue').first,
    );

    expect(button.onPressed, isNull);
  });

  testWidgets('completed saved customer profile opens home after me check', (
    tester,
  ) async {
    final store = MemoryCustomerSessionStore();
    await store.saveSession(
      CustomerSession(
        accessToken: 'access-token',
        refreshToken: 'refresh-token',
        expiresAt: DateTime.now().add(const Duration(minutes: 15)),
        customer: const CustomerProfile(
          id: 'customer-1',
          phone: '+2348012345678',
          onboardingStatus: 'complete',
        ),
      ),
    );

    final controller = CustomerAuthController(
      api: CustomerAuthApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8101/api/v1/customer',
        ),
        client: MockClient((request) async {
          expect(request.url.path.endsWith('/me'), isTrue);
          return http.Response(
            jsonEncode({
              'success': true,
              'data': {
                'id': 'customer-1',
                'phone': '+2348012345678',
                'first_name': 'Ada',
                'last_name': 'Okafor',
                'onboarding_status': 'complete',
                'status': 'active',
              },
            }),
            200,
          );
        }),
      ),
      sessionStore: store,
    );

    await tester.pumpWidget(CustomerApp(controller: controller));
    await tester.pumpAndSettle();

    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Welcome, Ada Okafor'), findsOneWidget);
  });
}
