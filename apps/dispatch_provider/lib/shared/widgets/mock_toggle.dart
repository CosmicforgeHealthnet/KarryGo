import 'package:flutter/material.dart';
import '../../core/app_mode.dart';
import '../../core/theme/dispatch_colors.dart';

class MockModeBanner extends StatefulWidget {
  const MockModeBanner({super.key, required this.child});

  final Widget child;

  @override
  State<MockModeBanner> createState() => _MockModeBannerState();
}

class _MockModeBannerState extends State<MockModeBanner> {
  @override
  Widget build(BuildContext context) {
    if (!AppModeConfig.instance.isMock) return widget.child;

    return Column(
      children: [
        Container(
          width: double.infinity,
          color: DispatchColors.warning,
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: const Text(
            'MOCK MODE — showing fake data',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: Colors.white,
              letterSpacing: 0.5,
            ),
          ),
        ),
        Expanded(child: widget.child),
      ],
    );
  }
}
