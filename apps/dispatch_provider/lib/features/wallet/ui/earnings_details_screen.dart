import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../mock/wallet_tab_mock.dart';
import 'adjustment_detail_screen.dart';

class EarningsDetailsScreen extends StatelessWidget {
  const EarningsDetailsScreen({super.key, required this.day});

  final DailyEarning day;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final deliveries = _mockDeliveriesFor(day);

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    _buildDayHeader(context),
                    const SizedBox(height: 20),
                    _buildSummaryRow(context),
                    const SizedBox(height: 24),
                    Text(
                      'Deliveries',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                        color: colors.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                    const SizedBox(height: 12),
                    ...deliveries.map((d) => _DeliveryRow(item: d)),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
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
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Earnings details',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  // ── Day header ────────────────────────────────────────────────────

  Widget _buildDayHeader(BuildContext context) {
    final colors = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          day.date,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: colors.textMuted,
          ),
        ),
        const SizedBox(height: 6),
        Text(
          '₦${_formatAmount(day.amountNaira)}',
          style: TextStyle(
            fontSize: 32,
            fontWeight: FontWeight.w800,
            color: colors.brandTeal500,
            letterSpacing: -0.8,
            height: 1.0,
          ),
        ),
      ],
    );
  }

  // ── Summary stats ─────────────────────────────────────────────────

  Widget _buildSummaryRow(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: _MiniStat(
            label: 'Deliveries',
            value: '${day.deliveries}',
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: _MiniStat(
            label: 'Avg. per delivery',
            value: '₦${_formatAmount(day.deliveries == 0 ? 0 : day.amountNaira / day.deliveries)}',
          ),
        ),
      ],
    );
  }

  // ── Fake delivery breakdown for a given day ───────────────────────

  List<_DeliveryEntry> _mockDeliveriesFor(DailyEarning d) {
    if (d.deliveries <= 0) return const [];
    final per = d.amountNaira / d.deliveries;
    return List.generate(
      d.deliveries,
      (i) => _DeliveryEntry(
        time: _timeFor(i),
        reference: 'KAR-${280000 + i * 7 + d.deliveries}',
        amountNaira: per,
      ),
    );
  }

  static String _timeFor(int i) {
    const times = ['09:12 AM', '11:40 AM', '02:18 PM', '04:55 PM'];
    return times[i % times.length];
  }

  static String _formatAmount(double v) {
    final whole = v.round().toString();
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }
}

// ── Mini stat card ──────────────────────────────────────────────────

class _MiniStat extends StatelessWidget {
  const _MiniStat({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            value,
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.4,
              height: 1.0,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w500,
              color: colors.textMuted,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Delivery row ────────────────────────────────────────────────────

class _DeliveryEntry {
  const _DeliveryEntry({
    required this.time,
    required this.reference,
    required this.amountNaira,
  });

  final String time;
  final String reference;
  final double amountNaira;
}

class _DeliveryRow extends StatelessWidget {
  const _DeliveryRow({required this.item});

  final _DeliveryEntry item;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: () => Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => const AdjustmentDetailScreen(),
        ),
      ),
      behavior: HitTestBehavior.opaque,
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: colors.divider, width: 1),
        ),
        child: Row(
          children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: colors.teal10,
                borderRadius: BorderRadius.circular(11),
              ),
              child: Icon(
                Icons.local_shipping_outlined,
                size: 20,
                color: colors.brandTeal500,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Delivery ${item.reference}',
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                      color: colors.text,
                      height: 1.2,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    item.time,
                    style: TextStyle(
                      fontSize: 12.5,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
            Text(
              '+₦${_formatAmount(item.amountNaira)}',
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w700,
                color: colors.success,
              ),
            ),
          ],
        ),
      ),
    );
  }

  static String _formatAmount(double v) {
    final whole = v.round().toString();
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }
}