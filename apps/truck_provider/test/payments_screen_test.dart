import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/wallet/data/provider_wallet_api.dart';
import 'package:truck_provider/features/wallet/state/provider_withdrawal_controller.dart';
import 'package:truck_provider/features/wallet/ui/provider_add_bank_account_screen.dart';
import 'package:truck_provider/features/wallet/ui/provider_payments_screen.dart';

Map<String, dynamic> _account(String id, String number, {bool isDefault = false}) => {
      'id': id,
      'bank_code': '035',
      'bank_name': 'Wema Bank',
      'account_number': number,
      'account_name': 'Firepemi Adewale',
      'status': 'active',
      'is_default': isDefault,
    };

class _Harness {
  _Harness({List<Map<String, dynamic>>? accounts})
      : accounts = accounts ??
            [
              _account('ba1', '0450908723'),
              _account('ba2', '0450908724', isDefault: true),
            ] {
    final mock = MockClient((req) async {
      paths.add('${req.method} ${req.url.path}');
      final path = req.url.path;
      if (path.endsWith('/provider/earnings')) {
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'available_kobo': 20400000, 'escrow_kobo': 0, 'pending_kobo': 0, 'currency': 'NGN'},
          }),
          200,
        );
      }
      if (path.endsWith('/provider/pin/status')) {
        return http.Response(jsonEncode({'success': true, 'data': {'pin_set': true}}), 200);
      }
      if (path.endsWith('/provider/bank-accounts') && req.method == 'GET') {
        return http.Response(jsonEncode({'success': true, 'data': {'bank_accounts': this.accounts}}), 200);
      }
      if (path.endsWith('/provider/bank-accounts/resolve') && req.method == 'POST') {
        final body = Map<String, dynamic>.from(jsonDecode(req.body) as Map);
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'account_number': body['account_number'], 'account_name': 'FIREPEMI ADEWALE'},
          }),
          200,
        );
      }
      if (path.endsWith('/provider/bank-accounts') && req.method == 'POST') {
        final created = _account('ba-new', '0111111111', isDefault: this.accounts.isEmpty);
        this.accounts.add(created);
        return http.Response(jsonEncode({'success': true, 'data': created}), 201);
      }
      final deleteMatch = RegExp(r'/provider/bank-accounts/([^/]+)$').firstMatch(path);
      if (req.method == 'DELETE' && deleteMatch != null) {
        final id = deleteMatch.group(1);
        final wasDefault =
            this.accounts.any((a) => a['id'] == id && a['is_default'] == true);
        this.accounts.removeWhere((a) => a['id'] == id);
        if (wasDefault && this.accounts.isNotEmpty) {
          this.accounts.first['is_default'] = true;
        }
        return http.Response(jsonEncode({'success': true, 'data': {'deleted': true}}), 200);
      }
      final defaultMatch = RegExp(r'/provider/bank-accounts/([^/]+)/default$').firstMatch(path);
      if (req.method == 'PUT' && defaultMatch != null) {
        final id = defaultMatch.group(1);
        Map<String, dynamic>? target;
        for (final a in this.accounts) {
          a['is_default'] = a['id'] == id;
          if (a['id'] == id) target = a;
        }
        return http.Response(jsonEncode({'success': true, 'data': target ?? {}}), 200);
      }
      return http.Response(jsonEncode({'success': true, 'data': {}}), 200);
    });
    controller = ProviderWithdrawalController(
      api: ProviderWalletApi(
        config: const ApiCoreConfig(baseUrl: 'http://test/api/v1/payment-wallet'),
        client: mock,
      ),
      accessToken: () => 'token',
    );
  }

  final List<Map<String, dynamic>> accounts;
  final List<String> paths = [];
  late final ProviderWithdrawalController controller;
}

Future<void> _pump(WidgetTester tester, Widget screen) async {
  await tester.binding.setSurfaceSize(const Size(430, 932));
  addTearDown(() => tester.binding.setSurfaceSize(null));
  await tester.pumpWidget(MaterialApp(home: screen));
  await tester.pumpAndSettle();
}

void main() {
  test('withdrawal preselects the persisted default account, not the first', () async {
    final h = _Harness();
    await h.controller.startNewWithdrawal();
    expect(h.controller.selectedAccount?.id, 'ba2');
    expect(h.controller.selectedAccount?.isDefault, isTrue);
  });

  testWidgets('payments screen lists linked accounts and the default section', (tester) async {
    final h = _Harness();
    await _pump(tester, ProviderPaymentsScreen(controller: h.controller));

    expect(find.text('Payments & Withdrawals'), findsOneWidget);
    expect(find.text('Linked Accounts'), findsOneWidget);
    expect(find.text('Add Bank Account'), findsOneWidget);
    expect(find.text('Set Default withdrawal Account'), findsOneWidget);
    // Each account appears twice: linked list + default selector.
    expect(find.text('0450908723'), findsNWidgets(2));
    expect(find.text('0450908724'), findsNWidgets(2));
    expect(h.paths, contains('GET /api/v1/payment-wallet/provider/bank-accounts'));
  });

  testWidgets('deleting an account confirms then issues DELETE and refreshes', (tester) async {
    final h = _Harness();
    await _pump(tester, ProviderPaymentsScreen(controller: h.controller));

    await tester.tap(find.byIcon(Icons.delete_outline_rounded).first);
    await tester.pumpAndSettle();
    expect(find.text('Remove Bank Account?'), findsOneWidget);

    await tester.tap(find.text('Remove'));
    await tester.pumpAndSettle();

    expect(h.paths, contains('DELETE /api/v1/payment-wallet/provider/bank-accounts/ba1'));
    expect(find.text('0450908723'), findsNothing);
    expect(find.text('0450908724'), findsNWidgets(2));
  });

  testWidgets('tapping a non-default account issues PUT default', (tester) async {
    final h = _Harness();
    await _pump(tester, ProviderPaymentsScreen(controller: h.controller));

    // In the default selector section, tap the non-default account (ba1).
    await tester.tap(find.text('0450908723').last);
    await tester.pumpAndSettle();

    expect(h.paths, contains('PUT /api/v1/payment-wallet/provider/bank-accounts/ba1/default'));
    expect(h.controller.bankAccounts.firstWhere((a) => a.id == 'ba1').isDefault, isTrue);
    expect(h.controller.bankAccounts.firstWhere((a) => a.id == 'ba2').isDefault, isFalse);
  });

  testWidgets('add account auto-resolves the name and enables Save', (tester) async {
    final h = _Harness();
    await _pump(tester, ProviderAddBankAccountScreen(controller: h.controller));

    expect(find.text('Enter New Bank Details'), findsOneWidget);
    expect(find.text('Enter Acct Name'), findsOneWidget);

    await tester.tap(find.text('Select Bank'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Access Bank'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), '0450908723');
    await tester.pumpAndSettle();

    expect(h.paths, contains('POST /api/v1/payment-wallet/provider/bank-accounts/resolve'));
    expect(find.text('FIREPEMI ADEWALE'), findsOneWidget);

    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(h.paths, contains('POST /api/v1/payment-wallet/provider/bank-accounts'));
  });
}
