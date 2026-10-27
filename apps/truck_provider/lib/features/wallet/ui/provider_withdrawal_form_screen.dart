import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_withdrawal_confirm_screen.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Step 1 — enter the withdrawal amount on a custom keypad (Figma 2281).
class ProviderWithdrawalFormScreen extends StatefulWidget {
  const ProviderWithdrawalFormScreen({super.key, required this.controller, this.referral = false});

  final ProviderWithdrawalController controller;

  /// Withdraws referral earnings instead of job earnings.
  final bool referral;

  @override
  State<ProviderWithdrawalFormScreen> createState() => _ProviderWithdrawalFormScreenState();
}

class _ProviderWithdrawalFormScreenState extends State<ProviderWithdrawalFormScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.startNewWithdrawal(referral: widget.referral));
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
      body: AnimatedBuilder(
        animation: c,
        builder: (context, _) {
          return SafeArea(
            bottom: false,
            child: Column(
              children: [
                WalletFlowAppBar(
                  title: c.isReferral ? 'Cash out referrals' : 'Withdrawal Form',
                  subtitle: c.isReferral ? 'Move referral earnings to your bank' : 'Move earnings to your bank',
                ),
                const SizedBox(height: 12),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: _AvailableBalanceCard(
                    naira: c.withdrawableKobo / 100,
                    loading: c.loading,
                    label: c.isReferral ? 'Referral earnings' : 'Available Balance',
                  ),
                ),
                const SizedBox(height: 22),
                Padding(
                  padding: EdgeInsets.symmetric(horizontal: 20),
                  child: Align(alignment: Alignment.centerLeft, child: KEyebrow('How much?')),
                ),
                const SizedBox(height: 10),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: _AmountField(
                    amountDigits: c.amountInput,
                    exceeds: c.exceedsBalance,
                  ),
                ),
                if (c.exceedsBalance)
                  Padding(
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
  const _AvailableBalanceCard({required this.naira, required this.loading, required this.label});
  final double naira;
  final bool loading;
  final String label;

  @override
  Widget build(BuildContext context) {
    return KMoneyCard(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 18),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: const TextStyle(fontSize: 13, color: Color(0xCCFFFFFF))),
          const SizedBox(height: 6),
          Text(
            loading ? '₦ ...' : '₦ ${formatNaira(naira)}',
            style: const TextStyle(fontSize: 28, fontWeight: FontWeight.w700, letterSpacing: -0.6),
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
    return AnimatedContainer(
      duration: const Duration(milliseconds: 180),
      height: 72,
      padding: const EdgeInsets.symmetric(horizontal: 18),
      decoration: BoxDecoration(
        color: kCard,
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: exceeds ? kProviderRejectText : kTeal, width: 1.6),
      ),
      child: Row(
        children: [
          Text('₦', style: TextStyle(color: kProviderMuted, fontSize: 22, fontWeight: FontWeight.w600)),
          Expanded(
            child: FittedBox(
              fit: BoxFit.scaleDown,
              child: Text(
                display,
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: amountDigits.isEmpty ? kProviderGreenSoft : kProviderText,
                  fontSize: 32,
                  fontWeight: FontWeight.w700,
                  letterSpacing: -0.8,
                ),
              ),
            ),
          ),
          const SizedBox(width: 22),
        ],
      ),
    );
  }
}
