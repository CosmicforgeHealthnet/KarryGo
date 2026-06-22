import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../widgets/wallet_success_view.dart';

/// Success screen shown after a wallet top-up completes.
class WalletFundingResultView extends StatelessWidget {
  const WalletFundingResultView({super.key, required this.onDone});

  final VoidCallback onDone;

  @override
  Widget build(BuildContext context) {
    return WalletSuccessView(
      title: 'Wallet Funded',
      message:
          'Your payment was received. Your wallet balance will update shortly.',
      primaryLabel: 'Done',
      onPrimary: onDone,
      icon: Icons.account_balance_wallet_rounded,
      iconColor: CustomerFigmaColors.primary,
    );
  }
}
