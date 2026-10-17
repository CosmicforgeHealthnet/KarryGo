import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

// ── Data model (kept — wallet_screen.dart depends on this shape) ─────────

enum TransactionStatus { successful, failed, pending }

enum TransactionType { trip, debit }

class TransactionData {
  const TransactionData({
    required this.id,
    required this.amount,
    required this.commission,
    required this.total,
    required this.status,
    required this.type,
    this.customerName,
    this.customerAvatarUrl,
    this.distanceKm,
    this.durationMin,
    this.tripFee,
    this.tripReference,
    this.date,
    this.pickup,
    this.dropoff,
  });

  final String id;
  final double amount;
  final double commission;
  final double total;
  final TransactionStatus status;
  final TransactionType type;
  final String? customerName;
  final String? customerAvatarUrl;
  final int? distanceKm;
  final int? durationMin;
  final double? tripFee;
  final String? tripReference;
  final String? date;
  final String? pickup;
  final String? dropoff;

  bool get isTripLinked => type == TransactionType.trip;
}

// ── Screen ────────────────────────────────────────────────────────────────

class TransactionDetailScreen extends StatelessWidget {
  const TransactionDetailScreen({super.key, required this.transaction});

  final TransactionData transaction;

  bool get _isCredit => transaction.type == TransactionType.trip;

  IconData get _icon => transaction.type == TransactionType.debit
      ? Icons.arrow_upward_rounded
      : Icons.inventory_2_outlined;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
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
                    _buildHeroCard(context),
                    const SizedBox(height: 24),
                    _buildDetails(context),
                    const SizedBox(height: 32),
                    _buildShareButton(context),
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
            child: DispatchBackButton(
              onPressed: () => Navigator.of(context).pop(),
            ),
          ),
          Text(
            'Transaction detail',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          Align(
            alignment: Alignment.centerRight,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(Icons.close_rounded, size: 22, color: colors.text),
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Hero card ─────────────────────────────────────────────────────

  Widget _buildHeroCard(BuildContext context) {
    final colors = context.palette;
    final iconFg = _isCredit ? colors.success : colors.danger;
    final iconBg = _isCredit ? colors.successSoft : colors.dangerSoft;
    final amountColor = _isCredit ? colors.success : colors.danger;

    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider, width: 1),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 56,
            height: 56,
            decoration: BoxDecoration(color: iconBg, shape: BoxShape.circle),
            child: Icon(_icon, size: 26, color: iconFg),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  transaction.customerName ?? 'Transaction',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: colors.text,
                    letterSpacing: -0.1,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  '${_isCredit ? '+' : '-'} ₦${_formatAmount(transaction.amount)}',
                  style: TextStyle(
                    fontSize: 28,
                    fontWeight: FontWeight.w700,
                    color: amountColor,
                    letterSpacing: -0.8,
                    height: 1.0,
                  ),
                ),
                const SizedBox(height: 10),
                _buildStatusPill(context),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildStatusPill(BuildContext context) {
    final colors = context.palette;
    final Color fg;
    final Color bg;
    final String label;
    final IconData icon;

    switch (transaction.status) {
      case TransactionStatus.successful:
        fg = colors.success;
        bg = colors.successSoft;
        label = 'Completed';
        icon = Icons.check_rounded;
        break;
      case TransactionStatus.failed:
        fg = colors.danger;
        bg = colors.dangerSoft;
        label = 'Failed';
        icon = Icons.close_rounded;
        break;
      case TransactionStatus.pending:
        fg = colors.warning;
        bg = colors.warningSoft;
        label = 'Pending';
        icon = Icons.access_time_rounded;
        break;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 14,
            height: 14,
            alignment: Alignment.center,
            decoration: BoxDecoration(color: fg, shape: BoxShape.circle),
            child: Icon(icon, size: 9, color: Colors.white),
          ),
          const SizedBox(width: 6),
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w700,
              color: fg,
            ),
          ),
        ],
      ),
    );
  }

  // ── Details: flat rows with thin dividers, no per-row borders ─────

  Widget _buildDetails(BuildContext context) {
    final colors = context.palette;
    return Column(
      children: [
        _row(context, 'Transaction ID', transaction.id),
        _divider(colors),
        _row(context, 'TX Hash', _mockTxHash),
        _divider(colors),
        _row(context, 'Type', _typeLabel),
        _divider(colors),
        _row(context, 'Description', 'Delivery to Victoria Island'),
        _divider(colors),
        _row(context, 'From', 'Karryam'),
        _divider(colors),
        _row(context, 'Channel', 'Wallet'),
        _divider(colors),
        _row(context, 'Fee', '₦0.00'),
        _divider(colors),
        _row(
          context,
          'Status',
          _statusLabel,
          valueColor: _statusValueColor(colors),
        ),
      ],
    );
  }

  Widget _divider(dynamic colors) =>
      Divider(height: 1, thickness: 1, color: colors.divider);

  String get _mockTxHash => '0x7a3f4b2e8c9d1f6a';

  String get _typeLabel => transaction.type == TransactionType.trip
      ? 'Credit · Delivery payment'
      : 'Debit · Withdrawal';

  String get _statusLabel => switch (transaction.status) {
    TransactionStatus.successful => 'Completed',
    TransactionStatus.failed => 'Failed',
    TransactionStatus.pending => 'Pending',
  };

  Color _statusValueColor(dynamic colors) => switch (transaction.status) {
    TransactionStatus.successful => colors.success,
    TransactionStatus.failed => colors.danger,
    TransactionStatus.pending => colors.warning,
  };

  Widget _row(
    BuildContext context,
    String label,
    String value, {
    Color? valueColor,
  }) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 14),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(
            flex: 4,
            child: Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: colors.textMuted,
                height: 1.3,
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            flex: 6,
            child: Text(
              value,
              textAlign: TextAlign.right,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: valueColor ?? colors.text,
                height: 1.3,
                letterSpacing: -0.1,
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Share button ──────────────────────────────────────────────────

  Widget _buildShareButton(BuildContext context) {
    final colors = context.palette;
    return SizedBox(
      width: double.infinity,
      height: 54,
      child: OutlinedButton.icon(
        onPressed: () {},
        icon: Icon(
          Icons.ios_share_rounded,
          size: 18,
          color: colors.brandTeal500,
        ),
        label: Text(
          'Share receipt',
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: colors.brandTeal500,
          ),
        ),
        style: OutlinedButton.styleFrom(
          backgroundColor: colors.surface,
          side: BorderSide(color: colors.brandTeal500, width: 1.4),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
        ),
      ),
    );
  }

  // ── Formatting ────────────────────────────────────────────────────

  static String _formatAmount(double v) {
    final whole = v.toStringAsFixed(0);
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }
}
