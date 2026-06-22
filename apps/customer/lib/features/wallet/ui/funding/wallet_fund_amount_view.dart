import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/wallet_models.dart';
import '../../state/wallet_funding_controller.dart';
import '../widgets/wallet_flow_scaffold.dart';

/// Fund Wallet entry screen (mockup #12): choose a payment provider, enter an
/// amount, and continue to the Paystack checkout.
class WalletFundAmountView extends StatefulWidget {
  const WalletFundAmountView({super.key, required this.controller});

  final WalletFundingController controller;

  @override
  State<WalletFundAmountView> createState() => _WalletFundAmountViewState();
}

class _WalletFundAmountViewState extends State<WalletFundAmountView> {
  final _amountCtrl = TextEditingController();

  static const _presets = [1000, 2000, 5000, 10000, 20000, 50000];

  @override
  void dispose() {
    _amountCtrl.dispose();
    super.dispose();
  }

  void _setAmount(num naira) {
    _amountCtrl.text = naira.toStringAsFixed(0);
    widget.controller.setAmountNaira(naira);
  }

  @override
  Widget build(BuildContext context) {
    final controller = widget.controller;
    final loading = controller.status == WalletFundingStatus.initializing;

    return WalletFlowScaffold(
      title: 'Fund Wallet',
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
        children: [
          const WalletSectionLabel('Select Payment Provider'),
          const SizedBox(height: 12),
          ...WalletPaymentProvider.values.map(
            (p) => Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: _ProviderTile(
                provider: p,
                selected: controller.provider == p,
                onTap: () => controller.selectProvider(p),
              ),
            ),
          ),
          const SizedBox(height: 16),
          const WalletSectionLabel('Amount'),
          const SizedBox(height: 12),
          _AmountField(
            controller: _amountCtrl,
            onChanged: (v) =>
                controller.setAmountNaira(double.tryParse(v) ?? 0),
          ),
          const SizedBox(height: 14),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: _presets
                .map((p) => _PresetChip(
                      amount: p,
                      onTap: () => _setAmount(p),
                    ))
                .toList(),
          ),
          if (controller.status == WalletFundingStatus.error &&
              controller.error != null) ...[
            const SizedBox(height: 16),
            _ErrorBanner(message: controller.error!),
          ],
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'Continue',
        isLoading: loading,
        onPressed: controller.canSubmit && !loading
            ? controller.startCheckout
            : null,
      ),
    );
  }
}

class _ProviderTile extends StatelessWidget {
  const _ProviderTile({
    required this.provider,
    required this.selected,
    required this.onTap,
  });

  final WalletPaymentProvider provider;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final icon = provider == WalletPaymentProvider.paystackCard
        ? Icons.credit_card_rounded
        : Icons.account_balance_rounded;

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
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Icon(icon, color: CustomerFigmaColors.primary, size: 20),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    provider.label,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    provider.subtitle,
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

class _AmountField extends StatelessWidget {
  const _AmountField({required this.controller, required this.onChanged});

  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      keyboardType: const TextInputType.numberWithOptions(decimal: true),
      inputFormatters: [
        FilteringTextInputFormatter.allow(RegExp(r'[0-9.]')),
      ],
      onChanged: onChanged,
      style: const TextStyle(
        fontSize: 22,
        fontWeight: FontWeight.w800,
        color: CustomerFigmaColors.text,
      ),
      decoration: InputDecoration(
        prefixText: '₦ ',
        prefixStyle: const TextStyle(
          fontSize: 22,
          fontWeight: FontWeight.w800,
          color: CustomerFigmaColors.text,
        ),
        hintText: '0',
        filled: true,
        fillColor: CustomerFigmaColors.field,
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: const BorderSide(color: CustomerFigmaColors.primary),
        ),
      ),
    );
  }
}

class _PresetChip extends StatelessWidget {
  const _PresetChip({required this.amount, required this.onTap});

  final int amount;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        decoration: BoxDecoration(
          color: CustomerFigmaColors.primaryPale,
          borderRadius: BorderRadius.circular(99),
        ),
        child: Text(
          '₦${amount ~/ 1000}k',
          style: const TextStyle(
            color: CustomerFigmaColors.primary,
            fontSize: 13,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }
}

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0xFFFFF1F0),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: const Color(0xFFFFCDD2)),
      ),
      child: Text(
        message,
        style: const TextStyle(color: Color(0xFFC0392B), fontSize: 13),
      ),
    );
  }
}
