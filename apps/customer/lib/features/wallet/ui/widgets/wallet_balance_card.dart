import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/wallet_models.dart';

class WalletBalanceCard extends StatefulWidget {
  const WalletBalanceCard({
    super.key,
    required this.wallet,
    required this.onWithdraw,
  });

  final WalletSummary? wallet;
  final VoidCallback onWithdraw;

  @override
  State<WalletBalanceCard> createState() => _WalletBalanceCardState();
}

class _WalletBalanceCardState extends State<WalletBalanceCard> {
  bool _hidden = false;

  @override
  Widget build(BuildContext context) {
    final balance = widget.wallet?.formattedBalance ?? '₦0.00';
    final display = _hidden
        ? '₦ ••••••'
        : '₦ ${balance.replaceFirst('₦', '').trim()}';

    return Container(
      padding: const EdgeInsets.fromLTRB(22, 18, 22, 18),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            CustomerFigmaColors.primary,
            CustomerFigmaColors.darkGreen,
          ],
        ),
        borderRadius: BorderRadius.circular(20),
        boxShadow: [
          BoxShadow(
            color: CustomerFigmaColors.primary.withValues(alpha: 0.30),
            blurRadius: 22,
            offset: const Offset(0, 10),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'Available Balance',
                style: TextStyle(color: Colors.white70, fontSize: 13),
              ),
              GestureDetector(
                onTap: () => setState(() => _hidden = !_hidden),
                child: Container(
                  width: 34,
                  height: 34,
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.18),
                    shape: BoxShape.circle,
                  ),
                  child: Icon(
                    _hidden
                        ? Icons.visibility_off_outlined
                        : Icons.visibility_outlined,
                    color: Colors.white,
                    size: 18,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Expanded(
                child: Text(
                  display,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 28,
                    fontWeight: FontWeight.w900,
                    letterSpacing: -0.5,
                  ),
                ),
              ),
              GestureDetector(
                onTap: widget.onWithdraw,
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 22, vertical: 10),
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: const Text(
                    'Withdraw',
                    style: TextStyle(
                      color: CustomerFigmaColors.darkGreen,
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
