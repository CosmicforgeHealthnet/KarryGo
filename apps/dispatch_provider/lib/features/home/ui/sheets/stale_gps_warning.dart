import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

/// Design state 06 — Stale GPS Warning.
///
/// Shown when location pings have failed enough times in a row that the
/// server has likely stopped treating the rider as discoverable. Replaces
/// the incoming-requests carousel until the GPS recovers.
class StaleGpsWarningSheet extends StatelessWidget {
  const StaleGpsWarningSheet({
    super.key,
    required this.onLearnMore,
    required this.onKeepAppOpen,
  });

  final VoidCallback onLearnMore;
  final VoidCallback onKeepAppOpen;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Container(
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 20),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(20),
        boxShadow: colors.cardShadow,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // ── Amber warning badge ──────────────────────────────────
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: colors.warningSoft,
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(
              Icons.location_off_rounded,
              size: 22,
              color: colors.warning,
            ),
          ),
          const SizedBox(height: 14),

          // ── Title ────────────────────────────────────────────────
          Text(
            'Location not updating',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.3,
            ),
          ),
          const SizedBox(height: 6),

          // ── Subtitle ─────────────────────────────────────────────
          Text(
            "We haven't received your location in a while. "
            'Stay visible for more opportunities.',
            style: TextStyle(
              fontSize: 13.5,
              fontWeight: FontWeight.w400,
              color: colors.textMuted,
              height: 1.4,
            ),
          ),
          const SizedBox(height: 18),

          // ── Actions ──────────────────────────────────────────────
          Row(
            children: [
              Expanded(
                child: SizedBox(
                  height: 46,
                  child: OutlinedButton(
                    onPressed: onLearnMore,
                    style: OutlinedButton.styleFrom(
                      foregroundColor: colors.text,
                      backgroundColor: colors.navy10,
                      side: BorderSide.none,
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(999),
                      ),
                    ),
                    child: const Text(
                      'Learn more',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 13,
                      ),
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: SizedBox(
                  height: 46,
                  child: FilledButton(
                    onPressed: onKeepAppOpen,
                    child: const Text(
                      'Keep app open',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 13,
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}