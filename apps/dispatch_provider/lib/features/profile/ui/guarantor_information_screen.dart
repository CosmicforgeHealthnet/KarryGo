import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';

import '../models/provider_profile_models.dart';
import '../state/provider_profile_controller.dart';

class GuarantorInformationScreen extends StatefulWidget {
  const GuarantorInformationScreen({
    super.key,
    required this.profileController,
  });

  final ProviderProfileController profileController;

  @override
  State<GuarantorInformationScreen> createState() =>
      _GuarantorInformationScreenState();
}

class _GuarantorInformationScreenState
    extends State<GuarantorInformationScreen> {
  bool _isLoading = false;
  bool _isSaving = false;
  String? _loadError;

  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();
  bool get _canSave =>
      _nameController.text.trim().isNotEmpty &&
      _phoneController.text.trim().isNotEmpty &&
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

    final cached = widget.profileController.guarantor;
    if (cached != null) {
      _prefill(cached);
      setState(() => _isLoading = false);
      return;
    }

    final result = await widget.profileController.loadGuarantor();
    if (!mounted) return;
    result.when(
      success: (g) {
        _prefill(g);
        setState(() => _isLoading = false);
      },
      failure: (error) {
        final notFound =
            error.message.toLowerCase().contains('not found') ||
            error.message.toLowerCase().contains('no guarantor');
        setState(() {
          _isLoading = false;
          if (!notFound) _loadError = error.message;
        });
      },
    );
  }

  void _prefill(Guarantor g) {
    _nameController.text = g.fullName;
    _phoneController.text = g.phone.replaceFirst(RegExp(r'^\+234'), '');
  }

  Future<void> _onSave() async {
    final phone = '+234${_phoneController.text.trim()}';
    setState(() => _isSaving = true);
    final result = await widget.profileController.saveGuarantor(
      fullName: _nameController.text.trim(),
      phone: phone,
    );
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Guarantor saved',
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
    title: 'Guarantor',
    symbol: JourneySymbol.partner,
    description: 'Keep your guarantor contact details up to date.',
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
      ],
    ],
  );
}
