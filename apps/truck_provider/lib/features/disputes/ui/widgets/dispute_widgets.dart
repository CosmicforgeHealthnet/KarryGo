import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../earnings/models/earnings_models.dart';
import '../../../earnings/ui/widgets/earnings_transaction_list.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';
import '../../models/dispute_models.dart';

/// Circular back button + bold title used across the dispute screens.
class DisputeAppBar extends StatelessWidget {
  const DisputeAppBar({super.key, required this.title, this.trailing});
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

/// A row in the "Feedbacks" list (Figma 2266): subject + date + status.
class DisputeFeedbackRow extends StatelessWidget {
  const DisputeFeedbackRow({super.key, required this.complaint});
  final Complaint complaint;

  Color get _statusColor {
    switch (complaint.statusLabel) {
      case 'Completed':
        return kProviderGreen;
      case 'Processing':
        return const Color(0xFFE8A21B);
      default:
        return const Color(0xFFE8A21B);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  complaint.subject.isEmpty ? 'Dispute' : complaint.subject,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 6),
                Text(
                  complaint.statusLabel,
                  style: TextStyle(color: _statusColor, fontSize: 13, fontWeight: FontWeight.w700),
                ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Text(
            formatTransactionDate(complaint.createdAt),
            style: const TextStyle(color: kProviderMuted, fontSize: 12),
          ),
        ],
      ),
    );
  }
}

/// Selectable transaction card used by the picker and the select-type screen.
class DisputeTransactionCard extends StatelessWidget {
  const DisputeTransactionCard({
    super.key,
    required this.txn,
    required this.selected,
    this.onTap,
  });

  final EarningsTransaction txn;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final amountColor = txn.isCredit ? kProviderGreen : kProviderRejectText;
    final sign = txn.isCredit ? '+' : '-';
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected ? kProviderGreen : kProviderBorder,
            width: selected ? 1.4 : 1,
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              txn.title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w700),
            ),
            if (txn.subtitle.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                txn.subtitle,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(color: kProviderMuted, fontSize: 12.5),
              ),
            ],
            const SizedBox(height: 4),
            Text(
              formatTransactionDate(txn.occurredAt),
              style: const TextStyle(color: kProviderMuted, fontSize: 12),
            ),
            const SizedBox(height: 8),
            Text(
              '$sign₦${formatNaira(txn.amountNaira.abs())}',
              style: TextStyle(color: amountColor, fontSize: 14, fontWeight: FontWeight.w800),
            ),
          ],
        ),
      ),
    );
  }
}

/// Shared green pill button for the dispute flow.
class DisputePrimaryButton extends StatelessWidget {
  const DisputePrimaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.loading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null && !loading;
    return SizedBox(
      width: double.infinity,
      height: 56,
      child: FilledButton(
        onPressed: enabled ? onPressed : null,
        style: FilledButton.styleFrom(
          backgroundColor: kProviderGreen,
          disabledBackgroundColor: kProviderGreenSoft,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
        ),
        child: loading
            ? const SizedBox.square(
                dimension: 22,
                child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
              )
            : Text(
                label,
                style: const TextStyle(color: Colors.white, fontSize: 16, fontWeight: FontWeight.w700),
              ),
      ),
    );
  }
}
