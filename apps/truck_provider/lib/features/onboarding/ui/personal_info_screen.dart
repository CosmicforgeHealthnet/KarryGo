import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import 'onboarding_shared_widgets.dart';

class PersonalInfoScreen extends StatefulWidget {
  const PersonalInfoScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<PersonalInfoScreen> createState() => _PersonalInfoScreenState();
}

class _PersonalInfoScreenState extends State<PersonalInfoScreen> {
  final _fullNameCtrl = TextEditingController();
  final _stateCtrl = TextEditingController();
  final _cityCtrl = TextEditingController();
  final _emailCtrl = TextEditingController();
  String? _govIdFileName;

  bool get _canContinue =>
      _fullNameCtrl.text.trim().isNotEmpty &&
      _stateCtrl.text.trim().isNotEmpty &&
      _cityCtrl.text.trim().isNotEmpty;

  @override
  void dispose() {
    _fullNameCtrl.dispose();
    _stateCtrl.dispose();
    _cityCtrl.dispose();
    _emailCtrl.dispose();
    super.dispose();
  }

  Future<void> _pickGovId() async {
    final picker = ImagePicker();
    final file = await picker.pickImage(source: ImageSource.gallery);
    if (file != null && mounted) {
      setState(() => _govIdFileName = file.name);
    }
  }

  void _proceed() {
    final parts = _fullNameCtrl.text.trim().split(' ');
    final firstName = parts.first;
    final lastName = parts.length > 1 ? parts.sublist(1).join(' ') : '';
    widget.controller.savePersonalInfo(
      firstName: firstName,
      lastName: lastName,
      locationState: _stateCtrl.text.trim(),
      locationCity: _cityCtrl.text.trim(),
      email: _emailCtrl.text.trim(),
    );
  }

  @override
  Widget build(BuildContext context) {
    final phone = widget.controller.state.phone;
    return OnboardingScaffold(
      title: 'Tell us about you',
      subtitle: "Let's set up your account with some basic information.",
      step: 3,
      content: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Full Name
          const OnboardingSectionLabel('Full Name'),
          TextField(
            controller: _fullNameCtrl,
            textCapitalization: TextCapitalization.words,
            onChanged: (_) => setState(() {}),
            decoration: onboardingFieldDecoration('e.g. Emeka Okonkwo'),
          ),
          const SizedBox(height: 16),

          // Phone (read-only)
          const OnboardingSectionLabel('Phone Number'),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
            decoration: BoxDecoration(
              color: kProviderSurface,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: kProviderBorder),
            ),
            child: Row(
              children: [
                const Text('🇳🇬  +234 ', style: TextStyle(color: kProviderText, fontSize: 14)),
                Text(phone, style: const TextStyle(color: kProviderText, fontSize: 14)),
              ],
            ),
          ),
          const SizedBox(height: 16),

          // State + City row
          Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const OnboardingSectionLabel('State'),
                    TextField(
                      controller: _stateCtrl,
                      textCapitalization: TextCapitalization.words,
                      onChanged: (_) => setState(() {}),
                      decoration: onboardingFieldDecoration('e.g. Lagos'),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const OnboardingSectionLabel('City'),
                    TextField(
                      controller: _cityCtrl,
                      textCapitalization: TextCapitalization.words,
                      onChanged: (_) => setState(() {}),
                      decoration: onboardingFieldDecoration('e.g. Ikeja'),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),

          // Email
          const OnboardingSectionLabel('Email Address'),
          TextField(
            controller: _emailCtrl,
            keyboardType: TextInputType.emailAddress,
            onChanged: (_) => setState(() {}),
            decoration: onboardingFieldDecoration('email@example.com'),
          ),
          const SizedBox(height: 20),

          // Gov ID upload
          const OnboardingSectionLabel('Government ID'),
          const Text(
            'Upload a valid government-issued ID (NIN slip, passport, voter\'s card).',
            style: TextStyle(color: kProviderMuted, fontSize: 12, height: 1.4),
          ),
          const SizedBox(height: 8),
          UploadArea(
            label: 'Upload Government ID',
            subtitle: 'Tap to upload photo or scan',
            onTap: _pickGovId,
            fileName: _govIdFileName,
          ),
          const SizedBox(height: 8),
        ],
      ),
      onContinue: _canContinue ? _proceed : null,
    );
  }
}
