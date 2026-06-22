import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';

class HaulingPaymentView extends StatelessWidget {
  const HaulingPaymentView({super.key, required this.controller});

  final HaulingBookingController controller;

  HaulingBookingState get _state => controller.state;

  @override
  Widget build(BuildContext context) {
    final fare = _state.fareEstimate;
    final fareKobo = fare?.fareEstimateKobo ?? 0;
    final fareNaira = fareKobo / 100;
    final balance = _state.walletBalanceKobo ?? 0;
    final balanceNaira = balance / 100;
    final hasSufficientBalance = balance >= fareKobo;
    final selectedMethod = _state.paymentMethod;

    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back, color: CustomerFigmaColors.text),
          onPressed: controller.backToDetails,
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
                    'Total',
                    style: TextStyle(color: Colors.white, fontSize: 14, fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    '₦${fareNaira.toStringAsFixed(2)}',
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

            // Payment note
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
              decoration: BoxDecoration(
                color: const Color(0xFFE8F5EE),
                borderRadius: BorderRadius.circular(10),
              ),
              child: Row(
                children: const [
                  Icon(Icons.info_outline, color: CustomerFigmaColors.primary, size: 16),
                  SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Payment validates your trip booking.',
                      style: TextStyle(color: CustomerFigmaColors.darkGreen, fontSize: 12),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 20),

            const Text(
              'Select Payment Method',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontWeight: FontWeight.w700,
                fontSize: 15,
              ),
            ),
            const SizedBox(height: 10),

            // Wallet option
            _PaymentMethodRow(
              selected: selectedMethod == 'wallet',
              onTap: () => controller.setPaymentMethod('wallet'),
              leading: const Icon(Icons.account_balance_wallet_outlined, color: CustomerFigmaColors.primary, size: 22),
              title: 'Wallet Balance',
              subtitle: hasSufficientBalance
                  ? '₦${balanceNaira.toStringAsFixed(2)} available'
                  : '₦${balanceNaira.toStringAsFixed(2)} (insufficient)',
              subtitleColor: hasSufficientBalance ? CustomerFigmaColors.primary : Colors.red,
            ),
            const SizedBox(height: 8),

            // Direct transfer option
            _PaymentMethodRow(
              selected: selectedMethod == 'paystack',
              onTap: () => controller.setPaymentMethod('paystack'),
              leading: const Icon(Icons.credit_card_outlined, color: CustomerFigmaColors.primary, size: 22),
              title: 'Direct Transfer',
              subtitle: 'Pay via Paystack',
            ),

            // Insufficient balance warning
            if (selectedMethod == 'wallet' && !hasSufficientBalance) ...[
              const SizedBox(height: 10),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: Colors.red[50],
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: Colors.red[200]!),
                ),
                child: Text(
                  'Your wallet balance is insufficient. '
                  'You need ₦${((fareKobo - balance) / 100).toStringAsFixed(2)} more. '
                  'Please top up or choose direct transfer.',
                  style: const TextStyle(color: Colors.red, fontSize: 12),
                ),
              ),
            ],

            // Paystack provider cards
            if (selectedMethod == 'paystack') ...[
              const SizedBox(height: 16),
              const Text(
                'Select Payment Provider',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontWeight: FontWeight.w700,
                  fontSize: 15,
                ),
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  Expanded(
                    child: _ProviderCard(
                      label: 'Paystack',
                      icon: Icons.payment_rounded,
                      color: const Color(0xFF0BA4DB),
                      onTap: () => _launchPaystack(context),
                    ),
                  ),
                ],
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
              label: 'Confirm',
              isLoading: _state.isLoading,
              onPressed: _canConfirm(selectedMethod, hasSufficientBalance)
                  ? controller.confirmPayment
                  : null,
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }

  bool _canConfirm(String? method, bool hasSufficientBalance) {
    if (method == null) return false;
    if (method == 'wallet' && !hasSufficientBalance) return false;
    return true;
  }

  Future<void> _launchPaystack(BuildContext context) async {
    const url = 'https://paystack.com';
    final uri = Uri.parse(url);
    if (await canLaunchUrl(uri)) {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
    }
  }
}

class _PaymentMethodRow extends StatelessWidget {
  const _PaymentMethodRow({
    required this.selected,
    required this.onTap,
    required this.leading,
    required this.title,
    this.subtitle,
    this.subtitleColor,
  });

  final bool selected;
  final VoidCallback onTap;
  final Widget leading;
  final String title;
  final String? subtitle;
  final Color? subtitleColor;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: selected ? CustomerFigmaColors.primaryTint : Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.border,
            width: selected ? 1.5 : 1,
          ),
        ),
        child: Row(
          children: [
            leading,
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontWeight: FontWeight.w600,
                      fontSize: 14,
                    ),
                  ),
                  if (subtitle != null) ...[
                    const SizedBox(height: 2),
                    Text(
                      subtitle!,
                      style: TextStyle(
                        color: subtitleColor ?? CustomerFigmaColors.muted,
                        fontSize: 12,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            Radio<bool>(
              value: true,
              groupValue: selected ? true : null,
              onChanged: (_) => onTap(),
              activeColor: CustomerFigmaColors.primary,
              materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
            ),
          ],
        ),
      ),
    );
  }
}

class _ProviderCard extends StatelessWidget {
  const _ProviderCard({
    required this.label,
    required this.icon,
    required this.color,
    required this.onTap,
  });

  final String label;
  final IconData icon;
  final Color color;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: CustomerFigmaColors.border),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, color: color, size: 24),
            const SizedBox(width: 8),
            Text(
              label,
              style: TextStyle(
                color: color,
                fontWeight: FontWeight.w700,
                fontSize: 14,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
