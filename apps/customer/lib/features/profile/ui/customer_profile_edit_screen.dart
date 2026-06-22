import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/data/customer_auth_api.dart';
import '../../auth/models/customer_auth_models.dart';
import '../../media/data/media_upload_service.dart';

class CustomerProfileEditScreen extends StatefulWidget {
  const CustomerProfileEditScreen({
    super.key,
    required this.profile,
    required this.session,
    required this.api,
    required this.mediaUploadService,
    required this.onSaved,
  });

  final CustomerProfile profile;
  final CustomerSession session;
  final CustomerAuthApi api;
  final MediaUploadService mediaUploadService;
  final ValueChanged<CustomerProfile> onSaved;

  @override
  State<CustomerProfileEditScreen> createState() => _CustomerProfileEditScreenState();
}

class _CustomerProfileEditScreenState extends State<CustomerProfileEditScreen> {
  late final TextEditingController _nameCtrl;
  late final String _initialName;
  String? _selectedLanguage;
  bool _saving = false;
  bool _uploadingPhoto = false;
  String? _photoUrl;
  ApiException? _error;

  static const _languages = ['English (USA)', 'Yoruba', 'Igbo', 'Hausa'];

  @override
  void initState() {
    super.initState();
    _photoUrl = widget.profile.photoUrl;
    _initialName =
        '${widget.profile.firstName ?? ''} ${widget.profile.lastName ?? ''}'.trim();
    _nameCtrl = TextEditingController(text: _initialName);
    _nameCtrl.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    super.dispose();
  }

  bool get _isDirty => _nameCtrl.text.trim() != _initialName;

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final fullName = _nameCtrl.text.trim();
      final spaceIdx = fullName.indexOf(' ');
      final firstName = spaceIdx == -1 ? fullName : fullName.substring(0, spaceIdx);
      final lastName = spaceIdx == -1 ? '' : fullName.substring(spaceIdx + 1).trim();

