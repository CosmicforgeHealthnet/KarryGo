import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart' show kDebugMode;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
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
  final _phoneCtrl = TextEditingController();
  XFile? _govId;
  bool _uploading = false;
  bool _checking = false;
  String? _error;
  String? _contactError;

  /// Whether the provider signed up with a phone number. When true, phone is the
  /// fixed identifier (shown read-only) and email is the required field to add.
  /// When false they signed up with an email, so email is fixed and phone is the
  /// required field to add.
  late final bool _signedUpWithPhone;

  @override
  void initState() {
    super.initState();
    final state = widget.controller.state;
    _signedUpWithPhone = state.identifierType != 'email';

    // If we're returning here (e.g. bounced back because the email/phone was
    // already in use at final submit), restore what was already entered so the
    // form isn't wiped, and show the conflict message inline.
    final saved = state.onboarding;
    final returning = saved.firstName.isNotEmpty ||
        saved.locationState.isNotEmpty ||
        saved.locationCity.isNotEmpty;
    if (returning) {
      _fullNameCtrl.text = '${saved.firstName} ${saved.lastName}'.trim();
      _stateCtrl.text = saved.locationState;
      _cityCtrl.text = saved.locationCity;
      if (_signedUpWithPhone) {
        _emailCtrl.text = saved.email;
      } else {
        _phoneCtrl.text = _localPhone(saved.phone);
      }
      if (_isIdentifierConflict(state.error)) {
        _contactError = state.error;
      }
    } else if (kDebugMode) {
      // Dev convenience: prefill the form on the first visit so testing
      // onboarding doesn't require retyping. Never runs in release builds.
      _fullNameCtrl.text = 'Emeka Okonkwo';
      _stateCtrl.text = 'Lagos';
      _cityCtrl.text = 'Ikeja';
      if (_signedUpWithPhone) {
        _emailCtrl.text = 'emeka.okonkwo@karrygo.dev';
      } else {
        _phoneCtrl.text = '8023456789';
      }
    }
  }

  /// Strips a normalized +234 prefix so the editable phone field shows the local
  /// part the user typed.
  String _localPhone(String phone) {
    if (phone.startsWith('+234')) return phone.substring(4);
    if (phone.startsWith('234')) return phone.substring(3);
    return phone;
  }

  bool _isIdentifierConflict(String? error) {
    if (error == null) return false;
    final e = error.toLowerCase();
    return e.contains('already in use') &&
        (e.contains('email') || e.contains('phone') || e.contains('number'));
  }

  bool get _canContinue =>
      !_uploading &&
      !_checking &&
      _fullNameCtrl.text.trim().isNotEmpty &&
      _stateCtrl.text.trim().isNotEmpty &&
      _cityCtrl.text.trim().isNotEmpty &&
      _govId != null &&
      _otherIdentifierValid;

  bool get _otherIdentifierValid {
    if (_signedUpWithPhone) {
      final e = _emailCtrl.text.trim();
      return e.contains('@') && e.length >= 5;
    }
    return _phoneCtrl.text.trim().length >= 7;
  }

  @override
  void dispose() {
    _fullNameCtrl.dispose();
    _stateCtrl.dispose();
    _cityCtrl.dispose();
    _emailCtrl.dispose();
    _phoneCtrl.dispose();
    super.dispose();
  }

  Future<void> _pickGovId() async {
    final picker = ImagePicker();
    final file = await picker.pickImage(source: ImageSource.gallery, imageQuality: 85);
    if (file != null && mounted) {
      setState(() => _govId = file);
    }
  }

  void _onContactChanged() {
    // Editing the identifier clears any prior "already in use" error.
    setState(() => _contactError = null);
  }

  Widget _contactErrorText() {
    if (_contactError == null) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(top: 6),
      child: Text(_contactError!, style: const TextStyle(color: Colors.red, fontSize: 12)),
    );
  }

  Future<void> _proceed() async {
    final parts = _fullNameCtrl.text.trim().split(' ');
    final firstName = parts.first;
    final lastName = parts.length > 1 ? parts.sublist(1).join(' ') : '';

    // The newly entered identifier (the editable one). The read-only one is the
    // identifier already used at sign-in, so it can never collide with itself.
    final newEmail = _signedUpWithPhone ? _emailCtrl.text.trim() : '';
    final newPhone = _signedUpWithPhone ? '' : _phoneCtrl.text.trim();

    // Check up front whether that identifier already belongs to another provider,
    // and block advancing with an inline error if so. On a network failure, fall
    // through — the submit-time duplicate guard still catches it.
    setState(() {
      _checking = true;
      _contactError = null;
    });
    try {
      final result = await widget.controller.checkContactAvailability(
        email: newEmail,
        phone: newPhone,
      );
      if (!mounted) return;
      if (_signedUpWithPhone && result.emailTaken) {
        setState(() => _contactError = 'This email address is already in use.');
        return;
      }
      if (!_signedUpWithPhone && result.phoneTaken) {
        setState(() => _contactError = 'This phone number is already in use.');
        return;
      }
    } on ApiException catch (e) {
      // Treat a validation error (e.g. malformed phone) as a blocking inline error;
      // let other failures fall through to the submit-time guard.
      if (e.fields.isNotEmpty || e.statusCode == 422) {
        if (mounted) setState(() => _contactError = e.message);
        return;
      }
    } catch (_) {
      // Network/other failure — don't hard-block; the submit-time guard remains.
    } finally {
      if (mounted) setState(() => _checking = false);
    }
    if (!mounted) return;

    // Upload the government ID through media-file-service first (if provided),
    // then store its public URL on the onboarding data so submitOnboarding sends it.
    final govId = _govId;
    var govIdUrl = '';
    if (govId != null) {
      setState(() {
        _uploading = true;
        _error = null;
      });
      try {
        govIdUrl =
            await widget.controller.uploadDocument(govId, MediaPurpose.documentFile) ?? '';
      } on ApiException catch (e) {
        if (mounted) setState(() => _error = e.message);
        return;
      } catch (e) {
        if (mounted) setState(() => _error = 'Could not upload your ID. Please try again.');
        return;
      } finally {
        if (mounted) setState(() => _uploading = false);
      }
      if (!mounted) return;
    }

    // Forward both identifiers: the read-only one (unchanged from sign-in) and
    // the newly entered one. The backend persists/dedupes whichever changed.
    final email = _signedUpWithPhone ? _emailCtrl.text.trim() : widget.controller.state.email;
    final phone = _signedUpWithPhone ? widget.controller.state.phone : _phoneCtrl.text.trim();

    widget.controller.savePersonalInfo(
      firstName: firstName,
      lastName: lastName,
      locationState: _stateCtrl.text.trim(),
      locationCity: _cityCtrl.text.trim(),
      email: email,
      phone: phone,
      govIdUrl: govIdUrl,
    );
  }

  @override
  Widget build(BuildContext context) {
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

          // Contact: the identifier used at sign-in is read-only; the other is required.
          const OnboardingSectionLabel('Phone Number'),
          if (_signedUpWithPhone)
            _ReadOnlyPhone(phone: widget.controller.state.phone)
          else
            _PhoneField(controller: _phoneCtrl, onChanged: (_) => _onContactChanged()),
          if (!_signedUpWithPhone) _contactErrorText(),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Email Address'),
          if (_signedUpWithPhone)
            TextField(
              controller: _emailCtrl,
              keyboardType: TextInputType.emailAddress,
              onChanged: (_) => _onContactChanged(),
              decoration: onboardingFieldDecoration('email@example.com'),
            )
          else
            _ReadOnlyEmail(email: widget.controller.state.email),
          if (_signedUpWithPhone) _contactErrorText(),
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
            fileName: _govId?.name,
          ),
          const SizedBox(height: 8),
        ],
      ),
      onContinue: _canContinue ? _proceed : null,
      isLoading: _uploading || _checking,
      error: _error,
    );
  }
}

