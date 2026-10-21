import 'package:flutter/material.dart';
import 'package:widgetbook/widgetbook.dart';

import 'design_system/karryam_theme.dart';
import 'widgetbook_catalog/auth_catalog.dart';
import 'widgetbook_catalog/core_catalog.dart';
import 'widgetbook_catalog/home_catalog.dart';
import 'widgetbook_catalog/onboarding_catalog.dart';
import 'widgetbook_catalog/vehicle_catalog.dart';
import 'widgetbook_catalog/verification_catalog.dart';

void main() => runApp(const KarryamDispatchWidgetbook());

class KarryamDispatchWidgetbook extends StatelessWidget {
  const KarryamDispatchWidgetbook({super.key});

  @override
  Widget build(BuildContext context) {
    return Widgetbook.material(
      lightTheme: KarryamTheme.light(),
      darkTheme: KarryamTheme.dark(),
      directories: [
        buildFoundationsCatalog(),
        buildComponentsCatalog(),
        buildAuthCatalog(),
        buildOnboardingCatalog(),
        buildVerificationCatalog(),
        buildVehicleCatalog(),
        buildHomeCatalog(),
        buildProductionQueueCatalog(),
      ],
    );
  }
}
