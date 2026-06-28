import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../auth/state/provider_auth_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'provider_change_phone_screen.dart';
import 'widgets/provider_profile_widgets.dart';

/// Profile Info edit screen (Figma 2111/2112).
class ProviderProfileInfoScreen extends StatefulWidget {
  const ProviderProfileInfoScreen({
    super.key,
    required this.authController,
    required this.profileController,
  });

  final ProviderAuthController authController;
  final ProviderProfileController profileController;

  @override
  State<ProviderProfileInfoScreen> createState() => _ProviderProfileInfoScreenState();
}

class _ProviderProfileInfoScreenState extends State<ProviderProfileInfoScreen> {
  late final TextEditingController _nameCtrl;
  late final TextEditingController _emailCtrl;
  late final TextEditingController _cityCtrl;
  String? _state;
  String? _language;
  bool _saving = false;
  bool _uploadingPhoto = false;

  static const _languages = ['English (USA)', 'Yoruba', 'Igbo', 'Hausa'];
  static const _states = [
    'Abia', 'Adamawa', 'Akwa Ibom', 'Anambra', 'Bauchi', 'Bayelsa', 'Benue', 'Borno',
    'Cross River', 'Delta', 'Ebonyi', 'Edo', 'Ekiti', 'Enugu', 'FCT - Abuja', 'Gombe',
    'Imo', 'Jigawa', 'Kaduna', 'Kano', 'Katsina', 'Kebbi', 'Kogi', 'Kwara', 'Lagos',
    'Nasarawa', 'Niger', 'Ogun', 'Ondo', 'Osun', 'Oyo', 'Plateau', 'Rivers', 'Sokoto',
    'Taraba', 'Yobe', 'Zamfara',
  ];

  @override
  void initState() {
    super.initState();
    final p = widget.profileController.profile;
    _nameCtrl = TextEditingController(text: p?.displayName == p?.phone ? '' : (p?.displayName ?? ''));
    _emailCtrl = TextEditingController(text: p?.email ?? '');
    _cityCtrl = TextEditingController(text: p?.locationCity ?? '');
    _state = (p?.locationState.isNotEmpty ?? false) && _states.contains(p!.locationState) ? p.locationState : null;
    _language = (p?.language.isNotEmpty ?? false) && _languages.contains(p!.language) ? p.language : null;
    _nameCtrl.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _emailCtrl.dispose();
    _cityCtrl.dispose();
    super.dispose();
  }

  bool get _canSave => _nameCtrl.text.trim().isNotEmpty && !_saving;

