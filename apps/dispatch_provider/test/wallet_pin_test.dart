import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:dispatch_provider/features/wallet/data/wallet_api.dart';
import 'package:dispatch_provider/features/wallet/state/wallet_controller.dart';

void main() {
  WalletController buildController(MockClient client) {
    return WalletController(
      api: WalletApi(
        ApiCoreConfig(baseUrl: 'http://wallet.test/api/v1/payment-wallet'),
        client: client,
      ),
      getAccessToken: () => 'test-token',
    );
  }

  test('loadPinStatus reads pin_set from /provider/pin/status', () async {
    final controller = buildController(
      MockClient((request) async {
        expect(request.url.path, '/api/v1/payment-wallet/provider/pin/status');
        expect(request.headers['Authorization'], 'Bearer test-token');
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'pin_set': true},
          }),
          200,
        );
      }),
    );

    final result = await controller.loadPinStatus();
    expect(result, isA<ApiSuccess<bool>>());
    expect(controller.pinSet, isTrue);
  });

  test('setupPin posts the pin and flips pinSet', () async {
    late Map<String, dynamic> sentBody;
    final controller = buildController(
      MockClient((request) async {
        expect(request.url.path, '/api/v1/payment-wallet/provider/pin');
        expect(request.method, 'POST');
        sentBody = jsonDecode(request.body) as Map<String, dynamic>;
        return http.Response(
          jsonEncode({'success': true, 'data': {}}),
          200,
        );
      }),
    );

    final result = await controller.setupPin('1234');
    expect(result, isA<ApiSuccess<bool>>());
    expect(sentBody['pin'], '1234');
    expect(controller.pinSet, isTrue);
  });

  test('requestWithdrawal sends the pin in the body', () async {
    late Map<String, dynamic> sentBody;
    final controller = buildController(
      MockClient((request) async {
        expect(request.url.path, '/api/v1/payment-wallet/provider/withdrawals');
        sentBody = jsonDecode(request.body) as Map<String, dynamic>;
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {
              'id': 'w-1',
              'reference': 'ref-1',
              'amount_kobo': 50000,
              'currency': 'NGN',
              'status': 'pending',
            },
          }),
          201,
        );
      }),
    );

    final result = await controller.requestWithdrawal(
      bankAccountId: 'acct-1',
      amountKobo: 50000,
      idempotencyKey: 'idem-1',
      pin: '1234',
    );
    expect(result, isA<ApiSuccess>());
    expect(sentBody['pin'], '1234');
    expect(sentBody['bank_account_id'], 'acct-1');
    expect(sentBody['amount_kobo'], 50000);
    expect(sentBody['idempotency_key'], 'idem-1');
  });

  test('requestWithdrawal surfaces invalid_pin error', () async {
    final controller = buildController(
      MockClient((request) async {
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {'code': 'invalid_pin', 'message': 'Incorrect PIN.'},
          }),
          403,
        );
      }),
    );

    final result = await controller.requestWithdrawal(
      bankAccountId: 'acct-1',
      amountKobo: 50000,
      idempotencyKey: 'idem-1',
      pin: '9999',
    );
    result.when(
      success: (_) => fail('expected failure'),
      failure: (err) {
        expect(err.code, 'invalid_pin');
        expect(err.message, 'Incorrect PIN.');
      },
    );
  });
}
