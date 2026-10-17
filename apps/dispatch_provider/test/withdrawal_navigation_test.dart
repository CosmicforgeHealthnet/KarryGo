import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/mode_http_client.dart';
import 'package:dispatch_provider/features/wallet/ui/add_bank_account_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/wallet/data/wallet_api.dart';
import 'package:dispatch_provider/features/wallet/state/wallet_controller.dart';
import 'package:dispatch_provider/features/wallet/ui/withdrawal_form_screen.dart';
import 'package:dispatch_provider/features/wallet/ui/transaction_auth_screen.dart';

void main() {
  testWidgets(
    'Selected bank survives review and uncertain withdrawal retries reuse identity',
    (tester) async {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final writes = <Map<String, dynamic>>[];
      final client = MockClient((request) async {
        dynamic data;
        if (request.url.path.endsWith('/earnings')) {
          data = {
            'available_kobo': 100000,
            'pending_kobo': 0,
            'currency': 'NGN',
          };
        } else if (request.url.path.endsWith('/bank-accounts')) {
          data = {
            'bank_accounts': [
              {
                'id': 'first',
                'bank_name': 'First Bank',
                'account_name': 'Rider',
                'account_number': '1234561111',
                'is_default': true,
              },
              {
                'id': 'second',
                'bank_name': 'Second Bank',
                'account_name': 'Rider',
                'account_number': '1234562222',
                'is_default': false,
              },
            ],
          };
        } else if (request.url.path.endsWith('/pin/status')) {
          data = {'pin_set': true};
        } else if (request.url.path.endsWith('/withdrawals')) {
          writes.add(
            Map<String, dynamic>.from(jsonDecode(request.body) as Map),
          );
          if (writes.length == 1)
            throw http.ClientException('Connection lost after send');
          data = {
            'id': 'withdrawal',
            'reference': 'ref',
            'status': 'pending',
            'amount_kobo': 1000,
            'currency': 'NGN',
          };
        } else {
          throw StateError('Unexpected route: ${request.url}');
        }
        return http.Response(jsonEncode({'success': true, 'data': data}), 200);
      });
      final controller = WalletController(
        api: WalletApi(
          ApiCoreConfig(baseUrl: 'https://wallet.test'),
          client: client,
        ),
        getAccessToken: () => 'token',
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          home: WithdrawalFormScreen(walletController: controller),
        ),
      );
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('First Bank'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('First Bank'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Second Bank'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('1'));
      await tester.tap(find.text('0'));
      await tester.pump();
      await tester.tap(find.text('Continue'));
      await tester.pumpAndSettle();
      expect(find.text('Second Bank'), findsOneWidget);
      expect(find.text('First Bank'), findsNothing);
      expect(find.text('Fee'), findsNothing);
      await tester.tap(find.text('Continue to PIN'));
      await tester.pumpAndSettle();
      expect(
        tester
            .widget<TransactionAuthScreen>(find.byType(TransactionAuthScreen))
            .bankAccountId,
        'second',
      );
      for (var attempt = 0; attempt < 2; attempt++) {
        for (var digit = 0; digit < 6; digit++) {
          await tester.tap(find.text('1'));
        }
        await tester.pump();
        await tester.tap(find.text('Continue'));
        await tester.pumpAndSettle();
      }
      expect(writes, hasLength(2));
      expect(writes[0]['bank_account_id'], 'second');
      expect(writes[0]['idempotency_key'], writes[1]['idempotency_key']);
      expect(find.text('Withdrawal requested'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      controller.dispose();
      client.close();
    },
  );
  testWidgets(
    'Missing banks cannot submit and add-account navigation works on a small phone',
    (tester) async {
      tester.view.physicalSize = const Size(320, 640);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final client = MockClient(
        (request) async => http.Response(
          jsonEncode({
            'success': true,
            'data': request.url.path.endsWith('/earnings')
                ? {'available_kobo': 100000}
                : {'bank_accounts': []},
          }),
          200,
        ),
      );
      final controller = WalletController(
        api: WalletApi(
          ApiCoreConfig(baseUrl: 'https://wallet.test'),
          client: client,
        ),
        getAccessToken: () => 'token',
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.dark,
          home: WithdrawalFormScreen(walletController: controller),
        ),
      );
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('1'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('1'));
      await tester.pump();
      expect(
        tester
            .widget<FilledButton>(find.widgetWithText(FilledButton, 'Continue'))
            .onPressed,
        isNull,
      );
      await tester.ensureVisible(find.text('Add a bank account'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('Add a bank account'));
      await tester.pumpAndSettle();
      expect(find.text('Add bank account'), findsWidgets);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      controller.dispose();
      client.close();
    },
  );
  testWidgets(
    'Mock add-account saves through the same transport and appears in saved banks',
    (tester) async {
      FlutterSecureStorage.setMockInitialValues({});
      await AppModeConfig.instance.setMode(AppMode.mock);
      final client = ModeHttpClient(
        inner: MockClient(
          (_) async => throw StateError('Network used in Mock'),
        ),
      );
      final controller = WalletController(
        api: WalletApi(
          ApiCoreConfig(baseUrl: 'https://wallet.test'),
          client: client,
        ),
        getAccessToken: () => 'mock-token',
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          home: Scaffold(
            body: Builder(
              builder: (context) => TextButton(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) =>
                        AddBankAccountScreen(walletController: controller),
                  ),
                ),
                child: const Text('Add'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('Add'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Choose bank'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Access Bank').last);
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), '1234567890');
      await tester.pump();
      await tester.ensureVisible(find.text('Verify account'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('Verify account'));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('Save account'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('Save account'));
      await tester.pumpAndSettle();
      expect(find.text('Bank account saved'), findsOneWidget);
      await tester.tap(find.text('Continue'));
      await tester.pumpAndSettle();
      await controller.loadBankAccounts();
      expect(
        controller.accounts.where(
          (account) => account.accountNumber == '1234567890',
        ),
        hasLength(1),
      );
      expect(find.text('Add'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      controller.dispose();
      client.close();
    },
  );
}
