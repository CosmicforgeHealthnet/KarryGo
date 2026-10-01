import 'package:flutter/material.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../mock/wallet_tab_mock.dart';
import '../state/wallet_controller.dart';
import 'add_bank_account_screen.dart';
import 'transaction_auth_screen.dart';

class WithdrawalConfirmScreen extends StatefulWidget {
  const WithdrawalConfirmScreen({
    super.key,
    required this.walletController,
    required this.amount,
  });

  final WalletController walletController;
  final double amount;

  @override
  State<WithdrawalConfirmScreen> createState() =>
      _WithdrawalConfirmScreenState();
}

class _WithdrawalConfirmScreenState extends State<WithdrawalConfirmScreen> {
  WalletController get _controller => widget.walletController;

  double get _fee => 0.0;

  double get _netAmount => widget.amount - _fee;

  _BankRowData? get _bank {
    if (AppModeConfig.instance.isMock) {
      final list = WalletTabMock.bankAccounts;
      if (list.isEmpty) return null;
      final b = list.firstWhere(
        (a) => a.isDefault,
        orElse: () => list.first,
      );
      return _BankRowData(
        bankName: b.bankName,
        accountNumber: b.accountNumber,
        accountId: b.id,
      );
    }

    final real = _controller.bankAccount;
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
  Widget build(BuildContext context) {
    final colors = context.palette;

    return ListenableBuilder(
      listenable: _controller,
      builder: (context, _) {
        final bank = _bank;

        return Scaffold(
          backgroundColor: colors.background,
          body: SafeArea(
            child: Column(
              children: [
                _buildTopBar(context),
                Expanded(
                  child: SingleChildScrollView(
                    padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _buildAmountBlock(),
                        const SizedBox(height: 32),
                        if (bank == null)
                          _AddBankPrompt(onTap: _goToAddBankAccount)
                        else
                          _DetailsCard(
                            bank: bank,
                            fee: _fee,
                            net: _netAmount,
                          ),
                      ],
                    ),
                  ),
                ),
                _buildConfirmButton(enabled: bank != null),
              ],
            ),
          ),
        );
      },
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Review withdrawal',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  // ── Amount block ─────────────────────────────────────────────────

  Widget _buildAmountBlock() {
    final colors = context.palette;
    return Column(
      children: [
        Text(
          'You are withdrawing',
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w500,
            color: colors.textMuted,
          ),
        ),
        const SizedBox(height: 12),
        FittedBox(
          fit: BoxFit.scaleDown,
          child: Text(
            _formatNaira(widget.amount),
            style: TextStyle(
              fontSize: 44,
              fontWeight: FontWeight.w800,
              color: colors.brandTeal500,
              letterSpacing: -1.4,
              height: 1.0,
            ),
          ),
        ),
      ],
    );
  }

  // ── Bottom button ─────────────────────────────────────────────────

  Widget _buildConfirmButton({required bool enabled}) {
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
        child: SizedBox(
          width: double.infinity,
          height: 56,
          child: FilledButton(
            onPressed: enabled ? _onConfirm : null,
            style: FilledButton.styleFrom(
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: const Text('Confirm withdrawal'),
          ),
        ),
      ),
    );
  }

  static String _formatNaira(double value) {
    final parts = value.toStringAsFixed(2).split('.');
    final whole = parts[0];
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return '₦$buffer.${parts[1]}';
  }
}

// ── Details card ───────────────────────────────────────────────────

class _DetailsCard extends StatelessWidget {
  const _DetailsCard({
    required this.bank,
    required this.fee,
    required this.net,
  });

  final _BankRowData bank;
  final double fee;
  final double net;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: Column(
        children: [
          _BankRow(bank: bank),
          const _CardDivider(),
          _ValueRow(
            label: 'Fee',
            value: _formatNaira(fee),
            valueColor: colors.text,
          ),
          const _CardDivider(),
          _ValueRow(
            label: 'You will receive',
            value: _formatNaira(net),
            valueColor: colors.brandTeal500,
          ),
        ],
      ),
    );
  }

  static String _formatNaira(double value) {
    final parts = value.toStringAsFixed(2).split('.');
    final whole = parts[0];
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return '₦$buffer.${parts[1]}';
  }
}

class _CardDivider extends StatelessWidget {
  const _CardDivider();

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Divider(
      height: 1,
      thickness: 1,
      color: colors.divider,
      indent: 16,
      endIndent: 16,
    );
  }
}

// ── Bank row ───────────────────────────────────────────────────────

class _BankRow extends StatelessWidget {
  const _BankRow({required this.bank});
  final _BankRowData bank;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Row(
        children: [
          Container(
            width: 42,
            height: 42,
            decoration: BoxDecoration(
              color: colors.navy10,
              shape: BoxShape.circle,
            ),
            child: Center(
              child: Icon(
                Icons.account_balance_rounded,
                size: 20,
                color: colors.text,
              ),
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  bank.bankName,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                    height: 1.2,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  bank.accountNumber,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: colors.textMuted,
                    letterSpacing: 0.4,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ── Value row ─────────────────────────────────────────────────────

class _ValueRow extends StatelessWidget {
  const _ValueRow({
    required this.label,
    required this.value,
    required this.valueColor,
  });

  final String label;
  final String value;
  final Color valueColor;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
      child: Row(
        children: [
          Expanded(
            child: Text(
              label,
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: colors.textMuted,
              ),
            ),
          ),
          Text(
            value,
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w700,
              color: valueColor,
              letterSpacing: -0.2,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Add-bank prompt ───────────────────────────────────────────────

class _AddBankPrompt extends StatelessWidget {
  const _AddBankPrompt({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(20),
        decoration: BoxDecoration(
          color: colors.teal10,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: colors.brandTeal500, width: 1.2),
        ),
        child: Row(
          children: [
            Icon(
              Icons.add_circle_outline_rounded,
              size: 22,
              color: colors.brandTeal500,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                'Add a bank account to receive this withdrawal',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: colors.text,
                  height: 1.4,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Internal row data ─────────────────────────────────────────────

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