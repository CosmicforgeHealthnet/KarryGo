import 'package:flutter/material.dart';
import '../theme/vendor_theme.dart';

void showVendorToast(BuildContext context, String message) {
  final theme = Theme.of(context);
  final reducedMotion = MediaQuery.disableAnimationsOf(context);
  final messenger = ScaffoldMessenger.of(context);
  messenger.hideCurrentSnackBar();
  messenger.showSnackBar(
    SnackBar(
      behavior: SnackBarBehavior.floating,
      elevation: 0,
      backgroundColor: KarryamColors.mint.forSurface(context),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
        side: BorderSide(color: KarryamColors.teal.withValues(alpha: .28)),
      ),
      content: Semantics(
        liveRegion: true,
        child: Row(
          children: [
            TweenAnimationBuilder<double>(
              tween: Tween(begin: reducedMotion ? 1 : .7, end: 1),
              duration: reducedMotion
                  ? Duration.zero
                  : const Duration(milliseconds: 320),
              curve: Curves.easeOutCubic,
              builder: (context, value, child) => Opacity(
                opacity: value,
                child: Transform.scale(scale: value, child: child),
              ),
              child: Container(
                width: 34,
                height: 34,
                decoration: BoxDecoration(
                  color: KarryamColors.teal.withValues(alpha: .16),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  Icons.check_rounded,
                  color: KarryamColors.tealInk.forInk(context),
                  size: 22,
                ),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                message,
                style: TextStyle(
                  color: theme.colorScheme.onSurface,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ],
        ),
      ),
    ),
    snackBarAnimationStyle: reducedMotion
        ? AnimationStyle.noAnimation
        : const AnimationStyle(
            duration: Duration(milliseconds: 300),
            reverseDuration: Duration(milliseconds: 220),
          ),
  );
}
