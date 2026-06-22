import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/wallet_models.dart';
import '../../state/wallet_withdrawal_controller.dart';
import '../widgets/wallet_amount_keypad.dart';
import '../widgets/wallet_flow_scaffold.dart';

/// Withdrawal amount entry with a custom numeric keypad (mockup #8).
class WalletWithdrawAmountView extends StatelessWidget {
  const WalletWithdrawAmountView({super.key, required this.controller});

  final WalletWithdrawalController controller;

  @override
  Widget build(BuildContext context) {
    return WalletFlowScaffold(
      title: 'Withdrawal',
      body: Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
        child: Column(
          children: [
            const SizedBox(height: 16),
            const Text(
              'Enter Amount',
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 13,
              ),
            ),
            const SizedBox(height: 12),
            FittedBox(
              fit: BoxFit.scaleDown,
              child: Text(
                controller.amountDisplay,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 48,
                  fontWeight: FontWeight.w900,
                  letterSpacing: -1.5,
                ),
              ),
            ),
            const SizedBox(height: 10),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(99),
              ),
              child: Text(
                'Available: ${formatKobo(controller.availableKobo)}',
                style: const TextStyle(
                  color: CustomerFigmaColors.primary,
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
            if (controller.amountError != null) ...[
              const SizedBox(height: 10),
              Text(
                controller.amountError!,
                style: const TextStyle(
                  color: Color(0xFFC0392B),
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
            const SizedBox(height: 8),
            // Keypad fills the lower portion of the screen (mockup #8).
            Expanded(
              child: WalletAmountKeypad(
                expand: true,
                onDigit: controller.appendAmount,
                onBackspace: controller.backspaceAmountKey,
              ),
            ),
            const SizedBox(height: 12),
            FigmaPrimaryButton(
              label: 'Continue',
              onPressed: controller.confirmAmount,
            ),
          ],
        ),
      ),
    );
  }
}
