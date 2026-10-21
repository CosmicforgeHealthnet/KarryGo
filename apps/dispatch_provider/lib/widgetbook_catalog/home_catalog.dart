import 'package:widgetbook/widgetbook.dart';

import '../design_previews/home/karryam_home_preview.dart';

WidgetbookFolder buildHomeCatalog() => WidgetbookFolder(
      name: '06 Home & Availability',
      children: [
        WidgetbookComponent(
          name: 'Availability',
          useCases: [
            WidgetbookUseCase(name: 'Offline', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.offline)),
            WidgetbookUseCase(name: 'Online and ready', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.online)),
            WidgetbookUseCase(name: 'Busy with active mission', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.busy)),
          ],
        ),
        WidgetbookComponent(
          name: 'Readiness gates',
          useCases: [
            WidgetbookUseCase(name: 'Blocked from going online', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.blocked)),
            WidgetbookUseCase(name: 'Precise location permission', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.locationPermission)),
            WidgetbookUseCase(name: 'Degraded location', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.degradedLocation)),
          ],
        ),
        WidgetbookComponent(
          name: 'Operational summary',
          useCases: [
            WidgetbookUseCase(name: 'Session statistics', builder: (_) => const KarryamHomePreview(state: KarryamHomePreviewState.sessionStats)),
          ],
        ),
      ],
    );
