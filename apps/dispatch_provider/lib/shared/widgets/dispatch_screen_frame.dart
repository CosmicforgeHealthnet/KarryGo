import 'package:flutter/material.dart';
import '../../core/theme/theme_extensions.dart';
import '../../core/network/provider_session.dart';

class DispatchBackButton extends StatelessWidget {
  const DispatchBackButton({super.key, this.onPressed});
  final VoidCallback? onPressed;
  @override
  Widget build(BuildContext context) => IconButton(
    tooltip: 'Go back',
    onPressed: onPressed ?? () => Navigator.of(context).maybePop(),
    style: IconButton.styleFrom(
      minimumSize: const Size(48, 48),
      backgroundColor: Colors.transparent,
      foregroundColor: context.palette.text,
    ),
    icon: const Icon(Icons.chevron_left_rounded, size: 26),
  );
}

class DispatchSetupHeader extends StatelessWidget {
  const DispatchSetupHeader({
    super.key,
    required this.current,
    required this.total,
    required this.onBack,
  });
  final int current, total;
  final VoidCallback onBack;
  @override
  Widget build(BuildContext context) => Column(
    children: [
      Row(
        children: [
          DispatchBackButton(onPressed: onBack),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              'ACCOUNT SETUP',
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w700,
                letterSpacing: 2,
                color: context.palette.textMuted,
              ),
            ),
          ),
        ],
      ),
      const SizedBox(height: 20),
      DispatchStepProgress(current: current, total: total),
    ],
  );
}

class DispatchStepProgress extends StatelessWidget {
  const DispatchStepProgress({
    super.key,
    required this.current,
    required this.total,
    this.completed,
  });
  final int current, total;
  final Set<int>? completed;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final done =
        completed ??
        context
            .dependOnInheritedWidgetOfExactType<ProviderSession>()
            ?.completedSetupSteps
            ?.call() ??
        <int>{};
    return Semantics(
      label: 'Setup progress',
      child: Row(
        children: [
          for (var i = 1; i <= total; i++) ...[
            if (i > 1)
              Expanded(
                child: AnimatedContainer(
                  duration: Duration(
                    milliseconds: MediaQuery.disableAnimationsOf(context)
                        ? 0
                        : 260,
                  ),
                  height: 2,
                  color: done.contains(i - 1) ? c.brandTeal500 : c.divider,
                ),
              ),
            Semantics(
              label: i == current
                  ? 'Current step'
                  : done.contains(i)
                  ? 'Completed'
                  : 'Not started',
              child: AnimatedContainer(
                duration: Duration(
                  milliseconds: MediaQuery.disableAnimationsOf(context)
                      ? 0
                      : 260,
                ),
                width: 28,
                height: 28,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: done.contains(i) ? c.brandTeal600 : c.surface,
                  border: Border.all(
                    color: i == current ? c.brandTeal600 : c.divider,
                    width: i == current ? 2 : 1,
                  ),
                ),
                child: Center(
                  child: done.contains(i)
                      ? const Icon(
                          Icons.check_rounded,
                          color: Colors.white,
                          size: 16,
                        )
                      : i == current
                      ? Container(
                          width: 8,
                          height: 8,
                          decoration: BoxDecoration(
                            shape: BoxShape.circle,
                            color: c.brandTeal600,
                          ),
                        )
                      : const SizedBox.shrink(),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
