import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../auth/models/vendor_session.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/finance_workspace_controller.dart';
import 'finance_design.dart';

class WithdrawalRequestScreen extends StatefulWidget {
  const WithdrawalRequestScreen({
    super.key,
    required this.runtime,
    required this.controller,
  });
  final VendorRuntime runtime;
  final FinanceWorkspaceController controller;
  @override
  State<WithdrawalRequestScreen> createState() =>
      _WithdrawalRequestScreenState();
}

class _WithdrawalRequestScreenState extends State<WithdrawalRequestScreen> {
  late final _amount = TextEditingController(
    text: widget.runtime.mode == VendorDataMode.mock ? '10000.00' : '',
  );
  final _code = TextEditingController();
  JsonObject? _bank;
  VendorChallenge? _challenge;
  DateTime? _expires;
  Timer? _timer;
  bool _starting = false, _submitting = false, _verify = false;
  bool get _busy => _starting || _submitting;
  String? _error;
  int? _lockedAmount;
  String? _lockedCurrency;
  String? _staffId, _vendorId;
  FinanceWorkspaceController get c => widget.controller;
  int get _remaining => _expires == null
      ? 0
      : (_expires!.difference(DateTime.now()).inMilliseconds / 1000)
            .ceil()
            .clamp(0, 86400);
  bool get _valid =>
      c.canWithdraw &&
      !_busy &&
      _bank != null &&
      c.activeBanks.any(
        (bank) => bank['external_ref'] == _bank!['external_ref'],
      ) &&
      financeMinor(_amount.text) != null &&
      financeMinor(_amount.text)! > 0 &&
      financeMinor(_amount.text)! <= (c.summary['available_kobo'] as int);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_load());
    });
  }

  Future<void> _load() async {
    if (!c.initialized) {
      await c.initialize();
    } else {
      await c.load();
    }
    if (mounted) {
      setState(() {
        if (c.activeBanks.length == 1 ||
            widget.runtime.mode == VendorDataMode.mock) {
          _bank = c.activeBanks.firstOrNull;
        }
      });
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    _amount.dispose();
    _code.clear();
    _code.dispose();
    super.dispose();
  }

  Future<void> _selectBank() async {
    final value = await financeSheet<JsonObject>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const FinanceTitle(
            'Payout account',
            'Choose an active account linked to your business.',
          ),
          for (final bank in c.activeBanks)
            Padding(
              padding: const EdgeInsets.only(bottom: 12),
              child: ListTile(
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(20),
                ),
                tileColor: Colors.white.forSurface(context),
                leading: Icon(
                  Icons.account_balance_outlined,
                  color: KarryamColors.tealInk.forInk(context),
                ),
                title: Text(bank['resolved_account_name'].toString()),
                subtitle: Text(bank['masked_account'].toString()),
                trailing: _bank?['external_ref'] == bank['external_ref']
                    ? Icon(
                        Icons.check_rounded,
                        color: KarryamColors.tealInk.forInk(context),
                      )
                    : null,
                onTap: () => Navigator.pop(context, bank),
              ),
            ),
        ],
      ),
    );
    if (mounted && value != null) setState(() => _bank = value);
  }

  Future<void> _review() async {
    if (!_valid) return;
    FocusScope.of(context).unfocus();
    final amount = financeMinor(_amount.text)!;
    final currency = c.summary['currency'] as String;
    final bank = _bank!;
    final accepted = await financeSheet<bool>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const FinanceTitle(
            'Check your transfer.',
            'Confirm the amount and destination before verification.',
          ),
          FinanceCard(
            color: KarryamColors.mint.forInk(context),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('YOU ARE REQUESTING'),
                const SizedBox(height: 10),
                Text(
                  dashboardMoney(amount, currency),
                  style: const TextStyle(
                    fontSize: 28,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                const SizedBox(height: 18),
                FinanceFact('To account', bank['resolved_account_name']),
                FinanceFact('Account number', bank['masked_account']),
              ],
            ),
          ),
          const SizedBox(height: 20),
          const Text(
            'A verification code is required to authorise this withdrawal. An accepted request may still be processing.',
          ),
          const SizedBox(height: 24),
        ],
      ),
      footer: FinanceButton(
        'Verify and withdraw',
        icon: Icons.shield_outlined,
        onPressed: () => Navigator.pop(context, true),
      ),
    );
    if (accepted != true || !mounted || !_valid) return;
    setState(() {
      _verify = true;
      _lockedAmount = amount;
      _lockedCurrency = currency;
    });
    await _startCode();
  }

  Future<void> _startCode() async {
    if (_busy) return;
    setState(() {
      _starting = true;
      _error = null;
      _challenge = null;
      _code.clear();
    });
    _timer?.cancel();
    try {
      final challenge = await widget.runtime.auth.startStepUp(
        'finance.withdrawal',
      );
      final session = widget.runtime.auth.session;
      if (!mounted) return;
      if (session == null || challenge.expiresIn <= 0) {
        throw StateError('Verification session is unavailable');
      }
      setState(() {
        _challenge = challenge;
        _expires = DateTime.now().add(Duration(seconds: challenge.expiresIn));
        _staffId = session.staffId;
        _vendorId = session.vendorId;
        if (widget.runtime.mode == VendorDataMode.mock) _code.text = '123456';
      });
      _timer = Timer.periodic(const Duration(seconds: 1), (_) {
        if (!mounted || _remaining == 0) _timer?.cancel();
        if (mounted) setState(() {});
      });
    } catch (e) {
      if (mounted) {
        setState(() => _error = FinanceWorkspaceController.message(e));
      }
    } finally {
      if (mounted) setState(() => _starting = false);
    }
  }

  Future<void> _submit() async {
    if (_busy ||
        _challenge == null ||
        _remaining == 0 ||
        _code.text.length != 6) {
      return;
    }
    FocusScope.of(context).unfocus();
    setState(() {
      _submitting = true;
      _error = null;
    });
    try {
      final session = widget.runtime.auth.session;
      if (session == null ||
          session.staffId != _staffId ||
          session.vendorId != _vendorId) {
        throw StateError('Verification session changed');
      }
      final grant = await widget.runtime.auth.verifyStepUp(
        purpose: 'finance.withdrawal',
        challengeId: _challenge!.challengeId,
        otp: _code.text,
      );
      _code.clear();
      if (grant.expiresIn <= 0) throw StateError('Verification grant expired');
      final accepted = await c.submit(
        bankAccountId: _bank!['external_ref'] as String,
        amount: _lockedAmount!,
        currency: _lockedCurrency!,
        grantId: grant.grantId,
      );
      if (!mounted) return;
      _timer?.cancel();
      setState(() {
        _submitting = false;
        _challenge = null;
      });
      if (accepted) {
        await showOrderFeedback(
          context,
          title: 'Withdrawal requested',
          message: financeStatusCopy(c.lastWithdrawal?['status']),
          success: c.lastWithdrawal?['status'] != 'requires_otp',
          warning: c.lastWithdrawal?['status'] == 'requires_otp',
        );
        if (mounted) Navigator.pop(context, c.lastWithdrawal?['reference']);
      } else if (c.unknown || c.journalBlocked) {
        await showOrderFeedback(
          context,
          title: 'Your request needs checking',
          message:
              'We could not confirm the outcome. New withdrawals are paused to protect your funds. Finance will keep this request so you can retry the original safely.',
          warning: true,
        );
        if (mounted) Navigator.pop(context);
      } else {
        setState(() {
          _challenge = null;
          _error =
              c.commandError ??
              'Your balance or payout account changed. Go back and check your withdrawal details.';
        });
        await showOrderFeedback(
          context,
          title: 'Withdrawal not accepted',
          message: _error!,
        );
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _submitting = false;
          _error = FinanceWorkspaceController.message(e);
        });
        await showOrderFeedback(
          context,
          title: 'Verification could not be completed',
          message: _error!,
        );
      }
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !_busy,
      child: Scaffold(
        appBar: AppBar(
          title: Text(_verify ? 'Authorise withdrawal' : 'Withdraw funds'),
          backgroundColor: KarryamColors.canvas.forSurface(context),
        ),
        bottomNavigationBar: SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
            child: FinanceButton(
              _verify ? 'Authorise withdrawal' : 'Review withdrawal',
              busy: _submitting,
              icon: _verify
                  ? Icons.lock_outline_rounded
                  : Icons.arrow_forward_rounded,
              onPressed: _verify
                  ? (_challenge != null &&
                            _remaining > 0 &&
                            _code.text.length == 6 &&
                            !_busy &&
                            c.canWithdraw
                        ? _submit
                        : null)
                  : (_valid ? _review : null),
            ),
          ),
        ),
        body: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            AnimatedSwitcher(
              duration: MediaQuery.disableAnimationsOf(context)
                  ? Duration.zero
                  : const Duration(milliseconds: 280),
              child: _verify ? _security() : _details(),
            ),
          ],
        ),
      ),
    ),
  );
  Widget _details() {
    final amount = financeMinor(_amount.text);
    final available = c.summary['available_kobo'];
    return Column(
      key: const ValueKey('withdrawal-details'),
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const FinanceTitle(
          'Withdraw funds',
          'Choose an amount and payout account.',
        ),
        if (c.loading.contains('summary') || c.loading.contains('banks'))
          const PremiumSkeleton(rows: 2),
        for (final key in ['summary', 'banks'])
          if (c.errors[key] != null)
            FinanceNotice(c.errors[key]!, error: !c.restricted.contains(key)),
        FinanceCard(
          color: KarryamColors.mint.forInk(context),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'AVAILABLE TO WITHDRAW',
                style: TextStyle(
                  fontSize: 11,
                  letterSpacing: 1.2,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                dashboardMoney(available, c.summary['currency']),
                style: const TextStyle(
                  fontSize: 25,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 24),
        TextField(
          controller: _amount,
          onChanged: (_) => setState(() {}),
          keyboardType: const TextInputType.numberWithOptions(decimal: true),
          inputFormatters: [
            FilteringTextInputFormatter.allow(RegExp(r'[0-9.]')),
          ],
          decoration: InputDecoration(
            labelText: 'Amount (${c.summary['currency'] ?? 'wallet currency'})',
            hintText: '0.00',
            errorText:
                _amount.text.isNotEmpty && (amount == null || amount <= 0)
                ? 'Enter a valid positive amount with up to two decimal places.'
                : amount != null && available is int && amount > available
                ? 'This exceeds your available balance.'
                : null,
          ),
        ),
        const SizedBox(height: 20),
        Material(
          color: Colors.white.forSurface(context),
          borderRadius: BorderRadius.circular(22),
          clipBehavior: Clip.antiAlias,
          child: InkWell(
            onTap: _busy || c.activeBanks.isEmpty ? null : _selectBank,
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Row(
                children: [
                  Icon(
                    Icons.account_balance_outlined,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'PAYOUT ACCOUNT',
                          style: TextStyle(
                            fontSize: 10,
                            letterSpacing: 1,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(height: 7),
                        Text(
                          _bank?['resolved_account_name']?.toString() ??
                              'Select an active account',
                          style: const TextStyle(fontWeight: FontWeight.w600),
                        ),
                        if (_bank != null)
                          Text(_bank!['masked_account'].toString()),
                      ],
                    ),
                  ),
                  const Icon(Icons.expand_more_rounded),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(height: 18),
        if (c.activeBanks.isEmpty && !c.loading.contains('banks'))
          const FinanceNotice(
            'No active payout account is available. Check your bank details in Business setup.',
          ),

        const SizedBox(height: 28),
      ],
    );
  }

  Widget _security() => Column(
    key: const ValueKey('withdrawal-security'),
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      const FinanceTitle(
        'Verify withdrawal',
        'Enter the code sent to your sign-in contact.',
      ),
      FinanceCard(
        color: KarryamColors.mint.forInk(context),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.shield_outlined,
                  color: KarryamColors.tealInk.forInk(context),
                  size: 32,
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(
                    dashboardMoney(_lockedAmount, _lockedCurrency),
                    style: const TextStyle(
                      fontSize: 24,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 14),
            Text(_bank?['resolved_account_name']?.toString() ?? ''),
            Text(_bank?['masked_account']?.toString() ?? ''),
          ],
        ),
      ),
      const SizedBox(height: 24),
      if (_error != null) FinanceNotice(_error!, error: true),
      TextField(
        controller: _code,
        enabled: !_busy && _challenge != null && _remaining > 0,
        onChanged: (_) => setState(() {}),
        keyboardType: TextInputType.number,
        autofillHints: const [AutofillHints.oneTimeCode],
        inputFormatters: [
          FilteringTextInputFormatter.digitsOnly,
          LengthLimitingTextInputFormatter(6),
        ],
        obscureText: true,
        enableSuggestions: false,
        autocorrect: false,
        decoration: const InputDecoration(
          labelText: 'Verification code',
          hintText: '6 digits',
        ),
      ),
      const SizedBox(height: 10),
      if (_challenge != null)
        Text(
          _remaining > 0
              ? 'Code expires in ${_remaining ~/ 60}:${(_remaining % 60).toString().padLeft(2, '0')}'
              : 'This code has expired. Request a new code.',
        ),
      const SizedBox(height: 24),

      const SizedBox(height: 12),
      TextButton(
        onPressed: _busy ? null : _startCode,
        child: _starting
            ? const SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Text('Request a new code'),
      ),
      TextButton(
        onPressed: _busy
            ? null
            : () {
                _timer?.cancel();
                _code.clear();
                setState(() {
                  _verify = false;
                  _challenge = null;
                  _error = null;
                });
                unawaited(_load());
              },
        child: const Text('Change withdrawal details'),
      ),
    ],
  );
}
