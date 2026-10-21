import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../home/ui/widgets/provider_app_colors.dart';
import '../../models/earnings_models.dart';

/// Day-grouped list of earnings transactions (shared by the Earnings screen and
/// the Earning Summary screen).
class EarningsTransactionList extends StatelessWidget {
  const EarningsTransactionList({
    super.key,
    required this.transactions,
    required this.onGoToTrips,
    this.onTransactionTap,
  });

  final List<EarningsTransaction> transactions;
  final VoidCallback onGoToTrips;
  final ValueChanged<EarningsTransaction>? onTransactionTap;

  @override
  Widget build(BuildContext context) {
    final groups = EarningsTransactionGroup.group(transactions);

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final group in groups) ...[
            Padding(
              padding: const EdgeInsets.only(top: 6, bottom: 10),
              child: KEyebrow(group.label),
            ),
            KGroup(
              children: [
                for (final txn in group.items)
                  EarningsTransactionCard(
                    txn: txn,
                    onGoToTrips: onGoToTrips,
                    onTap: onTransactionTap == null ? null : () => onTransactionTap!(txn),
                  ),
              ],
            ),
            const SizedBox(height: 14),
          ],
        ],
      ),
    );
  }
}

/// One money movement: direction tile, title, when, and the signed amount.
class EarningsTransactionCard extends StatelessWidget {
  const EarningsTransactionCard({
    super.key,
    required this.txn,
    required this.onGoToTrips,
    this.onTap,
  });
  final EarningsTransaction txn;
  final VoidCallback onGoToTrips;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final credit = txn.isCredit;
    final amountText = '${credit ? '+' : '-'}₦${formatNaira(txn.amountNaira.abs())}';

    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            KIconTile(
              credit ? Icons.south_west_rounded : Icons.north_east_rounded,
              color: credit ? kTealText : kProviderRejectText,
              background: credit ? kTealTint : kProviderRejectBg,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    txn.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600),
                  ),
                  if (txn.subtitle.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(
                      txn.subtitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(color: kProviderMuted, fontSize: 12.5),
                    ),
                  ],
                  const SizedBox(height: 2),
                  Text(
                    formatTransactionDate(txn.occurredAt),
                    style: TextStyle(color: kProviderMuted, fontSize: 11.5),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 10),
            Column(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Text(
                  amountText,
                  style: TextStyle(
                    color: credit ? kTealText : kProviderRejectText,
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                if (txn.isTrip) ...[
                  const SizedBox(height: 8),
                  InkWell(
                    onTap: onGoToTrips,
                    borderRadius: BorderRadius.circular(6),
                    child: Padding(
                      padding: EdgeInsets.symmetric(vertical: 2),
                      child: Text(
                        'Go to Trips',
                        style: TextStyle(color: kProviderMuted, fontSize: 11.5, fontWeight: FontWeight.w600),
                      ),
                    ),
                  ),
                ],
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// Formats a transaction timestamp like the design: `2nd Jan 2025, 12:00:23`.
String formatTransactionDate(DateTime d) {
  const months = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  final hh = d.hour.toString().padLeft(2, '0');
  final mm = d.minute.toString().padLeft(2, '0');
  final ss = d.second.toString().padLeft(2, '0');
  return '${d.day}${_ordinalSuffix(d.day)} ${months[d.month - 1]} ${d.year}, $hh:$mm:$ss';
}

String _ordinalSuffix(int day) {
  if (day >= 11 && day <= 13) return 'th';
  switch (day % 10) {
    case 1:
      return 'st';
    case 2:
      return 'nd';
    case 3:
      return 'rd';
    default:
      return 'th';
  }
}
