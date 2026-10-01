import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../mock/wallet_tab_mock.dart';
import '../state/wallet_controller.dart';
import 'earnings_details_screen.dart';
import 'period_filter_sheet.dart';

class EarningSummaryScreen extends StatefulWidget {
  final WalletController walletController;

  const EarningSummaryScreen({super.key, required this.walletController});

  @override
  State<EarningSummaryScreen> createState() => _EarningSummaryScreenState();
}

class _EarningSummaryScreenState extends State<EarningSummaryScreen> {
  bool _visible = true;
  String _period = 'This month';

  WalletController get _controller => widget.walletController;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _controller.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
  }

  Future<void> _openPeriodFilter() async {
    final picked = await PeriodFilterSheet.show(context, selected: _period);
    if (picked != null && mounted) {
      setState(() => _period = picked);
    }
  }

  void _openDayDetails(String month) {
    if (WalletTabMock.dailyBreakdown.isEmpty) return;
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            EarningsDetailsScreen(day: WalletTabMock.dailyBreakdown.first),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _buildTopBar(context),
              const SizedBox(height: 20),
              _buildTotalRow(context),
              const SizedBox(height: 22),
              _buildStatCards(context),
              const SizedBox(height: 26),
              _buildChartCard(context),
            ],
          ),
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Row(
      children: [
        GestureDetector(
          onTap: () => Navigator.of(context).pop(),
          behavior: HitTestBehavior.opaque,
          child: Padding(
            padding: const EdgeInsets.all(6),
            child: Icon(
              Icons.arrow_back_rounded,
              size: 22,
              color: colors.text,
            ),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            'Earnings',
            style: TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.4,
            ),
          ),
        ),
        GestureDetector(
          onTap: () => setState(() => _visible = !_visible),
          behavior: HitTestBehavior.opaque,
          child: Padding(
            padding: const EdgeInsets.all(6),
            child: Icon(
              _visible
                  ? Icons.visibility_outlined
                  : Icons.visibility_off_outlined,
              size: 22,
              color: colors.text,
            ),
          ),
        ),
      ],
    );
  }

  // ── Total earnings row ────────────────────────────────────────────

  Widget _buildTotalRow(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Total earnings',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: colors.textMuted,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                _visible ? '₦424,350' : '₦ ••••••',
                style: TextStyle(
                  fontSize: 34,
                  fontWeight: FontWeight.w800,
                  color: colors.brandTeal500,
                  letterSpacing: -1.0,
                  height: 1.0,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 12),
        _PeriodChip(label: _period, onTap: _openPeriodFilter),
      ],
    );
  }

  // ── Two stat cards ────────────────────────────────────────────────

  Widget _buildStatCards(BuildContext context) {
    return const Row(
      children: [
        Expanded(
          child: _StatCard(value: '48', label: 'Deliveries'),
        ),
        SizedBox(width: 12),
        Expanded(
          child: _StatCard(value: '₦8,840', label: 'Avg. per delivery'),
        ),
      ],
    );
  }

  // ── Chart card ────────────────────────────────────────────────────

  Widget _buildChartCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 18, 16, 16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Earnings',
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            'Last 5 months',
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w400,
              color: colors.textMuted,
            ),
          ),
          const SizedBox(height: 20),
          _BarChart(
            data: WalletTabMock.earningsByMonth,
            onBarTap: _openDayDetails,
          ),
        ],
      ),
    );
  }
}

// ── Period chip ─────────────────────────────────────────────────────

class _PeriodChip extends StatelessWidget {
  const _PeriodChip({required this.label, required this.onTap});

  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(999),
          border: Border.all(color: colors.divider, width: 1.2),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: colors.text,
              ),
            ),
            const SizedBox(width: 4),
            Icon(
              Icons.keyboard_arrow_down_rounded,
              size: 16,
              color: colors.text,
            ),
          ],
        ),
      ),
    );
  }
}

// ── Stat card ───────────────────────────────────────────────────────

class _StatCard extends StatelessWidget {
  const _StatCard({required this.value, required this.label});

  final String value;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            value,
            style: TextStyle(
              fontSize: 22,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.6,
              height: 1.0,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            label,
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w500,
              color: colors.textMuted,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Bar chart ───────────────────────────────────────────────────────

class _BarChart extends StatelessWidget {
  const _BarChart({required this.data, required this.onBarTap});

  final Map<String, double> data;
  final ValueChanged<String> onBarTap;

  static const double _maxBarHeight = 120.0;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final values = data.values.toList();
    final maxValue =
        values.isEmpty ? 0.0 : values.reduce((a, b) => a > b ? a : b);
    final entries = data.entries.toList();

    return SizedBox(
      height: _maxBarHeight + 32,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: entries.map((entry) {
          final ratio = maxValue <= 0 ? 0.0 : entry.value / maxValue;
          final barHeight = _maxBarHeight * ratio;

          return Expanded(
            child: GestureDetector(
              onTap: () => onBarTap(entry.key),
              behavior: HitTestBehavior.opaque,
              child: Column(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  Container(
                    height: barHeight,
                    margin: const EdgeInsets.symmetric(horizontal: 6),
                    decoration: BoxDecoration(
                      color: colors.brandTeal500,
                      borderRadius: const BorderRadius.vertical(
                        top: Radius.circular(6),
                      ),
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    entry.key,
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
          );
        }).toList(),
      ),
    );
  }
}