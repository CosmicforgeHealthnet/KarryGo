import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/provider_profile_controller.dart';
import 'update_profile_photo_screen.dart';
import 'widgets/change_phone_number_sheet.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 13 — Edit Profile
// Fields: Full name · Email · Phone · Date of birth · Address (state + city)
// Save changes at the bottom.
// ═══════════════════════════════════════════════════════════════════════════

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
  DateTime? _dateOfBirth;
  bool _isDirty = false;
  bool _isSaving = false;

  String _countryCode = '+234';
  String _countryFlag = '🇳🇬';
  String _phoneNumber = '';

  static const _stateOptions = [
    'Abia', 'Adamawa', 'Akwa Ibom', 'Anambra', 'Bauchi', 'Bayelsa', 'Benue',
    'Borno', 'Cross River', 'Delta', 'Ebonyi', 'Edo', 'Ekiti', 'Enugu',
    'Abuja (FCT)', 'Gombe', 'Imo', 'Jigawa', 'Kaduna', 'Kano', 'Katsina',
    'Kebbi', 'Kogi', 'Kwara', 'Lagos', 'Nasarawa', 'Niger', 'Ogun', 'Ondo',
    'Osun', 'Oyo', 'Plateau', 'Rivers', 'Sokoto', 'Taraba', 'Yobe', 'Zamfara',
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

    final rawPhone = profile?.phone ?? '';
    if (rawPhone.startsWith('+234')) {
      _countryCode = '+234';
      _countryFlag = '🇳🇬';
      _phoneNumber = rawPhone.substring(4);
    } else if (rawPhone.startsWith('+')) {
      _countryCode = rawPhone.substring(0, 4);
      _phoneNumber = rawPhone.substring(4);
    } else {
      _phoneNumber = rawPhone;
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

  Future<void> _onPhoneTap() async {
    final result = await showModalBottomSheet<Map<String, String>>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: Colors.black.withValues(alpha: 0.72),
      builder: (_) => ChangePhoneNumberSheet(
        currentCountryFlag: _countryFlag,
        currentCountryCode: _countryCode,
        currentPhoneNumber: _phoneNumber,
      ),
    );
    if (result != null) {
      setState(() {
        _countryFlag = result['flag'] ?? _countryFlag;
        _countryCode = result['code'] ?? _countryCode;
        _phoneNumber = result['number'] ?? _phoneNumber;
        _isDirty = true;
      });
    }
  }

  Future<void> _pickDob() async {
    final picked = await showDatePicker(
      context: context,
      initialDate: _dateOfBirth ?? DateTime(1990, 1, 12),
      firstDate: DateTime(1940),
      lastDate: DateTime.now(),
    );
    if (picked != null) {
      setState(() {
        _dateOfBirth = picked;
        _isDirty = true;
      });
    }
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
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Profile updated')),
        );
        Navigator.of(context).pop();
      },
      failure: (error) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(error.message)),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final photoUrl = widget.profileController.profile?.profilePhotoUrl;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  Text(
                    'Edit profile',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.2,
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  const SizedBox(height: 8),
                  Center(
                    child: GestureDetector(
                      onTap: _openPhotoEditor,
                      behavior: HitTestBehavior.opaque,
                      child: _SmallAvatar(
                        photoUrl: photoUrl,
                        fallbackInitial:
                            (_fullNameController.text.isNotEmpty)
                                ? _fullNameController.text[0].toUpperCase()
                                : 'K',
                      ),
                    ),
                  ),
                  const SizedBox(height: 28),

                  const _FieldLabel('Full name'),
                  _TextInput(controller: _fullNameController),
                  const SizedBox(height: 20),

                  const _FieldLabel('Email'),
                  _TextInput(
                    controller: _emailController,
                    keyboardType: TextInputType.emailAddress,
                  ),
                  const SizedBox(height: 20),

                  const _FieldLabel('Phone number'),
                  _PhoneField(
                    flag: _countryFlag,
                    code: _countryCode,
                    number: _phoneNumber,
                    onTap: _onPhoneTap,
                  ),
                  const SizedBox(height: 20),

                  const _FieldLabel('Date of birth'),
                  _DateInput(
                    value: _dateOfBirth,
                    hint: 'Select date of birth',
                    onTap: _pickDob,
                  ),
                  const SizedBox(height: 20),

                  const _FieldLabel('State'),
                  _DropdownInput<String>(
                    value: _selectedState,
                    hint: 'Select state',
                    items: _stateOptions,
                    onChanged: (v) => setState(() {
                      _selectedState = v;
                      _isDirty = true;
                    }),
                  ),
                  const SizedBox(height: 20),

                  const _FieldLabel('City / LGA'),
                  _TextInput(
                    controller: _cityController,
                    hint: 'e.g. Ikeja',
                  ),
                ],
              ),
            ),

            // ── Save changes ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: (_isDirty && !_isSaving) ? _onSave : null,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: _isSaving
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: c.brandWhite,
                          ),
                        )
                      : const Text('Save changes'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Small widgets
// ═══════════════════════════════════════════════════════════════════════════

class _SmallAvatar extends StatelessWidget {
  const _SmallAvatar({
    required this.photoUrl,
    required this.fallbackInitial,
  });

  final String? photoUrl;
  final String fallbackInitial;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    const size = 88.0;

    Widget fallback = Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      color: c.teal10,
      child: Text(
        fallbackInitial,
        style: TextStyle(
          fontSize: 32,
          fontWeight: FontWeight.w800,
          color: c.brandTeal600,
        ),
      ),
    );

    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: c.divider),
      ),
      child: ClipOval(
        child: (photoUrl == null || photoUrl!.isEmpty)
            ? fallback
            : Image.network(
                photoUrl!,
                width: size,
                height: size,
                fit: BoxFit.cover,
                errorBuilder: (_, _, _) => fallback,
              ),
      ),
    );
  }
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Text(
        text,
        style: TextStyle(
          fontSize: 14,
          fontWeight: FontWeight.w700,
          color: c.text,
        ),
      ),
    );
  }
}

