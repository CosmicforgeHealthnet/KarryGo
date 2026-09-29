import 'package:flutter/material.dart';
import '../../core/app_mode.dart';
import '../../core/theme/dispatch_colors.dart';

class MockBadge extends StatelessWidget {
  const MockBadge({super.key});

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: AppModeConfig.instance,
      builder: (context, _) {
        final isMock = AppModeConfig.instance.isMock;
        return GestureDetector(
          onTap: () => AppModeConfig.instance.toggle(),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: isMock
                  ? DispatchColors.warning
                  : DispatchColors.success,
              borderRadius: BorderRadius.circular(999),
            ),
            child: Text(
              isMock ? 'MOCK' : 'LIVE',
              style: const TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w800,
                color: Colors.white,
                letterSpacing: 0.6,
              ),
            ),
          ),
        );
      },
    );
  }
}