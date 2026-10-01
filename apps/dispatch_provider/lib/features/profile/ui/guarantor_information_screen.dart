import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../models/provider_profile_models.dart';
import '../state/provider_profile_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 16 — Guarantor information
// NOTE: The `Guarantor` model has no relationship field. The Relationship
// dropdown is UI-only for now — it matches the Figma but is not persisted.
// Extend the model + API if you want it saved.
// ═══════════════════════════════════════════════════════════════════════════

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
    _load();
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
        final notFound = error.message.toLowerCase().contains('not found') ||
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
    // Relationship is UI-only — leave blank when loading existing.
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
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Guarantor information saved')),
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

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: _isLoading
            ? Center(
                child: CircularProgressIndicator(color: c.brandTeal500),
              )
            : Column(
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
                      ],
                    ),
                  ),

                  Expanded(
                    child: ListView(
                      padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                      children: [
                        Text(
                          'Guarantor information',
                          style: TextStyle(
                            fontSize: 26,
                            fontWeight: FontWeight.w800,
                            color: c.text,
                            letterSpacing: -0.4,
                          ),
                        ),
                        const SizedBox(height: 6),
                        Text(
                          'Your guarantor helps build trust on the platform.',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w500,
                            color: c.textMuted,
                            height: 1.45,
                          ),
                        ),
                        const SizedBox(height: 28),

                        if (_loadError != null) ...[
                          Text(
                            _loadError!,
                            style: TextStyle(
                              fontSize: 14,
                              color: c.danger,
                            ),
                          ),
                          const SizedBox(height: 12),
                          OutlinedButton(
                            onPressed: _load,
                            child: const Text('Retry'),
                          ),
                        ] else ...[
                          const _Label('Name'),
                          _Field(
                            controller: _nameController,
                            hint: 'Femi Okafor',
                            textCapitalization: TextCapitalization.words,
                            onChanged: (_) => setState(() {}),
                          ),
                          const SizedBox(height: 20),

                          const _Label('Phone number'),
                          _PhoneField(
                            controller: _phoneController,
                            onChanged: (_) => setState(() {}),
                          ),
                          const SizedBox(height: 20),

                          const _Label('Relationship'),
                          _RelationshipDropdown(
                            value: _relationship,
                            items: _relationships,
                            onChanged: (v) =>
                                setState(() => _relationship = v),
                          ),
                        ],
                      ],
                    ),
                  ),

                  // ── Save guarantor ──
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                    child: SizedBox(
                      height: 52,
                      width: double.infinity,
                      child: FilledButton(
                        onPressed: _canSave ? _onSave : null,
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
                            : const Text('Save guarantor'),
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
// Shared widgets (same as Figma 15)
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

class _Field extends StatelessWidget {
  const _Field({
    required this.controller,
    required this.hint,
    required this.onChanged,
    this.textCapitalization = TextCapitalization.none,
  });

  final TextEditingController controller;
  final String hint;
  final ValueChanged<String> onChanged;
  final TextCapitalization textCapitalization;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return TextField(
      controller: controller,
      textCapitalization: textCapitalization,
      onChanged: onChanged,
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

class _PhoneField extends StatelessWidget {
  const _PhoneField({required this.controller, required this.onChanged});
  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      decoration: BoxDecoration(
        color: c.inputFill,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14),
            child: Row(
              children: [
                const Text('🇳🇬', style: TextStyle(fontSize: 18)),
                const SizedBox(width: 8),
                Text(
                  '(+234)',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                  ),
                ),
              ],
            ),
          ),
          Container(width: 1, height: 22, color: c.divider),
          Expanded(
            child: TextField(
              controller: controller,
              onChanged: onChanged,
              keyboardType: TextInputType.phone,
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w500,
                color: c.text,
              ),
              decoration: InputDecoration(
                hintText: 'Enter phone number',
                hintStyle: TextStyle(
                  color: c.textDisabled,
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                ),
                border: InputBorder.none,
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 18,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

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
            'Select relationship',
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
                (e) => DropdownMenuItem(
                  value: e,
                  child: Text(
                    e,
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