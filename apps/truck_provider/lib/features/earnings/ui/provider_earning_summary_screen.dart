import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
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
  const ProviderEarningSummaryScreen({super.key, required this.controller});

  final ProviderEarningsController controller;

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
        builder: (_) => ProviderTransactionDetailScreen(txn: txn, controller: controller),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final e = controller.earnings;
          final year = e.summaryYear == 0 ? DateTime.now().year : e.summaryYear;
          return SafeArea(
            child: Column(
              children: [
                _Header(onBack: () => Navigator.of(context).maybePop()),
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.only(bottom: 32),
                    children: [
                      const SizedBox(height: 8),
                      Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 20),
                        child: _TotalEarningsCard(
                          naira: e.totalEarningsNaira,
                          hidden: controller.balanceHidden,
                          onToggle: controller.toggleBalanceVisibility,
                        ),
                      ),
                      const SizedBox(height: 18),
                      Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 20),
                        child: _ChartCard(
                          year: year,
                          monthlyKobo: e.monthlyEarningsKobo,
                          onYearTap: () => _comingSoon(context, 'Year filter'),
                        ),
                      ),
                      const SizedBox(height: 20),
                      if (e.transactions.isEmpty)
                        const _EmptyState()
                      else
                        EarningsTransactionList(
                          transactions: e.transactions,
                          onGoToTrips: () => _comingSoon(context, 'Trips'),
                          onTransactionTap: (txn) => _openTransactionDetail(context, txn),
                        ),
                    ],
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.onBack});
  final VoidCallback onBack;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
      child: Row(
        children: [
          GestureDetector(
            onTap: onBack,
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
          const Text(
            'Earning Summary',
            style: TextStyle(color: kProviderText, fontSize: 21, fontWeight: FontWeight.w800),
          ),
        ],
      ),
    );
  }
}

class _TotalEarningsCard extends StatelessWidget {
  const _TotalEarningsCard({required this.naira, required this.hidden, required this.onToggle});
  final double naira;
  final bool hidden;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(22, 20, 20, 22),
      decoration: BoxDecoration(
        gradient: kProviderBalanceGradient,
        borderRadius: BorderRadius.circular(16),
        boxShadow: [
          BoxShadow(color: kProviderGreen.withValues(alpha: 0.30), blurRadius: 20, offset: const Offset(0, 8)),
        ],
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Total Earnings',
                  style: TextStyle(color: Colors.white, fontSize: 13.5, fontWeight: FontWeight.w600),
                ),
                const SizedBox(height: 10),
                Text(
                  hidden ? '₦ ****' : '₦ ${formatNaira(naira)}',
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 28,
                    fontWeight: FontWeight.w800,
                    letterSpacing: 0.5,
                  ),
                ),
              ],
            ),
          ),
          GestureDetector(
            onTap: onToggle,
            behavior: HitTestBehavior.opaque,
            child: Icon(
              hidden ? Icons.visibility_off_outlined : Icons.visibility_outlined,
              color: Colors.white,
              size: 22,
            ),
          ),
        ],
      ),
    );
  }
}

class _ChartCard extends StatelessWidget {
  const _ChartCard({required this.year, required this.monthlyKobo, required this.onYearTap});
  final int year;
  final List<int> monthlyKobo;
  final VoidCallback onYearTap;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 18, 16, 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: kProviderBorder),
        boxShadow: const [BoxShadow(color: Color(0x0F000000), blurRadius: 16, offset: Offset(0, 6))],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Earnings',
                      style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
                    ),
                    SizedBox(height: 2),
                    Text(
                      'Monitor and track your earnings.',
                      style: TextStyle(color: kProviderMuted, fontSize: 12.5),
                    ),
                  ],
                ),
              ),
              GestureDetector(
                onTap: onYearTap,
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(color: kProviderBorder),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        '$year',
                        style: const TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w600),
                      ),
                      const SizedBox(width: 4),
                      const Icon(Icons.keyboard_arrow_down_rounded, size: 18, color: kProviderText),
                    ],
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          EarningsChart(monthlyKobo: monthlyKobo),
        ],
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState();
  @override
  Widget build(BuildContext context) {
    return const Padding(
      padding: EdgeInsets.fromLTRB(20, 24, 20, 24),
      child: Center(
        child: Text(
          'No earnings yet.',
          style: TextStyle(color: kProviderMuted, fontSize: 13),
        ),
      ),
    );
  }
}
