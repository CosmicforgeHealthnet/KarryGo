import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/earnings_models.dart';
import '../state/provider_earnings_controller.dart';
import 'provider_transaction_detail_screen.dart';
import 'widgets/earnings_chart.dart';
import 'widgets/earnings_transaction_list.dart';

/// Earning Summary screen (Figma 2273 / 2274 balance-hidden): lifetime total,
/// a monthly earnings chart, and the grouped transaction list. Reuses the
/// already-loaded [ProviderEarningsController] data.
class ProviderEarningSummaryScreen extends StatelessWidget {
  const ProviderEarningSummaryScreen({
    super.key,
    required this.controller,
    this.onGoToTrips,
    this.onContactSupport,
  });

  final ProviderEarningsController controller;
  final VoidCallback? onGoToTrips;
  final VoidCallback? onContactSupport;

  void _comingSoon(BuildContext context, String label) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('$label is coming soon'),
        duration: const Duration(seconds: 1),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  void _openTransactionDetail(BuildContext context, EarningsTransaction txn) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderTransactionDetailScreen(
          txn: txn,
          controller: controller,
          onContactSupport: onContactSupport,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: KTopBar(title: 'Earning Summary'),
      body: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final e = controller.earnings;
          final year = e.summaryYear == 0 ? DateTime.now().year : e.summaryYear;
          return ListView(
            padding: const EdgeInsets.only(top: 8, bottom: 32),
            children: [
              Padding(
                padding: kPageInset,
                child: KMoneyCard(
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const Text(
                              'Total Earnings',
                              style: TextStyle(fontSize: 13, color: Color(0xCCFFFFFF)),
                            ),
                            const SizedBox(height: 8),
                            FittedBox(
                              fit: BoxFit.scaleDown,
                              alignment: Alignment.centerLeft,
                              child: Text(
                                controller.balanceHidden ? '₦ ****' : '₦ ${formatNaira(e.totalEarningsNaira)}',
                                style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w700, letterSpacing: -0.8),
                              ),
                            ),
                            const SizedBox(height: 4),
                            const Text(
                              'Everything you have earned on Karryam.',
                              style: TextStyle(fontSize: 12, color: Color(0xB3FFFFFF)),
                            ),
                          ],
                        ),
                      ),
                      IconButton(
                        onPressed: controller.toggleBalanceVisibility,
                        tooltip: controller.balanceHidden ? 'Show total' : 'Hide total',
                        icon: Icon(
                          controller.balanceHidden ? Icons.visibility_off_outlined : Icons.visibility_outlined,
                          color: Colors.white,
                          size: 20,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 14),
              Padding(
                padding: kPageInset,
                child: KCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text('Monthly earnings', style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
                                SizedBox(height: 2),
                                Text(
                                  'Monitor and track your earnings.',
                                  style: TextStyle(fontSize: 12.5, color: kProviderMuted),
                                ),
                              ],
                            ),
                          ),
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                            decoration: BoxDecoration(
                              color: kCanvas,
                              borderRadius: BorderRadius.circular(999),
                              border: Border.all(color: kProviderBorder),
                            ),
                            child: Text(
                              '$year',
                              style: const TextStyle(fontSize: 12.5, fontWeight: FontWeight.w600),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 14),
                      EarningsChart(monthlyKobo: e.monthlyEarningsKobo),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 24),
              const Padding(padding: kPageInset, child: KSectionHeader('Transactions')),
              const SizedBox(height: 10),
              if (e.transactions.isEmpty)
                const KEmptyState(
                  icon: Icons.receipt_long_outlined,
                  title: 'No earnings yet.',
                  message: 'Your completed trips will appear here.',
                )
              else
                EarningsTransactionList(
                  transactions: e.transactions,
                  onGoToTrips: onGoToTrips ?? () => _comingSoon(context, 'Trips'),
                  onTransactionTap: (txn) => _openTransactionDetail(context, txn),
                ),
            ],
          );
        },
      ),
    );
  }
}
