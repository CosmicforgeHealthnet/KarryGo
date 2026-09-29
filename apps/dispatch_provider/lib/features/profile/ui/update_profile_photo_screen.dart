import 'dart:io';

import 'package:image_picker/image_picker.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/provider_profile_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 14 — Update profile photo
// White screen. Circular preview · Take photo · Choose from gallery ·
// Remove photo (red) · Save photo.
// ═══════════════════════════════════════════════════════════════════════════

class UpdateProfilePhotoScreen extends StatefulWidget {
  const UpdateProfilePhotoScreen({
    super.key,
    required this.profileController,
  });

  final ProviderProfileController profileController;

  @override
  State<UpdateProfilePhotoScreen> createState() =>
      _UpdateProfilePhotoScreenState();
}

class _UpdateProfilePhotoScreenState extends State<UpdateProfilePhotoScreen> {
  final _picker = ImagePicker();
  XFile? _picked;
  bool _isSaving = false;

  Future<void> _takePhoto() async {
    final result = await _picker.pickImage(
      source: ImageSource.camera,
      imageQuality: 85,
      maxWidth: 1200,
    );
    if (result != null && mounted) setState(() => _picked = result);
  }

  Future<void> _pickFromGallery() async {
    final result = await _picker.pickImage(
      source: ImageSource.gallery,
      imageQuality: 85,
      maxWidth: 1200,
    );
    if (result != null && mounted) setState(() => _picked = result);
  }

  void _removePhoto() {
    setState(() => _picked = null);
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Choose a new photo to replace.')),
    );
  }

  Future<void> _save() async {
    final picked = _picked;
    if (picked == null) return;

    setState(() => _isSaving = true);
    final result = await widget.profileController.uploadAvatar(picked.path);
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Profile photo updated')),
        );
        Navigator.of(context).pop();
      },
      failure: (error) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(error.message)),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final existingUrl = widget.profileController.profile?.profilePhotoUrl;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  Text(
                    'Update profile photo',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.2,
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
                children: [
                  Center(child: _buildPreview(existingUrl)),
                  const SizedBox(height: 36),
                  _OptionRow(
                    icon: Icons.photo_camera_outlined,
                    label: 'Take photo',
                    onTap: _isSaving ? null : _takePhoto,
                  ),
                  _OptionRow(
                    icon: Icons.image_outlined,
                    label: 'Choose from gallery',
                    onTap: _isSaving ? null : _pickFromGallery,
                  ),
                  _OptionRow(
                    icon: Icons.delete_outline_rounded,
                    label: 'Remove photo',
                    onTap: _isSaving ? null : _removePhoto,
                    destructive: true,
                    isLast: true,
                  ),
                ],
              ),
            ),

            // ── Save photo ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: (_picked != null && !_isSaving) ? _save : null,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: _isSaving
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: c.brandWhite,
                          ),
                        )
                      : const Text('Save photo'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildPreview(String? existingUrl) {
    final c = context.palette;
    const size = 160.0;

    Widget fallback() => Container(
          width: size,
          height: size,
          alignment: Alignment.center,
          color: c.teal10,
          child: Icon(
            Icons.person_rounded,
            size: 64,
            color: c.brandTeal600,
          ),
        );

    Widget content;
    if (_picked != null) {
      content = Image.file(
        File(_picked!.path),
        width: size,
        height: size,
        fit: BoxFit.cover,
      );
    } else if (existingUrl != null && existingUrl.isNotEmpty) {
      content = Image.network(
        existingUrl,
        width: size,
        height: size,
        fit: BoxFit.cover,
        errorBuilder: (_, _, _) => fallback(),
        loadingBuilder: (context, child, progress) {
          if (progress == null) return child;
          return fallback();
        },
      );
    } else {
      content = fallback();
    }

    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: c.divider, width: 2),
      ),
      child: ClipOval(child: content),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Option row
// ═══════════════════════════════════════════════════════════════════════════

class _OptionRow extends StatelessWidget {
  const _OptionRow({
    required this.icon,
    required this.label,
    required this.onTap,
    this.destructive = false,
    this.isLast = false,
  });

  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final bool destructive;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final fg = destructive ? c.danger : c.text;

    return Column(
      children: [
        GestureDetector(
          onTap: onTap,
          behavior: HitTestBehavior.opaque,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Row(
              children: [
                Icon(icon, size: 22, color: fg),
                const SizedBox(width: 16),
                Expanded(
                  child: Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: fg,
                      letterSpacing: -0.1,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
        if (!isLast) Divider(height: 1, color: c.divider),
      ],
    );
  }
}