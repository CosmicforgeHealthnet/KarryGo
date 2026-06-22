import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/wallet_models.dart';

class WalletTransactionTile extends StatelessWidget {
  const WalletTransactionTile({
    super.key,
    required this.txn,
    this.onTap,
    this.onGoToTrips,
  });

  final WalletTransaction txn;
  final VoidCallback? onTap;
  final VoidCallback? onGoToTrips;

  bool get _isTripRelated =>
      txn.type == 'wallet_payment_hold' ||
      txn.type == 'job_settlement' ||
      (txn.description.isNotEmpty &&
          txn.type != 'paystack_charge_success' &&
          !txn.type.startsWith('withdrawal') &&
          !txn.type.startsWith('refund'));

  @override
  Widget build(BuildContext context) {
    final color =
        txn.isCredit ? CustomerFigmaColors.primary : const Color(0xFFE53935);

    return InkWell(
      onTap: onTap,
      child: IntrinsicHeight(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Container(
              width: 3,
              decoration: BoxDecoration(
                color: color,
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 14),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      txn.title,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    if (txn.description.isNotEmpty) ...[
                      const SizedBox(height: 2),
                      Text(
                        txn.description,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          color: CustomerFigmaColors.muted,
                          fontSize: 12,
                        ),
                      ),
                    ],
                    const SizedBox(height: 2),
                    Text(
                      _formatDate(txn.createdAt),
                      style: const TextStyle(
                        color: CustomerFigmaColors.muted,
                        fontSize: 11,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      txn.formattedAmount,
                      style: TextStyle(
                        color: color,
                        fontSize: 14,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                  ],
                ),
              ),
            ),
            if (_isTripRelated)
              Padding(
                padding: const EdgeInsets.only(left: 8),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Icon(Icons.arrow_forward_rounded,
                        size: 18, color: CustomerFigmaColors.primary),
                    const SizedBox(height: 2),
                    Text(
                      'Go to Trips',
                      style: TextStyle(
                        color: CustomerFigmaColors.primary,
                        fontSize: 11,
                        fontWeight: FontWeight.w600,
                      ),
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

String _ordinal(int day) {
  if (day >= 11 && day <= 13) return '${day}th';
  return switch (day % 10) {
    1 => '${day}st',
    2 => '${day}nd',
    3 => '${day}rd',
    _ => '${day}th',
  };
}

const _months = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
];

String _formatDate(DateTime dt) {
  final h = dt.hour.toString().padLeft(2, '0');
  final m = dt.minute.toString().padLeft(2, '0');
  final s = dt.second.toString().padLeft(2, '0');
  return '${_ordinal(dt.day)} ${_months[dt.month - 1]} ${dt.year}, $h:$m:$s';
}

String formatTxnDate(DateTime dt) =>
    '${_months[dt.month - 1]} ${dt.day}, ${dt.year}';

String formatTxnDateTime(DateTime dt) => _formatDate(dt);

enum _DateGroup { today, yesterday, lastWeek, earlier }

_DateGroup _dateGroup(DateTime dt) {
  final now = DateTime.now();
  final today = DateTime(now.year, now.month, now.day);
  final txnDay = DateTime(dt.year, dt.month, dt.day);
  final diff = today.difference(txnDay).inDays;
  if (diff == 0) return _DateGroup.today;
  if (diff == 1) return _DateGroup.yesterday;
  if (diff <= 7) return _DateGroup.lastWeek;
  return _DateGroup.earlier;
}

String _dateGroupLabel(_DateGroup g) => switch (g) {
      _DateGroup.today => 'Today',
      _DateGroup.yesterday => 'Yesterday',
      _DateGroup.lastWeek => 'Last Week',
      _DateGroup.earlier => 'Earlier',
    };

typedef TransactionGroup = ({String label, List<WalletTransaction> items});

List<TransactionGroup> groupTransactionsByDate(List<WalletTransaction> txns) {
  final map = <_DateGroup, List<WalletTransaction>>{};
  for (final txn in txns) {
    final g = _dateGroup(txn.createdAt);
    (map[g] ??= []).add(txn);
  }
  final order = [
    _DateGroup.today,
    _DateGroup.yesterday,
    _DateGroup.lastWeek,
    _DateGroup.earlier,
  ];
  return [
    for (final g in order)
      if (map.containsKey(g))
        (label: _dateGroupLabel(g), items: map[g]!),
  ];
}
