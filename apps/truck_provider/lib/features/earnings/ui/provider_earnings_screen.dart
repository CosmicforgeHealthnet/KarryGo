import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/earnings_models.dart';
import '../state/provider_earnings_controller.dart';
import 'provider_earning_summary_screen.dart';
import 'provider_transaction_detail_screen.dart';
import 'widgets/earnings_transaction_list.dart';

/// Provider Earnings / Wallet screen (Figma 2271 shown, 2272 balance-hidden).
///
/// Lives as a bottom-nav tab in the provider home shell. Loads the trip-earnings
/// projection from the hauling service and renders the balance card, today's
/// stats, and the grouped recent-transactions list.
class ProviderEarningsScreen extends StatefulWidget {
  const ProviderEarningsScreen({
    super.key,
    required this.controller,
    this.onWithdraw,
    this.onDispute,
  });

  final ProviderEarningsController controller;

  /// Launches the withdrawal flow. When null the Withdraw button shows a
  /// "coming soon" hint.
  final VoidCallback? onWithdraw;

  /// Launches the disputes flow. When null the Dispute badge shows a
  /// "coming soon" hint.
  final VoidCallback? onDispute;

  @override
  State<ProviderEarningsScreen> createState() => _ProviderEarningsScreenState();
}

class _ProviderEarningsScreenState extends State<ProviderEarningsScreen> {
  @override
  void initState() {
    super.initState();
    // Load once when the shell mounts; the home shell also refreshes on tab tap.
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.load());
  }

  void _comingSoon(String label) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('$label is coming soon'),
        duration: const Duration(seconds: 1),
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  void _openEarningSummary() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderEarningSummaryScreen(controller: widget.controller),
      ),
    );
  }

  void _openTransactionDetail(EarningsTransaction txn) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderTransactionDetailScreen(txn: txn, controller: widget.controller),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: kProviderPageBg,
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final c = widget.controller;
          final earnings = c.earnings;

          return SafeArea(
            bottom: false,
            child: RefreshIndicator(
              color: kProviderGreen,
              onRefresh: c.load,
              child: ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.only(bottom: 120),
                children: [
                  _Header(onEarningSummary: _openEarningSummary),
                  const SizedBox(height: 16),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    child: _BalanceCard(
                      earnings: earnings,
                      hidden: c.balanceHidden,
                      onToggleHidden: c.toggleBalanceVisibility,
                      onWithdraw: widget.onWithdraw ?? () => _comingSoon('Withdraw'),
                      onDispute: widget.onDispute ?? () => _comingSoon('Disputes'),
                    ),
                  ),
                  const SizedBox(height: 14),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    child: _StatsCard(
                      tripsToday: earnings.tripsCompletedToday,
                      hoursOnline: earnings.hoursOnline,
                    ),
                  ),
                  const SizedBox(height: 24),
                  _RecentTransactionsHeader(onViewAll: () => _comingSoon('View All')),
                  const SizedBox(height: 12),
                  if (c.error != null && !c.hasLoaded)
                    _ErrorState(message: c.error!, onRetry: c.load)
                  else if (c.isLoading && !c.hasLoaded)
                    const _LoadingState()
                  else if (earnings.transactions.isEmpty)
                    const _EmptyState()
                  else
                    EarningsTransactionList(
                      transactions: earnings.transactions,
                      onGoToTrips: () => _comingSoon('Trips'),
                      onTransactionTap: _openTransactionDetail,
                    ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

// ─── Header ────────────────────────────────────────────────────────────────────

class _Header extends StatelessWidget {
  const _Header({required this.onEarningSummary});
  final VoidCallback onEarningSummary;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Earnings',
                  style: TextStyle(
                    color: kProviderText,
                    fontSize: 26,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                SizedBox(height: 2),
                Text(
                  'Manage your Income here.',
                  style: TextStyle(color: kProviderMuted, fontSize: 13),
                ),
              ],
            ),
          ),
          GestureDetector(
            onTap: onEarningSummary,
            child: Container(
              padding: const EdgeInsets.fromLTRB(8, 8, 14, 8),
              decoration: BoxDecoration(
                color: kProviderGreen,
                borderRadius: BorderRadius.circular(30),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Container(
                    width: 20,
                    height: 20,
                    decoration: const BoxDecoration(color: Colors.white, shape: BoxShape.circle),
                    alignment: Alignment.center,
                    child: const Text(
                      r'$',
                      style: TextStyle(
                        color: kProviderGreen,
                        fontSize: 13,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  const Text(
                    'Earning Summary',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Balance card ──────────────────────────────────────────────────────────────

class _BalanceCard extends StatelessWidget {
  const _BalanceCard({
    required this.earnings,
    required this.hidden,
    required this.onToggleHidden,
    required this.onWithdraw,
    required this.onDispute,
  });

  final ProviderEarnings earnings;
  final bool hidden;
  final VoidCallback onToggleHidden;
  final VoidCallback onWithdraw;
  final VoidCallback onDispute;

  @override
  Widget build(BuildContext context) {
    // Withdraw sits INSIDE the card (bottom-right, in the column flow); the
    // Dispute badge is overlaid in the upper-right above it.
    return Stack(
      children: [
        Container(
          width: double.infinity,
          padding: const EdgeInsets.fromLTRB(20, 18, 18, 18),
          decoration: BoxDecoration(
            gradient: kProviderBalanceGradient,
            borderRadius: BorderRadius.circular(20),
            boxShadow: [
              BoxShadow(
                color: const Color(0xFF0A5626).withValues(alpha: 0.32),
                blurRadius: 22,
                offset: const Offset(0, 12),
              ),
            ],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const Text(
                    'Available Balance',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 13.5,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const Spacer(),
                  GestureDetector(
                    onTap: onToggleHidden,
                    behavior: HitTestBehavior.opaque,
                    child: Icon(
                      hidden ? Icons.visibility_off_outlined : Icons.visibility_outlined,
                      color: Colors.white,
                      size: 22,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Text(
                hidden ? '₦ ****' : '₦ ${formatNaira(earnings.availableBalanceNaira)}',
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 30,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 0.5,
                ),
              ),
              const SizedBox(height: 16),
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: _SubBalance(
                      label: 'Pending Balance',
                      value: hidden ? '****' : '₦ ${formatNaira(earnings.pendingBalanceNaira)}',
                    ),
                  ),
                  Expanded(
                    child: _SubBalance(
                      label: "Today's Earnings",
                      value: hidden ? '****' : '₦ ${formatNaira(earnings.todayEarningsNaira)}',
                    ),
                  ),
                  // Reserve space on the right for the positioned Dispute badge.
                  const SizedBox(width: 56),
                ],
              ),
              const SizedBox(height: 14),
              Align(
                alignment: Alignment.centerRight,
                child: GestureDetector(
                  onTap: onWithdraw,
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 9),
                    decoration: BoxDecoration(
                      color: Colors.white,
                      borderRadius: BorderRadius.circular(30),
                      border: Border.all(color: kProviderGreen, width: 1.3),
                    ),
                    child: const Text(
                      'Withdraw',
                      style: TextStyle(
                        color: kProviderGreen,
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),

        // Dispute badge — upper-right of the lower area, above the Withdraw pill.
        Positioned(
          top: 78,
          right: 16,
          child: GestureDetector(
            onTap: onDispute,
            behavior: HitTestBehavior.opaque,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    color: const Color(0xFFFCF3DA),
                    borderRadius: BorderRadius.circular(11),
                  ),
                  child: const Icon(Icons.assignment_rounded, color: Color(0xFFE8A21B), size: 20),
                ),
                const SizedBox(height: 4),
                const Text(
                  'Dispute',
                  style: TextStyle(color: Colors.white, fontSize: 10.5, fontWeight: FontWeight.w600),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _SubBalance extends StatelessWidget {
  const _SubBalance({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(color: Colors.white, fontSize: 12.5, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 5),
        Text(
          value,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(
            color: Colors.white.withValues(alpha: 0.92),
            fontSize: 13,
            fontWeight: FontWeight.w500,
          ),
        ),
      ],
    );
  }
}

// ─── Stats card ────────────────────────────────────────────────────────────────

class _StatsCard extends StatelessWidget {
  const _StatsCard({required this.tripsToday, required this.hoursOnline});
  final int tripsToday;
  final int hoursOnline;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        boxShadow: const [
          BoxShadow(color: Color(0x14000000), blurRadius: 20, offset: Offset(0, 8)),
        ],
      ),
      child: Row(
        children: [
          Expanded(child: _StatItem(label: 'Trips Completed Today', value: '$tripsToday')),
          Expanded(child: _StatItem(label: 'Hours Online', value: '$hoursOnline')),
        ],
      ),
    );
  }
}

class _StatItem extends StatelessWidget {
  const _StatItem({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Text(
          label,
          textAlign: TextAlign.center,
          style: const TextStyle(color: kProviderGreen, fontSize: 13.5, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 8),
        Text(
          value,
          style: const TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w600),
        ),
      ],
    );
  }
}

// ─── Recent transactions ───────────────────────────────────────────────────────

class _RecentTransactionsHeader extends StatelessWidget {
  const _RecentTransactionsHeader({required this.onViewAll});
  final VoidCallback onViewAll;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Row(
        children: [
          const Text(
            'Recent Transactions',
            style: TextStyle(color: kProviderText, fontSize: 19, fontWeight: FontWeight.w800),
          ),
          const Spacer(),
          GestureDetector(
            onTap: onViewAll,
            behavior: HitTestBehavior.opaque,
            child: const Text(
              'View All',
              style: TextStyle(color: kProviderGreen, fontSize: 14, fontWeight: FontWeight.w600),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── States (loading / error / empty) ─────────────────────────────────────────

class _LoadingState extends StatelessWidget {
  const _LoadingState();
  @override
  Widget build(BuildContext context) {
    return const Padding(
      padding: EdgeInsets.symmetric(vertical: 48),
      child: Center(child: CircularProgressIndicator(color: kProviderGreen)),
    );
  }
}

class _ErrorState extends StatelessWidget {
  const _ErrorState({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
      child: Column(
        children: [
          const Icon(Icons.cloud_off_rounded, color: kProviderMuted, size: 40),
          const SizedBox(height: 12),
          Text(
            message,
            textAlign: TextAlign.center,
            style: const TextStyle(color: kProviderMuted, fontSize: 13),
          ),
          const SizedBox(height: 16),
          OutlinedButton(
            onPressed: onRetry,
            style: OutlinedButton.styleFrom(
              foregroundColor: kProviderGreen,
              side: const BorderSide(color: kProviderGreen),
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
            ),
            child: const Text('Retry'),
          ),
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
      padding: EdgeInsets.fromLTRB(20, 40, 20, 40),
      child: Column(
        children: [
          Icon(Icons.receipt_long_outlined, color: kProviderMuted, size: 40),
          SizedBox(height: 12),
          Text(
            'No transactions yet.\nCompleted trips will show up here.',
            textAlign: TextAlign.center,
            style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.4),
          ),
        ],
      ),
    );
  }
}

