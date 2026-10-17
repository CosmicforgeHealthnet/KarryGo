import '../../../shared/widgets/async_content.dart';
import '../../../core/app_mode.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../profile/state/provider_profile_controller.dart';

class ProfileDetailsScreen extends StatefulWidget {
  const ProfileDetailsScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
    this.initialPhone,
    this.initialEmail,
    required this.profileController,
    required this.operationType,
  });

  final Future<void> Function(ProfileDetailsData) onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;
  final String? initialPhone;
  final String? initialEmail;
  final ProviderProfileController profileController;
  final String? operationType;

  @override
  State<ProfileDetailsScreen> createState() => _ProfileDetailsScreenState();
}

class _ProfileDetailsScreenState extends State<ProfileDetailsScreen> {
  final _nameController = TextEditingController();
  final _emailController = TextEditingController();
  final _cityController = TextEditingController();
  DateTime? _dateOfBirth;
  String? _selectedState;
  bool _isLoading = false;
  bool _emailReadOnly = false;

  static const _states = [
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
    'FCT',
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
    if (AppModeConfig.instance.isMock) {
      _dateOfBirth = DateTime(1995, 6, 15);
      _selectedState = 'Lagos';
    }
    _loadProfile();
  }

  Future<void> _loadProfile() async {
    if (widget.initialEmail != null && widget.initialEmail!.isNotEmpty) {
      _emailController.text = widget.initialEmail!;
      _emailReadOnly = true;
    }

    setState(() => _isLoading = true);
    final result = await widget.profileController.loadMe();
    result.when(
      success: (profile) {
        if (!mounted) return;
        setState(() {
          _nameController.text =
              profile.fullName ??
              (AppModeConfig.instance.isMock ? 'Tobi Adeyemi' : '');
          if (!_emailReadOnly) {
            _emailController.text =
                profile.email ??
                (AppModeConfig.instance.isMock ? 'rider@example.com' : '');
          }
          _cityController.text =
              profile.city ?? (AppModeConfig.instance.isMock ? 'Ikeja' : '');
          if (profile.state != null && profile.state!.isNotEmpty) {
            _selectedState = profile.state;
          }
        });
      },
      failure: (error) {
        debugPrint('[PROFILE] loadMe failed: ${error.message}');
      },
    );
    if (mounted) setState(() => _isLoading = false);
  }

  @override
  void dispose() {
    _nameController.dispose();
    _emailController.dispose();
    _cityController.dispose();
    super.dispose();
  }

  bool get _canContinue =>
      _nameController.text.trim().isNotEmpty &&
      _dateOfBirth != null &&
      _emailController.text.trim().isNotEmpty &&
      _selectedState != null &&
      _cityController.text.trim().isNotEmpty;

  static const _monthNames = [
    'January',
    'February',
    'March',
    'April',
    'May',
    'June',
    'July',
    'August',
    'September',
    'October',
    'November',
    'December',
  ];

  String _formatDate(DateTime d) =>
      '${d.day} ${_monthNames[d.month - 1]} ${d.year}';

