import 'package:flutter/material.dart';

import '../../../core/theme/dispatch_colors.dart';
import '../mock/wallet_tab_mock.dart';

class AdjustmentDetailScreen extends StatelessWidget {
  const AdjustmentDetailScreen({super.key, this.adjustment});

  final AdjustmentDetail? adjustment;

  @override
  Widget build(BuildContext context) {
    final a = adjustment ?? WalletTabMock.adjustmentDetail;

    return Scaffold(
      backgroundColor: DispatchColors.brandWhite,
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
                    _buildHero(),
                    const SizedBox(height: 32),
                    _buildDetails(a),
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
              child: const Padding(
                padding: EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: DispatchColors.brandNavy900,
                ),
              ),
            ),
          ),
          const Text(
            'Adjustment detail',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w700,
              color: DispatchColors.brandNavy900,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  // ── Hero: circle icon + "Earnings adjustment" + amount + status ───

  Widget _buildHero() {
    return Column(
      children: [
        Container(
          width: 88,
          height: 88,
          decoration: const BoxDecoration(
            color: Color(0xFFE8F0FE),
            shape: BoxShape.circle,
          ),
          child: const Icon(
            Icons.tune_rounded,
            size: 40,
            color: Color(0xFF3B82F6),
          ),
        ),
        const SizedBox(height: 22),
        const Text(
          'Earnings adjustment',
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w500,
            color: DispatchColors.navy60,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          '+₦${_formatAmount(WalletTabMock.adjustmentDetail.amountNaira)}',
          style: const TextStyle(
            fontSize: 34,
            fontWeight: FontWeight.w800,
            color: Color(0xFF0E9F6E),
            letterSpacing: -0.8,
            height: 1.05,
          ),
        ),
        const SizedBox(height: 14),
        _buildStatusPill(),
      ],
    );
  }

  Widget _buildStatusPill() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: const Color(0xFFD1FAE5),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 14,
            height: 14,
            decoration: const BoxDecoration(
              color: Color(0xFF0E9F6E),
              shape: BoxShape.circle,
            ),
            child: const Icon(
              Icons.check_rounded,
              size: 10,
              color: Colors.white,
            ),
          ),
          const SizedBox(width: 6),
          const Text(
            'Completed',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w700,
              color: Color(0xFF0E9F6E),
            ),
          ),
        ],
      ),
    );
  }

  // ── Details ──────────────────────────────────────────────────────

  Widget _buildDetails(AdjustmentDetail a) {
    return Column(
      children: [
        _row('Reference', a.reference),
        _row('Date', a.date),
        _row('Type', a.type),
        _row('Reason', a.reason),
        _row('Status', a.status, valueColor: const Color(0xFF0E9F6E)),
      ],
    );
  }

  Widget _row(String label, String value, {Color? valueColor}) {
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
      decoration: BoxDecoration(
        color: DispatchColors.brandWhite,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: DispatchColors.navy20, width: 1),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          SizedBox(
            width: 110,
            child: Text(
              label,
              style: const TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: DispatchColors.navy60,
                height: 1.3,
              ),
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              value,
              textAlign: TextAlign.right,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: valueColor ?? DispatchColors.brandNavy900,
                height: 1.3,
              ),
            ),
          ),
        ],
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