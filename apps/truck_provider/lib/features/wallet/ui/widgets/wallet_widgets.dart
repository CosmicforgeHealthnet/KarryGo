import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';

/// Circular back button + bold title used across the withdrawal flow screens.
class WalletFlowAppBar extends StatelessWidget {
  const WalletFlowAppBar({super.key, required this.title, this.trailing});

  final String title;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
      child: Row(
        children: [
          GestureDetector(
            onTap: () => Navigator.of(context).maybePop(),
            child: Container(
              width: 44,
              height: 44,
              decoration: const BoxDecoration(
                color: Colors.white,
                shape: BoxShape.circle,
                boxShadow: [BoxShadow(color: Color(0x22000000), blurRadius: 10, offset: Offset(0, 3))],
              ),
              child: const Icon(Icons.arrow_back, color: kProviderText, size: 20),
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Text(
              title,
              style: const TextStyle(color: kProviderText, fontSize: 21, fontWeight: FontWeight.w800),
            ),
          ),
          ?trailing,
        ],
      ),
    );
  }
}

/// The solid-green "Amount to pay / Total / ₦X" card (Figma 2282 / 2283).
class WalletAmountToPayCard extends StatelessWidget {
  const WalletAmountToPayCard({super.key, required this.naira});
  final double naira;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(vertical: 20, horizontal: 20),
      decoration: BoxDecoration(
        color: kProviderGreen,
        borderRadius: BorderRadius.circular(14),
      ),
      child: Column(
        children: [
          const Text(
            'Amount to pay',
            style: TextStyle(color: Colors.white, fontSize: 13),
          ),
          const SizedBox(height: 8),
          const Text(
            'Total',
            style: TextStyle(color: Colors.white, fontSize: 13, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 6),
          Text(
            '₦ ${formatNaira(naira)}',
            style: const TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.w800),
          ),
        ],
      ),
    );
  }
}

/// A selectable saved-bank-account row with a leading radio indicator.
class WalletBankAccountTile extends StatelessWidget {
  const WalletBankAccountTile({
    super.key,
    required this.accountName,
    required this.bankName,
    required this.accountNumber,
    required this.selected,
    this.onTap,
  });

  final String accountName;
  final String bankName;
  final String accountNumber;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected ? kProviderGreen : kProviderBorder,
            width: selected ? 1.4 : 1,
          ),
        ),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    accountName,
                    style: const TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    bankName,
                    style: const TextStyle(color: kProviderText, fontSize: 13.5, fontWeight: FontWeight.w600),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    accountNumber,
                    style: const TextStyle(color: kProviderMuted, fontSize: 13),
                  ),
                ],
              ),
            ),
            _RadioDot(selected: selected),
          ],
        ),
      ),
    );
  }
}

class _RadioDot extends StatelessWidget {
  const _RadioDot({required this.selected});
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 22,
      height: 22,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: selected ? kProviderGreen : kProviderBorder, width: 2),
      ),
      child: selected
          ? Center(
              child: Container(
                width: 11,
                height: 11,
                decoration: const BoxDecoration(color: kProviderGreen, shape: BoxShape.circle),
              ),
            )
          : null,
    );
  }
}