/// Read-only display of the phone the provider signed in with.
class _ReadOnlyPhone extends StatelessWidget {
  const _ReadOnlyPhone({required this.phone});
  final String phone;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: kProviderBorder),
      ),
      child: Row(
        children: [
          const Text('🇳🇬  +234 ', style: TextStyle(color: kProviderText, fontSize: 14)),
          Expanded(child: Text(phone, style: const TextStyle(color: kProviderText, fontSize: 14))),
          const Icon(Icons.lock_outline_rounded, size: 16, color: kProviderMuted),
        ],
      ),
    );
  }
}

/// Read-only display of the email the provider signed in with.
class _ReadOnlyEmail extends StatelessWidget {
  const _ReadOnlyEmail({required this.email});
  final String email;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: kProviderBorder),
      ),
      child: Row(
        children: [
          Expanded(child: Text(email, style: const TextStyle(color: kProviderText, fontSize: 14))),
          const Icon(Icons.lock_outline_rounded, size: 16, color: kProviderMuted),
        ],
      ),
    );
  }
}

/// Editable +234 phone input (used when the provider signed up with email).
class _PhoneField extends StatelessWidget {
  const _PhoneField({required this.controller, required this.onChanged});
  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: kProviderBorder),
      ),
      child: Row(
        children: [
          const Padding(
            padding: EdgeInsets.symmetric(horizontal: 12),
            child: Text('🇳🇬  +234', style: TextStyle(fontSize: 14, color: kProviderText)),
          ),
          Container(width: 1, height: 24, color: kProviderBorder),
          Expanded(
            child: TextField(
              controller: controller,
              keyboardType: TextInputType.phone,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              onChanged: onChanged,
              decoration: const InputDecoration(
                hintText: '801 234 5678',
                hintStyle: TextStyle(color: kProviderMuted, fontSize: 14),
                border: InputBorder.none,
                contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 14),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
