import 'package:flutter/material.dart';

import '../karryam_tokens.dart';
import 'karryam_status_chip.dart';

enum KarryamVerificationStepState { notStarted, submitted, approved, rejected }

class KarryamVerificationStepCard extends StatelessWidget {
  const KarryamVerificationStepCard({
    super.key,
    required this.title,
    required this.subtitle,
    required this.icon,
    required this.state,
    this.onTap,
  });

  final String title;
  final String subtitle;
  final IconData icon;
  final KarryamVerificationStepState state;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final (label, tone) = switch (state) {
      KarryamVerificationStepState.notStarted => ('Not started', KarryamStatusTone.neutral),
      KarryamVerificationStepState.submitted => ('In review', KarryamStatusTone.warning),
      KarryamVerificationStepState.approved => ('Verified', KarryamStatusTone.success),
      KarryamVerificationStepState.rejected => ('Needs attention', KarryamStatusTone.danger),
    };

    return Material(
      color: KarryamColors.pureWhite,
      borderRadius: BorderRadius.circular(KarryamRadius.lg),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(KarryamRadius.lg),
        child: Container(
          padding: const EdgeInsets.all(KarryamSpacing.md),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(KarryamRadius.lg),
            border: Border.all(color: KarryamColors.border),
          ),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: state == KarryamVerificationStepState.approved
                      ? KarryamColors.teal10
                      : KarryamColors.navy10,
                  borderRadius: BorderRadius.circular(KarryamRadius.md),
                ),
                child: Icon(icon, color: KarryamColors.midnightNavy),
              ),
              const SizedBox(width: KarryamSpacing.sm),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(title, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800)),
                    const SizedBox(height: KarryamSpacing.xxs),
                    Text(subtitle, style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.indigoGray)),
                    const SizedBox(height: KarryamSpacing.xs),
                    KarryamStatusChip(label: label, tone: tone),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right_rounded, color: KarryamColors.forgeGray),
            ],
          ),
        ),
      ),
    );
  }
}
