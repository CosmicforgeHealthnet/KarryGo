import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import '../state/provider_profile_controller.dart';
import 'widgets/change_phone_number_sheet.dart';

class ProfileInfoScreen extends StatefulWidget {
  const ProfileInfoScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProfileInfoScreen> createState() => _ProfileInfoScreenState();
}

class _ProfileInfoScreenState extends State<ProfileInfoScreen> {
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
  String _selectedLanguage = 'English (USA)';

  late String _phoneNumber;
  String _countryCode = '+234';
  String _countryFlag = '🇳🇬';

  bool _isDirty = false;
  bool _isSaving = false;
  bool _isUploadingAvatar = false;
  bool _isEditing = false;

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

  static const _languageOptions = ['English (USA)', 'English (UK)', 'French'];

  @override
  void initState() {
    super.initState();

    final profile = widget.profileController.profile;

    // Seed location dropdown if the stored state matches a known option.
    // Map legacy 'Federal Capital Territory' value to the display label.
    final storedState = profile?.state == 'Federal Capital Territory'
        ? 'Abuja (FCT)'
        : profile?.state;
    if (storedState != null && _stateOptions.contains(storedState)) {
      _selectedState = storedState;
    }

    // Phone numbers are stored in international format (e.g. "+2348067735987").
    // Split into a country code + local number for display/editing.
    // TODO: replace this with a proper phone-number/country-code utility
    // (e.g. via a country-picker package) once one is added to the project —
    // this currently only handles the +234 (Nigeria) case correctly.
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

  Widget _buildAvatar() {
    final photoUrl = widget.profileController.profile?.profilePhotoUrl;
    const double radius = 44;

    Widget placeholder = const CircleAvatar(
      radius: radius,
      backgroundColor: Color(0xFFE3EEFD),
      child: Icon(Icons.person, size: 42, color: Color(0xFF1A1A1A)),
    );

    if (_isUploadingAvatar) {
      return Stack(
        alignment: Alignment.center,
        children: [
          placeholder,
          const SizedBox(
            width: radius * 2,
            height: radius * 2,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: Color(0xFF4CAF50),
            ),
          ),
        ],
      );
    }

    if (photoUrl == null || photoUrl.isEmpty) return placeholder;

    return ClipOval(
      child: Image.network(
        photoUrl,
        width: radius * 2,
        height: radius * 2,
        fit: BoxFit.cover,
        errorBuilder: (_, _, _) => placeholder,
        loadingBuilder: (context, child, progress) {
          if (progress == null) return child;
          return placeholder;
        },
      ),
    );
  }

