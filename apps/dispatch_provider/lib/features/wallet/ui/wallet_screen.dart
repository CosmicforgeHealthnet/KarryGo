import 'package:flutter/material.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../mock/wallet_tab_mock.dart';
import '../state/wallet_controller.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../trips/state/trips_controller.dart';

import 'add_bank_account_screen.dart';
import 'earning_summary_screen.dart';
import 'log_dispute_screen.dart';
import 'quick_actions_sheet.dart';
import 'transaction_detail_screen.dart';
import 'withdrawal_form_screen.dart';
import 'transaction_list_screen.dart';
import 'bank_accounts_screen.dart';

// ── Balance card gradient — brand navy with a subtle lift.
//    No teal bleed, no blue-tinted shadow. Reads as one solid premium panel
//    in both light and dark mode.
const LinearGradient _kBalanceGradient = LinearGradient(
  begin: Alignment.topLeft,
  end: Alignment.bottomRight,
  colors: [
    Color(0xFF081F44), // Midnight Navy
    Color(0xFF0F2A5C), // subtle lift
  ],
);

class WalletScreen extends StatefulWidget {
  final WalletController walletController;
  final RiderDisputeController? disputeController;
  final TripsController? tripsController;

  const WalletScreen({
    super.key,
    required this.walletController,
    this.disputeController,
    this.tripsController,
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
    _controller.loadEarnings();
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

  // ── Data ──────────────────────────────────────────────────────────

  double get _balance {
    if (AppModeConfig.instance.isMock) return WalletTabMock.balanceNaira;
    return _controller.earnings?.availableKobo != null
        ? _controller.earnings!.availableKobo / 100
        : 0;
  }

  List<ActivityItem> get _activity {
    if (AppModeConfig.instance.isMock) return WalletTabMock.recentActivity;
    return const [];
  }

  // ── Actions ───────────────────────────────────────────────────────

  void _openWithdraw() => Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => WithdrawalFormScreen(walletController: _controller),
        ),
      );

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

