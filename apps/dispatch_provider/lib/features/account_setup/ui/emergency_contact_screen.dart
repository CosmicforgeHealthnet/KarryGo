import '../../../shared/widgets/async_content.dart';
import '../../../core/app_mode.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../profile/state/provider_profile_controller.dart';

class EmergencyContactData {
  const EmergencyContactData({
    required this.fullName,
    required this.phone,
    required this.relationship,
  });

  final String fullName;
  final String phone;
  final String relationship;
}

class EmergencyContactScreen extends StatefulWidget {
  const EmergencyContactScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
    required this.profileController,
    this.initialData,
  });

  final ValueChanged<EmergencyContactData> onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;
  final ProviderProfileController profileController;
  final EmergencyContactData? initialData;

  @override
  State<EmergencyContactScreen> createState() => _EmergencyContactScreenState();
}

class _EmergencyContactScreenState extends State<EmergencyContactScreen> {
  final _nameController = TextEditingController();
  final _phoneController = TextEditingController();
  String? _relationship;
  bool _isLoading = false;

  static const _relationships = [
    'Parent',
    'Sibling',
    'Spouse',
    'Friend',
    'Child',
    'Other',
  ];

  @override
  void initState() {
    super.initState();
    if (AppModeConfig.instance.isMock) {
      _nameController.text = 'Ada Adeyemi';
      _phoneController.text = '8000000001';
      _relationship = 'Sibling';
    }
    final init = widget.initialData;
    if (init != null) {
      _nameController.text = init.fullName;
      String p = init.phone;
      if (p.startsWith('+234')) p = p.substring(4);
      _phoneController.text = p;
      _relationship = init.relationship;
    } else {
      _loadExisting();
    }
  }

  Future<void> _loadExisting() async {
    final result = await widget.profileController.loadEmergencyContact();
    if (!mounted) return;
    result.when(
      success: (data) {
        setState(() {
          _nameController.text = data.fullName;
          String p = data.phone;
          if (p.startsWith('+234')) p = p.substring(4);
          _phoneController.text = p;
          if (_relationships.contains(data.relationship)) {
            _relationship = data.relationship;
          } else {
            _relationship = 'Other';
          }
        });
      },
      failure: (_) {},
    );
  }

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    super.dispose();
  }

  bool get _canContinue =>
      _nameController.text.trim().isNotEmpty &&
      _phoneController.text.trim().isNotEmpty &&
      _relationship != null;

  void _continue() async {
    final name = _nameController.text.trim();
    if (name.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).length < 2) {
      _showError('Please enter the full name.');
      return;
    }
    final rawPhone = _phoneController.text.trim();
    String subscriber = rawPhone;
    while (subscriber.startsWith('0')) {
      subscriber = subscriber.substring(1);
    }
    if (subscriber.length != 10 || !RegExp(r'^\d{10}$').hasMatch(subscriber)) {
      _showError('Please enter a valid Nigerian phone number.');
      return;
    }
    final phone = '+234$subscriber';

    setState(() => _isLoading = true);
    try {
      final result = await widget.profileController.saveEmergencyContact(
        fullName: name,
        phone: phone,
        relationship: _relationship!,
      );

      bool ok = false;
      String? err;
      result.when(success: (_) => ok = true, failure: (e) => err = e.message);

      if (!ok) {
        if (mounted && err != null) _showError('Save failed: $err');
        return;
      }

      widget.onContinue(
        EmergencyContactData(
          fullName: name,
          phone: phone,
          relationship: _relationship!,
        ),
      );
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
                      'Emergency contact',
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
                      'Add someone we can reach in case of an emergency.',
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
                      hint: 'Grace Okafor',
                      keyboardType: TextInputType.name,
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(r"[a-zA-Z '\-]"),
                        ),
                      ],
                      onChanged: (_) => setState(() {}),
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Relationship'),
                    const SizedBox(height: 10),
                    _RelationshipDropdown(
                      value: _relationship,
                      items: _relationships,
                      onChanged: (v) => setState(() => _relationship = v),
                    ),
                    const SizedBox(height: 18),

                    const _FieldLabel('Phone number'),
                    const SizedBox(height: 10),
                    _PhoneField(
                      controller: _phoneController,
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

// ─── Step Progress ──────────────────────────────────────────────────────────

// ─── Field Label ────────────────────────────────────────────────────────────

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Text(
      text,
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
  });

  final TextEditingController controller;
  final String hint;
  final TextInputType? keyboardType;
  final List<TextInputFormatter>? inputFormatters;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      onChanged: onChanged,
      style: TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w500,
        color: c.text,
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: TextStyle(
          fontSize: 15,
          fontWeight: FontWeight.w400,
          color: c.textDisabled,
        ),
        filled: true,
        fillColor: c.inputFill,
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

// ─── Relationship Dropdown ──────────────────────────────────────────────────

class _RelationshipDropdown extends StatelessWidget {
  const _RelationshipDropdown({
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final String? value;
  final List<String> items;
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
            'Select relationship',
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
          items: items
              .map((s) => DropdownMenuItem(value: s, child: Text(s)))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// ─── Phone Field (with +234 prefix) ─────────────────────────────────────────

class _PhoneField extends StatelessWidget {
  const _PhoneField({required this.controller, this.onChanged});

  final TextEditingController controller;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      height: 56,
      decoration: BoxDecoration(
        color: c.inputFill,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: c.border),
      ),
      child: Row(
        children: [
          const SizedBox(width: 14),
          const _NigeriaFlag(),
          const SizedBox(width: 8),
          Text(
            '+234',
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: c.text,
            ),
          ),
          const SizedBox(width: 12),
          Container(width: 1, height: 24, color: c.border),
          const SizedBox(width: 12),
          Expanded(
            child: Theme(
              data: Theme.of(context).copyWith(
                inputDecorationTheme: const InputDecorationTheme(
                  border: InputBorder.none,
                  enabledBorder: InputBorder.none,
                  focusedBorder: InputBorder.none,
                  disabledBorder: InputBorder.none,
                  errorBorder: InputBorder.none,
                  focusedErrorBorder: InputBorder.none,
                  filled: false,
                ),
              ),
              child: TextField(
                controller: controller,
                keyboardType: TextInputType.phone,
                inputFormatters: [
                  FilteringTextInputFormatter.digitsOnly,
                  LengthLimitingTextInputFormatter(11),
                ],
                onChanged: onChanged,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: c.text,
                ),
                decoration: InputDecoration(
                  hintText: '803 123 4567',
                  hintStyle: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w400,
                    color: c.textDisabled,
                  ),
                  border: InputBorder.none,
                  isDense: true,
                  contentPadding: const EdgeInsets.symmetric(vertical: 18),
                ),
              ),
            ),
          ),
          const SizedBox(width: 16),
        ],
      ),
    );
  }
}

// ─── Nigeria flag ───────────────────────────────────────────────────────────

class _NigeriaFlag extends StatelessWidget {
  const _NigeriaFlag();

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(3),
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          _Stripe(color: Color(0xFF008751)),
          _Stripe(color: Colors.white),
          _Stripe(color: Color(0xFF008751)),
        ],
      ),
    );
  }
}

class _Stripe extends StatelessWidget {
  const _Stripe({required this.color});
  final Color color;

  @override
  Widget build(BuildContext context) =>
      Container(width: 7, height: 16, color: color);
}
