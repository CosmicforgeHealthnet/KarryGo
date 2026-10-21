import 'package:flutter/material.dart';

import '../karryam_tokens.dart';

enum KarryamNoticeTone { info, success, warning, danger }

class KarryamNotice extends StatelessWidget {
  const KarryamNotice({
    super.key,
    required this.title,
    required this.message,
    this.tone = KarryamNoticeTone.info,
    this.icon,
  });

  final String title;
  final String message;
  final KarryamNoticeTone tone;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final (background, foreground, fallbackIcon) = switch (tone) {
      KarryamNoticeTone.info => (
          KarryamColors.infoSurface,
          KarryamColors.info,
          Icons.info_outline_rounded,
        ),
      KarryamNoticeTone.success => (
          KarryamColors.successSurface,
          KarryamColors.success,
          Icons.check_circle_outline_rounded,
        ),
      KarryamNoticeTone.warning => (
          KarryamColors.warningSurface,
          KarryamColors.warning,
          Icons.warning_amber_rounded,
        ),
      KarryamNoticeTone.danger => (
          KarryamColors.dangerSurface,
          KarryamColors.danger,
          Icons.error_outline_rounded,
        ),
    };

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(KarryamSpacing.md),
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(KarryamRadius.md),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon ?? fallbackIcon, color: foreground, size: 22),
          const SizedBox(width: KarryamSpacing.sm),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: Theme.of(context).textTheme.titleSmall?.copyWith(
                        color: KarryamColors.midnightNavy,
                        fontWeight: FontWeight.w800,
                      ),
                ),
                const SizedBox(height: KarryamSpacing.xxs),
                Text(
                  message,
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: KarryamColors.indigoGray,
                      ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
