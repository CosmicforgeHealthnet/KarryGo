import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'widgets/provider_profile_widgets.dart';
import 'widgets/provider_safety_widgets.dart';

/// Emergency Contact screen: shows the saved contact card (with "+ Add New"),
/// or the add/edit form when none is saved yet or the card is tapped.
class ProviderEmergencyContactScreen extends StatefulWidget {
  const ProviderEmergencyContactScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProviderEmergencyContactScreen> createState() => _ProviderEmergencyContactScreenState();
}

class _ProviderEmergencyContactScreenState extends State<ProviderEmergencyContactScreen> {
  static const _relationships = ['Brother', 'Sister', 'Father', 'Mother', 'Spouse', 'Friend', 'Other'];

  final _nameCtrl = TextEditingController();
  final _phoneCtrl = TextEditingController();
  String? _relationship;
  bool _editing = false;
  bool _saving = false;

  ProviderProfileController get _controller => widget.profileController;

  bool get _hasSaved => _controller.profile?.hasEmergencyContact ?? false;

  bool get _canSave =>
      _nameCtrl.text.trim().isNotEmpty && _phoneCtrl.text.trim().length >= 7 && _relationship != null && !_saving;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _phoneCtrl.dispose();
    super.dispose();
  }

  void _startEditing() {
    final p = _controller.profile;
    _nameCtrl.text = p?.emergencyContactName ?? '';
    _phoneCtrl.text = safetyLocalDigits(p?.emergencyContactPhone ?? '');
    setState(() {
      _relationship =
          _relationships.contains(p?.emergencyContactRelationship) ? p?.emergencyContactRelationship : null;
      _editing = true;
    });
  }

  void _onBack() {
    if (_editing && _hasSaved) {
      setState(() => _editing = false);
    } else {
      Navigator.of(context).maybePop();
    }
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    final ok = await _controller.saveEmergencyContact(
      name: _nameCtrl.text.trim(),
      phone: safetyFullPhone(_phoneCtrl.text),
      relationship: _relationship!,
    );
    if (!mounted) return;
    setState(() {
      _saving = false;
      if (ok) _editing = false;
    });
    if (ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Emergency contact saved.'), behavior: SnackBarBehavior.floating),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: AnimatedBuilder(
          animation: _controller,
          builder: (context, _) {
            final showForm = _editing || !_hasSaved;
            return Column(
              children: [
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                    children: [
                      ProviderProfileHeader(
                        title: 'Emergency Contact',
                        subtitle: 'Provide information about who we can contact in case of an emergency.',
                        onBack: _onBack,
                      ),
                      const SizedBox(height: 20),
                      if (showForm) ..._buildForm() else ..._buildSavedCard(),
                    ],
                  ),
                ),
                if (showForm)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
                    child: Column(
                      children: [
                        ProviderErrorText(_controller.error),
                        ProviderPrimaryButton(
                          label: 'Save',
                          isLoading: _saving,
                          onPressed: _canSave ? _save : null,
                        ),
                      ],
                    ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  List<Widget> _buildSavedCard() {
    final p = _controller.profile;
    return [
      Align(
        alignment: Alignment.centerRight,
        child: SafetyAddNewPill(onTap: _startEditing),
      ),
      const SizedBox(height: 12),
      SafetyContactCard(
        name: p?.emergencyContactName ?? '',
        phone: p?.emergencyContactPhone ?? '',
        relationship: p?.emergencyContactRelationship,
        onTap: _startEditing,
      ),
    ];
  }

  List<Widget> _buildForm() {
    return [
      const SafetyHelperText(
        'Provide emergency Contact information. This could be a family member or close relative but not same individual as guarantor.',
      ),
      const SizedBox(height: 20),
      const Text(
        'Emergency Contact',
        style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
      ),
      const SizedBox(height: 16),
      const SafetyFieldLabel('Full Name'),
      TextField(
        controller: _nameCtrl,
        onChanged: (_) => setState(() {}),
        textCapitalization: TextCapitalization.words,
        style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w500),
        decoration: providerGrayField('Enter name'),
      ),
      const SizedBox(height: 16),
      const SafetyFieldLabel('Mobile Number'),
      SafetyPhoneField(controller: _phoneCtrl, onChanged: (_) => setState(() {})),
      const SizedBox(height: 16),
      const SafetyFieldLabel('Relationship Type'),
      DropdownButtonFormField<String>(
        initialValue: _relationship,
        isExpanded: true,
        hint: const Text('Select Relationship Type', style: TextStyle(color: kProviderMuted, fontSize: 14)),
        icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
        style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w500),
        decoration: providerGrayField(''),
        items: _relationships.map((s) => DropdownMenuItem(value: s, child: Text(s))).toList(),
        onChanged: (v) => setState(() => _relationship = v),
      ),
    ];
  }
}
