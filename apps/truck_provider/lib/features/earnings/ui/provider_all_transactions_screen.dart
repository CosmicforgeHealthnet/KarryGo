import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
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
      backgroundColor: kProviderPageBg,
      appBar: AppBar(
        backgroundColor: kProviderPageBg,
        surfaceTintColor: Colors.transparent,
        title: const Text('All transactions', style: TextStyle(fontWeight: FontWeight.w800)),
      ),
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
            color: kProviderGreen,
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
                          selectedColor: kProviderGreen.withValues(alpha: 0.18),
                          onSelected: (_) => setState(() => _filter = f),
                        ),
                    ],
                  ),
                ),
                if (shown.isEmpty)
                  const Padding(
                    padding: EdgeInsets.all(40),
                    child: Center(
                      child: Text('No transactions here yet.', style: TextStyle(color: kProviderMuted)),
                    ),
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
