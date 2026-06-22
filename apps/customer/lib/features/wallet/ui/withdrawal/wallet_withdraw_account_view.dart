import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/wallet_models.dart';
import '../../state/wallet_withdrawal_controller.dart';
import '../widgets/wallet_flow_scaffold.dart';

/// Select the bank account to withdraw to (mockup #9).
class WalletWithdrawAccountView extends StatelessWidget {
  const WalletWithdrawAccountView({super.key, required this.controller});

  final WalletWithdrawalController controller;

  @override
  Widget build(BuildContext context) {
    return WalletFlowScaffold(
      title: 'Withdrawal',
      onBack: controller.back,
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
        children: [
          _AmountBanner(amount: controller.amountDisplay),
          const SizedBox(height: 20),
          const WalletSectionLabel('Select Credit Account'),
          const SizedBox(height: 12),
          ...controller.bankAccounts.map(
            (account) => Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: _AccountTile(
                account: account,
                selected: controller.selectedAccount?.id == account.id,
                onTap: () => controller.selectAccount(account),
              ),
            ),
          ),
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'Continue',
        onPressed:
            controller.selectedAccount == null ? null : controller.confirmAccount,
      ),
    );
  }
}

class _AmountBanner extends StatelessWidget {
  const _AmountBanner({required this.amount});
  final String amount;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          colors: [CustomerFigmaColors.primary, CustomerFigmaColors.darkGreen],
        ),
        borderRadius: BorderRadius.circular(18),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Withdrawal Amount',
            style: TextStyle(color: Colors.white70, fontSize: 12),
          ),
          const SizedBox(height: 6),
          Text(
            amount,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 28,
              fontWeight: FontWeight.w900,
            ),
          ),
        ],
      ),
    );
  }
}

class _AccountTile extends StatelessWidget {
  const _AccountTile({
    required this.account,
    required this.selected,
    required this.onTap,
  });

  final BankAccount account;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected
                ? CustomerFigmaColors.primary
                : CustomerFigmaColors.border,
            width: selected ? 1.6 : 1,
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 42,
              height: 42,
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(10),
              ),
              child: const Icon(
                Icons.account_balance_rounded,
                color: CustomerFigmaColors.primary,
                size: 20,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    account.bankName,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    '${account.maskedNumber} • ${account.accountName}',
                    style: const TextStyle(
                      color: CustomerFigmaColors.muted,
                      fontSize: 12,
                    ),
                  ),
                ],
              ),
            ),
            Icon(
              selected
                  ? Icons.radio_button_checked_rounded
                  : Icons.radio_button_unchecked_rounded,
              color: selected
                  ? CustomerFigmaColors.primary
                  : CustomerFigmaColors.border,
              size: 22,
            ),
          ],
        ),
      ),
    );
  }
}
