import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../wallet/ui/funding/wallet_funding_flow_screen.dart';
import '../../state/hauling_booking_controller.dart';

/// Wallet-only payment step. The wallet is the single payment method; the fare
/// is held when a provider accepts. When the balance is below the fare, the
/// customer is prompted to top up via the existing wallet funding flow.
class HaulingPaymentView extends StatefulWidget {
  const HaulingPaymentView({super.key, required this.controller});

  final HaulingBookingController controller;

  @override
  State<HaulingPaymentView> createState() => _HaulingPaymentViewState();
}

class _HaulingPaymentViewState extends State<HaulingPaymentView> {
  HaulingBookingController get _controller => widget.controller;
  HaulingBookingState get _state => _controller.state;

  Future<void> _openTopUp() async {
    final token = _controller.accessTokenForWallet;
    if (token == null) return;
    final funded = await Navigator.of(context).push<bool>(
      MaterialPageRoute<bool>(
        builder: (_) => WalletFundingFlowScreen(
          walletApi: _controller.walletApi,
          accessToken: token,
          customerEmail: _controller.customerEmail,
        ),
      ),
    );
    if (funded == true) {
      await _controller.refreshWalletBalance();
    }
  }

  @override
  Widget build(BuildContext context) {
    final fare = _state.fareEstimate;
    final fareKobo = fare?.fareEstimateKobo ?? 0;
    final fareNaira = fareKobo / 100;
    final balance = _state.walletBalanceKobo ?? 0;
    final balanceNaira = balance / 100;
    final hasSufficientBalance = balance >= fareKobo;
    final canConfirm = hasSufficientBalance && fareKobo > 0;

    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back, color: CustomerFigmaColors.text),
          onPressed: _controller.backToTierSelection,
        ),
        title: const Text(
          'Payment',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w700,
            fontSize: 16,
          ),
        ),
        centerTitle: true,
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // Fare card
            Container(
              padding: const EdgeInsets.all(20),
              decoration: BoxDecoration(
                gradient: const LinearGradient(
                  colors: [Color(0xFF1F7A4D), Color(0xFF2EA65A)],
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                ),
                borderRadius: BorderRadius.circular(16),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Amount to pay',
                    style: TextStyle(color: Colors.white70, fontSize: 13),
                  ),
                  const SizedBox(height: 4),
                  const Text(
                    'Total (estimate)',
                    style: TextStyle(color: Colors.white, fontSize: 14, fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    fareKobo > 0 ? '₦${fareNaira.toStringAsFixed(2)}' : '—',
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 32,
                      fontWeight: FontWeight.w800,
                      letterSpacing: -0.5,
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),

            // Info note
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
              decoration: BoxDecoration(
                color: const Color(0xFFE8F5EE),
                borderRadius: BorderRadius.circular(10),
              ),
              child: const Row(
                children: [
                  Icon(Icons.info_outline, color: CustomerFigmaColors.primary, size: 16),
                  SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Your wallet is charged only when a driver accepts. '
                      'Final price is confirmed on pickup.',
                      style: TextStyle(color: CustomerFigmaColors.darkGreen, fontSize: 12),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 20),

            const Text(
              'Payment Method',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontWeight: FontWeight.w700,
                fontSize: 15,
              ),
            ),
            const SizedBox(height: 10),

            // Wallet is the only payment method.
            Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: CustomerFigmaColors.primary, width: 1.5),
              ),
              child: Row(
                children: [
                  const Icon(Icons.account_balance_wallet_outlined,
                      color: CustomerFigmaColors.primary, size: 22),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Wallet Balance',
                          style: TextStyle(
                            color: CustomerFigmaColors.text,
                            fontWeight: FontWeight.w600,
                            fontSize: 14,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          hasSufficientBalance
                              ? '₦${balanceNaira.toStringAsFixed(2)} available'
                              : '₦${balanceNaira.toStringAsFixed(2)} (insufficient)',
                          style: TextStyle(
                            color: hasSufficientBalance
                                ? CustomerFigmaColors.primary
                                : Colors.red,
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const Icon(Icons.check_circle, color: CustomerFigmaColors.primary, size: 20),
                ],
              ),
            ),

            const SizedBox(height: 8),
            const Text(
              'No money leaves your wallet until a driver accepts your booking.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
            ),

            // Insufficient-balance block with a top-up call to action.
            if (!hasSufficientBalance && fareKobo > 0) ...[
              const SizedBox(height: 10),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: const Color(0xFFFFEBEE),
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: const Color(0xFFEF9A9A)),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      'Your wallet balance is too low (need '
                      '₦${((fareKobo - balance) / 100).toStringAsFixed(2)} more). '
                      'Top up your wallet to continue.',
                      style: const TextStyle(color: Color(0xFFB71C1C), fontSize: 12),
                    ),
                    const SizedBox(height: 10),
                    OutlinedButton.icon(
                      onPressed: _openTopUp,
                      icon: const Icon(Icons.add, size: 18, color: CustomerFigmaColors.primary),
                      label: const Text(
                        'Top up wallet',
                        style: TextStyle(
                          color: CustomerFigmaColors.primary,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      style: OutlinedButton.styleFrom(
                        side: const BorderSide(color: CustomerFigmaColors.primary),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(10),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],

            const SizedBox(height: 24),

            const Text(
              'By confirming, you agree to our Terms & Conditions and Privacy Policy.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 16),

            if (_state.error != null) ...[
              Text(
                _state.error!,
                style: const TextStyle(color: Colors.red, fontSize: 12),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 8),
            ],

            FigmaPrimaryButton(
              label: 'Confirm & Find Truck',
              isLoading: _state.isLoading,
              onPressed: canConfirm ? _controller.confirmPayment : null,
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}
