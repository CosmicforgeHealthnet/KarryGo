import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart' show kDebugMode;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
import 'onboarding_shared_widgets.dart';

class DriverDocumentsScreen extends StatefulWidget {
  const DriverDocumentsScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<DriverDocumentsScreen> createState() => _DriverDocumentsScreenState();
}

class _DriverDocumentsScreenState extends State<DriverDocumentsScreen> {
  XFile? _license;
  XFile? _vehicleReg;
  bool _uploading = false;
  String? _error;

  final _guarantorNameCtrl = TextEditingController();
  final _guarantorPhoneCtrl = TextEditingController();
  final _emergencyNameCtrl = TextEditingController();
  final _emergencyPhoneCtrl = TextEditingController();
  String _emergencyRelationship = '';

  bool get _canContinue =>
      !_uploading &&
      _license != null &&
      _vehicleReg != null &&
      _guarantorNameCtrl.text.trim().isNotEmpty &&
      _guarantorPhoneCtrl.text.trim().isNotEmpty &&
      _emergencyNameCtrl.text.trim().isNotEmpty &&
      _emergencyPhoneCtrl.text.trim().isNotEmpty &&
      _emergencyRelationship.isNotEmpty;

  final _relationships = [
    'Parent', 'Sibling', 'Spouse', 'Friend', 'Colleague', 'Other'
  ];

  @override
  void initState() {
    super.initState();
    // Dev convenience: prefill the contact fields so testing onboarding doesn't
    // require retyping. The license/vehicle-reg uploads still need a real pick.
    // Never runs in release builds.
    if (kDebugMode) {
      _guarantorNameCtrl.text = 'Chidi Eze';
      _guarantorPhoneCtrl.text = '8023456789';
      _emergencyNameCtrl.text = 'Ada Okafor';
      _emergencyPhoneCtrl.text = '8034567890';
      _emergencyRelationship = 'Sibling';
    }
  }

  @override
  void dispose() {
    _guarantorNameCtrl.dispose();
    _guarantorPhoneCtrl.dispose();
    _emergencyNameCtrl.dispose();
    _emergencyPhoneCtrl.dispose();
    super.dispose();
  }

  Future<void> _pickLicense() async {
    final file = await ImagePicker().pickImage(source: ImageSource.gallery, imageQuality: 85);
    if (file != null && mounted) setState(() => _license = file);
  }

  Future<void> _pickVehicleReg() async {
    final file = await ImagePicker().pickImage(source: ImageSource.gallery, imageQuality: 85);
    if (file != null && mounted) setState(() => _vehicleReg = file);
  }

  Future<void> _proceed() async {
    final license = _license;
    final vehicleReg = _vehicleReg;
    if (license == null || vehicleReg == null) return;

    setState(() {
      _uploading = true;
      _error = null;
    });
    try {
      final licenseUrl =
          await widget.controller.uploadDocument(license, MediaPurpose.documentFile);
      final vehicleRegUrl =
          await widget.controller.uploadDocument(vehicleReg, MediaPurpose.documentFile);

      if (!mounted) return;
      widget.controller.saveDriverDocuments(
        driverLicenseUrl: licenseUrl ?? '',
        vehicleRegUrl: vehicleRegUrl ?? '',
        guarantorName: _guarantorNameCtrl.text.trim(),
        guarantorPhone: _guarantorPhoneCtrl.text.trim(),
        emergencyContactName: _emergencyNameCtrl.text.trim(),
        emergencyContactPhone: _emergencyPhoneCtrl.text.trim(),
        emergencyContactRelationship: _emergencyRelationship,
      );
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } catch (e) {
      if (mounted) setState(() => _error = 'Could not upload your documents. Please try again.');
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return OnboardingScaffold(
      title: 'Driver Information!',
      subtitle: 'You need to provide your driving documents and contact persons.',
      step: 4,
      content: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // ── Documents ──────────────────────────────────────────────────
          UploadArea(
            label: "Driver's License",
            subtitle: 'Upload front of your license',
            onTap: _pickLicense,
            fileName: _license?.name,
          ),
          const SizedBox(height: 12),
          UploadArea(
            label: 'Vehicle Registration',
            subtitle: 'Upload vehicle registration document',
            onTap: _pickVehicleReg,
            fileName: _vehicleReg?.name,
          ),
          const SizedBox(height: 24),

          // ── Guarantor ──────────────────────────────────────────────────
          Container(
            padding: const EdgeInsets.all(4),
            child: RichText(
              text: const TextSpan(
                style: TextStyle(color: kProviderMuted, fontSize: 12, height: 1.5),
                children: [
                  TextSpan(text: '🔒  ', style: TextStyle(fontSize: 14)),
                  TextSpan(
                    text: 'We need you to provide guarantor information to vouch for your integrity on the platform.',
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 12),
          const Text(
            'Guarantor Information',
            style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 15),
          ),
          const SizedBox(height: 12),
          const OnboardingSectionLabel('Full Name'),
          TextField(
            controller: _guarantorNameCtrl,
            textCapitalization: TextCapitalization.words,
            onChanged: (_) => setState(() {}),
            decoration: onboardingFieldDecoration('Guarantor full name'),
          ),
          const SizedBox(height: 12),
          const OnboardingSectionLabel('Mobile Number'),
          _PhoneField(controller: _guarantorPhoneCtrl, onChanged: (_) => setState(() {})),
          const SizedBox(height: 24),

          // ── Emergency contact ──────────────────────────────────────────
          Container(
            padding: const EdgeInsets.all(4),
            child: RichText(
              text: const TextSpan(
                style: TextStyle(color: kProviderMuted, fontSize: 12, height: 1.5),
                children: [
                  TextSpan(text: '🆘  ', style: TextStyle(fontSize: 14)),
                  TextSpan(
                    text: 'Provide emergency contact information so we can reach someone you trust in case of an emergency.',
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 12),
          const Text(
            'Emergency Contact',
            style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 15),
          ),
          const SizedBox(height: 12),
          const OnboardingSectionLabel('Full Name'),
          TextField(
            controller: _emergencyNameCtrl,
            textCapitalization: TextCapitalization.words,
            onChanged: (_) => setState(() {}),
            decoration: onboardingFieldDecoration('Contact full name'),
          ),
          const SizedBox(height: 12),
          const OnboardingSectionLabel('Mobile Number'),
          _PhoneField(controller: _emergencyPhoneCtrl, onChanged: (_) => setState(() {})),
          const SizedBox(height: 12),
          const OnboardingSectionLabel('Relationship'),
          DropdownButtonFormField<String>(
            initialValue: _emergencyRelationship.isEmpty ? null : _emergencyRelationship,
            hint: const Text('Select relationship', style: TextStyle(color: kProviderMuted, fontSize: 14)),
            decoration: onboardingFieldDecoration(''),
            items: _relationships
                .map((r) => DropdownMenuItem(value: r, child: Text(r)))
                .toList(),
            onChanged: (v) => setState(() => _emergencyRelationship = v ?? ''),
          ),
          const SizedBox(height: 8),
        ],
      ),
      onContinue: _canContinue ? _proceed : null,
      continueLabel: 'Final Step',
      isLoading: _uploading,
      error: _error,
    );
  }
}

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
