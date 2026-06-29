import 'package:flutter/material.dart';
import 'dart:math' as math;

class WithdrawalReceiptScreen extends StatelessWidget {
  const WithdrawalReceiptScreen({super.key, required this.amount});

  final double amount;

  static const _accountName = 'Firepemi Adewale';
  static const _bankName = 'Wema Bank';
  static const _accountNumber = '0450908723';

  static String _formatAmount(double value) {
    final s = value.toStringAsFixed(2);
    final parts = s.split('.');
    final whole = parts[0];
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return '₦ $buffer.${parts[1]}';
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFF1B5E20),
      body: SafeArea(
        child: Column(
          children: [
            const SizedBox(height: 24),
            // title
            const Text(
              'Withdrawal Receipt',
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w800,
                color: Colors.white,
              ),
            ),
            const SizedBox(height: 24),
            // receipt card
            Expanded(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: CustomPaint(
                  painter: _ReceiptPainter(),
                  child: Padding(
                    padding: const EdgeInsets.fromLTRB(20, 32, 20, 48),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        // checkmark badge
                        _OctagonBadge(),
                        const SizedBox(height: 20),
                        const Text(
                          'Withdrawal Success!',
                          style: TextStyle(
                            fontSize: 18,
                            fontWeight: FontWeight.w800,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 6),
                        const Text(
                          'Your withdrawal has successfully been processed.',
                          textAlign: TextAlign.center,
                          style: TextStyle(
                            fontSize: 13,
                            color: Color(0xFF888888),
                          ),
                        ),
                        const SizedBox(height: 20),
                        const Text(
                          'Total Amount',
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w700,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          _formatAmount(amount),
                          style: const TextStyle(
                            fontSize: 28,
                            fontWeight: FontWeight.w800,
                            color: Color(0xFF4CAF50),
                          ),
                        ),
                        const SizedBox(height: 20),
                        // dashed divider
                        _DashedDivider(),
                        const SizedBox(height: 20),
                        // payment to section
                        Align(
                          alignment: Alignment.centerLeft,
                          child: const Text(
                            'Payment To',
                            style: TextStyle(
                              fontSize: 15,
                              fontWeight: FontWeight.w700,
                              color: Color(0xFF1A1A1A),
                            ),
                          ),
                        ),
                        const SizedBox(height: 12),
                        Container(
                          width: double.infinity,
                          padding: const EdgeInsets.symmetric(
                            horizontal: 14,
                            vertical: 14,
                          ),
                          decoration: BoxDecoration(
                            color: const Color(0xFFF5F5F5),
                            borderRadius: BorderRadius.circular(12),
                          ),
                          child: Row(
                            children: [
                              Container(
                                width: 44,
                                height: 44,
                                decoration: BoxDecoration(
                                  color: Colors.white,
                                  shape: BoxShape.circle,
                                  border: Border.all(
                                    color: const Color(0xFFDDDDDD),
                                  ),
                                ),
                                child: const Icon(
                                  Icons.arrow_back,
                                  size: 20,
                                  color: Color(0xFF4CAF50),
                                ),
                              ),
                              const SizedBox(width: 12),
                              Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  const Text(
                                    _accountName,
                                    style: TextStyle(
                                      fontSize: 14,
                                      fontWeight: FontWeight.w700,
                                      color: Color(0xFF1A1A1A),
                                    ),
                                  ),
                                  const SizedBox(height: 2),
                                  const Text(
                                    _bankName,
                                    style: TextStyle(
                                      fontSize: 13,
                                      fontWeight: FontWeight.w700,
                                      color: Color(0xFF1A1A1A),
                                    ),
                                  ),
                                  const SizedBox(height: 2),
                                  const Text(
                                    _accountNumber,
                                    style: TextStyle(
                                      fontSize: 12,
                                      color: Color(0xFF888888),
                                    ),
                                  ),
                                ],
                              ),
                            ],
                          ),
                        ),
                        const Spacer(),
                        // Done button
                        GestureDetector(
                          onTap: () => Navigator.of(
                            context,
                          ).popUntil((route) => route.isFirst),
                          child: Container(
                            width: double.infinity,
                            height: 52,
                            alignment: Alignment.center,
                            decoration: BoxDecoration(
                              color: const Color(0xFF4CAF50),
                              borderRadius: BorderRadius.circular(999),
                            ),
                            child: const Text(
                              'Done',
                              style: TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w700,
                                color: Colors.white,
                              ),
                            ),
                          ),
                        ),
                        const SizedBox(height: 16),
                        GestureDetector(
                          onTap: () {
                            // TODO: implement download receipt
                          },
                          child: const Text(
                            'Download Receipt',
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: Color(0xFF1A1A1A),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    );
  }
}

// ── Receipt shape with wavy bottom ───────────────────────────────────────────

class _ReceiptPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    const radius = 16.0;
    const waveHeight = 10.0;
    const waveCount = 14;

    final paint = Paint()
      ..color = Colors.white
      ..style = PaintingStyle.fill;

    final path = Path();

    // top-left corner
    path.moveTo(radius, 0);
    // top edge
    path.lineTo(size.width - radius, 0);
    // top-right corner
    path.arcToPoint(
      Offset(size.width, radius),
      radius: const Radius.circular(radius),
    );
    // right edge
    path.lineTo(size.width, size.height - waveHeight * 2);
    // wavy bottom (right to left)
    final waveWidth = size.width / waveCount;
    for (int i = waveCount; i > 0; i--) {
      final x1 = waveWidth * i - waveWidth * 0.75;
      final x2 = waveWidth * i - waveWidth * 0.25;
      final xEnd = waveWidth * (i - 1);
      final yMid = size.height - waveHeight;
      final yEnd = size.height - waveHeight * 2;
      path.cubicTo(x1, yMid + waveHeight, x2, yMid + waveHeight, xEnd, yEnd);
    }
    // left edge
    path.lineTo(0, radius);
    // top-left corner close
    path.arcToPoint(Offset(radius, 0), radius: const Radius.circular(radius));
    path.close();

    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter old) => false;
}

// ── Dashed divider ────────────────────────────────────────────────────────────

class _DashedDivider extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 1,
      child: CustomPaint(
        painter: _DashedLinePainter(),
        size: const Size(double.infinity, 1),
      ),
    );
  }
}

class _DashedLinePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = const Color(0xFFDDDDDD)
      ..strokeWidth = 1.5;
    double x = 0;
    while (x < size.width) {
      canvas.drawLine(Offset(x, 0), Offset(x + 8, 0), paint);
      x += 14;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter old) => false;
}

// ── Octagon badge ─────────────────────────────────────────────────────────────

class _OctagonBadge extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 90,
      height: 90,
      child: CustomPaint(
        painter: _OctagonPainter(),
        child: const Center(
          child: Icon(Icons.check, color: Colors.white, size: 38),
        ),
      ),
    );
  }
}

class _OctagonPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final cx = size.width / 2;
    final cy = size.height / 2;
    final r = size.width / 2;

    // fill
    final fillPaint = Paint()..color = const Color(0xFFB7E4C7);
    // stroke
    final strokePaint = Paint()
      ..color = const Color(0xFF4CAF50)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3;

    final path = Path();
    for (int i = 0; i < 8; i++) {
      final angle = (math.pi / 8) + (i * math.pi / 4);
      final x = cx + r * math.cos(angle);
      final y = cy + r * math.sin(angle);
      if (i == 0) {
        path.moveTo(x, y);
      } else {
        path.lineTo(x, y);
      }
    }
    path.close();

    canvas.drawPath(path, fillPaint);
    canvas.drawPath(path, strokePaint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter old) => false;
}
