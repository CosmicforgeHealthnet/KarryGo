import 'package:flutter/material.dart';

import '../karryam_tokens.dart';

enum KarryamStatusTone { neutral, active, success, warning, danger, info }

class KarryamStatusChip extends StatelessWidget {
  const KarryamStatusChip({
    super.key,
    required this.label,
    this.tone = KarryamStatusTone.neutral,
    this.icon,
  });

  final String label;
  final KarryamStatusTone tone;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final (foreground, background) = switch (tone) {
      KarryamStatusTone.neutral => (KarryamColors.indigoGray, KarryamColors.navy10),
      KarryamStatusTone.active => (KarryamColors.midnightNavy, KarryamColors.teal20),
      KarryamStatusTone.success => (KarryamColors.success, KarryamColors.successSurface),
      KarryamStatusTone.warning => (KarryamColors.warning, KarryamColors.warningSurface),
      KarryamStatusTone.danger => (KarryamColors.danger, KarryamColors.dangerSurface),
      KarryamStatusTone.info => (KarryamColors.info, KarryamColors.infoSurface),
    };

    return DecoratedBox(
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(KarryamRadius.pill),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (icon != null) ...[
              Icon(icon, size: 14, color: foreground),
              const SizedBox(width: 5),
            ],
            Text(
              label,
              style: Theme.of(context).textTheme.labelMedium?.copyWith(
                    color: foreground,
                    fontWeight: FontWeight.w600,
                  ),
            ),
          ],
        ),
      ),
    );
  }
}
