import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../wallet/models/wallet_models.dart';
import '../models/earnings_models.dart';
import '../state/provider_earnings_controller.dart';
import 'provider_earning_summary_screen.dart';
import 'provider_all_transactions_screen.dart';
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
    this.onGoToTrips,
  });

  final ProviderEarningsController controller;

  /// Launches the withdrawal flow. When null the Withdraw button shows a
  /// "coming soon" hint.
  final VoidCallback? onWithdraw;

  /// Launches the disputes flow. When null the Dispute badge shows a
  /// "coming soon" hint.
  final VoidCallback? onDispute;

  /// Switches the home shell to the Trips tab.
  final VoidCallback? onGoToTrips;

  @override
  State<ProviderEarningsScreen> createState() => _ProviderEarningsScreenState();
}

class _ProviderEarningsScreenState extends State<ProviderEarningsScreen> {
  @override
  void initState() {
    super.initState();
    // Load once when the shell mounts; the home shell also refreshes on tab tap.
    WidgetsBinding.instance.addPostFrameCallback(
      (_) => widget.controller.load(),
    );
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
        builder: (_) => ProviderEarningSummaryScreen(
          controller: widget.controller,
          onGoToTrips: widget.onGoToTrips,
          onContactSupport: widget.onDispute,
        ),
      ),
    );
  }

  void _openAllTransactions() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderAllTransactionsScreen(
          controller: widget.controller,
          onGoToTrips: widget.onGoToTrips,
          onTransactionTap: _openTransactionDetail,
        ),
      ),
    );
  }

  void _openTransactionDetail(EarningsTransaction txn) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderTransactionDetailScreen(
          txn: txn,
          controller: widget.controller,
          onContactSupport: widget.onDispute,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: AnimatedBuilder(
        animation: widget.controller,
        builder: (context, _) {
          final c = widget.controller;
          final earnings = c.earnings;

          return SafeArea(
            bottom: false,
            child: RefreshIndicator(
              onRefresh: c.load,
              child: ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.only(top: 24, bottom: 32),
                children: [
                  Padding(
                    padding: kPageInset,
                    child: KHeading(
                      eyebrow: 'Your money',
                      title: 'Earnings',
                      subtitle: 'Manage your Income here.',
                      trailing: KCircleButton(
                        icon: Icons.insights_rounded,
                        tooltip: 'Earning Summary',
                        onTap: _openEarningSummary,
                      ),
                    ),
                  ),
                  const SizedBox(height: 20),
                  Padding(
                    padding: kPageInset,
                    child: _BalanceCard(
                      earnings: earnings,
                      walletBalance: c.walletBalance,
                      hidden: c.balanceHidden,
                      onToggleHidden: c.toggleBalanceVisibility,
                      onWithdraw:
                          widget.onWithdraw ?? () => _comingSoon('Withdraw'),
                    ),
                  ),
                  const SizedBox(height: 12),
                  Padding(
                    padding: kPageInset,
                    child: IntrinsicHeight(
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          Expanded(
                            child: KStatTile(
                              icon: Icons.local_shipping_outlined,
                              label: 'Trips Completed Today',
                              value: '${earnings.tripsCompletedToday}',
                            ),
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: KStatTile(
                              icon: Icons.schedule_rounded,
                              label: 'Hours Online',
                              value: '${earnings.hoursOnline}',
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(height: 12),
                  Padding(
                    padding: kPageInset,
                    child: KGroup(
                      children: [
                        KListRow(
                          icon: Icons.insights_rounded,
                          title: 'Earning Summary',
                          subtitle: 'Your year at a glance, month by month',
                          onTap: _openEarningSummary,
                        ),
                        KListRow(
                          icon: Icons.report_gmailerrorred_rounded,
                          iconColor: kWarning,
                          iconBackground: kWarningTint,
                          title: 'Dispute a payment',
                          subtitle: 'Something wrong with a payout? Tell us.',
                          onTap:
                              widget.onDispute ?? () => _comingSoon('Disputes'),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 26),
                  Padding(
                    padding: kPageInset,
                    child: KSectionHeader(
                      'Recent transactions',
                      action: earnings.transactions.isEmpty ? null : 'View all',
                      onAction: _openAllTransactions,
                    ),
                  ),
                  const SizedBox(height: 10),
                  if (c.error != null && !c.hasLoaded)
                    KEmptyState(
                      icon: Icons.cloud_off_rounded,
                      title: "Couldn't load earnings",
                      message: c.error!,
                      actionLabel: 'Retry',
                      onAction: c.load,
                    )
                  else if (c.isLoading && !c.hasLoaded)
                    const Padding(
                      padding: EdgeInsets.symmetric(vertical: 48),
                      child: Center(child: CircularProgressIndicator()),
                    )
                  else if (earnings.transactions.isEmpty)
                    const KEmptyState(
                      icon: Icons.receipt_long_outlined,
                      title: 'No transactions yet',
                      message: 'Completed trips and payouts will show up here.',
                    )
                  else
                    EarningsTransactionList(
                      transactions: earnings.transactions,
                      onGoToTrips:
                          widget.onGoToTrips ?? () => _comingSoon('Trips'),
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

// ─── Balance card ──────────────────────────────────────────────────────────────

class _BalanceCard extends StatelessWidget {
  const _BalanceCard({
    required this.earnings,
    required this.walletBalance,
    required this.hidden,
    required this.onToggleHidden,
    required this.onWithdraw,
  });

  final ProviderEarnings earnings;

  /// Real ledger balance from payment-wallet-service — the same figure the
  /// withdrawal flow uses. Null when it could not be fetched, in which case
  /// the card falls back to the hauling trip-earnings estimate.
  final WalletBalance? walletBalance;
  final bool hidden;
  final VoidCallback onToggleHidden;
  final VoidCallback onWithdraw;

  double get _availableNaira =>
      walletBalance?.availableNaira ?? earnings.availableBalanceNaira;

  double get _pendingNaira =>
      walletBalance?.pendingNaira ?? earnings.pendingBalanceNaira;

  @override
  Widget build(BuildContext context) {
    return KMoneyCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  walletBalance == null
                      ? 'Available Balance (estimated)'
                      : 'Available Balance',
                  maxLines: 2,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: Color(0xCCFFFFFF),
                  ),
                ),
              ),
              IconButton(
                onPressed: onToggleHidden,
                visualDensity: VisualDensity.compact,
                tooltip: hidden ? 'Show balance' : 'Hide balance',
                icon: Icon(
                  hidden
                      ? Icons.visibility_off_outlined
                      : Icons.visibility_outlined,
                  color: Colors.white,
                  size: 20,
                ),
              ),
            ],
          ),
          FittedBox(
            fit: BoxFit.scaleDown,
            alignment: Alignment.centerLeft,
            child: Text(
              hidden ? '₦ ****' : '₦ ${formatNaira(_availableNaira)}',
              style: const TextStyle(
                fontSize: 32,
                fontWeight: FontWeight.w700,
                letterSpacing: -0.8,
              ),
            ),
          ),
          const SizedBox(height: 16),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: const Color(0x1AFFFFFF),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Row(
              children: [
                Expanded(
                  child: _SubBalance(
                    label: 'Pending Balance',
                    value: hidden ? '****' : '₦ ${formatNaira(_pendingNaira)}',
                  ),
                ),
                Container(width: 1, height: 30, color: const Color(0x33FFFFFF)),
                const SizedBox(width: 12),
                Expanded(
                  child: _SubBalance(
                    label: "Today's Earnings",
                    value: hidden
                        ? '****'
                        : '₦ ${formatNaira(earnings.todayEarningsNaira)}',
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 16),
          KPrimaryButton(
            label: 'Withdraw',
            icon: Icons.north_east_rounded,
            color: kTeal,
            foreground: kNavy,
            onPressed: onWithdraw,
          ),
        ],
      ),
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
          style: const TextStyle(fontSize: 11.5, color: Color(0xB3FFFFFF)),
        ),
        const SizedBox(height: 3),
        Text(
          value,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w600,
            color: Colors.white,
          ),
        ),
      ],
    );
  }
}
