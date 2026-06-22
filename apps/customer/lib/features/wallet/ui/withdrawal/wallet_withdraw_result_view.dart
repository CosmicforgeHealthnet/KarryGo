import 'package:flutter/material.dart';

import '../widgets/wallet_success_view.dart';

/// Withdrawal submitted success screen (mockup #11).
class WalletWithdrawResultView extends StatelessWidget {
  const WalletWithdrawResultView({super.key, required this.onGoToHistory});

  final VoidCallback onGoToHistory;

  @override
  Widget build(BuildContext context) {
    return WalletSuccessView(
      title: 'Withdrawal Submitted',
      message:
          'Your withdrawal request was submitted successfully. We will notify '
          'you once it has been processed.',
      primaryLabel: 'Go to Transaction History',
      onPrimary: onGoToHistory,
    );
  }
}
