import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
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

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final group in groups) ...[
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 6, 20, 0),
            child: Row(
              children: [
                Text(
                  group.label,
                  style: const TextStyle(
                    color: kProviderText,
                    fontSize: 15,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(width: 3),
                const Icon(Icons.arrow_drop_down, size: 24, color: kProviderText),
              ],
            ),
          ),
          const SizedBox(height: 8),
          for (final txn in group.items)
            EarningsTransactionCard(
              txn: txn,
              onGoToTrips: onGoToTrips,
              onTap: onTransactionTap == null ? null : () => onTransactionTap!(txn),
            ),
          const SizedBox(height: 8),
        ],
      ],
    );
  }
}

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
    final amountColor = txn.isCredit ? kProviderGreen : kProviderRejectText;
    final sign = txn.isCredit ? '+' : '-';
    final amountText = '$sign₦${formatNaira(txn.amountNaira.abs())}';

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
      margin: const EdgeInsets.fromLTRB(20, 0, 20, 10),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        boxShadow: const [
          BoxShadow(color: Color(0x12000000), blurRadius: 16, offset: Offset(0, 6)),
        ],
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  txn.title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: kProviderText,
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                  ),
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
                  amountText,
                  style: TextStyle(color: amountColor, fontSize: 14, fontWeight: FontWeight.w800),
                ),
              ],
            ),
          ),
          if (txn.isTrip)
            GestureDetector(
              onTap: onGoToTrips,
              behavior: HitTestBehavior.opaque,
              child: const Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(Icons.arrow_forward_rounded, color: kProviderGreen, size: 20),
                  SizedBox(height: 18),
                  Text(
                    'Go to Trips',
                    style: TextStyle(color: kProviderGreen, fontSize: 12, fontWeight: FontWeight.w600),
                  ),
                ],
              ),
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
