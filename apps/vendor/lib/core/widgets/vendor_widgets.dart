import 'package:flutter/material.dart';
import '../theme/vendor_theme.dart';

class KarryamLogo extends StatelessWidget {
  const KarryamLogo({super.key, this.width = 128});
  final double width;
  @override
  Widget build(BuildContext context) => Image.asset(
    'assets/branding/karryam-logo.png',
    width: width,
    fit: BoxFit.contain,
    semanticLabel: 'Karryam',
  );
}

class VendorScreenFrame extends StatelessWidget {
  const VendorScreenFrame({
    super.key,
    required this.body,
    required this.modeLabel,
    required this.onSettings,
    this.footer,
    this.onBack,
    this.onSkip,
    this.dark = false,
    this.leading,
  });
  final Widget body;
  final Widget? leading;
  final Widget? footer;
  final String modeLabel;
  final VoidCallback? onSettings, onBack, onSkip;
  final bool dark;
  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: (dark ? KarryamColors.navy : KarryamColors.canvas)
        .forSurface(context),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 430),
          child: Column(
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 8, 16, 6),
                child: Row(
                  children: [
                    if (onBack != null) ...[
                      IconButton(
                        onPressed: onBack,
                        tooltip: 'Go back',
                        icon: const Icon(Icons.chevron_left_rounded),
                        style: IconButton.styleFrom(
                          backgroundColor:
                              (dark
                                      ? Colors.white.withValues(alpha: .06)
                                      : const Color(0xFFF3F4F6))
                                  .forSurface(context),
                          foregroundColor: dark
                              ? Colors.white
                              : KarryamColors.navy.forInk(context),
                        ),
                      ),
                      const SizedBox(width: 10),
                    ],
                    if (leading != null)
                      leading!
                    else
                      Text(
                        'VENDOR ACCESS',
                        style: TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w700,
                          letterSpacing: 2,
                          color: (dark ? Colors.white70 : KarryamColors.indigo)
                              .forInk(context),
                        ),
                      ),
                    const Spacer(),
                    IconButton(
                      key: const Key('data-mode-button'),
                      onPressed: onSettings,
                      tooltip: 'Data source: $modeLabel',
                      icon: const Icon(Icons.tune_rounded, size: 21),
                      color: (dark ? KarryamColors.teal : KarryamColors.navy)
                          .forInk(context),
                    ),
                    if (onSkip != null)
                      TextButton(
                        onPressed: onSkip,
                        style: TextButton.styleFrom(
                          foregroundColor:
                              (dark
                                      ? KarryamColors.teal
                                      : KarryamColors.tealInk)
                                  .forInk(context),
                        ),
                        child: const Text('Skip'),
                      ),
                  ],
                ),
              ),
              Expanded(child: body),
              if (footer != null)
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
                  child: footer!,
                ),
            ],
          ),
        ),
      ),
    ),
  );
}

class PrimaryAction extends StatelessWidget {
  const PrimaryAction({
    super.key,
    required this.label,
    required this.onPressed,
    this.busy = false,
    this.arrow = true,
  });
  final String label;
  final VoidCallback? onPressed;
  final bool busy, arrow;
  @override
  Widget build(BuildContext context) => FilledButton(
    onPressed: busy ? null : onPressed,
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        if (busy) ...[
          SizedBox.square(
            dimension: 18,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: KarryamColors.navy.forInk(context),
            ),
          ),
          const SizedBox(width: 12),
        ],
        Expanded(child: Text(label, textAlign: TextAlign.center)),
        if (!busy && arrow) ...[
          const SizedBox(width: 12),
          Container(
            padding: const EdgeInsets.all(6),
            decoration: BoxDecoration(
              color: (KarryamColors.navy.withValues(
                alpha: .09,
              )).forSurface(context),
              shape: BoxShape.circle,
            ),
            child: const Icon(Icons.arrow_forward_rounded, size: 18),
          ),
        ],
      ],
    ),
  );
}

