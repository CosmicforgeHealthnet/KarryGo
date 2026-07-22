import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'widgets/provider_profile_widgets.dart';
import 'widgets/provider_safety_widgets.dart';

/// Guarantor Information screen: shows the saved guarantor card (with
/// "+ Add New"), or the add/edit form (name + phone only) otherwise.
class ProviderGuarantorScreen extends StatefulWidget {
  const ProviderGuarantorScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProviderGuarantorScreen> createState() => _ProviderGuarantorScreenState();
}

class _ProviderGuarantorScreenState extends State<ProviderGuarantorScreen> {
  final _nameCtrl = TextEditingController();
  final _phoneCtrl = TextEditingController();
  bool _editing = false;
  bool _saving = false;

  ProviderProfileController get _controller => widget.profileController;

  bool get _hasSaved => _controller.profile?.hasGuarantor ?? false;

  bool get _canSave => _nameCtrl.text.trim().isNotEmpty && _phoneCtrl.text.trim().length >= 7 && !_saving;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _phoneCtrl.dispose();
    super.dispose();
  }

  void _startEditing() {
    final p = _controller.profile;
    _nameCtrl.text = p?.guarantorName ?? '';
    _phoneCtrl.text = safetyLocalDigits(p?.guarantorPhone ?? '');
    setState(() => _editing = true);
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
    final ok = await _controller.saveGuarantor(
      name: _nameCtrl.text.trim(),
      phone: safetyFullPhone(_phoneCtrl.text),
    );
    if (!mounted) return;
    setState(() {
      _saving = false;
      if (ok) _editing = false;
    });
    if (ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Guarantor information saved.'), behavior: SnackBarBehavior.floating),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
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
                        title: 'Guarantor Information',
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
        name: p?.guarantorName ?? '',
        phone: p?.guarantorPhone ?? '',
        onTap: _startEditing,
      ),
    ];
  }

  List<Widget> _buildForm() {
    return [
      const SafetyHelperText(
        'We need you to provide guarantor information. This could be a family member or close relative.',
      ),
      const SizedBox(height: 20),
      const Text(
        'Guarantor Information',
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
    ];
  }
}
