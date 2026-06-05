import 'package:flutter/material.dart';

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
        onPressed: state.hasProfilePhoto
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
              child: state.hasProfilePhoto
                  ? Container(
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
                    )
                  : const SizedBox(
                      width: 210,
                      height: 210,
                      child: FittedBox(child: FigmaCheckeredCircle()),
                    ),
            ),
            const SizedBox(height: 24),
            Row(
              children: [
                Expanded(
                  child: FigmaSecondaryButton(
                    label: 'Take Photo',
                    onPressed: controller.markProfilePhotoSelected,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: FigmaPrimaryButton(
                    label: 'Upload Photo',
                    onPressed: controller.markProfilePhotoSelected,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