  Future<void> _changePhoto() async {
    final file = await ImagePicker().pickImage(source: ImageSource.gallery, imageQuality: 85);
    if (file == null || !mounted) return;
    setState(() => _uploadingPhoto = true);
    final ok = await widget.profileController.updateProfilePhoto(file);
    if (!mounted) return;
    setState(() => _uploadingPhoto = false);
    if (!ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(widget.profileController.error ?? 'Could not upload photo.')),
      );
    }
  }

  Future<void> _save() async {
    final full = _nameCtrl.text.trim();
    final idx = full.indexOf(' ');
    final firstName = idx == -1 ? full : full.substring(0, idx);
    final lastName = idx == -1 ? '' : full.substring(idx + 1).trim();

    setState(() => _saving = true);
    final ok = await widget.profileController.saveProfileInfo(
      firstName: firstName,
      lastName: lastName,
      email: _emailCtrl.text.trim(),
      locationState: _state ?? '',
      locationCity: _cityCtrl.text.trim(),
      language: _language ?? '',
    );
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) {
      Navigator.of(context).pop();
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(widget.profileController.error ?? 'Could not save profile.')),
      );
    }
  }

  Future<void> _changePhone() async {
    final confirmed = await showProviderConfirmDialog(
      context,
      icon: Icons.phone_iphone_rounded,
      title: 'Change Phone Number?',
      message: 'You are about to change your phone number, do you confirm this decision? If yes proceed confirm to OTP.',
      confirmLabel: 'Change Phone Number',
    );
    if (confirmed != true || !mounted) return;

    final changed = await Navigator.of(context).push<bool>(
      MaterialPageRoute(builder: (_) => ProviderChangePhoneScreen(profileController: widget.profileController)),
    );
    if (changed == true && mounted) {
      await showProviderConfirmDialog(
        context,
        icon: Icons.check_circle_outline_rounded,
        title: 'Phone Number has been changed!',
        message: 'You have successfully changed your phone number',
        confirmLabel: 'Okay',
      );
      setState(() {});
    }
  }

  @override
  Widget build(BuildContext context) {
    final phone = widget.profileController.profile?.phone ?? '';
    final phoneLocal = phone.startsWith('+234') ? phone.substring(4) : phone;
    final photoUrl = widget.profileController.profile?.profilePhotoUrl;

    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
              child: ProviderProfileHeader(
                title: 'Profile Info',
                subtitle: 'Manage your Personal Information and Account',
              ),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  // Avatar + change/delete
                  Center(
                    child: Column(
                      children: [
                        Container(
                          width: 92,
                          height: 92,
                          decoration: const BoxDecoration(color: Color(0xFFE7EEF7), shape: BoxShape.circle),
                          clipBehavior: Clip.antiAlias,
                          child: _uploadingPhoto
                              ? const Center(child: CircularProgressIndicator(color: kProviderGreen))
                              : (photoUrl != null && photoUrl.isNotEmpty)
                                  ? Image.network(photoUrl, fit: BoxFit.cover,
                                      errorBuilder: (_, _, _) =>
                                          const Icon(Icons.person_rounded, color: kProviderGreen, size: 44))
                                  : const Icon(Icons.person_rounded, color: kProviderGreen, size: 44),
                        ),
                        const SizedBox(height: 14),
                        Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            FilledButton(
                              onPressed: _uploadingPhoto ? null : _changePhoto,
                              style: FilledButton.styleFrom(
                                backgroundColor: kProviderGreen,
                                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                                padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 10),
                              ),
                              child: const Text('Change',
                                  style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 13)),
                            ),
                            const SizedBox(width: 12),
                            FilledButton(
                              onPressed: () {
                                ScaffoldMessenger.of(context).showSnackBar(
                                  const SnackBar(content: Text('Photo removal coming soon.')),
                                );
                              },
                              style: FilledButton.styleFrom(
                                backgroundColor: kProviderGreenSoft,
                                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                                padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 10),
                              ),
                              child: const Text('Delete',
                                  style: TextStyle(color: kProviderGreen, fontWeight: FontWeight.w700, fontSize: 13)),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 24),

                  const ProviderFieldLabel('Full Name'),
                  TextField(
                    controller: _nameCtrl,
                    textCapitalization: TextCapitalization.words,
                    decoration: providerGrayField('e.g. Odanye Titilayomi Ayomide'),
                  ),
                  const SizedBox(height: 16),

                  // Phone (read-only, with change affordance)
                  Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      const ProviderFieldLabel('Phone Number'),
                      GestureDetector(
                        onTap: _changePhone,
                        child: const Text('Change',
                            style: TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w700)),
                      ),
                    ],
                  ),
                  Container(
                    decoration: BoxDecoration(color: kProviderSurface, borderRadius: BorderRadius.circular(12)),
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
                    child: Row(
                      children: [
                        const Text('🇳🇬', style: TextStyle(fontSize: 18)),
                        const SizedBox(width: 4),
                        const Icon(Icons.keyboard_arrow_down_rounded, size: 18, color: kProviderMuted),
                        const SizedBox(width: 10),
                        const Text('(+234)', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                        const SizedBox(width: 10),
                        Expanded(
                          child: Text(
                            phoneLocal.isEmpty ? '—' : phoneLocal,
                            style: const TextStyle(color: kProviderMuted, fontSize: 14),
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Email Address'),
                  TextField(
                    controller: _emailCtrl,
                    keyboardType: TextInputType.emailAddress,
                    decoration: providerGrayField('uiuxwithdema@gmail.com'),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Location'),
                  DropdownButtonFormField<String>(
                    initialValue: _state,
                    isExpanded: true,
                    hint: const Text('Select State', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                    icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
                    decoration: providerGrayField(''),
                    items: _states.map((s) => DropdownMenuItem(value: s, child: Text(s))).toList(),
                    onChanged: (v) => setState(() => _state = v),
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: _cityCtrl,
                    textCapitalization: TextCapitalization.words,
                    decoration: providerGrayField('City'),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Language'),
                  DropdownButtonFormField<String>(
                    initialValue: _language,
                    isExpanded: true,
                    hint: const Text('Select Language', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                    icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
                    decoration: providerGrayField(''),
                    items: _languages.map((l) => DropdownMenuItem(value: l, child: Text(l))).toList(),
                    onChanged: (v) => setState(() => _language = v),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: ProviderPrimaryButton(
                label: 'Save',
                isLoading: _saving,
                onPressed: _canSave ? _save : null,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
