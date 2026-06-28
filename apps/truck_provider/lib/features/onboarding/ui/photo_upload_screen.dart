import 'dart:io';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import 'onboarding_shared_widgets.dart';

class PhotoUploadScreen extends StatefulWidget {
  const PhotoUploadScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<PhotoUploadScreen> createState() => _PhotoUploadScreenState();
}

class _PhotoUploadScreenState extends State<PhotoUploadScreen> {
  XFile? _photo;

  Future<void> _pick(ImageSource source) async {
    final file = await ImagePicker().pickImage(source: source, imageQuality: 85);
    if (file != null && mounted) setState(() => _photo = file);
  }

  void _submit() {
    widget.controller.submitOnboarding(_photo);
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.controller.state;
    return OnboardingScaffold(
      title: 'Upload a Photo of yourself',
      subtitle: 'A clear, recent photo builds trust with customers and verifies your identity.',
      step: 6,
      content: Column(
        children: [
          const SizedBox(height: 16),

          // ── Circle photo area ──────────────────────────────────────────
          Center(
            child: Stack(
              alignment: Alignment.bottomRight,
              children: [
                Container(
                  width: 140,
                  height: 140,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: kProviderSurface,
                    border: Border.all(
                      color: _photo != null ? kProviderGreen : kProviderBorder,
                      width: _photo != null ? 3 : 1.5,
                    ),
                    image: _photo != null
                        ? DecorationImage(
                            image: FileImage(File(_photo!.path)),
                            fit: BoxFit.cover,
                          )
                        : null,
                  ),
                  child: _photo == null
                      ? const Icon(Icons.person_rounded, size: 64, color: kProviderBorder)
                      : null,
                ),
                if (_photo != null)
                  Container(
                    width: 34,
                    height: 34,
                    decoration: const BoxDecoration(
                      shape: BoxShape.circle,
                      color: kProviderGreen,
                    ),
                    child: const Icon(Icons.check, color: Colors.white, size: 18),
                  ),
              ],
            ),
          ),
          const SizedBox(height: 32),

          // ── Pick buttons ──────────────────────────────────────────────
          Row(
            children: [
              Expanded(
                child: _PhotoButton(
                  icon: Icons.camera_alt_rounded,
                  label: 'Take Photo',
                  onTap: () => _pick(ImageSource.camera),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: _PhotoButton(
                  icon: Icons.photo_library_rounded,
                  label: 'Upload Photo',
                  onTap: () => _pick(ImageSource.gallery),
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
        ],
      ),
      onContinue: _photo != null && !state.isLoading ? _submit : null,
      continueLabel: 'Continue',
      isLoading: state.isLoading,
      error: state.error,
    );
  }
}

class _PhotoButton extends StatelessWidget {
  const _PhotoButton({required this.icon, required this.label, required this.onTap});
  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: kProviderBorder),
        ),
        child: Column(
          children: [
            Icon(icon, color: kProviderGreen, size: 28),
            const SizedBox(height: 6),
            Text(
              label,
              style: const TextStyle(
                color: kProviderText,
                fontWeight: FontWeight.w600,
                fontSize: 13,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
