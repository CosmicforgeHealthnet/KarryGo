import 'wallet_widgets.dart';
import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../state/wallet_controller.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../disputes/ui/support/help_centre_screen.dart';
import '../../trips/state/trips_controller.dart';
import 'earning_summary_screen.dart';
import 'withdrawal_form_screen.dart';
import 'transaction_list_screen.dart';
import 'bank_accounts_screen.dart';

class WalletScreen extends StatefulWidget {
  final WalletController walletController;
  final RiderDisputeController? disputeController;
  final TripsController? tripsController;
  final VoidCallback? onViewOpportunities;

  const WalletScreen({
    super.key,
    required this.walletController,
    this.disputeController,
    this.tripsController,
    this.onViewOpportunities,
  });

  @override
  State<WalletScreen> createState() => _WalletScreenState();
}

class _WalletScreenState extends State<WalletScreen> {
  bool _balanceVisible = true;

  WalletController get _controller => widget.walletController;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onUpdate);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _refresh();
    });
  }

  @override
  void dispose() {
    _controller.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
  }

  void _toggleBalance() => setState(() => _balanceVisible = !_balanceVisible);

  Future<void> _refresh() async {
    await Future.wait([
      _controller.loadEarnings(),
      _controller.loadTransactions(),
    ]);
  }

  Future<void> _openWithdraw() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => WithdrawalFormScreen(walletController: _controller),
      ),
    );
    if (mounted) await _refresh();
  }

  void _openAddBank() => Navigator.of(context).push(
    MaterialPageRoute(
      builder: (_) => BankAccountsScreen(walletController: _controller),
    ),
  );

  void _openEarnings() => Navigator.of(context).push(
    MaterialPageRoute(
      builder: (_) => EarningSummaryScreen(walletController: _controller),
    ),
  );

  void _openDispute() {
    final d = widget.disputeController;
    if (d == null) return;
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => HelpCentreScreen(disputeController: d)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final earnings = _controller.earnings;
    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: _refresh,
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.only(bottom: 24),
            children: [
              DeliveryCollectionHeader(
                title: 'Wallet',
                subtitle: 'Your earnings, at a glance.',
                action: widget.disputeController == null
                    ? null
                    : IconButton(
                        tooltip: 'Payment support',
                        onPressed: _openDispute,
                        icon: const JourneyIcon(
                          JourneySymbol.contact,
                          size: 26,
                        ),
                      ),
              ),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Container(
                      padding: const EdgeInsets.all(22),
                      decoration: BoxDecoration(
                        borderRadius: BorderRadius.circular(28),
                        gradient: const LinearGradient(
                          begin: Alignment.topLeft,
                          end: Alignment.bottomRight,
                          colors: [Color(0xff102e53), Color(0xff081a33)],
                        ),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          Row(
                            children: [
                              const JourneyIcon(
                                JourneySymbol.wallet,
                                size: 27,
                                color: Color(0xff04d4c0),
                              ),
                              const SizedBox(width: 10),
                              const Expanded(
                                child: Text(
                                  'Available balance',
                                  style: TextStyle(color: Colors.white70),
                                ),
                              ),
                              IconButton(
                                tooltip: _balanceVisible
                                    ? 'Hide balance'
                                    : 'Show balance',
                                onPressed: _toggleBalance,
                                icon: Icon(
                                  _balanceVisible
                                      ? Icons.visibility_outlined
                                      : Icons.visibility_off_outlined,
                                  color: Colors.white70,
                                  size: 22,
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 18),
                          AnimatedSize(
                            duration: const Duration(milliseconds: 220),
                            child: Align(
                              key: ValueKey(_balanceVisible),
                              alignment: Alignment.centerLeft,
                              child: Text(
                                !_balanceVisible
                                    ? '\u2022\u2022\u2022\u2022\u2022\u2022'
                                    : earnings == null
                                    ? (_controller.isLoading
                                          ? 'Loading...'
                                          : 'Unavailable')
                                    : walletAmount(
                                        earnings.availableKobo,
                                        earnings.currency,
                                      ),
                                style: Theme.of(context).textTheme.headlineLarge
                                    ?.copyWith(
                                      fontSize: 32,
                                      color: Colors.white,
                                    ),
                              ),
                            ),
                          ),
                          const SizedBox(height: 20),
                          const Divider(color: Colors.white24, height: 1),
                          const SizedBox(height: 16),
                          Wrap(
                            spacing: 10,
                            runSpacing: 6,
                            children: [
                              const Text(
                                'Pending',
                                style: TextStyle(color: Colors.white70),
                              ),
                              Text(
                                !_balanceVisible
                                    ? '\u2022\u2022\u2022\u2022'
                                    : earnings == null
                                    ? 'Unavailable'
                                    : walletAmount(
                                        earnings.pendingKobo,
                                        earnings.currency,
                                      ),
                                style: const TextStyle(
                                  color: Colors.white,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 22),
                          FilledButton.icon(
                            onPressed:
                                earnings == null ||
                                    _controller.error != null ||
                                    earnings.availableKobo <= 0
                                ? null
                                : _openWithdraw,
                            icon: const Icon(
                              Icons.north_east_rounded,
                              size: 20,
                            ),
                            label: const Text('Withdraw funds'),
                          ),
                        ],
                      ),
                    ),
                    if (_controller.error != null) ...[
                      const SizedBox(height: 14),
                      ActionNotice(
                        message: _controller.error!,
                        isError: true,
                        onRetry: _controller.loadEarnings,
                      ),
                    ],
                    const SizedBox(height: 22),
                    LayoutBuilder(
                      builder: (context, size) => Wrap(
                        spacing: 10,
                        runSpacing: 10,
                        children: [
                          for (final action in [
                            ('Banks', JourneySymbol.bank, _openAddBank),
                            ('Earnings', JourneySymbol.earnings, _openEarnings),
                            (
                              'History',
                              JourneySymbol.transfer,
                              () {
                                Navigator.of(context).push(
                                  MaterialPageRoute(
                                    builder: (_) =>
                                        const TransactionListScreen(),
                                  ),
                                );
                              },
                            ),
                          ])
                            SizedBox(
                              width: (size.maxWidth - 20) / 3,
                              child: DeliveryPanel(
                                padding: EdgeInsets.zero,
                                onTap: action.$3,
                                child: Padding(
                                  padding: const EdgeInsets.symmetric(
                                    vertical: 18,
                                    horizontal: 5,
                                  ),
                                  child: Column(
                                    children: [
                                      AccountEmblem(symbol: action.$2),
                                      const SizedBox(height: 10),
                                      Text(
                                        action.$1,
                                        textAlign: TextAlign.center,
                                        style: Theme.of(context)
                                            .textTheme
                                            .bodySmall
                                            ?.copyWith(
                                              fontWeight: FontWeight.w700,
                                            ),
                                      ),
                                    ],
                                  ),
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 24),
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            'Recent activity',
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                        ),
                        TextButton(
                          onPressed: () => Navigator.of(context).push(
                            MaterialPageRoute(
                              builder: (_) => const TransactionListScreen(),
                            ),
                          ),
                          child: const Text('View all'),
                        ),
                      ],
                    ),
                    if (_controller.historyLoading &&
                        _controller.transactions.isEmpty)
                      const SizedBox(
                        height: 230,
                        child: ContentSkeleton(rows: 2),
                      )
                    else if (_controller.historyError != null)
                      ActionNotice(
                        message: _controller.historyError!,
                        isError: true,
                        onRetry: _controller.loadTransactions,
                      )
                    else if (_controller.transactions.isEmpty)
                      DispatchEmptyState(
                        story: EmptyStory.wallet,
                        title: 'No wallet activity yet',
                        message: 'Confirmed wallet entries will appear here.',
                        actionLabel: widget.onViewOpportunities == null
                            ? null
                            : 'View opportunities',
                        onAction: widget.onViewOpportunities,
                      )
                    else
                      for (final entry in _controller.transactions.take(5))
                        Padding(
                          padding: const EdgeInsets.only(bottom: 10),
                          child: WalletEntryCard(
                            entry: entry,
                            onTap: () => Navigator.of(context).push(
                              MaterialPageRoute(
                                builder: (_) => TransactionListScreen(
                                  initialReference: entry['reference']
                                      ?.toString(),
                                ),
                              ),
                            ),
                          ),
                        ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
