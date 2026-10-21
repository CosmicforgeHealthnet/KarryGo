import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../models/earnings_models.dart';
import '../state/provider_earnings_controller.dart';
import 'widgets/earnings_transaction_list.dart';

/// Every earnings transaction, with a filter for money in and money out.
class ProviderAllTransactionsScreen extends StatefulWidget {
  const ProviderAllTransactionsScreen({
    super.key,
    required this.controller,
    this.onGoToTrips,
    this.onTransactionTap,
  });

  final ProviderEarningsController controller;
  final VoidCallback? onGoToTrips;
  final ValueChanged<EarningsTransaction>? onTransactionTap;

  @override
  State<ProviderAllTransactionsScreen> createState() => _ProviderAllTransactionsScreenState();
}

enum _Filter { all, earned, paidOut }

class _ProviderAllTransactionsScreenState extends State<ProviderAllTransactionsScreen> {
  _Filter _filter = _Filter.all;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: const KTopBar(title: 'All transactions'),
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final all = widget.controller.earnings.transactions;
          final shown = switch (_filter) {
            _Filter.all => all,
            _Filter.earned => all.where((t) => t.isCredit).toList(),
            _Filter.paidOut => all.where((t) => !t.isCredit).toList(),
          };
          return RefreshIndicator(
            onRefresh: widget.controller.load,
            child: ListView(
              padding: const EdgeInsets.only(bottom: 32),
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 4, 20, 12),
                  child: Wrap(
                    spacing: 8,
                    children: [
                      for (final (f, label) in const [
                        (_Filter.all, 'All'),
                        (_Filter.earned, 'Earned'),
                        (_Filter.paidOut, 'Paid out'),
                      ])
                        ChoiceChip(
                          label: Text(label),
                          selected: _filter == f,
                          showCheckmark: false,
                          onSelected: (_) => setState(() => _filter = f),
                        ),
                    ],
                  ),
                ),
                if (shown.isEmpty)
                  const KEmptyState(
                    icon: Icons.receipt_long_outlined,
                    title: 'No transactions here yet.',
                  )
                else
                  EarningsTransactionList(
                    transactions: shown,
                    onGoToTrips: widget.onGoToTrips ?? () => Navigator.of(context).maybePop(),
                    onTransactionTap: widget.onTransactionTap,
                  ),
              ],
            ),
          );
        },
      ),
    );
  }
}
