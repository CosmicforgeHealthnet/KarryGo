import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/wallet/data/provider_wallet_api.dart';
import 'package:truck_provider/features/wallet/state/provider_withdrawal_controller.dart';
import 'package:truck_provider/features/wallet/ui/provider_transaction_auth_screen.dart';
import 'package:truck_provider/features/wallet/ui/provider_withdrawal_confirm_screen.dart';
import 'package:truck_provider/features/wallet/ui/provider_withdrawal_form_screen.dart';

Map<String, dynamic> _balance({int available = 20400000}) => {
      'available_kobo': available,
      'escrow_kobo': 0,
      'pending_kobo': 5400000,
      'currency': 'NGN',
    };

Map<String, dynamic> _accounts() => {
      'bank_accounts': [
        {
          'id': 'ba1', 'bank_code': '035', 'bank_name': 'Wema Bank',
          'account_number': '0450908723', 'account_name': 'Firepemi Adewale', 'status': 'active',
        },
      ],
    };

ProviderWithdrawalController _controller({
  List<String>? capturedPaths,
  List<Map<String, dynamic>>? capturedBodies,
  Map<String, dynamic>? withdrawalResponse,
  int statusForWithdrawal = 201,
  bool pinSet = true,
}) {
  final mock = MockClient((req) async {
    capturedPaths?.add('${req.method} ${req.url.path}');
    if (req.method != 'GET' && req.body.isNotEmpty) {
      capturedBodies?.add(Map<String, dynamic>.from(jsonDecode(req.body) as Map));
    }
    final path = req.url.path;
    if (path.endsWith('/provider/earnings')) {
      return http.Response(jsonEncode({'success': true, 'data': _balance()}), 200);
    }
    if (path.endsWith('/provider/bank-accounts') && req.method == 'GET') {
      return http.Response(jsonEncode({'success': true, 'data': _accounts()}), 200);
    }
    if (path.endsWith('/provider/pin/status')) {
      return http.Response(jsonEncode({'success': true, 'data': {'pin_set': pinSet}}), 200);
    }
    if (path.endsWith('/provider/pin')) {
      return http.Response(jsonEncode({'success': true, 'data': {'pin_set': true}}), 201);
    }
    if (path.endsWith('/provider/withdrawals')) {
      if (statusForWithdrawal >= 300) {
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {'code': 'conflict', 'message': 'Paystack Balance is not enough to process this withdrawal yet.'},
          }),
          statusForWithdrawal,
        );
      }
      return http.Response(
        jsonEncode({'success': true, 'data': withdrawalResponse ?? {
          'id': 'wd1', 'reference': 'wd_abc', 'amount_kobo': 20400000, 'status': 'pending',
        }}),
        201,
      );
    }
    return http.Response(jsonEncode({'success': true, 'data': {}}), 200);
  });
  return ProviderWithdrawalController(
    api: ProviderWalletApi(
      config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/payment-wallet'),
      client: mock,
    ),
    accessToken: () => 'token',
  );
}

Future<void> _pump(WidgetTester tester, Widget screen) async {
  await tester.binding.setSurfaceSize(const Size(430, 932));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(MaterialApp(home: screen));
  await tester.pumpAndSettle();
}