      final updated = await widget.api.updateProfile(
        accessToken: widget.session.accessToken,
        firstName: firstName,
        lastName: lastName,
      );
      widget.onSaved(updated);
      if (mounted) Navigator.of(context).pop();
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _saving = false;
      });
    }
  }

  Future<void> _changePhoto() async {
    setState(() => _uploadingPhoto = true);
    try {
      final result = await widget.mediaUploadService.pickAndUpload(
        source: ImageSource.gallery,
        purpose: 'profile_photo',
        ownerId: widget.profile.id,
      );
      if (result == null) {
        setState(() => _uploadingPhoto = false);
        return;
      }
      await widget.api.saveProfilePhotoUrl(
        accessToken: widget.session.accessToken,
        photoUrl: result.url,
        assetId: result.id,
      );
      setState(() {
        _photoUrl = result.url;
        _uploadingPhoto = false;
      });
    } catch (e) {
      setState(() => _uploadingPhoto = false);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Failed to upload photo. Try again.')),
        );
      }
    }
  }

  String _formatPhone(String phone) {
    if (phone.startsWith('+234') && phone.length > 4) {
      return '(+234) ${phone.substring(4)}';
    }
    return phone;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_rounded, color: CustomerFigmaColors.text),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: const [
            Text(
              'Profile Info',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 17,
                fontWeight: FontWeight.w800,
              ),
            ),
            Text(
              'Manage your Personal Information and Account',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11),
            ),
          ],
        ),
      ),
      body: Column(
        children: [
          Expanded(
            child: ListView(
              padding: const EdgeInsets.all(24),
              children: [
                Center(
                  child: Column(
                    children: [
                      Container(
                        width: 90,
                        height: 90,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: CustomerFigmaColors.primaryPale,
                          border: Border.all(color: CustomerFigmaColors.primarySoft, width: 3),
                        ),
                        child: ClipOval(
                          child: _uploadingPhoto
                              ? const Center(
                                  child: CircularProgressIndicator(
                                    color: CustomerFigmaColors.primary,
                                  ),
                                )
                              : _photoUrl != null && _photoUrl!.isNotEmpty
                                  ? Image.network(
                                      _photoUrl!,
                                      fit: BoxFit.cover,
                                      errorBuilder: (_, __, ___) =>
                                          const Icon(Icons.person_rounded, size: 44, color: CustomerFigmaColors.primary),
                                    )
                                  : const Icon(Icons.person_rounded, size: 44, color: CustomerFigmaColors.primary),
                        ),
                      ),
                      const SizedBox(height: 12),
                      Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          OutlinedButton(
                            onPressed: _uploadingPhoto ? null : _changePhoto,
                            style: OutlinedButton.styleFrom(
                              foregroundColor: CustomerFigmaColors.primary,
                              side: const BorderSide(color: CustomerFigmaColors.primary, width: 1.5),
                              shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(20)),
                              padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 8),
                              textStyle: const TextStyle(
                                  fontSize: 13, fontWeight: FontWeight.w700),
                            ),
                            child: const Text('Change'),
                          ),
                          const SizedBox(width: 10),
                          OutlinedButton(
                            onPressed: () {
                              ScaffoldMessenger.of(context).showSnackBar(
                                const SnackBar(
                                    content: Text('Photo deletion coming soon.')),
                              );
                            },
                            style: OutlinedButton.styleFrom(
                              foregroundColor: const Color(0xFFE53935),
                              side: const BorderSide(color: Color(0xFFE53935), width: 1.5),
                              shape: RoundedRectangleBorder(
                                  borderRadius: BorderRadius.circular(20)),
                              padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 8),
                              textStyle: const TextStyle(
                                  fontSize: 13, fontWeight: FontWeight.w700),
                            ),
                            child: const Text('Delete'),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 28),
                if (_error != null) ...[
                  Container(
                    padding: const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: const Color(0xFFFFF1F0),
                      borderRadius: BorderRadius.circular(10),
                      border: Border.all(color: const Color(0xFFFFCDD2)),
                    ),
                    child: Text(
                      _error!.message,
                      style: const TextStyle(color: Color(0xFFC0392B), fontSize: 13),
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
                FigmaTextField(
                  controller: _nameCtrl,
                  label: 'Full Name',
                  hintText: 'e.g. Emeka Okafor',
                ),
                const SizedBox(height: 16),
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'Phone Number',
                      style: TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 13,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Container(
                      decoration: BoxDecoration(
                        color: CustomerFigmaColors.surface,
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: Row(
                        children: [
                          Container(
                            padding: const EdgeInsets.symmetric(
                                horizontal: 12, vertical: 14),
                            decoration: const BoxDecoration(
                              border: Border(
                                right: BorderSide(
                                    color: CustomerFigmaColors.border, width: 1),
                              ),
                            ),
                            child: Row(
                              mainAxisSize: MainAxisSize.min,
                              children: const [
                                Text('🇳🇬', style: TextStyle(fontSize: 16)),
                                SizedBox(width: 4),
                                Icon(Icons.keyboard_arrow_down_rounded,
                                    size: 16, color: CustomerFigmaColors.muted),
                                SizedBox(width: 6),
                                Text(
                                  '(+234)',
                                  style: TextStyle(
                                      color: CustomerFigmaColors.muted, fontSize: 14),
                                ),
                              ],
                            ),
                          ),
                          Expanded(
                            child: Padding(
                              padding: const EdgeInsets.symmetric(
                                  horizontal: 12, vertical: 14),
                              child: Text(
                                _formatPhone(widget.profile.phone)
                                    .replaceFirst('(+234) ', ''),
                                style: const TextStyle(
                                    color: CustomerFigmaColors.muted, fontSize: 14),
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
                if (widget.profile.email.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  FigmaTextField(
                    controller: TextEditingController(text: widget.profile.email),
                    label: 'Email Address',
                    readOnly: true,
                  ),
                ],
                const SizedBox(height: 16),
                const Text(
                  'Language',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 8),
                Container(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(10),
                  ),
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: DropdownButtonHideUnderline(
                    child: DropdownButton<String>(
                      isExpanded: true,
                      value: _selectedLanguage,
                      hint: const Text(
                        'Select Language',
                        style: TextStyle(color: CustomerFigmaColors.muted),
                      ),
                      icon: const Icon(Icons.keyboard_arrow_down_rounded,
                          color: CustomerFigmaColors.muted),
                      items: _languages
                          .map((l) => DropdownMenuItem(value: l, child: Text(l)))
                          .toList(),
                      onChanged: (v) => setState(() => _selectedLanguage = v),
                    ),
                  ),
                ),
                const SizedBox(height: 32),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
            child: FigmaPrimaryButton(
              label: 'Save',
              isLoading: _saving,
              onPressed: _isDirty && !_saving ? _save : null,
            ),
          ),
        ],
      ),
    );
  }
}
