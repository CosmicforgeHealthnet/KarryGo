import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

/// Shows the "Allow location access" modal (design state 04).
///
/// Returns `true` if the user tapped "Allow location access" and `false` if
/// they dismissed it (tapped "Not now" or the scrim). The caller decides what
/// to do with that — typically re-attempt `goOnline()` or just close.
Future<bool> showLocationPermissionBlocker(BuildContext context) async {
  final result = await showDialog<bool>(
    context: context,
    barrierDismissible: true,
   barrierColor: Colors.black.withValues(alpha: 0.82),
    builder: (_) => const _LocationPermissionBlockerDialog(),
  );
  return result ?? false;
}

class _LocationPermissionBlockerDialog extends StatelessWidget {
  const _LocationPermissionBlockerDialog();

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.symmetric(horizontal: 24),
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.fromLTRB(24, 32, 24, 24),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(28),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.14),
              blurRadius: 40,
              offset: const Offset(0, 18),
            ),
          ],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // ── Hero icon ─────────────────────────────────────────────
            Container(
              width: 64,
              height: 64,
              decoration: BoxDecoration(
                color: colors.info,
                shape: BoxShape.circle,
                boxShadow: [
                  BoxShadow(
                    color: colors.info.withValues(alpha: 0.30),
                    blurRadius: 16,
                    offset: const Offset(0, 6),
                  ),
                ],
              ),
              child: const Icon(
                Icons.location_on_rounded,
                size: 34,
                color: Colors.white,
              ),
            ),
            const SizedBox(height: 22),

            // ── Title ─────────────────────────────────────────────────
            Text(
              'Allow location access',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 21,
                fontWeight: FontWeight.w800,
                color: colors.text,
                letterSpacing: -0.5,
                height: 1.15,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'Karryam needs your location to find nearby opportunities and '
              'keep you visible to dispatchers.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: colors.textMuted,
                height: 1.45,
              ),
            ),

            const SizedBox(height: 26),

            // ── Benefit bullets ───────────────────────────────────────
            const _BenefitRow(text: 'Find nearby jobs'),
            const SizedBox(height: 14),
            const _BenefitRow(text: 'Show you on the map'),
            const SizedBox(height: 14),
            const _BenefitRow(text: 'Keep you safe on the road'),

            const SizedBox(height: 28),

            // ── Primary action ────────────────────────────────────────
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: () => Navigator.of(context).pop(true),
                child: const Text('Allow location access'),
              ),
            ),

            const SizedBox(height: 10),

            // ── Dismiss ───────────────────────────────────────────────
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              style: TextButton.styleFrom(
                foregroundColor: colors.textMuted,
                minimumSize: const Size(double.infinity, 44),
              ),
              child: const Text(
                'Not now',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _BenefitRow extends StatelessWidget {
  const _BenefitRow({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Row(
      children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(
            color: colors.info,
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 14.5,
              fontWeight: FontWeight.w600,
              color: colors.text,
              height: 1.3,
              letterSpacing: -0.1,
            ),
          ),
        ),
      ],
    );
  }
}