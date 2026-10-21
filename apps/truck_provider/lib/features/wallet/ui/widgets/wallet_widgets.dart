import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';

/// Back circle with a title (and optional subtitle) for the payments and
/// withdrawal screens.
class WalletFlowAppBar extends StatelessWidget {
  const WalletFlowAppBar({super.key, required this.title, this.subtitle, this.trailing});

  final String title;
  final String? subtitle;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 10, 16, 4),
      child: Row(
        children: [
          KCircleButton(
            icon: Icons.arrow_back_rounded,
            tooltip: 'Back',
            onTap: () => Navigator.of(context).maybePop(),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: const TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w700),
                ),
                if (subtitle != null)
                  Text(subtitle!, style: const TextStyle(color: kProviderMuted, fontSize: 12.5)),
              ],
            ),
          ),
          ?trailing,
        ],
      ),
    );
  }
}

/// Navy card showing the amount being withdrawn.
class WalletAmountToPayCard extends StatelessWidget {
  const WalletAmountToPayCard({super.key, required this.naira});
  final double naira;

  @override
  Widget build(BuildContext context) {
    return KMoneyCard(
      child: Column(
        children: [
          const KEyebrow('Amount to pay', color: Color(0xB3FFFFFF)),
          const SizedBox(height: 4),
          const Text('Total', style: TextStyle(fontSize: 13, color: Color(0xCCFFFFFF))),
          const SizedBox(height: 6),
          FittedBox(
            fit: BoxFit.scaleDown,
            child: Text(
              '₦ ${formatNaira(naira)}',
              style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w700, letterSpacing: -0.8),
            ),
          ),
        ],
      ),
    );
  }
}

/// A selectable saved bank account with a bank tile and radio indicator.
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
    return KCard(
      onTap: onTap,
      borderColor: selected ? kTeal : kProviderBorder,
      color: selected ? const Color(0xFFF4FCFB) : kCard,
      child: Row(
        children: [
          const KIconTile(Icons.account_balance_rounded),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  accountName,
                  style: const TextStyle(color: kProviderText, fontSize: 14.5, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 2),
                Text(bankName, style: const TextStyle(color: kProviderText, fontSize: 13)),
                const SizedBox(height: 1),
                Text(accountNumber, style: const TextStyle(color: kProviderMuted, fontSize: 12.5)),
              ],
            ),
          ),
          WalletRadioDot(selected: selected),
        ],
      ),
    );
  }
}

/// Teal radio indicator.
class WalletRadioDot extends StatelessWidget {
  const WalletRadioDot({super.key, required this.selected});
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: const Duration(milliseconds: 180),
      width: 22,
      height: 22,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: selected ? kTealText : Colors.transparent,
        border: Border.all(color: selected ? kTealText : kProviderGreenSoft, width: 2),
      ),
      child: selected ? const Icon(Icons.check_rounded, size: 14, color: Colors.white) : null,
    );
  }
}
