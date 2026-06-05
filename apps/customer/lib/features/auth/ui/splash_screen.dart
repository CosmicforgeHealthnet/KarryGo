import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../shared/widgets/customer_screen_frame.dart';

class SplashScreen extends StatelessWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return const CustomerScreenFrame(
      scrollable: false,
      child: Expanded(
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              BrandMark(size: 72),
              SizedBox(height: CosmicforgeLogisticsSpacing.lg),
              Text(
                'Cosmicforge Logistics',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: CosmicforgeLogisticsColors.text,
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                ),
              ),
              SizedBox(height: CosmicforgeLogisticsSpacing.lg),
              CircularProgressIndicator(),
            ],
          ),
        ),
      ),
    );
  }
}
