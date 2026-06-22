import 'package:flutter/material.dart';

import '../../state/wallet_withdrawal_controller.dart';
import 'wallet_withdraw_account_view.dart';
import 'wallet_withdraw_amount_view.dart';
import 'wallet_withdraw_authorize_view.dart';
import 'wallet_withdraw_result_view.dart';

/// Entry point + router for the withdrawal flow (mockup #8 -> #11).
///
/// UI-only: see [WalletWithdrawalController] for the mock/TODO notes. Pops with
/// `true` if a withdrawal was "submitted" so the caller can route to history.
class WalletWithdrawalFlowScreen extends StatefulWidget {
  const WalletWithdrawalFlowScreen({super.key, required this.availableKobo});

  final int availableKobo;

  @override
  State<WalletWithdrawalFlowScreen> createState() =>
      _WalletWithdrawalFlowScreenState();
}

class _WalletWithdrawalFlowScreenState
    extends State<WalletWithdrawalFlowScreen> {
  late final WalletWithdrawalController _controller;

  @override
  void initState() {
    super.initState();
    _controller = WalletWithdrawalController(availableKobo: widget.availableKobo);
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        return switch (_controller.status) {
          WalletWithdrawalStatus.enterAmount =>
            WalletWithdrawAmountView(controller: _controller),
          WalletWithdrawalStatus.selectAccount =>
            WalletWithdrawAccountView(controller: _controller),
          WalletWithdrawalStatus.authorize ||
          WalletWithdrawalStatus.submitting =>
            WalletWithdrawAuthorizeView(controller: _controller),
          WalletWithdrawalStatus.success => WalletWithdrawResultView(
              onGoToHistory: () => Navigator.of(context).pop(true),
            ),
        };
      },
    );
  }
}
