import 'dart:convert';
import 'dart:math';

import 'package:http/http.dart' as http;

import '../app_mode.dart';
import '../../features/missions/mock/provider_operations_mock.dart';
import '../../features/missions/mock/commerce_pickup_mock.dart';

class ModeHttpClient extends http.BaseClient {
  ModeHttpClient({http.Client? inner})
    : _inner = inner ?? http.Client(),
      _mode = AppModeConfig.instance.current;

  final http.Client _inner;
  final _operations = ProviderOperationsMock();
  final _commerce = CommercePickupMock();
  final AppMode _mode;
  final _offlineReplays = <String, Map<String, dynamic>>{};
  String? _pin;
  int _balance = 12450000;
  final _withdrawalReplays = <String, Map<String, dynamic>>{};
  final _withdrawalEntries = <Map<String, dynamic>>[];
  final List<Map<String, dynamic>> _banks = [
    {
      'id': 'demo-bank-1',
      'bank_name': 'Demo Bank',
      'bank_code': '057',
      'account_number': '0000000000',
      'account_name': 'Karryam Demo Rider',
      'currency': 'NGN',
      'is_default': true,
    },
  ];

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (_mode != AppModeConfig.instance.current) {
      throw http.ClientException(
        'Data source changed. Open this screen again.',
      );
    }
    if (_mode == AppMode.live) return _inner.send(request);
    final bytes = await request.finalize().toBytes();
    Map<String, dynamic> body = request is http.MultipartRequest
        ? Map<String, dynamic>.from(request.fields)
        : {};
    if (body['parcel_refs'] is String) {
      try {
        body['parcel_refs'] = jsonDecode(body['parcel_refs'] as String);
      } catch (_) {}
    }
    try {
      body = jsonDecode(utf8.decode(bytes)) as Map<String, dynamic>;
    } catch (_) {}
    final path = request.url.path;
    final method = request.method;
    var status = 200;
    dynamic data;
    String? error;
    final operation =
        _commerce.handle(
          method,
          path,
          body,
          request.headers['Idempotency-Key'],
        ) ??
        _operations.handle(
          method,
          path,
          body,
          request.headers['Idempotency-Key'],
        );
    if (operation != null) {
      status = operation.status;
      data = operation.data;
      error = operation.error;
    } else if (path == '/api/v1/provider/offline-commands' &&
        method == 'POST') {
      final key = request.headers['Idempotency-Key'] ?? '';
      final fingerprint = jsonEncode(body);
      final existing = _offlineReplays[key];
      final kind = body['command_type'];
      final timestamp = DateTime.tryParse(
        body['client_timestamp']?.toString() ?? '',
      );
      if (key.isEmpty ||
          timestamp == null ||
          DateTime.now().toUtc().difference(timestamp) >
              const Duration(hours: 24) ||
          timestamp.isAfter(
            DateTime.now().toUtc().add(const Duration(minutes: 5)),
          )) {
        status = 422;
        error =
            'This update is outside the replay window or has no operation identity.';
      } else if (existing != null && existing['fingerprint'] != fingerprint) {
        status = 409;
        error =
            'This operation identity was already used for a different update.';
      } else if (existing != null) {
        data = existing['data'];
      } else {
        final payload = body['payload'] as Map? ?? {};
        final locationValid =
            ['latitude', 'longitude', 'heading', 'speed', 'accuracy'].every(
              (key) => payload[key] is num && (payload[key] as num).isFinite,
            ) &&
            (payload['latitude'] as num).abs() <= 90 &&
            (payload['longitude'] as num).abs() <= 180 &&
            (payload['heading'] as num) >= 0 &&
            (payload['heading'] as num) <= 360 &&
            (payload['speed'] as num) >= 0 &&
            (payload['accuracy'] as num) >= 0;
        if (kind == 'LOCATION_FIX' && !locationValid) {
          status = 422;
          error = 'The saved location is invalid.';
        } else {
          data = {
            'command_id': key,
            'command_type': kind,
            'server_version': 0,
            'state': kind == 'LOCATION_FIX'
                ? 'ACCEPTED'
                : kind == 'EVIDENCE_REFERENCE'
                ? 'PENDING_EVIDENCE_SYNC'
                : 'REJECTED',
            if (kind != 'LOCATION_FIX')
              'reason_code': kind == 'EVIDENCE_REFERENCE'
                  ? 'MEDIA_AUTHORIZATION_REQUIRED'
                  : 'ONLINE_VERIFICATION_REQUIRED',
          };
          _offlineReplays[key] = {'fingerprint': fingerprint, 'data': data};
        }
      }
    } else if (path.endsWith('/auth/verify')) {
      if (body['otp_code'] != '123456') {
        status = 422;
        error = 'Use 123456 for this demo account.';
      } else {
        data = {
          'provider_id': 'mock-provider',
          'role': 'dispatch_provider',
          'access_token': 'mock-access',
          'refresh_token': 'mock-refresh',
          'token_type': 'Bearer',
          'expires_in_seconds': 3600,
        };
      }
    } else if (path.endsWith('/start') && path.contains('/auth/')) {
      data = {
        'message': 'Demo verification code ready.',
        'expires_in_seconds': 300,
        'debug_otp': '123456',
      };
    } else if (path.endsWith('/auth/phone-change/verify')) {
      if (body['otp_code'] != '123456') {
        status = 422;
        error = 'Use 123456 for the demo.';
      } else {
        data = {
          'message': 'Phone number updated.',
          'phone_number': body['new_phone_number'],
        };
      }
    } else if (path.endsWith('/auth/refresh')) {
      data = {
        'access_token': 'mock-access',
        'refresh_token': 'mock-refresh',
        'token_type': 'Bearer',
        'expires_in_seconds': 3600,
      };
    } else if (path.endsWith('/auth/logout')) {
      data = {'message': 'Signed out.'};
    } else if (path.endsWith('/provider/transactions')) {
      data = {
        'transactions': [
          ..._withdrawalEntries,
          ...List.generate(
            6,
            (i) => {
              'reference': 'DEMO-${1000 + i}',
              'transaction_type': i == 2 ? 'withdrawal' : 'job_payment',
              'side': i == 2 ? 'debit' : 'credit',
              'amount_kobo': (i + 1) * 150000,
              'currency': 'NGN',
              'memo': i == 2 ? 'Bank withdrawal' : 'Delivery earnings',
              'created_at': DateTime.now()
                  .subtract(Duration(days: i))
                  .toUtc()
                  .toIso8601String(),
            },
          ),
        ],
        'page': 1,
        'limit': 20,
        'has_more': false,
      };
    } else if (path.endsWith('/provider/earnings')) {
      data = {
        'available_kobo': _balance,
        'pending_kobo': 180000,
        'currency': 'NGN',
      };
    } else if (path.endsWith('/provider/pin/status')) {
      data = {'pin_set': _pin != null};
    } else if (path.endsWith('/provider/pin')) {
      if (method == 'PUT' && body['current_pin'] != _pin) {
        status = 422;
        error = 'Your current PIN is incorrect.';
      } else if (method == 'POST' && _pin != null) {
        status = 409;
        error = 'A PIN is already set.';
      } else {
        _pin = (body['new_pin'] ?? body['pin']) as String?;
        data = {'message': 'PIN saved.'};
      }
    } else if (path.endsWith('/provider/banks') && method == 'GET') {
      data = {
        'banks': [
          {'name': 'Access Bank', 'code': '044', 'currency': 'NGN'},
          {'name': 'First Bank', 'code': '011', 'currency': 'NGN'},
          {'name': 'Zenith Bank', 'code': '057', 'currency': 'NGN'},
        ],
        'country': 'NG',
        'currency': 'NGN',
      };
    } else if (path.endsWith('/bank-accounts/resolve')) {
      data = {
        'bank_code': body['bank_code'],
        'account_number': body['account_number'],
        'account_name': 'Karryam Demo Rider',
        'bank_name': 'Demo Bank',
        'currency': 'NGN',
      };
    } else if (path.endsWith('/bank-accounts') && method == 'GET') {
      data = {'bank_accounts': _banks};
    } else if (path.endsWith('/bank-accounts') && method == 'POST') {
      final bank = {
        ...body,
        'id': 'demo-bank-${_banks.length + 1}',
        'account_name': 'Karryam Demo Rider',
        'is_default': _banks.isEmpty,
      };
      _banks.add(bank);
      data = bank;
    } else if (path.contains('/bank-accounts/') && method == 'DELETE') {
      _banks.removeWhere((b) => b['id'] == path.split('/').last);
      data = {'message': 'Account removed.'};
    } else if (path.endsWith('/default') && method == 'PUT') {
      final id = path.split('/')[path.split('/').length - 2];
      for (final bank in _banks) {
        bank['is_default'] = bank['id'] == id;
      }
      data = {'message': 'Default account updated.'};
    } else if (path.endsWith('/provider/withdrawals')) {
      final amount = (body['amount_kobo'] as num?)?.toInt() ?? 0;
      final key = body['idempotency_key']?.toString() ?? '';
      if (key.isNotEmpty && _withdrawalReplays.containsKey(key)) {
        final prior = _withdrawalReplays[key]!;
        if (prior['amount_kobo'] != amount ||
            prior['bank_account_id'] != body['bank_account_id']) {
          status = 409;
          error = 'This request reference belongs to a different withdrawal.';
        } else {
          data = prior;
        }
      } else if (_pin == null || body['pin'] != _pin) {
        status = 422;
        error = 'Enter your transaction PIN.';
      } else if (amount <= 0 || amount > _balance) {
        status = 422;
        error = 'Enter an amount within your available balance.';
      } else {
        _balance -= amount;
        data = {
          'id': 'demo-${Random().nextInt(1000000)}',
          'reference': 'DEMO-WITHDRAWAL-${_withdrawalEntries.length + 1}',
          'bank_account_id': body['bank_account_id'],
          'amount_kobo': amount,
          'currency': 'NGN',
          'status': 'pending',
          'created_at': DateTime.now().toUtc().toIso8601String(),
        };
        if (key.isNotEmpty)
          _withdrawalReplays[key] = Map<String, dynamic>.from(data as Map);
        _withdrawalEntries.insert(0, {
          'reference': data['reference'],
          'transaction_type': 'withdrawal',
          'side': 'debit',
          'amount_kobo': amount,
          'currency': 'NGN',
          'memo': 'Withdrawal requested',
          'created_at': data['created_at'],
        });
      }
    } else {
      status = 503;
      error = 'This action is not available in this demo yet.';
    }
    final envelope = error == null
        ? {'success': true, 'data': data}
        : {
            'success': false,
            'error': {'code': 'validation_failed', 'message': error},
          };
    return http.StreamedResponse(
      Stream.value(utf8.encode(jsonEncode(envelope))),
      status,
      headers: {'content-type': 'application/json'},
      request: request,
    );
  }

  @override
  void close() => _inner.close();
}
