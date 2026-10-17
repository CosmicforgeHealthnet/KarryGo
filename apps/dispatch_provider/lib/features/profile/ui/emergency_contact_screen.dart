import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';

import '../models/provider_profile_models.dart';
import '../state/provider_profile_controller.dart';

class EmergencyContactScreen extends StatefulWidget {
  const EmergencyContactScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<EmergencyContactScreen> createState() => _EmergencyContactScreenState();
}

class _EmergencyContactScreenState extends State<EmergencyContactScreen> {
  bool _isLoading = false;
  bool _isSaving = false;
  String? _loadError;

  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();
  String? _relationship;

  static const _relationships = [
    'Brother',
    'Sister',
    'Father',
    'Mother',
    'Spouse',
    'Friend',
    'Other',
  ];

  bool get _canSave =>
      _nameController.text.trim().isNotEmpty &&
      _phoneController.text.trim().isNotEmpty &&
      _relationship != null &&
      !_isSaving;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _load();
    });
  }

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });

    final cached = widget.profileController.emergencyContact;
    if (cached != null) {
      _prefill(cached);
      setState(() => _isLoading = false);
      return;
    }

    final result = await widget.profileController.loadEmergencyContact();
    if (!mounted) return;
    result.when(
      success: (contact) {
        _prefill(contact);
        setState(() => _isLoading = false);
      },
      failure: (error) {
        final notFound =
            error.message.toLowerCase().contains('not found') ||
            error.message.toLowerCase().contains('no emergency');
        setState(() {
          _isLoading = false;
          if (!notFound) _loadError = error.message;
        });
      },
    );
  }

  void _prefill(EmergencyContact contact) {
    _nameController.text = contact.fullName;
    _phoneController.text = contact.phone.replaceFirst(RegExp(r'^\+234'), '');
    _relationship = _relationships.contains(contact.relationship)
        ? contact.relationship
        : null;
  }

  Future<void> _onSave() async {
    final phone = '+234${_phoneController.text.trim()}';
    setState(() => _isSaving = true);
    final result = await widget.profileController.saveEmergencyContact(
      fullName: _nameController.text.trim(),
      phone: phone,
      relationship: _relationship!,
    );
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Emergency contact saved',
          message: 'Your contact details have been updated.',
        );
        if (mounted) Navigator.of(context).pop();
      },
      failure: (error) async {
        await showHomeFeedback(
          context,
          title: 'Could not save details',
          message: error.message,
          error: true,
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Emergency contact',
    symbol: JourneySymbol.contact,
    description: 'Who should we contact in an emergency?',
    bottom: FilledButton(
      onPressed: _canSave && !_isLoading && _loadError == null ? _onSave : null,
      child: _isSaving
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Save details'),
    ),
    children: [
      if (_isLoading)
        const SizedBox(height: 240, child: ContentSkeleton(rows: 2))
      else if (_loadError != null)
        ActionNotice(message: _loadError!, isError: true, onRetry: _load)
      else ...[
        TextField(
          controller: _nameController,
          enabled: !_isSaving,
          textCapitalization: TextCapitalization.words,
          autofillHints: const [AutofillHints.name],
          onChanged: (_) => setState(() {}),
          decoration: const InputDecoration(labelText: 'Full name'),
        ),
        const SizedBox(height: 18),
        TextField(
          controller: _phoneController,
          enabled: !_isSaving,
          keyboardType: TextInputType.phone,
          autofillHints: const [AutofillHints.telephoneNumberNational],
          onChanged: (_) => setState(() {}),
          decoration: const InputDecoration(
            labelText: 'Phone number',
            prefixText: '+234 ',
          ),
        ),
        const SizedBox(height: 18),
        DropdownButtonFormField<String>(
          initialValue: _relationship,
          isExpanded: true,
          decoration: const InputDecoration(labelText: 'Relationship'),
          items: _relationships
              .map((r) => DropdownMenuItem(value: r, child: Text(r)))
              .toList(),
          onChanged: _isSaving
              ? null
              : (v) => setState(() => _relationship = v),
        ),
      ],
    ],
  );
}
