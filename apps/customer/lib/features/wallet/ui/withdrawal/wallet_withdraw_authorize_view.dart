import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/wallet_withdrawal_controller.dart';
import '../widgets/wallet_flow_scaffold.dart';
import '../widgets/wallet_pin_dots.dart';

/// Withdrawal authorization PIN entry (mockup #10).
///
/// The PIN is UI-only — it is not validated server-side. Entering all 4 digits
/// triggers the (mocked) submission.
class WalletWithdrawAuthorizeView extends StatelessWidget {
  const WalletWithdrawAuthorizeView({super.key, required this.controller});

  final WalletWithdrawalController controller;

  static const _keys = [
    '1', '2', '3',
    '4', '5', '6',
    '7', '8', '9',
    '', '0', '⌫',
  ];

  @override
  Widget build(BuildContext context) {
    final submitting =
        controller.status == WalletWithdrawalStatus.submitting;

    return WalletFlowScaffold(
      title: 'Authorization',
      onBack: submitting ? () {} : controller.back,
      body: Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
        child: Column(
          children: [
            const SizedBox(height: 16),
            const Text(
              'Enter your PIN',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 18,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 8),
            const Text(
              'Confirm this withdrawal with your transaction PIN.',
              textAlign: TextAlign.center,
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
            const SizedBox(height: 28),
            WalletPinDots(
              length: controller.pin.length,
              count: WalletWithdrawalController.pinLength,
            ),
            const SizedBox(height: 24),
            if (submitting)
              const Padding(
                padding: EdgeInsets.only(top: 8),
                child: CircularProgressIndicator(
                  color: CustomerFigmaColors.primary,
                ),
              ),
            const Spacer(),
            IgnorePointer(
              ignoring: submitting,
              child: GridView.count(
                crossAxisCount: 3,
                shrinkWrap: true,
                physics: const NeverScrollableScrollPhysics(),
                childAspectRatio: 1.9,
                children: _keys.map((key) {
                  if (key.isEmpty) return const SizedBox.shrink();
                  final isBackspace = key == '⌫';
                  return _PinKey(
                    label: key,
                    isBackspace: isBackspace,
                    onTap: () => isBackspace
                        ? controller.backspacePin()
                        : controller.appendPin(key),
                  );
                }).toList(),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PinKey extends StatelessWidget {
  const _PinKey({
    required this.label,
    required this.isBackspace,
    required this.onTap,
  });

  final String label;
  final bool isBackspace;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(16),
        child: Center(
          child: isBackspace
              ? const Icon(Icons.backspace_outlined,
                  color: CustomerFigmaColors.text, size: 24)
              : Text(
                  label,
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 26,
                    fontWeight: FontWeight.w700,
                  ),
                ),
        ),
      ),
    );
  }
}
