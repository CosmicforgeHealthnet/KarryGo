import '../../../core/network/friendly_messages.dart';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import 'withdrawal_attempt_store.dart';

class FinanceWorkspaceController extends ChangeNotifier {
  FinanceWorkspaceController(this.app, {WithdrawalAttemptStore? attempts})
    : attempts = attempts ?? MemoryWithdrawalAttemptStore();
  final VendorApp app;
  final WithdrawalAttemptStore attempts;
  JsonObject summary = {};
  List<JsonObject> transactions = [], banks = [], withdrawals = [];
  final Map<String, String> errors = {};
  final Set<String> restricted = {}, loading = {}, fresh = {};
  bool busy = false,
      disposed = false,
      initialized = false,
      withdrawDenied = false,
      journalBlocked = false;
  JsonObject? pending, lastWithdrawal;
  String? commandError;
  bool get unknown => pending != null;
  List<JsonObject> get activeBanks => banks
      .where(
        (b) =>
            const {'active', 'verified'}.contains(
              b['status']?.toString().toLowerCase(),
            ) &&
            b['external_ref'] is String &&
            (b['external_ref'] as String).isNotEmpty,
      )
      .toList();
  bool get canWithdraw =>
      initialized &&
      !busy &&
      !unknown &&
      !journalBlocked &&
      !withdrawDenied &&
      fresh.containsAll(['summary', 'banks']) &&
      summary['currency'] is String &&
      summary['available_kobo'] is int &&
      (summary['available_kobo'] as int) > 0 &&
      activeBanks.isNotEmpty;
  void _notify() {
    if (!disposed) notifyListeners();
  }

  Future<void> initialize() async {
    try {
      pending = await attempts.read();
      if (pending != null &&
          (pending!['key'] is! String ||
              pending!['body'] is! Map ||
              pending!['vendor_id'] != app.scope.vendorId)) {
        throw const FormatException('Incomplete withdrawal recovery record');
      }
    } catch (_) {
      journalBlocked = true;
      errors['recovery'] =
          'Your saved withdrawal request could not be read. New withdrawals are paused until it can be recovered.';
    }
    if (pending != null && !journalBlocked) {
      final body = jsonObject(pending!['body']);
      if ((pending!['key'] as String).isEmpty ||
          body.length != 4 ||
          body['amount_kobo'] is! int ||
          (body['amount_kobo'] as int) <= 0 ||
          !['bank_account_id', 'currency', 'step_up_grant_id'].every(
            (key) => body[key] is String && (body[key] as String).isNotEmpty,
          )) {
        journalBlocked = true;
        errors['recovery'] =
            'Your saved withdrawal request is incomplete. New withdrawals are paused until it can be recovered.';
      }
    }
    initialized = true;
    _notify();
    await load();
  }

  Future<void> _read(
    String key,
    Future<Object?> Function() request,
    void Function(Object?) accept,
  ) async {
    if (loading.contains(key) || disposed) return;
    loading.add(key);
    fresh.remove(key);
    errors.remove(key);
    restricted.remove(key);
    _notify();
    try {
      final value = await request();
      if (disposed) return;
      accept(value);
      fresh.add(key);
    } catch (e) {
      errors[key] = message(e);
      if (e is ApiException && e.statusCode == 403) {
        restricted.add(key);
        switch (key) {
          case 'summary':
            summary = {};
          case 'banks':
            banks = [];
          case 'transactions':
            transactions = [];
          case 'withdrawals':
            withdrawals = [];
        }
      }
    } finally {
      loading.remove(key);
      _notify();
    }
  }

  Future<void> load() async {
    if (busy) return;
    await Future.wait([
      _read(
        'summary',
        () async => (await app.financeApi.getFinanceOverview()).data,
        (v) => summary = jsonObject(v),
      ),
      _read(
        'transactions',
        () async => (await app.financeApi.getFinanceTransactions(
          query: {'limit': '100'},
        )).data,
        (v) => transactions = (v as List).map(jsonObject).toList(),
      ),
      _read(
        'banks',
        () async => (await app.financeApi.getFinanceBankAccounts()).data,
        (v) => banks = (v as List).map(jsonObject).toList(),
      ),
      _read(
        'withdrawals',
        () async => (await app.financeApi.getFinanceWithdrawals()).data,
        (v) => withdrawals = (v as List).map(jsonObject).toList(),
      ),
    ]);
  }

