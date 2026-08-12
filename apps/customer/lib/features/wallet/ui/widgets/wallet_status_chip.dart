import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/wallet_models.dart';

/// Small pill showing a transaction's status, matching the mockup's
/// pending / success / failed chips.
class WalletStatusChip extends StatelessWidget {
  const WalletStatusChip({super.key, required this.status});

  final WalletTxnStatus status;

  @override
  Widget build(BuildContext context) {
    final (bg, fg) = switch (status) {
      WalletTxnStatus.pending => (const Color(0xFFFCF3D9), const Color(0xFFB8801F)),
      WalletTxnStatus.success => (CustomerFigmaColors.successSoft, CustomerFigmaColors.success),
      WalletTxnStatus.failed => (const Color(0xFFF7E4E4), const Color(0xFFA81717)),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(99),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(color: fg, shape: BoxShape.circle),
          ),
          const SizedBox(width: 6),
          Text(
            status.label,
            style: TextStyle(
              color: fg,
              fontSize: 12,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }
}
