import '../../home/ui/widgets/home_feedback.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'dart:io';

import 'package:image_picker/image_picker.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/provider_profile_controller.dart';

class UpdateProfilePhotoScreen extends StatefulWidget {
  const UpdateProfilePhotoScreen({super.key, required this.profileController});

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
  }

  Future<void> _save() async {
    final picked = _picked;
    if (picked == null) return;

    setState(() => _isSaving = true);
    final result = await widget.profileController.uploadAvatar(picked.path);
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Profile photo updated',
          message: 'Your new photo has been saved.',
        );
        if (mounted) Navigator.of(context).pop();
      },
      failure: (error) async {
        await showHomeFeedback(
          context,
          title: 'Could not save photo',
          message: error.message,
          error: true,
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Profile photo',
    symbol: JourneySymbol.photo,
    description: 'Choose a clear photo so people can recognise you.',
    bottom: FilledButton(
      onPressed: _picked != null && !_isSaving ? _save : null,
      child: _isSaving
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Save photo'),
    ),
    children: [
      Center(
        child: _buildPreview(widget.profileController.profile?.profilePhotoUrl),
      ),
      const SizedBox(height: 28),
      AccountSection(
        title: 'Choose your photo',
        children: [
          AccountMenuTile(
            title: 'Take a photo',
            symbol: JourneySymbol.photo,
            onTap: _isSaving ? null : _takePhoto,
          ),
          AccountMenuTile(
            title: 'Choose from gallery',
            symbol: JourneySymbol.photo,
            onTap: _isSaving ? null : _pickFromGallery,
          ),
          if (_picked != null)
            AccountMenuTile(
              title: 'Discard selection',
              symbol: JourneySymbol.photo,
              color: context.palette.danger,
              onTap: _isSaving ? null : _removePhoto,
            ),
        ],
      ),
    ],
  );

  Widget _buildPreview(String? existingUrl) {
    final c = context.palette;
    const size = 160.0;

    Widget fallback() => Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      color: c.teal10,
      child: JourneyIcon(
        JourneySymbol.profile,
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
