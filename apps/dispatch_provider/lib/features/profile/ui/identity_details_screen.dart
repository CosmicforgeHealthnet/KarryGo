import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';
import 'identity_upload_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 03 — Identity details form
// ═══════════════════════════════════════════════════════════════════════════

class IdentityDetailsScreen extends StatefulWidget {
  const IdentityDetailsScreen({
    super.key,
    required this.verificationController,
  });

  final VerificationController verificationController;

  @override
  State<IdentityDetailsScreen> createState() => _IdentityDetailsScreenState();
}

class _IdentityDetailsScreenState extends State<IdentityDetailsScreen> {
  String? _idType;
  final _ninController = TextEditingController();
  final _nameController = TextEditingController();
  DateTime? _dob;
  String? _gender;

  static const _idTypes = {
    'nin': 'National Identification Number (NIN)',
    'voter_card': "Voter's Card",
    'passport': 'International Passport',
    'drivers_licence': "Driver's Licence",
  };

  static const _genders = {'male': 'Male', 'female': 'Female'};

  @override
  void dispose() {
    _ninController.dispose();
    _nameController.dispose();
    super.dispose();
  }

  bool get _valid =>
      _idType != null &&
      _ninController.text.trim().length >= 5 &&
      _nameController.text.trim().isNotEmpty &&
      _dob != null &&
      _gender != null;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header: back · Help ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const Spacer(),
                  GestureDetector(
                    onTap: () {},
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(
                        'Help',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: c.brandTeal500,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  // ── Title ──
                  Text(
                    'Identity details',
                    style: TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'Enter your details as they appear on your government ID.',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  // ── ID type ──
                  const _Label('ID type'),
                  _Dropdown(
                    hint: 'Select ID type',
                    value: _idType,
                    items: _idTypes,
                    onChanged: (v) => setState(() => _idType = v),
                  ),
                  const SizedBox(height: 20),

                  // ── NIN number ──
                  const _Label('NIN number'),
                  _Field(
                    controller: _ninController,
                    hint: '1234 5678 9012',
                    keyboardType: TextInputType.number,
                    inputFormatters: [
                      FilteringTextInputFormatter.digitsOnly,
                      LengthLimitingTextInputFormatter(11),
                    ],
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  // ── Full name ──
                  const _Label('Full name'),
                  _Field(
                    controller: _nameController,
                    hint: 'Daniel Chukwudi Okafor',
                    textCapitalization: TextCapitalization.words,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  // ── Date of birth ──
                  const _Label('Date of birth'),
                  _DateField(
                    value: _dob,
                    hint: '12 March 1992',
                    firstDate: DateTime(1940),
                    lastDate: DateTime.now(),
                    onChanged: (v) => setState(() => _dob = v),
                  ),
                  const SizedBox(height: 20),

                  // ── Gender ──
                  const _Label('Gender'),
                  _Dropdown(
                    hint: 'Select gender',
                    value: _gender,
                    items: _genders,
                    onChanged: (v) => setState(() => _gender = v),
                  ),
                ],
              ),
            ),

            // ── Continue ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _valid
                      ? () async {
                          final r = await Navigator.of(context).push<bool>(
                            MaterialPageRoute(
                              builder: (_) => IdentityUploadScreen(
                                verificationController:
                                    widget.verificationController,
                                idType: _idType!,
                                idNumber: _ninController.text.trim(),
                                fullName: _nameController.text.trim(),
                                dateOfBirth: _dob!,
                                gender: _gender!,
                              ),
                            ),
                          );
                          if (!context.mounted) return;
                          if (r == true) Navigator.of(context).pop(true);
                        }
                      : null,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Continue'),
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
// Label
// ═══════════════════════════════════════════════════════════════════════════

class _Label extends StatelessWidget {
  const _Label(this.text);
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

// ═══════════════════════════════════════════════════════════════════════════
// Text field
// ═══════════════════════════════════════════════════════════════════════════

class _Field extends StatelessWidget {
  const _Field({
    required this.controller,
    required this.hint,
    required this.onChanged,
    this.keyboardType,
    this.inputFormatters,
    this.textCapitalization = TextCapitalization.none,
  });

  final TextEditingController controller;
  final String hint;
  final VoidCallback onChanged;
  final TextInputType? keyboardType;
  final List<TextInputFormatter>? inputFormatters;
  final TextCapitalization textCapitalization;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      textCapitalization: textCapitalization,
      onChanged: (_) => onChanged(),
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
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 18,
        ),
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

// ═══════════════════════════════════════════════════════════════════════════
// Dropdown
// ═══════════════════════════════════════════════════════════════════════════

class _Dropdown extends StatelessWidget {
  const _Dropdown({
    required this.hint,
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final String hint;
  final String? value;
  final Map<String, String> items;
  final ValueChanged<String?> onChanged;

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
        child: DropdownButton<String>(
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
          items: items.entries
              .map(
                (e) => DropdownMenuItem(
                  value: e.key,
                  child: Text(
                    e.value,
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

// ═══════════════════════════════════════════════════════════════════════════
// Date field
// ═══════════════════════════════════════════════════════════════════════════

class _DateField extends StatelessWidget {
  const _DateField({
    required this.value,
    required this.hint,
    required this.firstDate,
    required this.lastDate,
    required this.onChanged,
  });

  final DateTime? value;
  final String hint;
  final DateTime firstDate;
  final DateTime lastDate;
  final ValueChanged<DateTime> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: () async {
        final picked = await showDatePicker(
          context: context,
          initialDate: value ?? DateTime(2000),
          firstDate: firstDate,
          lastDate: lastDate,
        );
        if (picked != null) onChanged(picked);
      },
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
            Icon(Icons.calendar_today_outlined, size: 20, color: c.textMuted),
          ],
        ),
      ),
    );
  }

  static String _fmt(DateTime d) {
    const m = [
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
    return '${d.day} ${m[d.month - 1]} ${d.year}';
  }
}