  Future<void> _onChangePhoto() async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.image,
      allowMultiple: false,
    );
    final path = result?.files.single.path;
    if (path == null || !mounted) return;

    setState(() => _isUploadingAvatar = true);
    final uploadResult = await widget.profileController.uploadAvatar(path);
    if (!mounted) return;
    setState(() => _isUploadingAvatar = false);
    uploadResult.when(
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Profile photo updated')),
        );
      },
      failure: (error) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(error.message)),
        );
      },
    );
  }

  Future<void> _onPhoneTap() async {
    final result = await showModalBottomSheet<Map<String, String>>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
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

  void _startEditing() {
    final profile = widget.profileController.profile;
    _fullNameController.text = profile?.fullName ?? '';
    _emailController.text = profile?.email ?? '';
    _cityController.text = profile?.city ?? '';
    final storedState = profile?.state == 'Federal Capital Territory'
        ? 'Abuja (FCT)'
        : profile?.state;
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
    setState(() {
      _selectedState = (storedState != null && _stateOptions.contains(storedState))
          ? storedState
          : null;
      _isDirty = false;
      _isEditing = true;
    });
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
    setState(() {
      _isSaving = false;
      _isDirty = false;
    });
    result.when(
      success: (_) {
        setState(() => _isEditing = false);
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('Profile updated')));
      },
      failure: (error) {
        setState(() => _isDirty = true);
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(error.message)));
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  // ── Header ────────────────────────────────────────
                  GestureDetector(
                    onTap: () {
                      if (_isEditing) {
                        setState(() => _isEditing = false);
                      } else {
                        Navigator.of(context).pop();
                      }
                    },
                    child: const Padding(
                      padding: EdgeInsets.only(bottom: 4),
                      child: Align(
                        alignment: Alignment.centerLeft,
                        child: Icon(
                          Icons.arrow_back_ios_new,
                          size: 20,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Profile Info',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Manage your Personal Information and Account',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 24),

                  // ── Avatar ────────────────────────────────────────
                  Center(
                    child: _isEditing
                        ? Column(
                            children: [
                              _buildAvatar(),
                              const SizedBox(height: 14),
                              Row(
                                mainAxisAlignment: MainAxisAlignment.center,
                                children: [
                                  _PillButton(
                                    label: _isUploadingAvatar
                                        ? 'Uploading…'
                                        : 'Change',
                                    filled: true,
                                    onTap: _isUploadingAvatar
                                        ? () {}
                                        : _onChangePhoto,
                                  ),
                                  const SizedBox(width: 12),
                                  _PillButton(
                                    label: 'Delete',
                                    filled: false,
                                    onTap: () {},
                                  ),
                                ],
                              ),
                            ],
                          )
                        : _buildAvatar(),
                  ),

                  const SizedBox(height: 28),

                  // ── View mode ─────────────────────────────────────
                  if (!_isEditing) ...[
                    _ViewRow(
                      label: 'Full Name',
                      value: widget.profileController.profile?.fullName ?? '',
                    ),
                    const SizedBox(height: 14),
                    _ViewRow(
                      label: 'Phone Number',
                      value: widget.profileController.profile?.phone ?? '',
                    ),
                    const SizedBox(height: 14),
                    _ViewRow(
                      label: 'Email Address',
                      value: widget.profileController.profile?.email ?? '',
                    ),
                    const SizedBox(height: 14),
                    _ViewRow(
                      label: 'State',
                      value: widget.profileController.profile?.state ?? '',
                    ),
                    const SizedBox(height: 14),
                    _ViewRow(
                      label: 'City / LGA',
                      value: widget.profileController.profile?.city ?? '',
                    ),
                    const SizedBox(height: 14),
                    _ViewRow(label: 'Language', value: _selectedLanguage),
                  ],

                  // ── Edit mode form fields ──────────────────────────
                  if (_isEditing) ...[
                    const _FieldLabel('Full Name'),
                    _TextInput(controller: _fullNameController),
                    const SizedBox(height: 18),

                    const _FieldLabel('Phone Number'),
                    _PhoneField(
                      flag: _countryFlag,
                      code: _countryCode,
                      number: _phoneNumber,
                      onTap: _onPhoneTap,
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Email Address'),
                    _TextInput(
                      controller: _emailController,
                      keyboardType: TextInputType.emailAddress,
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('State'),
                    _DropdownInput<String>(
                      value: _selectedState,
                      hint: 'Select State',
                      items: _stateOptions,
                      onChanged: (v) => setState(() {
                        _selectedState = v;
                        _cityController.clear();
                        _isDirty = true;
                      }),
                    ),
                    const SizedBox(height: 18),
                    const _FieldLabel('City / LGA'),
                    _TextInput(
                      controller: _cityController,
                      hint: 'e.g. Ikeja, Victoria Island',
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Language'),
                    _DropdownInput<String>(
                      value: _selectedLanguage,
                      hint: 'Select Language',
                      items: _languageOptions,
                      leadingFlag: '🇺🇸',
                      onChanged: (v) => setState(() {
                        if (v != null) _selectedLanguage = v;
                        _isDirty = true;
                      }),
                    ),
                  ],
                ],
              ),
            ),

            // ── Edit / Save button (pinned) ────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _isEditing
                      ? (_isDirty && !_isSaving ? _onSave : null)
                      : _startEditing,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor: const Color(
                      0xFF4CAF50,
                    ).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: _isSaving
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: Colors.white,
                          ),
                        )
                      : Text(
                          _isEditing ? 'Save' : 'Edit',
                          style: const TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w700,
                            color: Colors.white,
                          ),
                        ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Shared field widgets ────────────────────────────────────────────────────

class _ViewRow extends StatelessWidget {
  const _ViewRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(height: 6),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          decoration: BoxDecoration(
            color: const Color(0xFFF5F6F8),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Text(
            value.isEmpty ? '—' : value,
            style: const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w500,
              color: Color(0xFF1A1A1A),
            ),
          ),
        ),
      ],
    );
  }
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(
        text,
        style: const TextStyle(
          fontSize: 13,
          fontWeight: FontWeight.w700,
          color: Color(0xFF1A1A1A),
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
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      style: const TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w500,
        color: Color(0xFF1A1A1A),
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(color: Color(0xFFAAAAAA), fontSize: 14),
        filled: true,
        fillColor: const Color(0xFFF5F6F8),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 14,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
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
    this.leadingFlag,
  });

  final T? value;
  final String hint;
  final List<T> items;
  final ValueChanged<T?> onChanged;
  final String? leadingFlag;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: const Color(0xFFF5F6F8),
        borderRadius: BorderRadius.circular(12),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<T>(
          value: value,
          isExpanded: true,
          hint: Row(
            children: [
              if (leadingFlag != null) ...[
                Text(leadingFlag!, style: const TextStyle(fontSize: 16)),
                const SizedBox(width: 8),
              ],
              Text(
                hint,
                style: const TextStyle(color: Color(0xFFAAAAAA), fontSize: 14),
              ),
            ],
          ),
          icon: const Icon(Icons.keyboard_arrow_down, color: Color(0xFF888888)),
          items: items
              .map(
                (e) => DropdownMenuItem<T>(
                  value: e,
                  child: Row(
                    children: [
                      if (leadingFlag != null) ...[
                        Text(
                          leadingFlag!,
                          style: const TextStyle(fontSize: 16),
                        ),
                        const SizedBox(width: 8),
                      ],
                      Text(
                        '$e',
                        style: const TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w500,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    ],
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
    return Material(
      color: const Color(0xFFF5F6F8),
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          child: Row(
            children: [
              Text(flag, style: const TextStyle(fontSize: 16)),
              const SizedBox(width: 8),
              Text(
                '($code)',
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: Color(0xFF888888),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  number,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w500,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
              const Icon(
                Icons.chevron_right,
                size: 20,
                color: Color(0xFF888888),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _PillButton extends StatelessWidget {
  const _PillButton({
    required this.label,
    required this.filled,
    required this.onTap,
  });

  final String label;
  final bool filled;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 38,
      child: ElevatedButton(
        onPressed: onTap,
        style: ElevatedButton.styleFrom(
          backgroundColor: filled
              ? const Color(0xFF4CAF50)
              : const Color(0xFFEFF7EE),
          foregroundColor: filled ? Colors.white : const Color(0xFFAAAAAA),
          elevation: 0,
          padding: const EdgeInsets.symmetric(horizontal: 24),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(999),
          ),
        ),
        child: Text(
          label,
          style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700),
        ),
      ),
    );
  }
}