  Future<void> _pickDate() async {
    final c = context.palette;
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: _dateOfBirth ?? DateTime(now.year - 25, 1, 1),
      firstDate: DateTime(1940),
      lastDate: DateTime(now.year - 18, 12, 31),
      helpText: 'Select date of birth',
      builder: (context, child) {
        return Theme(
          data: Theme.of(context).copyWith(
            colorScheme: Theme.of(context).colorScheme.copyWith(
              primary: c.brandTeal500,
              onPrimary: Colors.white,
            ),
          ),
          child: child!,
        );
      },
    );
    if (picked != null) setState(() => _dateOfBirth = picked);
  }

  static bool _isValidEmail(String email) =>
      RegExp(r'^[\w.+\-]+@[\w\-]+\.[\w.\-]+$').hasMatch(email);

  void _continue() async {
    final name = _nameController.text.trim();
    if (name.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).length < 2) {
      _showError('Please enter your full name.');
      return;
    }
    final email = _emailController.text.trim();
    if (!_isValidEmail(email)) {
      _showError('Please enter a valid email address.');
      return;
    }
    if (_selectedState == null) {
      _showError('Please select your state.');
      return;
    }

    setState(() => _isLoading = true);
    try {
      // Saves name/email/state/city WITHOUT flipping onboarding_complete.
      // The completion flag is set on the Review screen's submit.
      final result = await widget.profileController.updateMe(
        fullName: name,
        email: email,
        state: _selectedState!,
        city: _cityController.text.trim(),
      );

      bool ok = false;
      String? err;
      result.when(
        success: (_) => ok = true,
        failure: (error) => err = error.message,
      );

      if (!ok) {
        if (mounted && err != null) _showError('Save failed: $err');
        return;
      }

      final data = ProfileDetailsData(
        fullName: name,
        phone: widget.initialPhone ?? '',
        state: _selectedState!,
        city: _cityController.text.trim(),
        email: email,
        dateOfBirth: _dateOfBirth!,
      );

      await widget.onContinue(data);
    } catch (e) {
      if (mounted) _showError('$e'.replaceAll('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  void _showError(String msg) {
    if (mounted) showDispatchError(context, msg);
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const SizedBox(height: 8),
                    DispatchSetupHeader(
                      current: widget.currentStep,
                      total: widget.totalSteps,
                      onBack: widget.onBack,
                    ),
                    const SizedBox(height: 28),
                    Text(
                      'Basic profile',
                      style: TextStyle(
                        fontSize: 28,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.4,
                        height: 1.2,
                      ),
                    ),
                    const SizedBox(height: 10),
                    Text(
                      'Tell us about yourself.',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w400,
                        color: c.textMuted,
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 32),

                    const _FieldLabel('Full name'),
                    const SizedBox(height: 10),
                    _InputField(
                      controller: _nameController,
                      hint: 'Daniel Okafor',
                      keyboardType: TextInputType.name,
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Date of birth'),
                    const SizedBox(height: 10),
                    _DateField(
                      value: _dateOfBirth == null
                          ? null
                          : _formatDate(_dateOfBirth!),
                      onTap: _pickDate,
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Email address', isOptional: true),
                    const SizedBox(height: 10),
                    _InputField(
                      controller: _emailController,
                      hint: 'youremail@example.com',
                      keyboardType: TextInputType.emailAddress,
                      inputFormatters: [
                        FilteringTextInputFormatter.deny(RegExp(r'\s')),
                      ],
                      onChanged: (_) => setState(() {}),
                      readOnly: _emailReadOnly,
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('State'),
                    const SizedBox(height: 10),
                    _StateDropdown(
                      value: _selectedState,
                      states: _states,
                      onChanged: (v) => setState(() => _selectedState = v),
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('City'),
                    const SizedBox(height: 10),
                    _InputField(
                      controller: _cityController,
                      hint: 'Lekki',
                      keyboardType: TextInputType.text,
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 24),
                  ],
                ),
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 20),
              child: SizedBox(
                height: 58,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canContinue && !_isLoading ? _continue : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: c.brandTeal500,
                    disabledBackgroundColor: c.brandTeal500.withValues(
                      alpha: 0.45,
                    ),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: _isLoading
                      ? const SizedBox(
                          width: 22,
                          height: 22,
                          child: CircularProgressIndicator(
                            color: Color(0xFF081F44),
                            strokeWidth: 2,
                          ),
                        )
                      : const Text(
                          'Continue',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: Color(0xFF081F44),
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

class ProfileDetailsData {
  const ProfileDetailsData({
    required this.fullName,
    required this.phone,
    required this.state,
    required this.city,
    required this.email,
    required this.dateOfBirth,
  });

  final String fullName;
  final String phone;
  final String state;
  final String city;
  final String email;
  final DateTime dateOfBirth;
}

// ─── Step Progress ──────────────────────────────────────────────────────────

// ─── Field Label ────────────────────────────────────────────────────────────

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text, {this.isOptional = false});
  final String text;
  final bool isOptional;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Text.rich(
      TextSpan(
        text: text,
        children: [
          if (isOptional)
            TextSpan(
              text: ' (optional)',
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w400,
                color: c.textMuted,
              ),
            ),
        ],
      ),
      style: TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w600,
        color: c.text,
      ),
    );
  }
}

// ─── Input Field ────────────────────────────────────────────────────────────

class _InputField extends StatelessWidget {
  const _InputField({
    required this.controller,
    required this.hint,
    this.keyboardType,
    this.inputFormatters,
    this.onChanged,
    this.readOnly = false,
  });

  final TextEditingController controller;
  final String hint;
  final TextInputType? keyboardType;
  final List<TextInputFormatter>? inputFormatters;
  final ValueChanged<String>? onChanged;
  final bool readOnly;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      onChanged: onChanged,
      readOnly: readOnly,
      style: TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w500,
        color: readOnly ? c.textMuted : c.text,
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: TextStyle(
          fontSize: 15,
          fontWeight: FontWeight.w400,
          color: c.textDisabled,
        ),
        filled: true,
        fillColor: readOnly ? c.navy10 : c.inputFill,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 18,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: c.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: c.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: c.brandTeal500, width: 1.5),
        ),
      ),
    );
  }
}

// ─── State Dropdown ─────────────────────────────────────────────────────────

class _StateDropdown extends StatelessWidget {
  const _StateDropdown({
    required this.value,
    required this.states,
    required this.onChanged,
  });

  final String? value;
  final List<String> states;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      height: 56,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: c.inputFill,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: c.border),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          hint: Text(
            'Select state',
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w400,
              color: c.textDisabled,
            ),
          ),
          isExpanded: true,
          icon: Icon(Icons.keyboard_arrow_down_rounded, color: c.text),
          dropdownColor: c.surface,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w500,
            color: c.text,
          ),
          items: states
              .map((s) => DropdownMenuItem(value: s, child: Text(s)))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// ─── Date Field ─────────────────────────────────────────────────────────────

class _DateField extends StatelessWidget {
  const _DateField({required this.value, required this.onTap});
  final String? value;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final hasValue = value != null && value!.isNotEmpty;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        height: 56,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: c.inputFill,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: c.border),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                hasValue ? value! : '12 May 1992',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: hasValue ? FontWeight.w500 : FontWeight.w400,
                  color: hasValue ? c.text : c.textDisabled,
                ),
              ),
            ),
            Icon(Icons.calendar_today_outlined, size: 20, color: c.text),
          ],
        ),
      ),
    );
  }
}
