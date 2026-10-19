import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/auth/models/provider_auth_models.dart';
import 'package:truck_provider/features/home/ui/screens/provider_delivery_complete_screen.dart';
import 'package:truck_provider/features/profile/ui/provider_security_screen.dart';
import 'package:truck_provider/features/wallet/data/provider_wallet_api.dart';
import 'package:truck_provider/features/wallet/state/provider_withdrawal_controller.dart';

ProviderWithdrawalController _wallet({required bool pinSet, List<String>? calls}) {
  return ProviderWithdrawalController(
    api: ProviderWalletApi(
      config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/payment-wallet'),
      client: MockClient((req) async {
        calls?.add('${req.method} ${req.url.path} ${req.body}');
        if (req.url.path.endsWith('/provider/pin/status')) {
          return http.Response(jsonEncode({'success': true, 'data': {'pin_set': pinSet}}), 200);
        }
        return http.Response(jsonEncode({'success': true, 'data': {}}), 200);
      }),
    ),
    accessToken: () => 'token',
  );
}

Future<void> _enterPin(WidgetTester tester, String pin) async {
  await tester.enterText(find.byType(TextField), pin);
  await tester.tap(find.text('Continue'));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('delivery complete shows the route and fare, and Done closes it', (tester) async {
    var done = false;
    await tester.pumpWidget(MaterialApp(
      home: ProviderDeliveryCompleteScreen(
        booking: ProviderBooking(
          id: 'abcd1234-0000',
          pickupAddress: 'Ikeja',
          pickupLat: 6.6,
          pickupLng: 3.35,
          dropoffAddress: 'Lekki',
          dropoffLat: 6.44,
          dropoffLng: 3.47,
          cargoType: 'general',
          cargoWeightKg: 100,
          fareFinalKobo: 1250000,
          status: 'completed',
          createdAt: DateTime(2026, 10, 19),
        ),
        onDone: () => done = true,
      ),
    ));

    expect(find.text('Delivery complete'), findsOneWidget);
    expect(find.text('Ikeja'), findsOneWidget);
    expect(find.text('Lekki'), findsOneWidget);
    expect(find.textContaining('12,500'), findsOneWidget);
    await tester.tap(find.text('Back to requests'));
    expect(done, isTrue);
  });

  testWidgets('security creates a PIN when none is set', (tester) async {
    final calls = <String>[];
    await tester.pumpWidget(MaterialApp(home: ProviderSecurityScreen(controller: _wallet(pinSet: false, calls: calls))));
    await tester.pumpAndSettle();

    expect(find.text('Not set yet. You will need one to withdraw.'), findsOneWidget);
    await tester.tap(find.text('Create PIN'));
    await tester.pumpAndSettle();
    await _enterPin(tester, '1357');
    await _enterPin(tester, '1357');

    expect(calls.any((c) => c.startsWith('POST /api/v1/payment-wallet/provider/pin {"pin":"1357"}')), isTrue);
    expect(find.text('Your PIN is set.'), findsOneWidget);
  });

  testWidgets('security changes an existing PIN with the current one', (tester) async {
    final calls = <String>[];
    await tester.pumpWidget(MaterialApp(home: ProviderSecurityScreen(controller: _wallet(pinSet: true, calls: calls))));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Change PIN'));
    await tester.pumpAndSettle();
    await _enterPin(tester, '1111');
    await _enterPin(tester, '2468');
    await _enterPin(tester, '2468');

    final put = calls.firstWhere((c) => c.startsWith('PUT'));
    expect(put, contains('"current_pin":"1111"'));
    expect(put, contains('"new_pin":"2468"'));
  });

  testWidgets('mismatched new PINs change nothing', (tester) async {
    final calls = <String>[];
    await tester.pumpWidget(MaterialApp(home: ProviderSecurityScreen(controller: _wallet(pinSet: false, calls: calls))));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Create PIN'));
    await tester.pumpAndSettle();
    await _enterPin(tester, '1111');
    await _enterPin(tester, '2222');

    expect(calls.where((c) => c.startsWith('POST')), isEmpty);
    expect(find.text('The two PINs did not match. Nothing was changed.'), findsOneWidget);
  });
}
