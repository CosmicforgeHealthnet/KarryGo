import 'journey_icon.dart';
import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';

class ContentSkeleton extends StatefulWidget {
  const ContentSkeleton({super.key, this.rows = 4});
  final int rows;
  @override
  State<ContentSkeleton> createState() => _ContentSkeletonState();
}

class _ContentSkeletonState extends State<ContentSkeleton>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1100),
  );
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _pulse.stop();
      _pulse.value = 1;
    } else if (!_pulse.isAnimating) {
      _pulse.repeat(reverse: true);
    }
  }

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Semantics(
      label: 'Loading content',
      child: ExcludeSemantics(
        child: FadeTransition(
          opacity: Tween<double>(
            begin: .45,
            end: 1,
          ).animate(CurvedAnimation(parent: _pulse, curve: Curves.easeInOut)),
          child: ListView.separated(
            physics: const NeverScrollableScrollPhysics(),
            padding: const EdgeInsets.all(20),
            itemCount: widget.rows,
            separatorBuilder: (_, _) => const SizedBox(height: 14),
            itemBuilder: (_, _) => Container(
              height: 110,
              padding: const EdgeInsets.all(18),
              decoration: BoxDecoration(
                color: c.surface,
                borderRadius: BorderRadius.circular(20),
                border: Border.all(color: c.divider),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  FractionallySizedBox(
                    widthFactor: .55,
                    child: Container(height: 14, color: c.divider),
                  ),
                  const SizedBox(height: 14),
                  FractionallySizedBox(
                    widthFactor: .85,
                    child: Container(height: 10, color: c.divider),
                  ),
                  const SizedBox(height: 10),
                  FractionallySizedBox(
                    widthFactor: .35,
                    child: Container(height: 10, color: c.divider),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class ActionNotice extends StatelessWidget {
  const ActionNotice({
    super.key,
    required this.message,
    this.isError = false,
    this.success = false,
    this.onRetry,
  });
  final String message;
  final bool isError;
  final bool success;
  final VoidCallback? onRetry;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final color = isError
        ? c.danger
        : success
        ? c.success
        : c.info;
    return Semantics(
      liveRegion: true,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: isError
              ? c.dangerSoft
              : success
              ? c.successSoft
              : c.surfaceElevated,
          borderRadius: BorderRadius.circular(18),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(
              isError
                  ? Icons.error_outline_rounded
                  : success
                  ? Icons.check_circle_outline_rounded
                  : Icons.info_outline_rounded,
              color: color,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                message,
                style: TextStyle(color: c.text, height: 1.45),
              ),
            ),
            if (onRetry != null)
              IconButton(
                tooltip: 'Try again',
                onPressed: onRetry,
                icon: Icon(Icons.refresh_rounded, color: color),
              ),
          ],
        ),
      ),
    );
  }
}

class ConfirmedResult extends StatelessWidget {
  const ConfirmedResult({
    super.key,
    required this.title,
    required this.message,
    required this.onDone,
  });
  final String title;
  final String message;
  final VoidCallback onDone;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TweenAnimationBuilder<double>(
              duration: Duration(
                milliseconds: MediaQuery.disableAnimationsOf(context) ? 0 : 450,
              ),
              tween: Tween(begin: .65, end: 1),
              curve: Curves.easeOutCubic,
              builder: (_, value, child) =>
                  Transform.scale(scale: value, child: child),
              child: Container(
                width: 96,
                height: 96,
                decoration: BoxDecoration(
                  color: c.successSoft,
                  borderRadius: BorderRadius.circular(32),
                ),
                child: JourneyIcon(
                  JourneySymbol.shield,
                  size: 48,
                  color: c.success,
                ),
              ),
            ),
            const SizedBox(height: 28),
            Text(
              title,
              textAlign: TextAlign.center,
              style: Theme.of(
                context,
              ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            Text(
              message,
              textAlign: TextAlign.center,
              style: TextStyle(color: c.textMuted, height: 1.5),
            ),
            const SizedBox(height: 28),
            SizedBox(
              width: double.infinity,
              child: FilledButton(onPressed: onDone, child: const Text('Done')),
            ),
          ],
        ),
      ),
    );
  }
}

void showDispatchError(BuildContext context, String message) {
  final c = context.palette;
  ScaffoldMessenger.of(context)
    ..clearSnackBars()
    ..showSnackBar(
      SnackBar(
        behavior: SnackBarBehavior.floating,
        backgroundColor: c.surfaceElevated,
        duration: const Duration(seconds: 5),
        content: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.error_outline_rounded, color: c.danger, size: 24),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                message,
                style: TextStyle(color: c.text, height: 1.5),
              ),
            ),
          ],
        ),
        action: SnackBarAction(
          label: 'Dismiss',
          textColor: c.textMuted,
          onPressed: () {},
        ),
      ),
    );
}
