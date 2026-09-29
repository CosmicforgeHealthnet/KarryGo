import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/vehicle_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Add a new bike
//
// Backend RegisterBikeInput requires: bike_type, brand, model, year, color,
// plate_number. bike_type defaults to "motorcycle" from this screen.
// ═══════════════════════════════════════════════════════════════════════════

class AddBikeScreen extends StatefulWidget {
  const AddBikeScreen({super.key, required this.vehicleController});

  final VehicleController vehicleController;

  @override
  State<AddBikeScreen> createState() => _AddBikeScreenState();
}

class _AddBikeScreenState extends State<AddBikeScreen> {
  final _modelController = TextEditingController();
  final _yearController = TextEditingController();
  final _plateController = TextEditingController();
  final _colorController = TextEditingController();
  String? _make;
  bool _isSaving = false;

  static const _makes = [
    'Honda', 'TVS', 'Bajaj', 'Yamaha', 'Suzuki',
    'Hero', 'KTM', 'Royal Enfield',
  ];

  bool get _valid =>
      _make != null &&
      _modelController.text.trim().isNotEmpty &&
      (int.tryParse(_yearController.text.trim()) ?? 0) > 1900 &&
      _plateController.text.trim().isNotEmpty &&
      _colorController.text.trim().isNotEmpty;

  @override
  void dispose() {
    _modelController.dispose();
    _yearController.dispose();
    _plateController.dispose();
    _colorController.dispose();
    super.dispose();
  }

  Future<void> _onContinue() async {
    setState(() => _isSaving = true);
    final result = await widget.vehicleController.createVehicle(
      bikeType: 'motorcycle',
      brand: _make!,
      model: _modelController.text.trim(),
      year: int.parse(_yearController.text.trim()),
      color: _colorController.text.trim(),
      plateNumber: _plateController.text.trim(),
    );
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Bike added')),
        );
        Navigator.of(context).pop(true);
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
        child: Column(
          children: [
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
                    'Add a new bike',
                    style: TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    "Let's get your vehicle on the road.",
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 28),

                  const _Label('Make'),
                  _Dropdown(
                    hint: 'Select make',
                    value: _make,
                    items: _makes,
                    onChanged: (v) => setState(() => _make = v),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Model'),
                  _Field(
                    controller: _modelController,
                    hint: 'e.g. Apache 160',
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Year'),
                  _Field(
                    controller: _yearController,
                    hint: 'e.g. 2022',
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Color'),
                  _Field(
                    controller: _colorController,
                    hint: 'e.g. Black',
                    textCapitalization: TextCapitalization.words,
                    onChanged: () => setState(() {}),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Plate number'),
                  _Field(
                    controller: _plateController,
                    hint: 'e.g. LND 784 KJ',
                    textCapitalization: TextCapitalization.characters,
                    onChanged: () => setState(() {}),
                  ),
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _valid && !_isSaving ? _onContinue : null,
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
                      : const Text('Continue'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

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
    this.keyboardType,
    this.inputFormatters,
    this.textCapitalization = TextCapitalization.words,
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

class _Dropdown extends StatelessWidget {
  const _Dropdown({
    required this.hint,
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final String hint;
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