  // ignore: unused_element
  void _openDispute() {
    final d = widget.disputeController;
    final t = widget.tripsController;
    if (d == null || t == null) return;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            LogDisputeScreen(disputeController: d, tripsController: t),
      ),
    );
  }

  void _openQuickActions() {
    QuickActionsSheet.show(
      context,
      onWithdraw: _openWithdraw,
      onAddBank: _openAddBank,
      onTransactions: () => Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => const TransactionListScreen(),
        ),
      ),
      onEarnings: _openEarnings,
      onHelp: () {},
    );
  }

  void _openTransaction(ActivityItem item) {
    if (!AppModeConfig.instance.isMock) return;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => TransactionDetailScreen(
          transaction: _mockTransactionFrom(item),
        ),
      ),
    );
  }

  TransactionData _mockTransactionFrom(ActivityItem item) {
    return TransactionData(
      id: item.id,
      amount: item.amountNaira,
      commission: 0,
      total: item.amountNaira,
      status: TransactionStatus.successful,
      type: item.isCredit ? TransactionType.trip : TransactionType.debit,
      customerName: item.title,
      date: item.subtitle,
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return AnimatedBuilder(
      animation: AppModeConfig.instance,
      builder: (context, _) => Scaffold(
        backgroundColor: colors.background,
        body: SafeArea(
          child: CustomScrollView(
            slivers: [
              _buildTopBar(),
              _buildBalanceCard(),
              _buildQuickActionsRow(),
              _buildRecentActivityHeader(),
              _buildRecentActivityList(),
              const SliverToBoxAdapter(child: SizedBox(height: 100)),
            ],
          ),
        ),
      ),
    );
  }

  // ── Top bar: Karryam logo + avatar ────────────────────────────────

  Widget _buildTopBar() {
    final colors = context.palette;
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 4),
        child: Row(
          children: [
            Image.asset(
              'assets/figma/splash_logo.png',
              width: 34,
              height: 34,
              fit: BoxFit.contain,
            ),
            const SizedBox(width: 8),
            // Text(
            //   'Karryam',
            //   style: TextStyle(
            //     fontSize: 22,
            //     fontWeight: FontWeight.w800,
            //     color: colors.text,
            //     letterSpacing: -0.6,
            //     height: 1.0,
            //   ),
            // ),
            const Spacer(),
            GestureDetector(
              onTap: () {},
              behavior: HitTestBehavior.opaque,
              child: Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: colors.navy10,
                  shape: BoxShape.circle,
                  border: Border.all(color: colors.navy20),
                ),
                child: Icon(
                  Icons.person_outline_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Balance card ──────────────────────────────────────────────────

  Widget _buildBalanceCard() {
    final hidden = !_balanceVisible;
    final isDark = context.isDark;

    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 14, 20, 0),
        child: GestureDetector(
          onTap: hidden ? _toggleBalance : null,
          behavior: HitTestBehavior.opaque,
          child: Container(
            width: double.infinity,
            decoration: BoxDecoration(
              gradient: _kBalanceGradient,
              borderRadius: BorderRadius.circular(22),
              boxShadow: [
                BoxShadow(
                  color: Colors.black.withValues(alpha: isDark ? 0.30 : 0.16),
                  blurRadius: 32,
                  spreadRadius: -8,
                  offset: const Offset(0, 14),
                ),
              ],
            ),
            child: Stack(
              children: [
                Padding(
                  padding: const EdgeInsets.all(20),
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(minHeight: 104),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        _buildCardIcon(hidden),
                        const SizedBox(width: 16),
                        Expanded(child: _buildCardBody(hidden)),
                      ],
                    ),
                  ),
                ),
                Positioned(
                  top: 8,
                  right: 8,
                  child: IconButton(
                    onPressed: _toggleBalance,
                    tooltip: hidden ? 'Show balance' : 'Hide balance',
                    splashRadius: 20,
                    padding: EdgeInsets.zero,
                    constraints:
                        const BoxConstraints(minWidth: 40, minHeight: 40),
                    icon: Icon(
                      hidden
                          ? Icons.visibility_off_outlined
                          : Icons.visibility_outlined,
                      size: 20,
                      color: Colors.white.withValues(alpha: 0.72),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildCardIcon(bool hidden) {
    return Container(
      width: 52,
      height: 52,
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: hidden ? 0.10 : 0.14),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Icon(
        Icons.account_balance_wallet_outlined,
        size: 26,
        color: Colors.white.withValues(alpha: hidden ? 0.55 : 1.0),
      ),
    );
  }

  Widget _buildCardBody(bool hidden) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          'Wallet balance',
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: Colors.white.withValues(alpha: 0.70),
            letterSpacing: 0.1,
          ),
        ),
        const SizedBox(height: 8),
        FittedBox(
          fit: BoxFit.scaleDown,
          alignment: Alignment.centerLeft,
          child: Text(
            hidden ? '₦ ••••••' : _formatNaira(_balance),
            style: const TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w800,
              color: Colors.white,
              letterSpacing: -0.8,
              height: 1.05,
            ),
          ),
        ),
        const SizedBox(height: 8),
        Text(
          hidden ? 'Tap to reveal your balance' : 'Available for withdrawal',
          style: TextStyle(
            fontSize: 12.5,
            fontWeight: FontWeight.w400,
            color: Colors.white.withValues(alpha: 0.65),
          ),
        ),
      ],
    );
  }

  // ── Quick actions ─────────────────────────────────────────────────

  Widget _buildQuickActionsRow() {
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 24, 12, 4),
        child: Row(
          children: [
            Expanded(
              child: _QuickActionIcon(
                icon: Icons.arrow_upward_rounded,
                label: 'Withdraw',
                onTap: _openWithdraw,
                isPrimary: true,
              ),
            ),
            Expanded(
              child: _QuickActionIcon(
                icon: Icons.account_balance_outlined,
                label: 'Add bank',
                onTap: _openAddBank,
                isPrimary: true,
              ),
            ),
            Expanded(
              child: _QuickActionIcon(
                icon: Icons.swap_horiz_rounded,
                label: 'Transactions',
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => const TransactionListScreen(),
                  ),
                ),
                isPrimary: true,
              ),
            ),
            Expanded(
              child: _QuickActionIcon(
                icon: Icons.more_horiz_rounded,
                label: 'More',
                onTap: _openQuickActions,
                isPrimary: false,
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Recent activity ───────────────────────────────────────────────

  Widget _buildRecentActivityHeader() {
    final colors = context.palette;
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 26, 20, 8),
        child: Text(
          'Recent activity',
          style: TextStyle(
            fontSize: 17,
            fontWeight: FontWeight.w700,
            color: colors.text,
            letterSpacing: -0.3,
          ),
        ),
      ),
    );
  }

  Widget _buildRecentActivityList() {
    final items = _activity;

    if (items.isEmpty) return _buildEmptyState();

    final colors = context.palette;
    return SliverList.separated(
      itemCount: items.length,
      separatorBuilder: (_, __) => Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20),
        child: Divider(height: 1, thickness: 1, color: colors.divider),
      ),
      itemBuilder: (context, i) {
        final item = items[i];
        return _ActivityRow(
          item: item,
          onTap: () => _openTransaction(item),
        );
      },
    );
  }

  // ── Empty state ───────────────────────────────────────────────────

  Widget _buildEmptyState() {
    final colors = context.palette;
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 32, 20, 32),
        child: Column(
          children: [
            Container(
              width: 104,
              height: 104,
              decoration: BoxDecoration(
                color: colors.navy10,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.account_balance_wallet_outlined,
                size: 48,
                color: colors.navy40,
              ),
            ),
            const SizedBox(height: 22),
            Text(
              'Your wallet is empty',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w700,
                color: colors.text,
                letterSpacing: -0.3,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'Your earnings will appear here\nafter you complete deliveries.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 14,
                height: 1.45,
                color: colors.textMuted,
              ),
            ),
            const SizedBox(height: 26),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: ElevatedButton(
                onPressed: () {},
                style: ElevatedButton.styleFrom(
                  backgroundColor: colors.brandTeal500,
                  foregroundColor: Colors.white,
                  elevation: 0,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(14),
                  ),
                ),
                child: const Text(
                  'View opportunities',
                  style: TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Formatting helpers ───────────────────────────────────────────────

String _groupThousands(String whole) {
  final buffer = StringBuffer();
  for (int i = 0; i < whole.length; i++) {
    if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
    buffer.write(whole[i]);
  }
  return buffer.toString();
}

String _formatNaira(double value) {
  final parts = value.toStringAsFixed(2).split('.');
  return '₦${_groupThousands(parts[0])}.${parts[1]}';
}

String _formatShort(double v) {
  if (v == v.truncateToDouble()) {
    return _groupThousands(v.toStringAsFixed(0));
  }
  final parts = v.toStringAsFixed(2).split('.');
  return '${_groupThousands(parts[0])}.${parts[1]}';
}

// ── Quick action icon ───────────────────────────────────────────────

class _QuickActionIcon extends StatelessWidget {
  const _QuickActionIcon({
    required this.icon,
    required this.label,
    required this.onTap,
    this.isPrimary = true,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;
  final bool isPrimary;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final bg = isPrimary ? colors.teal10 : colors.navy10;
    final fg = isPrimary ? colors.brandTeal500 : colors.text;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 56,
            height: 56,
            decoration: BoxDecoration(
              color: bg,
              shape: BoxShape.circle,
            ),
            child: Icon(icon, size: 24, color: fg),
          ),
          const SizedBox(height: 10),
          Text(
            label,
            maxLines: 1,
            softWrap: false,
            overflow: TextOverflow.visible,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w600,
              color: colors.text,
              letterSpacing: -0.1,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Activity row ───────────────────────────────────────────────────

class _ActivityRow extends StatelessWidget {
  const _ActivityRow({required this.item, required this.onTap});

  final ActivityItem item;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 13),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: _iconBg(item, colors),
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(
                _icon(item),
                size: 22,
                color: _iconColor(item, colors),
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    item.title,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: colors.text,
                      height: 1.2,
                      letterSpacing: -0.2,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    item.subtitle,
                    style: TextStyle(
                      fontSize: 12.5,
                      fontWeight: FontWeight.w400,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
            Text(
              '${item.isCredit ? '+' : '-'}₦${_formatShort(item.amountNaira)}',
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
                letterSpacing: -0.2,
                color: item.isCredit ? colors.success : colors.danger,
              ),
            ),
          ],
        ),
      ),
    );
  }

  static IconData _icon(ActivityItem i) {
    switch (i.kind) {
      case ActivityKind.delivery:
        return Icons.local_shipping_outlined;
      case ActivityKind.withdrawal:
        return Icons.arrow_upward_rounded;
      case ActivityKind.adjustment:
        return Icons.tune_rounded;
      case ActivityKind.store:
        return Icons.storefront_outlined;
    }
  }

  static Color _iconColor(ActivityItem i, dynamic colors) {
    return i.isCredit ? colors.brandTeal600 : colors.danger;
  }

  static Color _iconBg(ActivityItem i, dynamic colors) {
    return i.isCredit ? colors.teal10 : colors.dangerSoft;
  }
}