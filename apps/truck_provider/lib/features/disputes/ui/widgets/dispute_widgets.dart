import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../earnings/models/earnings_models.dart';
import '../../../earnings/ui/widgets/earnings_transaction_list.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';
import '../../models/dispute_models.dart';

/// Back circle + title used across the dispute screens.
class DisputeAppBar extends StatelessWidget {
  const DisputeAppBar({super.key, required this.title, this.trailing});
  final String title;
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
            child: Text(
              title,
              style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w700),
            ),
          ),
          ?trailing,
        ],
      ),
    );
  }
}

/// A row in the disputes list: subject, date and a status chip.
class DisputeFeedbackRow extends StatelessWidget {
  const DisputeFeedbackRow({super.key, required this.complaint});
  final Complaint complaint;

  KTone get _tone => switch (complaint.statusLabel) {
        'Completed' => KTone.success,
        'Processing' => KTone.info,
        _ => KTone.warning,
      };

  @override
  Widget build(BuildContext context) {
    return KCard(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      child: Row(
        children: [
          KIconTile(Icons.report_gmailerrorred_rounded, color: kWarning, background: kWarningTint),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  complaint.subject.isEmpty ? 'Dispute' : complaint.subject,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600),
                ),
                const SizedBox(height: 3),
                Text(
                  formatTransactionDate(complaint.createdAt),
                  style: TextStyle(color: kProviderMuted, fontSize: 12),
                ),
              ],
            ),
          ),
          const SizedBox(width: 8),
          KStatusChip(complaint.statusLabel, tone: _tone),
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
    final amountColor = txn.isCredit ? kTealText : kProviderRejectText;
    final sign = txn.isCredit ? '+' : '-';
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        decoration: BoxDecoration(
          color: selected ? kSelectedWash : kCard,
          borderRadius: kCardRadius,
          border: Border.all(
            color: selected ? kTeal : kProviderBorder,
            width: selected ? 1.8 : 1,
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              txn.title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w700),
            ),
            if (txn.subtitle.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                txn.subtitle,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(color: kProviderMuted, fontSize: 12.5),
              ),
            ],
            const SizedBox(height: 4),
            Text(
              formatTransactionDate(txn.occurredAt),
              style: TextStyle(color: kProviderMuted, fontSize: 12),
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

/// Primary action for the dispute flow.
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
  Widget build(BuildContext context) =>
      KPrimaryButton(label: label, onPressed: onPressed, loading: loading);
}
