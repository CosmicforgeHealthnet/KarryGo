import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';

class OnboardingScreen extends StatelessWidget {
  const OnboardingScreen({super.key, required this.onDone});

  final VoidCallback onDone;

  static const _cardTopFactor = 0.43;
  static const _imageTopFactor = 0.05;
  static const _imageHeightRatio = 0.415;
  static const _imageMaxWidthFactor = 0.94;
  static const _headScreenX = 0.52;
  static const _headAssetX = 0.524;
  static const _bulletMinSpacing = 16.0;

  static const _bullets = [
    _Bullet(
      Icons.assignment_turned_in_outlined,
      'Reliable bookings',
      'Get access to verified deliveries.',
    ),
    _Bullet(
      Icons.account_balance_wallet_outlined,
      'Flexible income',
      'Work on your terms.',
    ),
    _Bullet(
      Icons.groups_outlined,
      'A growing network',
      'Be part of something bigger.',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final size = MediaQuery.sizeOf(context);
    final topInset = MediaQuery.paddingOf(context).top;
    final cardTop = size.height * _cardTopFactor;
    final imageTop = size.height * _imageTopFactor;
    final imageWidth = math.min(
      size.width * _imageMaxWidthFactor,
      size.height * _imageHeightRatio,
    );
    final imageLeft = size.width * _headScreenX - imageWidth * _headAssetX;
    final backdropEnd = Color.lerp(c.teal10, c.navy10, 0.5)!;

    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.dark.copyWith(
        statusBarColor: Colors.transparent,
      ),
      child: Scaffold(
        backgroundColor: c.background,
        body: Stack(
          children: [
            Positioned.fill(
              child: DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [c.background, backdropEnd],
                  ),
                ),
              ),
            ),
            Positioned.fill(
              child: CustomPaint(
                painter: _ShapesPainter(
                  color: c.brandTeal500.withValues(alpha: 0.10),
                ),
              ),
            ),
            Positioned(
              top: imageTop,
              left: imageLeft,
              width: imageWidth,
              child: Image.asset(
                'assets/figma/welcome_hero.png',
                fit: BoxFit.fitWidth,
                alignment: Alignment.topCenter,
              ),
            ),
            Positioned.fill(
              child: CustomPaint(
                painter: _WedgePainter(
                  cardTopFactor: _cardTopFactor,
                  colorStart: c.teal60,
                  colorEnd: c.brandTeal500,
                ),
              ),
            ),
            Positioned(
              top: cardTop,
              left: 0,
              right: 0,
              bottom: 0,
              child: PhysicalShape(
                clipper: const _CardClipper(),
                clipBehavior: Clip.antiAlias,
                color: c.surface,
                elevation: 6,
                shadowColor: c.brandNavy900.withValues(alpha: 0.4),
                child: Material(
                  type: MaterialType.transparency,
                  child: SafeArea(
                    top: false,
                    child: Column(
                      children: [
                        Expanded(
                          child: LayoutBuilder(
                            builder: (context, constraints) {
                              return SingleChildScrollView(
                                child: ConstrainedBox(
                                  constraints: BoxConstraints(
                                    minHeight: constraints.maxHeight,
                                  ),
                                  child: IntrinsicHeight(
                                    child: Padding(
                                      padding: const EdgeInsets.fromLTRB(
                                        28,
                                        28,
                                        28,
                                        24,
                                      ),
                                      child: Column(
                                        crossAxisAlignment:
                                            CrossAxisAlignment.start,
                                        children: [
                                          Text(
                                            'Welcome to Karryam',
                                            style: TextStyle(
                                              fontSize: 28,
                                              fontWeight: FontWeight.w600,
                                              color: c.text,
                                              height: 1.15,
                                            ),
                                          ),
                                          const SizedBox(height: 8),
                                          ConstrainedBox(
                                            constraints: const BoxConstraints(
                                              maxWidth: 300,
                                            ),
                                            child: Text(
                                              'More opportunities. Greater routes. A brighter tomorrow.',
                                              style: TextStyle(
                                                fontSize: 16,
                                                color: c.textMuted,
                                                height: 1.4,
                                              ),
                                            ),
                                          ),
                                          const SizedBox(height: 24),
                                          const Spacer(),
                                          for (var i = 0;
                                              i < _bullets.length;
                                              i++) ...[
                                            if (i > 0) ...[
                                              const SizedBox(
                                                height: _bulletMinSpacing,
                                              ),
                                              const Spacer(),
                                            ],
                                            _BulletRow(bullet: _bullets[i]),
                                          ],
                                        ],
                                      ),
                                    ),
                                  ),
                                ),
                              );
                            },
                          ),
                        ),
                        Padding(
                          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
                          child: SizedBox(
                            height: 56,
                            width: double.infinity,
                            child: FilledButton(
                              onPressed: onDone,
                              style: FilledButton.styleFrom(
                                backgroundColor: c.brandTeal500,
                                foregroundColor: Colors.white,
                                shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(12),
                                ),
                                elevation: 0,
                              ),
                              child: const Row(
                                mainAxisAlignment: MainAxisAlignment.center,
                                children: [
                                  Text(
                                    'Get started',
                                    style: TextStyle(
                                      fontSize: 16,
                                      fontWeight: FontWeight.w700,
                                    ),
                                  ),
                                  SizedBox(width: 8),
                                  Icon(Icons.arrow_forward, size: 18),
                                ],
                              ),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
            Positioned(
              top: topInset + 4,
              right: 12,
              child: TextButton(
                onPressed: onDone,
                style: TextButton.styleFrom(
                  minimumSize: const Size(48, 48),
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  foregroundColor: c.textMuted,
                ),
                child: const Text(
                  'Skip',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CardClipper extends CustomClipper<Path> {
  const _CardClipper();

  @override
  Path getClip(Size size) {
    const radius = 28.0;
    const inset = 6.0;
    final w = size.width;
    final h = size.height;
    return Path()
      ..moveTo(0, h)
      ..lineTo(0, radius + inset)
      ..quadraticBezierTo(0, inset, radius, inset)
      ..cubicTo(w * 0.28, -8, w * 0.62, 22, w - radius, inset)
      ..quadraticBezierTo(w, inset, w, radius + inset)
      ..lineTo(w, h)
      ..close();
  }

  @override
  bool shouldReclip(_CardClipper oldClipper) => false;
}

class _ShapesPainter extends CustomPainter {
  const _ShapesPainter({required this.color});

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final w = size.width;
    final h = size.height;
    final paint = Paint()..color = color;

    const thickness = 0.11;
    const depth = 0.15;
    const left = 0.04 + thickness;
    final top = 0.115 * h;
    final height = 0.16 * h;

    canvas.drawPath(
      Path()
        ..moveTo(left * w, top)
        ..lineTo((left + depth) * w, top + height / 2)
        ..lineTo(left * w, top + height)
        ..lineTo((left - thickness) * w, top + height)
        ..lineTo((left + depth - thickness) * w, top + height / 2)
        ..lineTo((left - thickness) * w, top)
        ..close(),
      paint,
    );

    canvas.drawPath(
      Path()
        ..moveTo(0.55 * w, 0.263 * h)
        ..lineTo(0.87 * w, 0.194 * h)
        ..lineTo(0.87 * w, 0.297 * h)
        ..close(),
      paint,
    );
  }

  @override
  bool shouldRepaint(_ShapesPainter oldDelegate) => oldDelegate.color != color;
}

class _WedgePainter extends CustomPainter {
  const _WedgePainter({
    required this.cardTopFactor,
    required this.colorStart,
    required this.colorEnd,
  });

  final double cardTopFactor;
  final Color colorStart;
  final Color colorEnd;

  @override
  void paint(Canvas canvas, Size size) {
    final w = size.width;
    final h = size.height;
    final baseY = (cardTopFactor + 0.02) * h;
    final topY = (cardTopFactor - 0.055) * h;
    final rect = Rect.fromLTRB(0.127 * w, topY, 0.344 * w, baseY);
    final shader = LinearGradient(
      begin: Alignment.bottomLeft,
      end: Alignment.topRight,
      colors: [colorStart, colorEnd],
    ).createShader(rect);

    canvas.drawPath(
      Path()
        ..moveTo(0.127 * w, baseY)
        ..lineTo(0.344 * w, topY)
        ..lineTo(0.344 * w, baseY)
        ..close(),
      Paint()..shader = shader,
    );
  }

  @override
  bool shouldRepaint(_WedgePainter oldDelegate) =>
      oldDelegate.cardTopFactor != cardTopFactor ||
      oldDelegate.colorStart != colorStart ||
      oldDelegate.colorEnd != colorEnd;
}

class _BulletRow extends StatelessWidget {
  const _BulletRow({required this.bullet});

  final _Bullet bullet;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Container(
          width: 56,
          height: 56,
          decoration: BoxDecoration(
            color: c.teal10,
            shape: BoxShape.circle,
          ),
          child: Icon(
            bullet.icon,
            size: 28,
            color: c.brandTeal500,
          ),
        ),
        const SizedBox(width: 18),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                bullet.title,
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                bullet.body,
                style: TextStyle(
                  fontSize: 14,
                  color: c.textMuted,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _Bullet {
  const _Bullet(this.icon, this.title, this.body);

  final IconData icon;
  final String title;
  final String body;
}