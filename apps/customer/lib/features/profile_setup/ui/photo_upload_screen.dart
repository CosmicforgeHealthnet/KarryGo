import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/state/customer_auth_controller.dart';

class PhotoUploadScreen extends StatelessWidget {
  const PhotoUploadScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  Widget build(BuildContext context) {
    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.photoUpload),
      bottom: FigmaPrimaryButton(
        label: 'Final Step',
        onPressed: (state.hasProfilePhoto && !state.isLoading)
            ? controller.completePhotoUpload
            : null,
      ),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            FigmaProgressHeader(
              progress: 1,
              onBack: controller.goToProfileDetails,
            ),
            const SizedBox(height: 30),
            const Text(
              'Upload a Photo of yourself',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 18,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 12),
            const Text(
              'A profile photo helps drivers and delivery partners to recognize you.',
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 12,
                height: 1.45,
              ),
            ),
            const SizedBox(height: 34),
            Center(
              child: SizedBox(
                width: 210,
                height: 210,
                child: Stack(
                  alignment: Alignment.center,
                  children: [
                    _photoPreview(state),
                    if (state.isLoading)
                      const CircularProgressIndicator(
                        color: CustomerFigmaColors.primary,
                        strokeWidth: 3,
                      ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 24),
            Row(
              children: [
                Expanded(
                  child: FigmaSecondaryButton(
                    label: 'Take Photo',
                    onPressed: () {
                      if (!state.isLoading) {
                        controller.uploadProfilePhoto(ImageSource.camera);
                      }
                    },
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: FigmaPrimaryButton(
                    label: 'Upload Photo',
                    onPressed: state.isLoading
                        ? null
                        : () => controller.uploadProfilePhoto(ImageSource.gallery),
                  ),
                ),
              ],
            ),
            if (state.error != null) ...[
              const SizedBox(height: 12),
              Text(
                state.error!.message,
                style: const TextStyle(
                  color: Colors.red,
                  fontSize: 13,
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _photoPreview(CustomerAuthState state) {
    final photoUrl = state.profilePhotoUrl;
    if (photoUrl != null) {
      return ClipOval(
        child: Image.network(
          photoUrl,
          width: 210,
          height: 210,
          fit: BoxFit.cover,
          loadingBuilder: (context, child, progress) {
            if (progress == null) return child;
            return const Center(
              child: CircularProgressIndicator(
                color: CustomerFigmaColors.primary,
                strokeWidth: 2,
              ),
            );
          },
          errorBuilder: (context, error, stack) {
            debugPrint('Photo load error: $error\nURL: $photoUrl\nStack: $stack');
            return const _FallbackPhotoIcon();
          },
        ),
      );
    }
    return const FittedBox(child: FigmaCheckeredCircle());
  }
}

class _FallbackPhotoIcon extends StatelessWidget {
  const _FallbackPhotoIcon();

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 210,
      height: 210,
      decoration: const BoxDecoration(
        color: CustomerFigmaColors.primaryPale,
        shape: BoxShape.circle,
      ),
      child: const Icon(
        Icons.person_rounded,
        color: CustomerFigmaColors.primary,
        size: 88,
      ),
    );
  }
}
