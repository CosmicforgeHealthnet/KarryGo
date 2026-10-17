import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/wallet/data/wallet_api.dart';
import 'package:dispatch_provider/features/wallet/state/wallet_controller.dart';
import 'package:dispatch_provider/features/wallet/ui/add_bank_account_screen.dart';

void main() {
  testWidgets(
    'Directory failure disables verification; retry/search uses only server banks',
    (tester) async {
      var unavailable = true;
      String? resolvedCode;
      final client = MockClient((request) async {
        dynamic data;
        if (request.url.path.endsWith('/provider/banks')) {
          if (unavailable) return http.Response('unavailable', 503);
          data = {
            'banks': [
              {'name': 'Server Bank', 'code': '000123', 'currency': 'NGN'},
              {'name': 'Other Bank', 'code': '999', 'currency': 'NGN'},
            ],
          };
        } else {
          resolvedCode =
              (jsonDecode(request.body) as Map)['bank_code'] as String;
          data = {
            'account_name': 'Rider',
            'account_number': '1234567890',
            'bank_id': 7,
          };
        }
        return http.Response(jsonEncode({'success': true, 'data': data}), 200);
      });
      final c = WalletController(
        api: WalletApi(
          ApiCoreConfig(baseUrl: 'https://wallet.test'),
          client: client,
        ),
        getAccessToken: () => 'token',
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.dark,
          home: AddBankAccountScreen(walletController: c),
        ),
      );
      await tester.pumpAndSettle();
      expect(
        find.text('Banks could not be loaded. Please try again.'),
        findsOneWidget,
      );
      expect(find.text('Select your bank'), findsNothing);
      await tester.enterText(find.byType(TextField), '1234567890');
      await tester.pump();
      expect(
        tester
            .widget<FilledButton>(
              find.widgetWithText(FilledButton, 'Verify account'),
            )
            .onPressed,
        isNull,
      );
      unavailable = false;
      await tester.tap(find.byTooltip('Try again'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Choose bank'));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.widgetWithText(TextField, 'Search banks'),
        'Server',
      );
      await tester.pump();
      expect(find.text('Other Bank'), findsNothing);
      expect(find.text('Access Bank'), findsNothing);
      await tester.tap(find.text('Server Bank'));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('Verify account'));
      await tester.tap(find.text('Verify account'));
      await tester.pumpAndSettle();
      expect(resolvedCode, '000123');
      expect(find.text('Save account'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      client.close();
    },
  );
}
