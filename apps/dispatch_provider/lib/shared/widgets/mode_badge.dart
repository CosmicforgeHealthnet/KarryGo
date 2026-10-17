import 'package:flutter/material.dart';
import '../../core/app_mode.dart';
import '../../core/theme/dispatch_colors.dart';

class ModeBadge extends StatelessWidget {
  const ModeBadge({super.key});

  @override
  Widget build(BuildContext context) {
    if (AppModeConfig.instance.isLive) return const SizedBox.shrink();

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: DispatchColors.warning,
        borderRadius: BorderRadius.circular(999),
      ),
      child: const Text(
        'MOCK DATA',
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w800,
          color: Colors.white,
          letterSpacing: 0.8,
        ),
      ),
    );
  }
}
