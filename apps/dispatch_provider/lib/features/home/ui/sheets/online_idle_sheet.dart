import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

/// State 05 — Online Idle Map.
///
/// Shown when the rider is online but there are no pending requests yet.
/// Full-width card flush against the bottom of the map with only the top
/// corners rounded.
class OnlineIdleSheet extends StatelessWidget {
  const OnlineIdleSheet({super.key});

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final bottomPad = MediaQuery.of(context).padding.bottom;

    return Container(
      width: double.infinity,
      padding: EdgeInsets.fromLTRB(20, 20, 20, 20 + bottomPad),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: const BorderRadius.only(
          topLeft: Radius.circular(20),
          topRight: Radius.circular(20),
        ),
        boxShadow: colors.cardShadow,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // ── Broadcast badge ──────────────────────────────────────────
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: colors.teal10,
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(
              Icons.wifi_tethering_rounded,
              size: 22,
              color: colors.brandTeal500,
            ),
          ),
          const SizedBox(height: 14),

          // ── Title ────────────────────────────────────────────────────
          Text(
            "You're online",
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.3,
            ),
          ),
          const SizedBox(height: 6),

          // ── Subtitle ─────────────────────────────────────────────────
          Text(
            "Great! You'll receive nearby dispatch opportunities.",
            style: TextStyle(
              fontSize: 13.5,
              fontWeight: FontWeight.w400,
              color: colors.textMuted,
              height: 1.4,
            ),
          ),
        ],
      ),
    );
  }
}