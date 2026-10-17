import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../data/request_model.dart';

/// Full-screen confirmation shown after a successful accept (design 05).
/// Green animated check + summary + two CTAs.
class AcceptConfirmationScreen extends StatefulWidget {
  const AcceptConfirmationScreen({
    super.key,
    required this.request,
    required this.onViewTrips,
    required this.onBrowseMore,
  });

  final RequestModel request;
  final VoidCallback onViewTrips;
  final VoidCallback onBrowseMore;

  @override
  State<AcceptConfirmationScreen> createState() =>
      _AcceptConfirmationScreenState();
}

class _AcceptConfirmationScreenState extends State<AcceptConfirmationScreen>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  late final Animation<double> _circleScale;
  late final Animation<double> _checkProgress;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1100),
    );
    _circleScale = Tween<double>(begin: 0.6, end: 1.0).animate(
      CurvedAnimation(
        parent: _controller,
        curve: const Interval(0.0, 0.42, curve: Curves.easeOutBack),
      ),
    );
    _checkProgress = CurvedAnimation(
      parent: _controller,
      curve: const Interval(0.38, 1.0, curve: Curves.easeOutCubic),
    );
    _controller.forward();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            children: [
              const Spacer(flex: 2),

              // Animated check
              AnimatedBuilder(
                animation: _controller,
                builder: (context, _) {
                  return Transform.scale(
                    scale: _circleScale.value,
                    child: SizedBox(
                      width: 96,
                      height: 96,
                      child: CustomPaint(
                        painter: _SuccessCheckPainter(
                          circleColor: colors.success,
                          checkColor: Colors.white,
                          checkProgress: _checkProgress.value,
                        ),
                      ),
                    ),
                  );
                },
              ),
              const SizedBox(height: 28),

              Text(
                'Request accepted!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: colors.text,
                  letterSpacing: -0.4,
                  height: 1.2,
                ),
              ),
              const SizedBox(height: 12),

              Text(
                "You've successfully accepted this opportunity.",
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 14,
                  color: colors.textMuted,
                  height: 1.5,
                ),
              ),

              const SizedBox(height: 32),

              // Summary card
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: colors.surface,
                  borderRadius: BorderRadius.circular(16),
                  border: Border.all(color: colors.divider),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          widget.request.fareDisplayGrouped,
                          style: TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w800,
                            color: colors.text,
                            letterSpacing: -0.6,
                          ),
                        ),
                        const Spacer(),
                        Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 8,
                            vertical: 4,
                          ),
                          decoration: BoxDecoration(
                            color: colors.teal10,
                            borderRadius: BorderRadius.circular(999),
                          ),
                          child: Text(
                            widget.request.source.label,
                            style: TextStyle(
                              fontSize: 11,
                              fontWeight: FontWeight.w700,
                              color: colors.brandTeal600,
                            ),
                          ),
                        ),
                      ],
                    ),
                    if (widget.request.itemSummary != null) ...[
                      const SizedBox(height: 6),
                      Text(
                        widget.request.itemSummary!,
                        style: TextStyle(
                          fontSize: 13,
                          color: colors.textMuted,
                          height: 1.4,
                        ),
                      ),
                    ],
                    const SizedBox(height: 14),
                    _StopLine(
                      label: 'Pick-up',
                      address: widget.request.pickupAddress,
                      icon: Icons.radio_button_checked,
                      color: colors.brandNavy900,
                    ),
                    const SizedBox(height: 8),
                    _StopLine(
                      label: 'Drop-off',
                      address: widget.request.dropoffAddress,
                      icon: Icons.place_rounded,
                      color: colors.brandTeal500,
                    ),
                  ],
                ),
              ),

              const SizedBox(height: 20),

              // Info hint
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 12,
                ),
                decoration: BoxDecoration(
                  color: colors.warningSoft,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(
                    color: colors.warning.withValues(alpha: 0.35),
                  ),
                ),
                child: Row(
                  children: [
                    Icon(
                      Icons.info_outline_rounded,
                      size: 16,
                      color: colors.warning,
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        'Head to pickup location and keep the shipper updated.',
                        style: TextStyle(
                          fontSize: 12.5,
                          height: 1.4,
                          fontWeight: FontWeight.w600,
                          color: colors.warning,
                        ),
                      ),
                    ),
                  ],
                ),
              ),

              const Spacer(flex: 3),

              SizedBox(
                width: double.infinity,
                height: 54,
                child: FilledButton(
                  onPressed: widget.onViewTrips,
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('View in my trips'),
                ),
              ),
              const SizedBox(height: 10),
              SizedBox(
                width: double.infinity,
                height: 50,
                child: TextButton(
                  onPressed: widget.onBrowseMore,
                  style: TextButton.styleFrom(
                    foregroundColor: colors.textMuted,
                    textStyle: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  child: const Text('Browse more opportunities'),
                ),
              ),
              const SizedBox(height: 12),
            ],
          ),
        ),
      ),
    );
  }
}

// ── Small stop line ─────────────────────────────────────────────────

class _StopLine extends StatelessWidget {
  const _StopLine({
    required this.label,
    required this.address,
    required this.icon,
    required this.color,
  });

  final String label;
  final String address;
  final IconData icon;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Icon(icon, size: 14, color: color),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: colors.textMuted,
                  letterSpacing: 0.2,
                ),
              ),
              const SizedBox(height: 1),
              Text(
                address,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: colors.text,
                  height: 1.3,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// ── Success check painter ──────────────────────────────────────────

class _SuccessCheckPainter extends CustomPainter {
  _SuccessCheckPainter({
    required this.circleColor,
    required this.checkColor,
    required this.checkProgress,
  });

  final Color circleColor;
  final Color checkColor;
  final double checkProgress;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;
    canvas.drawCircle(center, radius, Paint()..color = circleColor);

    final shortStart = Offset(size.width * 0.30, size.height * 0.52);
    final joint = Offset(size.width * 0.44, size.height * 0.66);
    final tip = Offset(size.width * 0.72, size.height * 0.36);

    final fullPath = Path()
      ..moveTo(shortStart.dx, shortStart.dy)
      ..lineTo(joint.dx, joint.dy)
      ..lineTo(tip.dx, tip.dy);

    final metrics = fullPath.computeMetrics().toList();
    if (metrics.isEmpty || checkProgress <= 0) return;
    final metric = metrics.first;
    final drawLength = metric.length * checkProgress.clamp(0.0, 1.0);
    final partialPath = metric.extractPath(0.0, drawLength);

    final paint = Paint()
      ..color = checkColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = size.width * 0.085
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;

    canvas.drawPath(partialPath, paint);
  }

  @override
  bool shouldRepaint(covariant _SuccessCheckPainter old) =>
      old.checkProgress != checkProgress || old.circleColor != circleColor;
}