class FeatureBadge extends StatelessWidget {
  const FeatureBadge(this.icon, {super.key, this.size = 64});
  final IconData icon;
  final double size;
  @override
  Widget build(BuildContext context) => Container(
    width: size,
    height: size,
    decoration: BoxDecoration(
      color: KarryamColors.mint.forSurface(context),
      borderRadius: BorderRadius.circular(size * .3),
    ),
    child: Icon(
      icon,
      size: size * .45,
      color: KarryamColors.navy.forInk(context),
    ),
  );
}

class InlineNotice extends StatelessWidget {
  const InlineNotice({
    super.key,
    required this.message,
    this.error = false,
    this.onRetry,
  });
  final String message;
  final bool error;
  final VoidCallback? onRetry;
  @override
  Widget build(BuildContext context) => Semantics(
    liveRegion: error,
    child: Container(
      width: double.infinity,
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: (error ? const Color(0xFFFFF2F0) : KarryamColors.mint)
            .forSurface(context),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            error ? Icons.error_outline_rounded : Icons.shield_outlined,
            size: 18,
            color: (error ? KarryamColors.error : KarryamColors.navy).forInk(
              context,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  message,
                  style: TextStyle(
                    fontSize: 12,
                    height: 1.55,
                    color: (error ? KarryamColors.error : KarryamColors.indigo)
                        .forInk(context),
                  ),
                ),
                if (onRetry != null)
                  TextButton(
                    onPressed: onRetry,
                    child: const Text('Try again'),
                  ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

class AccessArtwork extends StatelessWidget {
  const AccessArtwork(
    this.icon, {
    super.key,
    this.size = 104,
    this.color = KarryamColors.teal,
  });
  final IconData icon;
  final double size;
  final Color color;
  @override
  Widget build(BuildContext context) => SizedBox.square(
    dimension: size,
    child: Stack(
      alignment: Alignment.center,
      children: [
        Positioned.fill(
          child: DecoratedBox(
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(
                color: (color.withValues(alpha: .3)).forOutline(context),
              ),
            ),
          ),
        ),
        Transform.rotate(
          angle: -.14,
          child: Container(
            width: size * .7,
            height: size * .7,
            decoration: BoxDecoration(
              color: KarryamColors.navy.forSurface(context),
              borderRadius: BorderRadius.circular(size * .22),
              boxShadow: [
                BoxShadow(
                  color: (KarryamColors.navy.withValues(
                    alpha: .13,
                  )).forInk(context),
                  blurRadius: 24,
                  offset: const Offset(0, 12),
                ),
              ],
            ),
            child: Icon(
              icon,
              size: size * .31,
              color: KarryamColors.teal.forInk(context),
            ),
          ),
        ),
        Positioned(
          right: 1,
          top: 5,
          child: Container(
            width: size * .26,
            height: size * .26,
            decoration: BoxDecoration(
              color: (color).forSurface(context),
              shape: BoxShape.circle,
              border: Border.all(
                color: KarryamColors.canvas.forOutline(context),
                width: 4,
              ),
            ),
            child: Icon(
              Icons.add_rounded,
              size: size * .15,
              color: KarryamColors.navy.forInk(context),
            ),
          ),
        ),
      ],
    ),
  );
}

class KarryamMark extends StatelessWidget {
  const KarryamMark({super.key, this.size = 32});
  final double size;
  @override
  Widget build(BuildContext context) => Semantics(
    label: 'Karryam',
    image: true,
    child: ClipRect(
      child: SizedBox(
        width: size,
        height: size * 304 / 371,
        child: OverflowBox(
          alignment: Alignment.centerLeft,
          minWidth: size * 1221 / 371,
          maxWidth: size * 1221 / 371,
          child: Image.asset(
            'assets/branding/karryam-logo.png',
            width: size * 1221 / 371,
            excludeFromSemantics: true,
          ),
        ),
      ),
    ),
  );
}
