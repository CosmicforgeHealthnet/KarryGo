import 'package:widgetbook/widgetbook.dart';

import '../design_previews/verification/karryam_verification_preview.dart';

WidgetbookFolder buildVerificationCatalog() => WidgetbookFolder(
      name: '04 Verification',
      children: [
        WidgetbookComponent(
          name: 'Readiness hub',
          useCases: [
            WidgetbookUseCase(name: 'Mixed verification states', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.hub)),
            WidgetbookUseCase(name: 'Fully verified', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.verified)),
          ],
        ),
        WidgetbookComponent(
          name: 'Identity',
          useCases: [
            WidgetbookUseCase(name: 'Start identity verification', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.identityStart)),
            WidgetbookUseCase(name: 'Identity submitted', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.identitySubmitted)),
          ],
        ),
        WidgetbookComponent(
          name: 'Face verification',
          useCases: [
            WidgetbookUseCase(name: 'Capture guidance', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.faceGuide)),
            WidgetbookUseCase(name: 'Processing match', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.faceProcessing)),
            WidgetbookUseCase(name: 'Match failed', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.faceFailed)),
          ],
        ),
        WidgetbookComponent(
          name: 'Driver licence',
          useCases: [
            WidgetbookUseCase(name: 'Submit licence', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.licence)),
            WidgetbookUseCase(name: 'Licence pending', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.licencePending)),
            WidgetbookUseCase(name: 'Licence rejected', builder: (_) => const KarryamVerificationPreview(state: KarryamVerificationPreviewState.licenceRejected)),
          ],
        ),
      ],
    );
