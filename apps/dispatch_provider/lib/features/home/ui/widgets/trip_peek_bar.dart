import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

/// Collapsed state of a trip sheet. Sits at the bottom of the map, 64px tall.
///
/// Tap anywhere on it → expands the trip sheet back to full height.
/// Chevron-up icon is the affordance (not X — X means "cancel/delete", this
/// just means "show me more map").
class TripPeekBar extends StatelessWidget {
  const TripPeekBar({
    super.key,
    required this.title,
    required this.statusText,
    required this.onExpand,
    this.accentColor,
  });

  /// e.g. "Arriving at pickup" / "You're on a delivery"
  final String title;

  /// e.g. "8 min · 8.4 km" / "Arrived" / "12 min remaining"
  final String statusText;

  final VoidCallback onExpand;
  final Color? accentColor;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final accent = accentColor ?? colors.brandTeal500;

    return GestureDetector(
      onTap: onExpand,
      behavior: HitTestBehavior.opaque,
      child: Container(
        margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(20),
          boxShadow: colors.cardShadow,
          border: Border.all(color: colors.divider),
        ),
        child: Row(
          children: [
            // ── Chevron-up ─────────────────────────────────────────
            Container(
              width: 36,
              height: 36,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: colors.teal10,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.keyboard_arrow_up_rounded,
                size: 22,
                color: accent,
              ),
            ),
            const SizedBox(width: 12),

            // ── Title + status ─────────────────────────────────────
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w800,
                      color: colors.text,
                      letterSpacing: -0.2,
                      height: 1.2,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    statusText,
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                      color: colors.textMuted,
                      height: 1.2,
                    ),
                  ),
                ],
              ),
            ),

            // ── Live dot ───────────────────────────────────────────
            _LiveDot(color: accent),
          ],
        ),
      ),
    );
  }
}

/// Small pulsing dot indicating the trip is actively running.
class _LiveDot extends StatefulWidget {
  const _LiveDot({required this.color});
  final Color color;

  @override
  State<_LiveDot> createState() => _LiveDotState();
}

class _LiveDotState extends State<_LiveDot>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c;

  @override
  void initState() {
    super.initState();
    _c = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1100),
    )..repeat(reverse: true);
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 20,
      height: 20,
      child: Center(
        child: AnimatedBuilder(
          animation: _c,
          builder: (context, _) {
            final scale = 0.85 + (_c.value * 0.3);
            final alpha = 0.55 + (_c.value * 0.45);
            return Container(
              width: 10 * scale,
              height: 10 * scale,
              decoration: BoxDecoration(
                color: widget.color.withValues(alpha: alpha),
                shape: BoxShape.circle,
              ),
            );
          },
        ),
      ),
    );
  }
}