import 'package:widgetbook/widgetbook.dart';

import '../design_previews/vehicle/karryam_vehicle_preview.dart';

WidgetbookFolder buildVehicleCatalog() => WidgetbookFolder(
      name: '05 Vehicle',
      children: [
        WidgetbookComponent(
          name: 'Vehicle hub',
          useCases: [
            WidgetbookUseCase(name: 'Default', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.hub)),
            WidgetbookUseCase(name: 'Verified', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.verified)),
            WidgetbookUseCase(name: 'Suspended', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.suspended)),
          ],
        ),
        WidgetbookComponent(
          name: 'Vehicle setup',
          useCases: [
            WidgetbookUseCase(name: 'Choose type', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.chooseType)),
            WidgetbookUseCase(name: 'Vehicle details', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.details)),
          ],
        ),
        WidgetbookComponent(
          name: 'Documents',
          useCases: [
            WidgetbookUseCase(name: 'Documents hub', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.documents)),
            WidgetbookUseCase(name: 'Registration upload', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.registrationUpload)),
            WidgetbookUseCase(name: 'Insurance upload', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.insuranceUpload)),
            WidgetbookUseCase(name: 'Rejected document', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.rejected)),
          ],
        ),
        WidgetbookComponent(
          name: 'Review lifecycle',
          useCases: [
            WidgetbookUseCase(name: 'Pending review', builder: (_) => const KarryamVehiclePreview(state: KarryamVehiclePreviewState.pending)),
          ],
        ),
      ],
    );
