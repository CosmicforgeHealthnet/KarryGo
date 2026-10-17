import 'dart:convert';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';
import 'mock_dashboard_data.dart';

class MockFinanceStore {
  MockFinanceStore(this.business, this.scenario)
    : available = scenario == MockDashboardScenario.empty ? 0 : 28465000,
      pending = scenario == MockDashboardScenario.empty ? 0 : 4275000 {
    if (scenario == MockDashboardScenario.empty) return;
    transactions.addAll([
      {
        'reference': 'demo-settlement-1048',
        'transaction_type': 'SETTLEMENT',
        'side': 'CREDIT',
        'amount_kobo': 1850000,
        'currency': 'NGN',
        'memo': 'Order settlement',
        'created_at': '2026-10-08T09:15:00Z',
      },
      {
        'reference': 'demo-settlement-1047',
        'transaction_type': 'SETTLEMENT',
        'side': 'CREDIT',
        'amount_kobo': 920000,
        'currency': 'NGN',
        'memo': 'Order settlement',
        'created_at': '2026-10-08T08:45:00Z',
      },
    ]);
    withdrawals.addAll([
      {
        'id': 'mock-payout-paid',
        'reference': 'mock-wd-paid',
        'bank_account_id': 'mock-bank-account',
        'amount_kobo': 4500000,
        'currency': 'NGN',
        'status': 'paid',
      },
      {
        'id': 'mock-payout-failed',
        'reference': 'mock-wd-failed',
        'bank_account_id': 'mock-bank-account',
        'amount_kobo': 1000000,
        'currency': 'NGN',
        'status': 'failed',
      },
    ]);
  }
  final MockOnboardingStore business;
  final MockDashboardScenario scenario;
  int available, pending;
  int _sequence = 0;
  final List<JsonObject> transactions = [], withdrawals = [];
  final Map<String, ({String fingerprint, JsonObject result})> _commands = {};
  JsonObject _copy(JsonObject value) =>
      jsonObject(jsonDecode(jsonEncode(value)));
  JsonObject? read(String path, Map<String, String> query) {
    if (!path.startsWith('/api/v1/vendor/finance/')) return null;
    if (scenario == MockDashboardScenario.restricted) {
      return {
        '_status': 403,
        '_message': 'You do not have access to Vendor finance.',
      };
    }
    if (scenario == MockDashboardScenario.financeUnavailable) {
      return {
        '_status': 503,
        '_message': 'Finance is temporarily unavailable.',
      };
    }
    if (path.endsWith('/overview')) {
      return {
        'owner_id': business.vendorId,
        'currency': 'NGN',
        'available_kobo': available,
        'pending_kobo': pending,
      };
    }
    if (path.endsWith('/transactions')) {
      final requested = int.tryParse(query['limit'] ?? '') ?? 50;
      final limit = requested <= 0 || requested > 100 ? 50 : requested;
      return {'_list': transactions.take(limit).map(_copy).toList()};
    }
    if (path.endsWith('/bank-accounts')) {
      return {
        '_list': business.bank['status'] == 'VERIFIED'
            ? [
                {
                  'external_ref': 'mock-bank-account',
                  'bank_code': business.bank['bank_code'],
                  'masked_account': business.bank['masked_account'],
                  'resolved_account_name':
                      business.bank['resolved_account_name'],
                  'status': 'active',
                },
              ]
            : [],
      };
    }
    if (path.endsWith('/withdrawals')) {
      return {'_list': withdrawals.take(50).map(_copy).toList()};
    }
    if (path.contains('/withdrawals/')) {
      final value = withdrawals
          .where((w) => w['reference'] == path.split('/').last)
          .firstOrNull;
      return value == null
          ? {'_status': 404, '_message': 'Withdrawal not found.'}
          : _copy(value);
    }
    return null;
  }

  JsonObject request(JsonObject body, Map<String, String> headers) {
    if (scenario == MockDashboardScenario.restricted) {
      return {
        '_status': 403,
        '_message': 'You do not have access to Vendor finance.',
      };
    }
    if (scenario == MockDashboardScenario.financeUnavailable) {
      return {
        '_status': 503,
        '_message': 'Finance is temporarily unavailable.',
      };
    }
    final key = headers.entries
        .where((e) => e.key.toLowerCase() == 'idempotency-key')
        .firstOrNull
        ?.value;
    if (key == null) {
      return {'_status': 400, '_message': 'Idempotency-Key is required.'};
    }
    final fingerprint = jsonEncode(body), saved = _commands[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? _copy(saved.result)
          : {
              '_status': 409,
              '_message':
                  'Withdrawal idempotency key was reused with different details.',
            };
    }
    final amount = body['amount_kobo'];
    if (amount is! int ||
        amount <= 0 ||
        body['bank_account_id'] == null ||
        body['step_up_grant_id'] == null) {
      return {
        '_status': 422,
        '_message':
            'Bank account, positive amount and step-up grant are required.',
      };
    }
    if (!business.consumeWithdrawalGrant(body['step_up_grant_id'].toString())) {
      return {
        '_status': 403,
        '_message': 'A fresh withdrawal step-up grant is required.',
      };
    }
    if (amount < 10000) {
      return {
        '_status': 422,
        '_message': 'Withdrawal amount is below the minimum.',
      };
    }
    if (body['currency'] != 'NGN') {
      return {'_status': 422, '_message': 'This wallet uses NGN.'};
    }
    if (body['bank_account_id'] != 'mock-bank-account' ||
        business.bank['status'] != 'VERIFIED') {
      return {
        '_status': 409,
        '_message': 'The selected bank account is not active.',
      };
    }
    if (amount > available) {
      return {'_status': 409, '_message': 'Insufficient available balance.'};
    }
    available -= amount;
    final value = {
      'id': 'mock-withdrawal-${++_sequence}',
      'reference': 'mock-wd-$_sequence',
      'bank_account_id': body['bank_account_id'],
      'amount_kobo': amount,
      'currency': 'NGN',
      'status': scenario == MockDashboardScenario.payoutVerification
          ? 'requires_otp'
          : 'processing',
    };
    withdrawals.insert(0, value);
    transactions.insert(0, {
      'reference': 'mock-ledger-$_sequence',
      'transaction_type': 'withdrawal_reserved',
      'side': 'DEBIT',
      'amount_kobo': amount,
      'currency': 'NGN',
      'memo': 'Provider withdrawal reserved',
      'created_at': DateTime.now().toUtc().toIso8601String(),
    });
    _commands[key] = (fingerprint: fingerprint, result: _copy(value));
    return _copy(value);
  }
}
