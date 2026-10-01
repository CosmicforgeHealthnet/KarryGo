import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';
import 'licence_upload_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 08 — Driver's licence details
// ═══════════════════════════════════════════════════════════════════════════

class LicenceDetailsScreen extends StatefulWidget {
  const LicenceDetailsScreen({
    super.key,
    required this.verificationController,
  });

  final VerificationController verificationController;

  @override
  State<LicenceDetailsScreen> createState() =>
      _LicenceDetailsScreenState();
}

class _LicenceDetailsScreenState extends State<LicenceDetailsScreen> {
  final _numberController = TextEditingController();
  String? _type;
  DateTime? _expiry;
  String? _state;

  static const _types = {
    'private_b': 'Private (Class B)',
    'commercial': 'Commercial',
    'learner': 'Learner',
  };

  static const _states = [
    'Lagos',
    'Abuja FCT',
    'Rivers',
    'Oyo',
    'Kano',
    'Enugu',
    'Kaduna',
    'Ogun',
    'Anambra',
    'Delta',
  ];

  @override
  void dispose() {
    _numberController.dispose();
    super.dispose();
  }

  bool get _valid =>
      _numberController.text.trim().length >= 5 &&
      _type != null &&
      _expiry != null &&
      _state != null;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header: back · title · Help ──
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
                  Expanded(
                    child: Text(
                      "Driver's licence details",
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
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
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 24),
                children: [
                  // ── Subtitle ──
                  Text(
                    "Enter your driver's licence information.",
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  // ── Licence number ──
                  const _Label('Licence number'),
                  _Field(
                    controller: _numberController,
                    hint: 'LAG12345678',
                    textCapitalization: TextCapitalization.characters,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  // ── Licence type ──
                  const _Label('Licence type'),
                  _Dropdown(
                    hint: 'Select licence type',
                    value: _type,
                    items: _types,
                    onChanged: (v) => setState(() => _type = v),
                  ),
                  const SizedBox(height: 20),

                  // ── Expiry date ──
                  const _Label('Expiry date'),
                  _DateField(
                    value: _expiry,
                    hint: 'Select expiry date',
                    firstDate: DateTime.now(),
                    lastDate:
                        DateTime.now().add(const Duration(days: 365 * 20)),
                    onChanged: (v) => setState(() => _expiry = v),
                  ),
                  const SizedBox(height: 20),

                  // ── Issuing state ──
                  const _Label('Issuing state'),
                  _Dropdown(
                    hint: 'Select issuing state',
                    value: _state,
                    items: {for (final s in _states) s: s},
                    onChanged: (v) => setState(() => _state = v),
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
                              builder: (_) => LicenceUploadScreen(
                                verificationController:
                                    widget.verificationController,
                                licenceNumber:
                                    _numberController.text.trim(),
                                licenceType: _type!,
                                expiry: _expiry!,
                                issuingState: _state!,
                                resubmission: false,
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
    this.textCapitalization = TextCapitalization.none,
  });

  final TextEditingController controller;
  final String hint;
  final VoidCallback onChanged;
  final TextCapitalization textCapitalization;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
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
          initialDate:
              value ?? DateTime.now().add(const Duration(days: 365)),
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