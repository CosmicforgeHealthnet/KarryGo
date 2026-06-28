import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_withdrawal_confirm_screen.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Step 1 — enter the withdrawal amount on a custom keypad (Figma 2281).
class ProviderWithdrawalFormScreen extends StatefulWidget {
  const ProviderWithdrawalFormScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderWithdrawalFormScreen> createState() => _ProviderWithdrawalFormScreenState();
}

class _ProviderWithdrawalFormScreenState extends State<ProviderWithdrawalFormScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.startNewWithdrawal());
  }

  void _next() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderWithdrawalConfirmScreen(controller: widget.controller),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: c,
        builder: (context, _) {
          return SafeArea(
            bottom: false,
            child: Column(
              children: [
                // Light-green wash behind the header + balance card.
                Container(
                  color: kProviderGreenPale,
                  child: Column(
                    children: [
                      const WalletFlowAppBar(title: 'Withdrawal Form'),
                      const SizedBox(height: 8),
                      Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 20),
                        child: _AvailableBalanceCard(naira: c.balance.availableNaira, loading: c.loading),
                      ),
                      const SizedBox(height: 20),
                    ],
                  ),
                ),
                const SizedBox(height: 24),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: _AmountField(
                    amountDigits: c.amountInput,
                    exceeds: c.exceedsBalance,
                  ),
                ),
                if (c.exceedsBalance)
                  const Padding(
                    padding: EdgeInsets.only(top: 10),
                    child: Text(
                      'Amount exceeds your available balance.',
                      style: TextStyle(color: kProviderRejectText, fontSize: 12.5),
                    ),
                  ),
                const Spacer(),
                WalletKeypad(onDigit: c.appendDigit, onDelete: c.deleteDigit),
                const SizedBox(height: 12),
                Padding(
                  padding: EdgeInsets.fromLTRB(20, 0, 20, 16 + MediaQuery.of(context).padding.bottom),
                  child: WalletPrimaryButton(
                    label: 'Withdraw Now',
                    onPressed: c.canSubmitAmount ? _next : null,
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _AvailableBalanceCard extends StatelessWidget {
  const _AvailableBalanceCard({required this.naira, required this.loading});
  final double naira;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(20, 18, 20, 18),
      decoration: BoxDecoration(
        gradient: kProviderBalanceGradient,
        borderRadius: BorderRadius.circular(16),
        boxShadow: [
          BoxShadow(
            color: kProviderGreen.withValues(alpha: 0.30),
            blurRadius: 20,
            offset: const Offset(0, 8),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Available Balance',
            style: TextStyle(color: Colors.white, fontSize: 13.5, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          Text(
            loading ? '₦ ...' : '₦ ${formatNaira(naira)}',
            style: const TextStyle(
              color: Colors.white,
              fontSize: 28,
              fontWeight: FontWeight.w800,
              letterSpacing: 0.5,
            ),
          ),
        ],
      ),
    );
  }
}

class _AmountField extends StatelessWidget {
  const _AmountField({required this.amountDigits, required this.exceeds});
  final String amountDigits;
  final bool exceeds;

  @override
  Widget build(BuildContext context) {
    final display = amountDigits.isEmpty ? '0' : groupThousands(amountDigits);
    return Container(
      height: 60,
      padding: const EdgeInsets.symmetric(horizontal: 18),
      decoration: BoxDecoration(
        color: const Color(0xFFF1F2F4),
        borderRadius: BorderRadius.circular(14),
        border: exceeds ? Border.all(color: kProviderRejectText, width: 1.2) : null,
      ),
      child: Row(
        children: [
          Text(
            '₦',
            style: TextStyle(color: kProviderMuted.withValues(alpha: 0.7), fontSize: 20, fontWeight: FontWeight.w700),
          ),
          Expanded(
            child: Text(
              display,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: amountDigits.isEmpty ? kProviderMuted : kProviderGreen,
                fontSize: 24,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          const SizedBox(width: 20),
        ],
      ),
    );
  }
}
