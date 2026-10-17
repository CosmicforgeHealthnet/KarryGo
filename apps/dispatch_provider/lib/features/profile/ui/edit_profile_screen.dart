import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'new_phone_number_screen.dart';
import 'package:flutter/material.dart';

import '../state/provider_profile_controller.dart';
import 'update_profile_photo_screen.dart';

class EditProfileScreen extends StatefulWidget {
  const EditProfileScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<EditProfileScreen> createState() => _EditProfileScreenState();
}

class _EditProfileScreenState extends State<EditProfileScreen> {
  late final _fullNameController = TextEditingController(
    text: widget.profileController.profile?.fullName ?? '',
  );
  late final _emailController = TextEditingController(
    text: widget.profileController.profile?.email ?? '',
  );
  late final _cityController = TextEditingController(
    text: widget.profileController.profile?.city ?? '',
  );

  String? _selectedState;
  bool _isDirty = false;
  bool _isSaving = false;

  static const _stateOptions = [
    'Abia',
    'Adamawa',
    'Akwa Ibom',
    'Anambra',
    'Bauchi',
    'Bayelsa',
    'Benue',
    'Borno',
    'Cross River',
    'Delta',
    'Ebonyi',
    'Edo',
    'Ekiti',
    'Enugu',
    'Abuja (FCT)',
    'Gombe',
    'Imo',
    'Jigawa',
    'Kaduna',
    'Kano',
    'Katsina',
    'Kebbi',
    'Kogi',
    'Kwara',
    'Lagos',
    'Nasarawa',
    'Niger',
    'Ogun',
    'Ondo',
    'Osun',
    'Oyo',
    'Plateau',
    'Rivers',
    'Sokoto',
    'Taraba',
    'Yobe',
    'Zamfara',
  ];

  @override
  void initState() {
    super.initState();
    final profile = widget.profileController.profile;

    final storedState = profile?.state == 'Federal Capital Territory'
        ? 'Abuja (FCT)'
        : profile?.state;
    if (storedState != null && _stateOptions.contains(storedState)) {
      _selectedState = storedState;
    }

    _fullNameController.addListener(_markDirty);
    _emailController.addListener(_markDirty);
    _cityController.addListener(_markDirty);
  }

  void _markDirty() {
    if (!_isDirty) setState(() => _isDirty = true);
  }

  @override
  void dispose() {
    _fullNameController.dispose();
    _emailController.dispose();
    _cityController.dispose();
    super.dispose();
  }

  Future<void> _openPhotoEditor() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => UpdateProfilePhotoScreen(
          profileController: widget.profileController,
        ),
      ),
    );
    if (mounted) setState(() {});
  }

  Future<void> _onSave() async {
    setState(() => _isSaving = true);
    final result = await widget.profileController.updateMe(
      fullName: _fullNameController.text.trim().isNotEmpty
          ? _fullNameController.text.trim()
          : null,
      email: _emailController.text.trim().isNotEmpty
          ? _emailController.text.trim()
          : null,
      state: _selectedState,
      city: _cityController.text.trim().isNotEmpty
          ? _cityController.text.trim()
          : null,
    );
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Profile updated',
          message: 'Your changes have been saved.',
        );
        if (mounted) Navigator.of(context).pop();
      },
      failure: (error) async {
        await showHomeFeedback(
          context,
          title: 'Could not update profile',
          message: error.message,
          error: true,
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Edit profile',
    symbol: JourneySymbol.profile,
    description: 'Keep your personal details up to date.',
    bottom: FilledButton(
      onPressed: _isDirty && !_isSaving ? _onSave : null,
      child: _isSaving
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Save changes'),
    ),
    children: [
      DeliveryPanel(
        padding: EdgeInsets.zero,
        child: AccountMenuTile(
          title: 'Profile photo',
          symbol: JourneySymbol.photo,
          subtitle: 'Take a photo or choose from your gallery',
          onTap: _isSaving ? null : _openPhotoEditor,
        ),
      ),
      const SizedBox(height: 24),
      TextField(
        controller: _fullNameController,
        enabled: !_isSaving,
        textCapitalization: TextCapitalization.words,
        autofillHints: const [AutofillHints.name],
        decoration: const InputDecoration(labelText: 'Full name'),
      ),
      const SizedBox(height: 18),
      TextField(
        controller: _emailController,
        enabled: !_isSaving,
        keyboardType: TextInputType.emailAddress,
        autofillHints: const [AutofillHints.email],
        decoration: const InputDecoration(labelText: 'Email address'),
      ),
      const SizedBox(height: 18),
      DeliveryPanel(
        padding: EdgeInsets.zero,
        child: AccountMenuTile(
          title: 'Phone number',
          symbol: JourneySymbol.contact,
          subtitle: widget.profileController.profile?.phone,
          onTap: _isSaving
              ? null
              : () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => const NewPhoneNumberScreen(
                      countryFlag: 'NG',
                      countryCode: '+234',
                    ),
                  ),
                ),
        ),
      ),
      const SizedBox(height: 24),
      DropdownButtonFormField<String>(
        initialValue: _selectedState,
        isExpanded: true,
        decoration: const InputDecoration(labelText: 'State'),
        items: _stateOptions
            .map((s) => DropdownMenuItem(value: s, child: Text(s)))
            .toList(),
        onChanged: _isSaving
            ? null
            : (v) => setState(() {
                _selectedState = v;
                _isDirty = true;
              }),
      ),
      const SizedBox(height: 18),
      TextField(
        controller: _cityController,
        enabled: !_isSaving,
        textCapitalization: TextCapitalization.words,
        decoration: const InputDecoration(labelText: 'City'),
      ),
    ],
  );
}
