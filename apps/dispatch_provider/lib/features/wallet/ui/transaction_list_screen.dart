import 'package:flutter/material.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../mock/wallet_tab_mock.dart';
import 'transaction_detail_screen.dart';

class TransactionListScreen extends StatefulWidget {
  const TransactionListScreen({super.key});

  @override
  State<TransactionListScreen> createState() => _TransactionListScreenState();
}

class _TransactionListScreenState extends State<TransactionListScreen> {
  int _tab = 0; // 0=All · 1=Credits · 2=Debits

  List<TransactionItem> get _items {
    final all = AppModeConfig.instance.isMock
        ? WalletTabMock.allTransactions
        : <TransactionItem>[];

    switch (_tab) {
      case 1:
        return all.where((t) => t.isCredit).toList();
      case 2:
        return all.where((t) => !t.isCredit).toList();
      default:
        return all;
    }
  }

  Map<String, List<TransactionItem>> get _grouped {
    final map = <String, List<TransactionItem>>{};
    for (final item in _items) {
      map.putIfAbsent(item.monthLabel, () => []).add(item);
    }
    return map;
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildTopBar(),
            const SizedBox(height: 16),
            _buildTabs(),
            const SizedBox(height: 8),
            Expanded(child: _buildList()),
          ],
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar() {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 12, 20, 0),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 24,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Transactions',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.3,
            ),
          ),
          Align(
            alignment: Alignment.centerRight,
            child: GestureDetector(
              onTap: () {},
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.tune_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Tabs row ──────────────────────────────────────────────────────

  Widget _buildTabs() {
    const labels = ['All', 'Credits', 'Debits'];
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Row(
        children: [
          for (int i = 0; i < labels.length; i++) ...[
            _TabChip(
              label: labels[i],
              isActive: _tab == i,
              onTap: () => setState(() => _tab = i),
            ),
            const SizedBox(width: 8),
          ],
          const Spacer(),
          _FilterChip(onTap: () {}),
        ],
      ),
    );
  }

  // ── List with month grouping ──────────────────────────────────────

  Widget _buildList() {
    final grouped = _grouped;

    if (grouped.isEmpty) return _EmptyState(tab: _tab);

    final children = <Widget>[];
    grouped.forEach((month, items) {
      children.add(_MonthHeader(label: month));
      for (final item in items) {
        children.add(
          _TransactionRow(
            item: item,
            onTap: () => Navigator.of(context).push(
              MaterialPageRoute(
                builder: (_) => TransactionDetailScreen(
                  transaction: TransactionData(
                    id: item.id,
                    amount: item.amountNaira,
                    commission: 0,
                    total: item.amountNaira,
                    status: TransactionStatus.successful,
                    type: item.isCredit
                        ? TransactionType.trip
                        : TransactionType.debit,
                    customerName: item.title,
                    date: item.subtitle,
                  ),
                ),
              ),
            ),
          ),
        );
      }
    });

    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
      children: children,
    );
  }
}

// ── Tab chip ────────────────────────────────────────────────────────
//
// Active state uses the brand's soft teal tint — quiet, on-brand, and
// consistent with the "Default" badge and other accent treatments used
// across the wallet screens. Doesn't scream like a solid white pill.

class _TabChip extends StatelessWidget {
  const _TabChip({
    required this.label,
    required this.isActive,
    required this.onTap,
  });

  final String label;
  final bool isActive;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 9),
        decoration: BoxDecoration(
          color: isActive ? colors.teal10 : colors.surface,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(
            color: isActive
                ? colors.brandTeal500.withValues(alpha: 0.5)
                : colors.divider,
            width: 1.2,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: isActive ? colors.brandTeal600 : colors.text,
          ),
        ),
      ),
    );
  }
}

// ── Filter chip ─────────────────────────────────────────────────────

class _FilterChip extends StatelessWidget {
  const _FilterChip({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: colors.divider, width: 1.2),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.filter_alt_outlined,
              size: 15,
              color: colors.text,
            ),
            const SizedBox(width: 5),
            Text(
              'Filters',
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: colors.text,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Month header ────────────────────────────────────────────────────

class _MonthHeader extends StatelessWidget {
  const _MonthHeader({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(2, 14, 2, 10),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 13,
          fontWeight: FontWeight.w700,
          color: colors.textMuted,
          letterSpacing: -0.1,
        ),
      ),
    );
  }
}

// ── Transaction row ─────────────────────────────────────────────────

class _TransactionRow extends StatelessWidget {
  const _TransactionRow({required this.item, required this.onTap});

  final TransactionItem item;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final style = _styleFor(item.kind, colors);

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 11),
        child: Row(
          children: [
            Container(
              width: 42,
              height: 42,
              decoration: BoxDecoration(
                color: style.bg,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(style.icon, size: 20, color: style.fg),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    item.title,
                    style: TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w600,
                      color: colors.text,
                      height: 1.2,
                      letterSpacing: -0.1,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    item.subtitle,
                    style: TextStyle(
                      fontSize: 12.5,
                      color: colors.textMuted,
                      height: 1.2,
                    ),
                  ),
                ],
              ),
            ),
            Text(
              '${item.isCredit ? '+' : '-'}₦${_formatShort(item.amountNaira)}',
              style: TextStyle(
                fontSize: 14.5,
                fontWeight: FontWeight.w700,
                letterSpacing: -0.1,
                color: item.isCredit ? colors.success : colors.danger,
              ),
            ),
          ],
        ),
      ),
    );
  }

  static _IconStyle _styleFor(TransactionKind kind, dynamic colors) {
    switch (kind) {
      case TransactionKind.delivery:
      case TransactionKind.store:
        return _IconStyle(
          icon: Icons.inventory_2_outlined,
          fg: colors.success,
          bg: colors.successSoft,
        );
      case TransactionKind.withdrawal:
        return _IconStyle(
          icon: Icons.arrow_upward_rounded,
          fg: colors.danger,
          bg: colors.dangerSoft,
        );
      case TransactionKind.adjustment:
        return _IconStyle(
          icon: Icons.tune_rounded,
          fg: colors.textMuted,
          bg: colors.navy10,
        );
    }
  }

  static String _formatShort(double v) {
    if (v == v.truncateToDouble()) {
      return _groupThousands(v.toStringAsFixed(0));
    }
    final parts = v.toStringAsFixed(2).split('.');
    return '${_groupThousands(parts[0])}.${parts[1]}';
  }

  static String _groupThousands(String whole) {
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }
}

class _IconStyle {
  const _IconStyle({required this.icon, required this.fg, required this.bg});
  final IconData icon;
  final Color fg;
  final Color bg;
}

// ── Empty state ─────────────────────────────────────────────────────

class _EmptyState extends StatelessWidget {
  const _EmptyState({required this.tab});
  final int tab;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final label = switch (tab) {
      1 => 'No credits yet',
      2 => 'No debits yet',
      _ => 'No transactions yet',
    };
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 88,
            height: 88,
            decoration: BoxDecoration(
              color: colors.navy10,
              shape: BoxShape.circle,
            ),
            child: Icon(
              Icons.receipt_long_outlined,
              size: 40,
              color: colors.navy40,
            ),
          ),
          const SizedBox(height: 18),
          Text(
            label,
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            'Your transaction history will appear here.',
            style: TextStyle(fontSize: 14, color: colors.textMuted),
          ),
        ],
      ),
    );
  }
}