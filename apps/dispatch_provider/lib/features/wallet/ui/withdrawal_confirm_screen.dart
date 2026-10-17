import 'wallet_widgets.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../state/wallet_controller.dart';
import 'add_bank_account_screen.dart';
import 'transaction_auth_screen.dart';

class WithdrawalConfirmScreen extends StatefulWidget {
  const WithdrawalConfirmScreen({
    super.key,
    required this.walletController,
    required this.amount,
    this.bankAccountId,
  });

  final WalletController walletController;
  final double amount;
  final String? bankAccountId;

  @override
  State<WithdrawalConfirmScreen> createState() =>
      _WithdrawalConfirmScreenState();
}

class _WithdrawalConfirmScreenState extends State<WithdrawalConfirmScreen> {
  WalletController get _controller => widget.walletController;

  _BankRowData? get _bank {
    final real = widget.bankAccountId == null
        ? _controller.defaultAccount
        : _controller.accounts
              .where((a) => a.id == widget.bankAccountId)
              .firstOrNull;
    if (real == null) return null;
    return _BankRowData(
      bankName: real.bankName,
      accountNumber: _maskAccount(real.accountNumber),
      accountId: real.id,
    );
  }

  static String _maskAccount(String raw) {
    final trimmed = raw.replaceAll(' ', '');
    if (trimmed.length <= 4) return trimmed;
    return '**** ${trimmed.substring(trimmed.length - 4)}';
  }

  Future<void> _goToAddBankAccount() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => AddBankAccountScreen(walletController: _controller),
      ),
    );
    if (mounted) setState(() {});
  }

  void _onConfirm() {
    final bank = _bank;
    if (bank == null) return;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => TransactionAuthScreen(
          walletController: _controller,
          amount: widget.amount,
          bankAccountId: bank.accountId,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: _controller,
    builder: (context, _) {
      final bank = _bank;
      final account = _controller.accounts
          .where((a) => a.id == bank?.accountId)
          .firstOrNull;
      return AccountPage(
        title: 'Review withdrawal',
        symbol: JourneySymbol.transfer,
        description:
            'Check the amount and receiving account before continuing.',
        bottom: FilledButton(
          onPressed: bank == null ? null : _onConfirm,
          child: const Text('Continue to PIN'),
        ),
        children: [
          DeliveryPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Text('Amount requested'),
                const SizedBox(height: 12),
                Text(
                  walletAmount(
                    (widget.amount * 100).round(),
                    account?.currency ?? 'NGN',
                  ),
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 20),
                const Divider(height: 1),
                if (bank != null) ...[
                  AccountValue(label: 'Bank', value: bank.bankName),
                  AccountValue(
                    label: 'Account holder',
                    value: account?.accountName ?? '',
                  ),
                  AccountValue(
                    label: 'Account number',
                    value: bank.accountNumber,
                  ),
                ],
              ],
            ),
          ),
          if (bank == null) ...[
            const SizedBox(height: 20),
            OutlinedButton.icon(
              onPressed: _goToAddBankAccount,
              icon: const Icon(Icons.add_rounded),
              label: const Text('Add bank account'),
            ),
          ],
        ],
      );
    },
  );
}

class _BankRowData {
  const _BankRowData({
    required this.bankName,
    required this.accountNumber,
    required this.accountId,
  });

  final String bankName;
  final String accountNumber;
  final String accountId;
}
