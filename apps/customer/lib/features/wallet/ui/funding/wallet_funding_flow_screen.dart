import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../data/wallet_api.dart';
import '../../state/wallet_funding_controller.dart';
import '../widgets/wallet_flow_scaffold.dart';
import 'wallet_checkout_view.dart';
import 'wallet_fund_amount_view.dart';
import 'wallet_funding_result_view.dart';

/// Entry point + router for the Fund Wallet flow (mockup #12 -> Paystack
/// checkout -> success). Returns `true` to the caller if funding succeeded so
/// the wallet screen can refresh.
class WalletFundingFlowScreen extends StatefulWidget {
  const WalletFundingFlowScreen({
    super.key,
    required this.walletApi,
    required this.accessToken,
    required this.customerEmail,
  });

  final WalletApi walletApi;
  final String accessToken;
  final String customerEmail;

  @override
  State<WalletFundingFlowScreen> createState() =>
      _WalletFundingFlowScreenState();
}

class _WalletFundingFlowScreenState extends State<WalletFundingFlowScreen> {
  late final WalletFundingController _controller;

  @override
  void initState() {
    super.initState();
    _controller = WalletFundingController(
      walletApi: widget.walletApi,
      accessToken: widget.accessToken,
      customerEmail: widget.customerEmail,
    );
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
          WalletFundingStatus.enterAmount ||
          WalletFundingStatus.initializing ||
          WalletFundingStatus.error =>
            WalletFundAmountView(controller: _controller),
          WalletFundingStatus.checkout => WalletCheckoutView(
              controller: _controller,
            ),
          WalletFundingStatus.verifying => const _VerifyingView(),
          WalletFundingStatus.success => WalletFundingResultView(
              onDone: () => Navigator.of(context).pop(true),
            ),
        };
      },
    );
  }
}

class _VerifyingView extends StatelessWidget {
  const _VerifyingView();

  @override
  Widget build(BuildContext context) {
    return WalletFlowScaffold(
      title: 'Confirming Payment',
      onBack: () {},
      body: const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircularProgressIndicator(color: CustomerFigmaColors.primary),
            SizedBox(height: 20),
            Text(
              'Confirming your payment…',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 15,
                fontWeight: FontWeight.w700,
              ),
            ),
            SizedBox(height: 6),
            Text(
              'This only takes a moment.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
          ],
        ),
      ),
    );
  }
}
