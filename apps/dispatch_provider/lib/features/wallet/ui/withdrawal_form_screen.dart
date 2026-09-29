import 'package:flutter/material.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/numeric_keypad.dart';
import '../mock/wallet_tab_mock.dart';
import '../state/wallet_controller.dart';
import 'withdrawal_confirm_screen.dart';

class WithdrawalFormScreen extends StatefulWidget {
  const WithdrawalFormScreen({super.key, required this.walletController});

  final WalletController walletController;

  @override
  State<WithdrawalFormScreen> createState() => _WithdrawalFormScreenState();
}

class _WithdrawalFormScreenState extends State<WithdrawalFormScreen> {
  String _amount = '';

  WalletController get _controller => widget.walletController;

  double get _availableBalance {
    if (AppModeConfig.instance.isMock) return WalletTabMock.balanceNaira;
    return (_controller.earnings?.availableKobo ?? 0) / 100;
  }

  List<BankAccount> get _accounts {
    if (AppModeConfig.instance.isMock) return WalletTabMock.bankAccounts;
    return const [];
  }

  BankAccount? get _selectedAccount {
    final list = _accounts;
    if (list.isEmpty) return null;
    return list.firstWhere(
      (a) => a.isDefault,
      orElse: () => list.first,
    );
  }

  double get _amountValue => double.tryParse(_amount) ?? 0;

  bool get _isValid => _amountValue > 0 && _amountValue <= _availableBalance;

  bool get _isOverBalance =>
      _amountValue > 0 && _amountValue > _availableBalance;

  void _onKeyTap(String key) {
    setState(() {
      if (key == 'backspace') {
        if (_amount.isNotEmpty) {
          _amount = _amount.substring(0, _amount.length - 1);
        }
      } else if (_amount.length < 9) {
        _amount = (_amount == '0') ? key : _amount + key;
      }
    });
  }

  void _onContinue() {
    if (!_isValid) return;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => WithdrawalConfirmScreen(
          walletController: _controller,
          amount: _amountValue,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(child: _buildBody()),
            NumericKeypad(onKeyTap: _onKeyTap),
            const SizedBox(height: 12),
            _buildBottomButton(),
          ],
        ),
      ),
    );
  }

  // ── Top bar: back · Withdraw funds ────────────────────────────────

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
            'Withdraw funds',
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

  // ── Body ──────────────────────────────────────────────────────────

  Widget _buildBody() {
    final colors = context.palette;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(height: 8),

          // Small muted label
          Text(
            'Enter amount',
            style: TextStyle(
              fontSize: 13.5,
              fontWeight: FontWeight.w500,
              color: colors.textMuted,
            ),
          ),
          const SizedBox(height: 14),

          // Big amount centered
          Center(
            child: Text(
              _amount.isEmpty ? '₦0' : '₦${_formatGrouped(_amountValue)}',
              style: TextStyle(
                fontSize: 40,
                fontWeight: FontWeight.w800,
                color: _isOverBalance ? colors.danger : colors.text,
                letterSpacing: -1.2,
                height: 1.05,
              ),
            ),
          ),
          const SizedBox(height: 12),

          // Available balance
          Center(
            child: Text(
              'Available balance: ${_formatNaira(_availableBalance)}',
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: _isOverBalance ? colors.danger : colors.textMuted,
              ),
            ),
          ),

          const SizedBox(height: 32),

          // "To" label
          Text(
            'To',
            style: TextStyle(
              fontSize: 13.5,
              fontWeight: FontWeight.w500,
              color: colors.textMuted,
            ),
          ),
          const SizedBox(height: 12),

          // Bank account selector
          _buildBankSelector(),
        ],
      ),
    );
  }

  // ── Bank selector row ─────────────────────────────────────────────

  Widget _buildBankSelector() {
    final colors = context.palette;
    final account = _selectedAccount;

    if (account == null) {
      return GestureDetector(
        onTap: () {
          // TODO: navigate to Add bank account
        },
        behavior: HitTestBehavior.opaque,
        child: Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color: colors.surface,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: colors.divider),
          ),
          child: Row(
            children: [
              Icon(
                Icons.add_circle_outline_rounded,
                size: 20,
                color: colors.brandTeal500,
              ),
              const SizedBox(width: 12),
              Text(
                'Add a bank account',
                style: TextStyle(
                  fontSize: 14.5,
                  fontWeight: FontWeight.w600,
                  color: colors.brandTeal500,
                ),
              ),
            ],
          ),
        ),
      );
    }

    return GestureDetector(
      onTap: () {
        // TODO: open bank account picker
      },
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: colors.divider),
        ),
        child: Row(
          children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: colors.navy10,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.account_balance_rounded,
                size: 20,
                color: colors.text,
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    account.bankName,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: colors.text,
                      height: 1.2,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    account.accountNumber,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Bottom button ─────────────────────────────────────────────────

  Widget _buildBottomButton() {
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
        child: SizedBox(
          width: double.infinity,
          height: 56,
          child: FilledButton(
            onPressed: _isValid ? _onContinue : null,
            style: FilledButton.styleFrom(
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: const Text('Continue'),
          ),
        ),
      ),
    );
  }

  // ── Formatting ────────────────────────────────────────────────────

  static String _formatNaira(double value) {
    final parts = value.toStringAsFixed(2).split('.');
    return '₦${_formatGrouped(double.parse(parts[0]))}.${parts[1]}';
  }

  static String _formatGrouped(double v) {
    final whole = v.toStringAsFixed(0);
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }
}