  Future<JsonObject> readWithdrawal(String reference) async =>
      (await app.financeApi.getFinanceWithdrawalsReference(
        reference: reference,
      )).object;
  Future<bool> submit({
    required String bankAccountId,
    required int amount,
    required String currency,
    required String grantId,
  }) async {
    if (!canWithdraw ||
        currency != summary['currency'] ||
        amount <= 0 ||
        amount > (summary['available_kobo'] as int) ||
        !activeBanks.any((b) => b['external_ref'] == bankAccountId)) {
      return false;
    }
    busy = true;
    commandError = null;
    final command = VendorCommand.create();
    pending = {
      'vendor_id': app.scope.vendorId,
      'key': command.idempotencyKey,
      'body': {
        'bank_account_id': bankAccountId,
        'amount_kobo': amount,
        'currency': currency,
        'step_up_grant_id': grantId,
      },
    };
    try {
      await attempts.save(pending!);
    } catch (_) {
      pending = null;
      busy = false;
      commandError =
          'Could not save withdrawal recovery information. No request was sent.';
      _notify();
      return false;
    }
    return _send(previouslyUnknown: false);
  }

  Future<void> clearConfirmedRecovery() async {
    if (busy || lastWithdrawal?['reference'] is! String) return;
    busy = true;
    _notify();
    try {
      await attempts.clear();
      pending = null;
      journalBlocked = false;
      commandError = null;
      errors.remove('recovery');
    } catch (_) {
      commandError =
          'Your confirmed request is safe, but its saved recovery record could not be cleared. Try again.';
    } finally {
      busy = false;
      _notify();
    }
  }

  Future<bool> retryOriginal() async {
    if (busy || pending == null || journalBlocked) return false;
    return _send(previouslyUnknown: true);
  }

  Future<bool> _send({required bool previouslyUnknown}) async {
    busy = true;
    commandError = null;
    lastWithdrawal = null;
    _notify();
    var saved = false;
    try {
      final response = await app.financeApi.request(
        'POST',
        '/api/v1/vendor/finance/withdrawals',
        body: VendorPayload(jsonObject(pending!['body'])),
        command: VendorCommand(idempotencyKey: pending!['key'] as String),
        retryAuthentication: false,
      );
      lastWithdrawal = response.object;
      if (lastWithdrawal!['reference'] is! String ||
          (lastWithdrawal!['reference'] as String).isEmpty) {
        throw const FormatException('Incomplete withdrawal response');
      }
      saved = true;
      await attempts.clear();
      pending = null;
    } catch (e) {
      if (saved) {
        commandError =
            'The request was accepted, but recovery information could not be cleared. New withdrawals are paused until the saved recovery record can be cleared.';
        journalBlocked = true;
      } else {
        commandError = message(e);
        final uncertain =
            previouslyUnknown ||
            (e is! ApiException && e is! VendorContractException) ||
            (e is ApiException && e.statusCode == null) ||
            (e is ApiException && (e.statusCode ?? 0) >= 500) ||
            (e is ApiException &&
                e.statusCode == 409 &&
                serverMessageOf(e).contains('idempotency'));
        if (!uncertain) {
          try {
            await attempts.clear();
            pending = null;
          } catch (_) {
            journalBlocked = true;
          }
        }
        if (e is ApiException &&
            e.statusCode == 403 &&
            !serverMessageOf(e).contains('fresh withdrawal step-up')) {
          withdrawDenied = true;
        }
      }
    } finally {
      busy = false;
      _notify();
    }
    await load();
    return saved;
  }

  static String message(Object error) => error is ApiException
      ? error.message
      : error is VendorContractException
      ? 'This request could not be prepared. Update the app and try again.'
      : error is FormatException || error is TypeError
      ? 'The server returned incomplete finance information. Please try again.'
      : 'Could not reach the server. Check your connection and try again.';
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
