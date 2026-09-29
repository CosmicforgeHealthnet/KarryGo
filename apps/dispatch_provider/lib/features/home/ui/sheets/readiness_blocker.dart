import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

/// Whether a required setup item is missing entirely (red ✗) or already
/// submitted and waiting for review (amber ⏳).
enum ReadinessItemStatus { missing, pending }

/// One row in the readiness checklist.
class ReadinessItem {
  const ReadinessItem({
    required this.title,
    required this.subtitle,
    required this.status,
  });

  final String title;
  final String subtitle;
  final ReadinessItemStatus status;
}

/// Shows the "Complete your setup" modal (design state 03) and returns when
/// the user dismisses it. Call [onCompleteSetup] to send them into the setup
/// flow; omit it if the only action available is dismissal.
Future<void> showReadinessBlocker(
  BuildContext context, {
  required List<ReadinessItem> items,
  required VoidCallback onCompleteSetup,
  VoidCallback? onMaybeLater,
}) {
  return showDialog<void>(
    context: context,
    barrierDismissible: true,
        barrierColor: Colors.black.withValues(alpha: 0.82),
    builder: (_) => _ReadinessBlockerDialog(
      items: items,
      onCompleteSetup: onCompleteSetup,
      onMaybeLater: onMaybeLater,
    ),
  );
}

class _ReadinessBlockerDialog extends StatelessWidget {
  const _ReadinessBlockerDialog({
    required this.items,
    required this.onCompleteSetup,
    this.onMaybeLater,
  });

  final List<ReadinessItem> items;
  final VoidCallback onCompleteSetup;
  final VoidCallback? onMaybeLater;

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
                color: colors.danger,
                shape: BoxShape.circle,
                boxShadow: [
                  BoxShadow(
                    color: colors.danger.withValues(alpha: 0.30),
                    blurRadius: 16,
                    offset: const Offset(0, 6),
                  ),
                ],
              ),
              child: const Icon(
                Icons.priority_high_rounded,
                size: 34,
                color: Colors.white,
              ),
            ),
            const SizedBox(height: 22),

            // ── Title ─────────────────────────────────────────────────
            Text(
              'Complete your setup',
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
              'You need to complete the following before you can go online.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w400,
                color: colors.textMuted,
                height: 1.45,
              ),
            ),

            const SizedBox(height: 26),

            // ── Checklist ─────────────────────────────────────────────
            Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                for (int i = 0; i < items.length; i++) ...[
                  _ReadinessRow(item: items[i]),
                  if (i != items.length - 1) const SizedBox(height: 16),
                ],
              ],
            ),

            const SizedBox(height: 28),

            // ── Primary action ────────────────────────────────────────
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: () {
                  Navigator.of(context).pop();
                  onCompleteSetup();
                },
                child: const Text('Complete setup'),
              ),
            ),

            const SizedBox(height: 10),

            // ── Dismiss ───────────────────────────────────────────────
            TextButton(
              onPressed: () {
                Navigator.of(context).pop();
                onMaybeLater?.call();
              },
              style: TextButton.styleFrom(
                foregroundColor: colors.textMuted,
                minimumSize: const Size(double.infinity, 44),
              ),
              child: const Text(
                'Maybe later',
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

class _ReadinessRow extends StatelessWidget {
  const _ReadinessRow({required this.item});
  final ReadinessItem item;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final isPending = item.status == ReadinessItemStatus.pending;
    final accent = isPending ? colors.warning : colors.danger;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 26,
          height: 26,
          margin: const EdgeInsets.only(top: 1),
          decoration: BoxDecoration(
            color: accent,
            shape: BoxShape.circle,
          ),
          child: Icon(
            isPending
                ? Icons.access_time_rounded
                : Icons.close_rounded,
            size: 16,
            color: Colors.white,
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                item.title,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                  height: 1.2,
                  letterSpacing: -0.1,
                ),
              ),
              const SizedBox(height: 3),
              Text(
                item.subtitle,
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w400,
                  color: colors.textMuted,
                  height: 1.35,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}