class _TextInput extends StatelessWidget {
  const _TextInput({required this.controller, this.hint, this.keyboardType});

  final TextEditingController controller;
  final String? hint;
  final TextInputType? keyboardType;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      style: TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w500,
        color: c.text,
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: TextStyle(
          color: c.textDisabled,
          fontSize: 15,
          fontWeight: FontWeight.w500,
        ),
        filled: true,
        fillColor: c.inputFill,
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: c.brandTeal500, width: 1.6),
        ),
      ),
    );
  }
}

class _DropdownInput<T> extends StatelessWidget {
  const _DropdownInput({
    required this.value,
    required this.hint,
    required this.items,
    required this.onChanged,
  });

  final T? value;
  final String hint;
  final List<T> items;
  final ValueChanged<T?> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: c.inputFill,
        borderRadius: BorderRadius.circular(12),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<T>(
          value: value,
          isExpanded: true,
          hint: Text(
            hint,
            style: TextStyle(
              color: c.textDisabled,
              fontSize: 15,
              fontWeight: FontWeight.w500,
            ),
          ),
          icon: Icon(
            Icons.keyboard_arrow_down_rounded,
            size: 22,
            color: c.textMuted,
          ),
          dropdownColor: c.surface,
          borderRadius: BorderRadius.circular(12),
          padding: const EdgeInsets.symmetric(vertical: 6),
          items: items
              .map(
                (e) => DropdownMenuItem<T>(
                  value: e,
                  child: Text(
                    '$e',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.text,
                    ),
                  ),
                ),
              )
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

class _DateInput extends StatelessWidget {
  const _DateInput({
    required this.value,
    required this.hint,
    required this.onTap,
  });

  final DateTime? value;
  final String hint;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
        decoration: BoxDecoration(
          color: c.inputFill,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                value == null ? hint : _fmt(value!),
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: value == null ? c.textDisabled : c.text,
                ),
              ),
            ),
            Icon(
              Icons.calendar_today_outlined,
              size: 20,
              color: c.textMuted,
            ),
          ],
        ),
      ),
    );
  }

  static String _fmt(DateTime d) {
    const m = [
      'January', 'February', 'March', 'April', 'May', 'June',
      'July', 'August', 'September', 'October', 'November', 'December',
    ];
    return '${d.day} ${m[d.month - 1]} ${d.year}';
  }
}

class _PhoneField extends StatelessWidget {
  const _PhoneField({
    required this.flag,
    required this.code,
    required this.number,
    required this.onTap,
  });

  final String flag;
  final String code;
  final String number;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Material(
      color: c.inputFill,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 18),
          child: Row(
            children: [
              Text(flag, style: const TextStyle(fontSize: 16)),
              const SizedBox(width: 8),
              Text(
                '($code)',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  number,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.text,
                  ),
                ),
              ),
              Icon(
                Icons.chevron_right_rounded,
                size: 22,
                color: c.textMuted,
              ),
            ],
          ),
        ),
      ),
    );
  }
}