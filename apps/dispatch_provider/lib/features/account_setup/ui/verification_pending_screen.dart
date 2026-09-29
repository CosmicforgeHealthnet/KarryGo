import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

class VerificationPendingScreen extends StatelessWidget {
  const VerificationPendingScreen({
    super.key,
    required this.onGoToDashboard,
    this.verificationController,
  });

  final VoidCallback onGoToDashboard;
  final dynamic verificationController;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Expanded(
                child: SingleChildScrollView(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const SizedBox(height: 8),
                      const Center(child: _SuccessBurst()),
                      const SizedBox(height: 24),
                      Text(
                        'Setup submitted!',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w700,
                          color: c.text,
                          letterSpacing: -0.4,
                          height: 1.2,
                        ),
                      ),
                      const SizedBox(height: 16),
                      Center(
                        child: ConstrainedBox(
                          constraints: const BoxConstraints(maxWidth: 280),
                          child: Text(
                            "Your information has been sent for verification. We'll review your details and notify you once you're all set.",
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              fontSize: 17,
                              fontWeight: FontWeight.w400,
                              color: c.textMuted,
                              height: 1.6,
                            ),
                          ),
                        ),
                      ),
                      const SizedBox(height: 24),
                      Container(
                        padding: const EdgeInsets.all(20),
                        decoration: BoxDecoration(
                          color: c.navy10,
                          borderRadius: BorderRadius.circular(18),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              'What happens next?',
                              style: TextStyle(
                                fontSize: 17,
                                fontWeight: FontWeight.w700,
                                color: c.text,
                                height: 1.2,
                              ),
                            ),
                            const SizedBox(height: 20),
                            const _NextStep(
                              number: '1',
                              text:
                                  'We review your information (usually within 24 hours)',
                            ),
                            const SizedBox(height: 20),
                            const _NextStep(
                              number: '2',
                              text: "You'll receive a notification",
                            ),
                            const SizedBox(height: 20),
                            const _NextStep(
                              number: '3',
                              text: 'Start receiving opportunities',
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 20),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 8),
              SizedBox(
                height: 58,
                child: FilledButton(
                  onPressed: onGoToDashboard,
                  style: FilledButton.styleFrom(
                    backgroundColor: c.btnPrimary,
                    foregroundColor: c.btnPrimaryFg,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: Text(
                    'Back to sign in',
                    style: TextStyle(
                      fontSize: 17,
                      fontWeight: FontWeight.w700,
                      color: c.btnPrimaryFg,
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 20),
            ],
          ),
        ),
      ),
    );
  }
}

class _NextStep extends StatelessWidget {
  const _NextStep({required this.number, required this.text});

  final String number;
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Container(
          width: 44,
          height: 44,
          decoration: BoxDecoration(
            color: c.brandTeal500,
            shape: BoxShape.circle,
          ),
          child: Center(
            child: Text(
              number,
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w700,
                color: c.btnPrimaryFg,
              ),
            ),
          ),
        ),
        const SizedBox(width: 16),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w400,
              color: c.textMuted,
              height: 1.35,
            ),
          ),
        ),
      ],
    );
  }
}

class _SuccessBurst extends StatelessWidget {
  const _SuccessBurst();

  static const double _width = 300;
  static const double _height = 220;
  static const double _centerY = 128;
  static const double _diameter = 104;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return SizedBox(
      width: _width,
      height: _height,
      child: Stack(
        children: [
          const Positioned.fill(
            child: CustomPaint(
              painter: _ConfettiPainter(
                center: Offset(_width / 2, _centerY),
              ),
            ),
          ),
          Positioned(
            left: (_width - _diameter) / 2,
            top: _centerY - _diameter / 2,
            child: Container(
              width: _diameter,
              height: _diameter,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                gradient: LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [c.brandTeal500, c.brandTeal600],
                ),
                boxShadow: [
                  BoxShadow(
                    color: c.brandTeal500.withValues(alpha: 0.20),
                    blurRadius: 24,
                    offset: const Offset(0, 8),
                  ),
                ],
              ),
              child: const CustomPaint(painter: _CheckPainter()),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Painters (decorative — theme-agnostic) ────────────────────────────────

class _CheckPainter extends CustomPainter {
  const _CheckPainter();

  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width / 104;
    final path = Path()
      ..moveTo(28 * s, 55 * s)
      ..lineTo(45 * s, 72 * s)
      ..lineTo(78 * s, 34 * s);
    canvas.drawPath(
      path,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 9 * s
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round,
    );
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

class _ConfettiPiece {
  const _ConfettiPiece(
    this.dx,
    this.dy,
    this.length,
    this.thickness,
    this.angle,
    this.color,
  );

  final double dx;
  final double dy;
  final double length;
  final double thickness;
  final double angle;
  final Color color;
}

class _ConfettiPainter extends CustomPainter {
  const _ConfettiPainter({required this.center});

  final Offset center;

  static const Color _gold = Color(0xFFF5B800);
  static const Color _blue = Color(0xFF2563EB);
  static const Color _teal = Color(0xFF14B8A6);
  static const Color _cyan = Color(0xFF1094AA);
  static const Color _red = Color(0xFFE5484D);
  static const Color _orange = Color(0xFFF59E0B);

  static const List<_ConfettiPiece> _pieces = [
    _ConfettiPiece(-84, 71, 24, 8, 129, _cyan),
    _ConfettiPiece(81, -116, 18, 8, 67, _teal),
    _ConfettiPiece(134, -17, 19, 8, 128, _blue),
    _ConfettiPiece(84, -53, 19, 8, 130, _gold),
    _ConfettiPiece(-125, -37, 19, 7, 135, _gold),
    _ConfettiPiece(103, 40, 14, 8, -143, _gold),
    _ConfettiPiece(-98, -84, 14, 7, 54, _blue),
    _ConfettiPiece(-108, 24, 12, 7, -148, _red),
    _ConfettiPiece(44, 76, 11, 7, 58, _blue),
    _ConfettiPiece(63, 53, 15, 4, -148, _teal),
    _ConfettiPiece(33, -84, 6, 6, 0, _orange),
    _ConfettiPiece(-31, -77, 6, 4, 72, _teal),
    _ConfettiPiece(-56, -64, 5, 5, 0, _teal),
    _ConfettiPiece(-107, 12, 6, 4, 90, _teal),
    _ConfettiPiece(47, -69, 5, 5, 0, _teal),
    _ConfettiPiece(96, -19, 4, 4, 0, _orange),
    _ConfettiPiece(79, 1, 4, 4, 0, _orange),
  ];

  @override
  void paint(Canvas canvas, Size size) {
    for (final piece in _pieces) {
      final paint = Paint()..color = piece.color;
      final radius = piece.thickness <= 6 ? piece.thickness / 2 : 2.0;
      canvas.save();
      canvas.translate(center.dx + piece.dx, center.dy + piece.dy);
      canvas.rotate(piece.angle * math.pi / 180);
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromCenter(
            center: Offset.zero,
            width: piece.length,
            height: piece.thickness,
          ),
          Radius.circular(radius),
        ),
        paint,
      );
      canvas.restore();
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}