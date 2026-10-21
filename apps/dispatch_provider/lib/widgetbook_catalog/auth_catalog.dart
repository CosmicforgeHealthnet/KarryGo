import 'package:widgetbook/widgetbook.dart';

import '../design_previews/auth/karryam_auth_edge_preview.dart';
import '../design_previews/auth/karryam_auth_preview.dart';

WidgetbookFolder buildAuthCatalog() => WidgetbookFolder(
      name: '02 Launch & Auth',
      children: [
        WidgetbookComponent(
          name: 'Launch',
          useCases: [
            WidgetbookUseCase(name: 'Splash', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.splash)),
            WidgetbookUseCase(name: 'Restoring secure session', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.restoringSession)),
            WidgetbookUseCase(name: 'No network on launch', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.noNetwork)),
            WidgetbookUseCase(name: 'Session expired', builder: (_) => const KarryamAuthEdgePreview(state: KarryamAuthEdgeState.sessionExpired)),
          ],
        ),
        WidgetbookComponent(
          name: 'Login',
          useCases: [
            WidgetbookUseCase(name: 'Default', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.login)),
            WidgetbookUseCase(name: 'Sending code', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.loginSubmitting)),
            WidgetbookUseCase(name: 'Account not found', builder: (_) => const KarryamAuthEdgePreview(state: KarryamAuthEdgeState.accountNotFound)),
            WidgetbookUseCase(name: 'Account restricted', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.suspended)),
          ],
        ),
        WidgetbookComponent(
          name: 'Signup',
          useCases: [
            WidgetbookUseCase(name: 'Default', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.signup)),
            WidgetbookUseCase(name: 'Validation errors', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.signupValidation)),
            WidgetbookUseCase(name: 'Existing account', builder: (_) => const KarryamAuthEdgePreview(state: KarryamAuthEdgeState.duplicateSignup)),
          ],
        ),
        WidgetbookComponent(
          name: 'OTP verification',
          useCases: [
            WidgetbookUseCase(name: 'Entering code', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.otp)),
            WidgetbookUseCase(name: 'Invalid code', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.otpInvalid)),
            WidgetbookUseCase(name: 'Expired code', builder: (_) => const KarryamAuthPreview(state: KarryamAuthPreviewState.otpExpired)),
            WidgetbookUseCase(name: 'Resend cooldown', builder: (_) => const KarryamAuthEdgePreview(state: KarryamAuthEdgeState.resendCooldown)),
            WidgetbookUseCase(name: 'Fresh code sent', builder: (_) => const KarryamAuthEdgePreview(state: KarryamAuthEdgeState.resendSuccess)),
            WidgetbookUseCase(name: 'Verification locked', builder: (_) => const KarryamAuthEdgePreview(state: KarryamAuthEdgeState.verificationLocked)),
          ],
        ),
      ],
    );
