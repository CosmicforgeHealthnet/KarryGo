import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

Future<void> showHomeFeedback(
  BuildContext context, {
  required String title,
  required String message,
  String action = 'Continue',
  bool error = false,
  bool warning = false,
}) => showModalBottomSheet<void>(
  context: context,
  useSafeArea: true,
  isScrollControlled: true,
  showDragHandle: true,
  barrierColor: Colors.black.withValues(alpha: .5),
  builder: (sheet) => SingleChildScrollView(
    padding: EdgeInsets.fromLTRB(
      24,
      8,
      24,
      24 + MediaQuery.paddingOf(sheet).bottom,
    ),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Center(
          child: TweenAnimationBuilder<double>(
            tween: Tween(begin: .75, end: 1),
            duration: Duration(
              milliseconds: MediaQuery.disableAnimationsOf(sheet) ? 0 : 350,
            ),
            curve: Curves.easeOutCubic,
            builder: (context, value, child) =>
                Transform.scale(scale: value, child: child),
            child: Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                color: error
                    ? sheet.palette.dangerSoft
                    : warning
                    ? sheet.palette.warningSoft
                    : sheet.palette.successSoft,
                borderRadius: BorderRadius.circular(28),
              ),
              child: Center(
                child: error
                    ? Icon(
                        Icons.error_outline_rounded,
                        size: 36,
                        color: sheet.palette.danger,
                      )
                    : warning
                    ? Icon(
                        Icons.schedule_rounded,
                        size: 36,
                        color: sheet.palette.warning,
                      )
                    : JourneyIcon(
                        JourneySymbol.shield,
                        size: 42,
                        color: sheet.palette.success,
                      ),
              ),
            ),
          ),
        ),
        const SizedBox(height: 20),
        Text(
          title,
          textAlign: TextAlign.center,
          style: Theme.of(sheet).textTheme.headlineSmall,
        ),
        const SizedBox(height: 10),
        Text(
          message,
          textAlign: TextAlign.center,
          style: Theme.of(sheet).textTheme.bodyLarge,
        ),
        const SizedBox(height: 24),
        FilledButton(
          onPressed: () => Navigator.pop(sheet),
          child: Text(action),
        ),
      ],
    ),
  ),
);
