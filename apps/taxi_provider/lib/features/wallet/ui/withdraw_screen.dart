import 'dart:math';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/format/money_format.dart';
import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/wallet_api.dart';

/// Withdraw to a payout account, authorised with the transaction PIN (set
/// up here the first time). Pops `true` once the withdrawal is accepted.
class WithdrawScreen extends StatefulWidget {
  const WithdrawScreen({
    super.key,
    required this.api,
    required this.availableKobo,
    required this.accounts,
    this.referral = false,
  });

  final WalletApi api;

  /// Pays out referral earnings; [availableKobo] is then the referral balance.
  final bool referral;
  final int availableKobo;
  final List<BankAccount> accounts;

  @override
  State<WithdrawScreen> createState() => _WithdrawScreenState();
}

class _WithdrawScreenState extends State<WithdrawScreen> {
  final _amount = TextEditingController();
  late BankAccount? _account = widget.accounts.isEmpty
      ? null
      : widget.accounts.firstWhere(
          (a) => a.isDefault,
          orElse: () => widget.accounts.first,
        );
  bool _busy = false;
  String? _error;

  /// One key per withdrawal attempt on this screen: a retry after a lost
  /// response reuses it, so the payout cannot happen twice.
  final _idempotencyKey = List.generate(
    16,
    (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0'),
  ).join();

  @override
  void dispose() {
    _amount.dispose();
    super.dispose();
  }

  int get _amountKobo =>
      ((double.tryParse(_amount.text.replaceAll(',', '')) ?? 0) * 100).round();

  String? get _amountProblem {
    final kobo = _amountKobo;
    if (kobo <= 0) return null;
    if (kobo > widget.availableKobo) {
      return 'That is more than your available balance.';
    }
    return null;
  }

  Future<void> _submit() async {
    final account = _account;
    if (account == null || _amountKobo <= 0 || _amountProblem != null) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final hasPin = await widget.api.hasPin();
      if (!mounted) return;
      final String? pin;
      if (hasPin) {
        pin = await askPin(context, title: 'Enter your transaction PIN');
      } else {
        pin = await _createPin();
      }
      if (pin == null) return;
      await widget.api.withdraw(
        bankAccountId: account.id,
        amountKobo: _amountKobo,
        pin: pin,
        idempotencyKey: _idempotencyKey,
        referral: widget.referral,
      );
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (c) => AlertDialog(
          icon: const Icon(Icons.check_circle_rounded, size: 40),
          title: const Text('Withdrawal requested'),
          content: Text(
            '${formatNaira(_amountKobo)} is on its way to ${account.masked}. '
            'Bank transfers usually arrive within minutes.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(c),
              child: const Text('Done'),
            ),
          ],
        ),
      );
      if (mounted) Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<String?> _createPin() async {
    final first = await askPin(
      context,
      title: 'Create a transaction PIN',
      message: 'You will use these 4 digits to approve every withdrawal.',
    );
    if (first == null || !mounted) return null;
    final second = await askPin(context, title: 'Enter the PIN again');
    if (second == null) return null;
    if (first != second) {
      if (mounted) {
        setState(() => _error = 'The two PINs did not match. Try again.');
      }
      return null;
    }
    await widget.api.setPin(first);
    return first;
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return KarryamPage(
      title: widget.referral ? 'Cash out referrals' : 'Withdraw',
      subtitle: widget.referral
          ? 'Referral earnings: ${formatNaira(widget.availableKobo)}'
          : 'Available: ${formatNaira(widget.availableKobo)}',
      bottom: KarryamButton(
        label: 'Withdraw',
        busy: _busy,
        onPressed:
            _account == null || _amountKobo <= 0 || _amountProblem != null
            ? null
            : _submit,
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          TextField(
            controller: _amount,
            autofocus: true,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            inputFormatters: [
              FilteringTextInputFormatter.allow(RegExp(r'[0-9.]')),
            ],
            onChanged: (_) => setState(() {}),
            style: const TextStyle(fontSize: 28, fontWeight: FontWeight.w700),
            decoration: InputDecoration(
              prefixText: '₦ ',
              hintText: '0.00',
              errorText: _amountProblem,
              filled: true,
              fillColor: colors.card,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          const KarryamSectionTitle('Pay to'),
          if (widget.accounts.isEmpty)
            Text(
              'Add a bank account first.',
              style: TextStyle(color: colors.textMuted),
            )
          else
            RadioGroup<BankAccount>(
              groupValue: _account,
              onChanged: (a) => setState(() => _account = a),
              child: Column(
                children: [
                  for (final account in widget.accounts)
                    RadioListTile<BankAccount>(
                      value: account,
                      contentPadding: EdgeInsets.zero,
                      title: Text(account.masked),
                      subtitle: Text(account.accountName),
                    ),
                ],
              ),
            ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: TextStyle(color: colors.danger)),
          ],
        ],
      ),
    );
  }
}

/// Asks for a 4-digit PIN; returns null if cancelled.
Future<String?> askPin(
  BuildContext context, {
  required String title,
  String? message,
}) => showDialog<String>(
  context: context,
  builder: (_) => _PinDialog(title: title, message: message),
);

/// Owns its text field so the field outlives the dialog's closing animation
/// (disposing it when the dialog returns crashes that animation).
class _PinDialog extends StatefulWidget {
  const _PinDialog({required this.title, this.message});
  final String title;
  final String? message;

  @override
  State<_PinDialog> createState() => _PinDialogState();
}

class _PinDialogState extends State<_PinDialog> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.title),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (widget.message != null) ...[
            Text(widget.message!),
            const SizedBox(height: 12),
          ],
          TextField(
            controller: _controller,
            autofocus: true,
            obscureText: true,
            maxLength: 4,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            textAlign: TextAlign.center,
            style: const TextStyle(fontSize: 24, letterSpacing: 12),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: () {
            if (_controller.text.length == 4) {
              Navigator.pop(context, _controller.text);
            }
          },
          child: const Text('Continue'),
        ),
      ],
    );
  }
}
