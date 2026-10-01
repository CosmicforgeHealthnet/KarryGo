import 'package:flutter/material.dart';

import '../../../requests/data/request_model.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../widgets/trip_stats.dart';
import '../widgets/route_info.dart';
import '../widgets/trip_peek_bar.dart';

class TripSheet extends StatelessWidget {
  const TripSheet({
    super.key,
    required this.request,
    required this.title,
    required this.subtitle,
    required this.showCall,
    required this.primaryLabel,
    required this.primaryEnabled,
    required this.secondaryLabel,
    required this.onPrimary,
    required this.onSecondary,
    required this.collapsed,
    required this.onToggleCollapse,
    this.peekStatusText = '',
    this.extra,
  });

  final RequestModel request;
  final String title;
  final String subtitle;
  final bool showCall;
  final String primaryLabel;
  final bool primaryEnabled;
  final String secondaryLabel;
  final VoidCallback onPrimary;
  final VoidCallback onSecondary;

  /// Whether the sheet is currently collapsed to the peek bar.
  final bool collapsed;

  /// Called when the user taps the collapse chevron or the peek bar itself.
  final VoidCallback onToggleCollapse;

  /// Secondary line shown on the collapsed peek bar.
  /// e.g. "8 min · 8.4 km" — caller decides.
  final String peekStatusText;

  final Widget? extra;

  @override
  Widget build(BuildContext context) {
    // ── Collapsed → peek bar ────────────────────────────────────
    if (collapsed) {
      return TripPeekBar(
        title: title,
        statusText: peekStatusText,
        onExpand: onToggleCollapse,
      );
    }

    // ── Expanded → full sheet ───────────────────────────────────
    final colors = context.palette;
    final bottomPad = MediaQuery.of(context).padding.bottom;
    final maxHeight = MediaQuery.of(context).size.height * 0.75;

    return Container(
      margin: EdgeInsets.fromLTRB(12, 0, 12, 12 + bottomPad),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(24),
        boxShadow: colors.cardShadow,
      ),
      clipBehavior: Clip.antiAlias,
      constraints: BoxConstraints(maxHeight: maxHeight),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 22, 20, 18),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ── Header row: hero badge + collapse chevron ───────────
            Row(
              children: [
                Container(
                  width: 44,
                  height: 44,
                  decoration: BoxDecoration(
                    color: colors.teal10,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Icon(
                    Icons.delivery_dining_rounded,
                    size: 22,
                    color: colors.brandTeal500,
                  ),
                ),
                const Spacer(),
                GestureDetector(
                  onTap: onToggleCollapse,
                  behavior: HitTestBehavior.opaque,
                  child: Container(
                    width: 36,
                    height: 36,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: colors.navy10,
                      shape: BoxShape.circle,
                    ),
                    child: Icon(
                      Icons.keyboard_arrow_down_rounded,
                      size: 22,
                      color: colors.textMuted,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 14),

            // ── Title + subtitle ────────────────────────────────────
            Text(
              title,
              style: TextStyle(
                fontSize: 19,
                fontWeight: FontWeight.w800,
                color: colors.text,
                letterSpacing: -0.4,
                height: 1.15,
              ),
            ),
            const SizedBox(height: 5),
            Text(
              subtitle,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: colors.textMuted,
                height: 1.4,
              ),
            ),

            const SizedBox(height: 18),

            // ── Info block ──────────────────────────────────────────
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: colors.divider),
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Container(
                        width: 40,
                        height: 40,
                        decoration: BoxDecoration(
                          color: colors.navy10,
                          shape: BoxShape.circle,
                        ),
                        child: Icon(
                          Icons.person,
                          size: 22,
                          color: colors.textMuted,
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Text(
                          request.customerName,
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: colors.text,
                            letterSpacing: -0.2,
                          ),
                        ),
                      ),
                      if (showCall)
                        Container(
                          width: 36,
                          height: 36,
                          decoration: BoxDecoration(
                            color: colors.teal10,
                            shape: BoxShape.circle,
                          ),
                          child: Icon(
                            Icons.phone_rounded,
                            size: 18,
                            color: colors.brandTeal500,
                          ),
                        ),
                    ],
                  ),
                  const SizedBox(height: 14),

                  TripStats(request: request),
                  const SizedBox(height: 14),

                  Container(height: 1, color: colors.divider),
                  const SizedBox(height: 14),

                  RouteInfo(
                    pickup: request.pickupAddress,
                    dropoff: request.dropoffAddress,
                  ),

                  if (extra != null) ...[
                    const SizedBox(height: 16),
                    extra!,
                  ],
                ],
              ),
            ),

            const SizedBox(height: 20),

            // ── Primary action ──────────────────────────────────────
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: primaryEnabled ? onPrimary : null,
                child: Text(
                  primaryLabel,
                  style: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ),

            const SizedBox(height: 6),

            // ── Secondary ───────────────────────────────────────────
            SizedBox(
              width: double.infinity,
              height: 40,
              child: TextButton(
                onPressed: onSecondary,
                style: TextButton.styleFrom(
                  foregroundColor: colors.danger,
                  minimumSize: const Size(double.infinity, 40),
                  textStyle: const TextStyle(
                    fontSize: 13.5,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                child: Text(secondaryLabel),
              ),
            ),
          ],
        ),
      ),
    );
  }
}