void main() {
  group('controller', () {
    test('amount keypad builds kobo and blocks leading zeros', () {
      final c = _controller();
      c.appendDigit('0'); // ignored
      c.appendDigit('2');
      c.appendDigit('0');
      c.appendDigit('0');
      expect(c.amountInput, '200');
      expect(c.amountNaira, 200);
      expect(c.amountKobo, 20000);
      c.deleteDigit();
      expect(c.amountInput, '20');
    });

    test('exceedsBalance gates submission', () async {
      final c = _controller();
      await c.startNewWithdrawal();
      for (final d in '2040000'.split('')) {
        c.appendDigit(d); // ₦2,040,000 > ₦204,000 available
      }
      expect(c.exceedsBalance, isTrue);
      expect(c.canSubmitAmount, isFalse);
    });

    test('loads balance and default account on start', () async {
      final c = _controller();
      await c.startNewWithdrawal();
      expect(c.balance.availableKobo, 20400000);
      expect(c.bankAccounts, hasLength(1));
      expect(c.selectedAccount?.accountName, 'Firepemi Adewale');
    });

    test('submitWithdrawal posts to /provider/withdrawals with the entered PIN', () async {
      final paths = <String>[];
      final bodies = <Map<String, dynamic>>[];
      final c = _controller(capturedPaths: paths, capturedBodies: bodies);
      await c.startNewWithdrawal();
      for (final d in '200000'.split('')) {
        c.appendDigit(d);
      }
      c.setEnteredPin('1234');
      final ok = await c.submitWithdrawal();
      expect(ok, isTrue);
      expect(c.result?.reference, 'wd_abc');
      expect(paths, contains('POST /api/v1/payment-wallet/provider/withdrawals'));
      final withdrawalBody = bodies.lastWhere((b) => b.containsKey('bank_account_id'));
      expect(withdrawalBody['pin'], '1234');
    });

    test('loads pin status on start and setupPin posts to /provider/pin', () async {
      final paths = <String>[];
      final c = _controller(capturedPaths: paths, pinSet: false);
      await c.startNewWithdrawal();
      expect(c.pinSet, isFalse);

      final ok = await c.setupPin('1234');
      expect(ok, isTrue);
      expect(c.pinSet, isTrue);
      expect(paths, contains('POST /api/v1/payment-wallet/provider/pin'));
    });

    test('surfaces backend error on failed withdrawal', () async {
      final c = _controller(statusForWithdrawal: 409);
      await c.startNewWithdrawal();
      for (final d in '200000'.split('')) {
        c.appendDigit(d);
      }
      final ok = await c.submitWithdrawal();
      expect(ok, isFalse);
      expect(c.submitError, contains('Paystack Balance is not enough'));
    });
  });

  group('screens', () {
    testWidgets('form shows balance and disables withdraw until amount entered', (tester) async {
      final c = _controller();
      await _pump(tester, ProviderWithdrawalFormScreen(controller: c));

      expect(find.text('Withdrawal Form'), findsOneWidget);
      expect(find.text('₦ 204,000.00'), findsOneWidget);

      // Tap keypad: 5 0 0 0 0
      for (final d in ['5', '0', '0', '0', '0']) {
        await tester.tap(find.text(d).first);
        await tester.pump();
      }
      expect(find.text('50,000'), findsOneWidget);
    });

    testWidgets('auth screen asks to enter the PIN when one is set', (tester) async {
      final c = _controller(pinSet: true);
      await c.startNewWithdrawal();
      await _pump(tester, ProviderTransactionAuthScreen(controller: c));

      expect(
        find.text('Please input your secure pin to authorize withdrawal'),
        findsOneWidget,
      );
      expect(find.text('Withdraw Now'), findsOneWidget);
      // The old visual-only password fallback is gone.
      expect(find.text('Use Password'), findsNothing);
    });

    testWidgets('auth screen walks through create + confirm when no PIN is set', (tester) async {
      final c = _controller(pinSet: false);
      await c.startNewWithdrawal();
      await _pump(tester, ProviderTransactionAuthScreen(controller: c));

      expect(
        find.text('Create a secure 4-digit pin to authorize your withdrawals'),
        findsOneWidget,
      );

      for (final d in ['1', '2', '3', '4']) {
        await tester.tap(find.text(d).first);
        await tester.pump();
      }
      await tester.tap(find.text('Continue'));
      await tester.pump();

      expect(find.text('Re-enter your new pin to confirm'), findsOneWidget);

      // Mismatched confirmation restarts the create phase with an error.
      for (final d in ['9', '9', '9', '9']) {
        await tester.tap(find.text(d).first);
        await tester.pump();
      }
      await tester.tap(find.text('Save Pin & Withdraw'));
      await tester.pump();

      expect(find.text('Pins did not match. Try again.'), findsOneWidget);
      expect(
        find.text('Create a secure 4-digit pin to authorize your withdrawals'),
        findsOneWidget,
      );
    });

    testWidgets('confirm shows amount and selected payout account', (tester) async {
      final c = _controller();
      await c.startNewWithdrawal();
      for (final d in '204000'.split('')) {
        c.appendDigit(d);
      }
      await _pump(tester, ProviderWithdrawalConfirmScreen(controller: c));

      expect(find.text('₦ 204,000.00'), findsOneWidget);
      expect(find.text('Firepemi Adewale'), findsOneWidget);
      expect(find.text('Wema Bank'), findsOneWidget);
      expect(find.text('Change default account'), findsOneWidget);
    });
  });
}
