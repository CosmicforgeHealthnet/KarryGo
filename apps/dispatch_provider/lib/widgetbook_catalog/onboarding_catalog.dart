import 'package:widgetbook/widgetbook.dart';

import '../design_previews/onboarding/karryam_dispatch_intro_preview.dart';
import '../design_previews/onboarding/karryam_onboarding_preview.dart';

WidgetbookFolder buildOnboardingCatalog() => WidgetbookFolder(
      name: '03 Onboarding & Provider Setup',
      children: [
        WidgetbookComponent(
          name: 'Dispatch introduction',
          useCases: [
            WidgetbookUseCase(name: '1 — Earn on your terms', builder: (_) => const KarryamDispatchIntroPreview(state: KarryamDispatchIntroState.earn)),
            WidgetbookUseCase(name: '2 — Stay in control', builder: (_) => const KarryamDispatchIntroPreview(state: KarryamDispatchIntroState.operate)),
            WidgetbookUseCase(name: '3 — Secure every handover', builder: (_) => const KarryamDispatchIntroPreview(state: KarryamDispatchIntroState.secure)),
          ],
        ),
        WidgetbookComponent(
          name: 'Provider setup intro',
          useCases: [WidgetbookUseCase(name: 'Setup overview', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.intro))],
        ),
        WidgetbookComponent(
          name: 'Provider choices',
          useCases: [
            WidgetbookUseCase(name: 'Account type', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.accountType)),
            WidgetbookUseCase(name: 'Service type', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.serviceType)),
            WidgetbookUseCase(name: 'Operation mode', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.operationMode)),
          ],
        ),
        WidgetbookComponent(
          name: 'Profile & photo',
          useCases: [
            WidgetbookUseCase(name: 'Profile details', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.profileDetails)),
            WidgetbookUseCase(name: 'Photo guidance', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.photoGuidance)),
            WidgetbookUseCase(name: 'Photo preview', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.photoPreview)),
          ],
        ),
        WidgetbookComponent(
          name: 'Vehicle & safety setup',
          useCases: [
            WidgetbookUseCase(name: 'Vehicle type', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.bikeType)),
            WidgetbookUseCase(name: 'Driver information', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.driverInformation)),
          ],
        ),
        WidgetbookComponent(
          name: 'Submission',
          useCases: [
            WidgetbookUseCase(name: 'Review setup', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.review)),
            WidgetbookUseCase(name: 'Submitting setup', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.submitting)),
            WidgetbookUseCase(name: 'Verification pending', builder: (_) => const KarryamOnboardingPreview(state: KarryamOnboardingPreviewState.verificationPending)),
          ],
        ),
      ],
    );
