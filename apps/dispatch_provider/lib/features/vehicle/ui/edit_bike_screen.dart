import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/vehicle_controller.dart';

class EditBikeScreen extends StatefulWidget {
  const EditBikeScreen({
    super.key,
    required this.vehicleController,
    required this.bike,
  });

  final VehicleController vehicleController;
  final Map<String, dynamic> bike;

  @override
  State<EditBikeScreen> createState() => _EditBikeScreenState();
}

class _EditBikeScreenState extends State<EditBikeScreen> {
  late final _modelController =
      TextEditingController(text: widget.bike['model'] as String? ?? '');
  late final _yearController =
      TextEditingController(text: '${widget.bike['year'] ?? ''}');
  late final _colorController =
      TextEditingController(text: widget.bike['color'] as String? ?? '');
  String? _make;
  bool _isSaving = false;

  static const _makes = [
    'Honda', 'TVS', 'Bajaj', 'Yamaha', 'Suzuki',
    'Hero', 'KTM', 'Royal Enfield',
  ];

  @override
  void initState() {
    super.initState();
    final b = widget.bike['brand'] as String?;
    if (b != null && _makes.contains(b)) _make = b;
  }

  @override
  void dispose() {
    _modelController.dispose();
    _yearController.dispose();
    _colorController.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => _isSaving = true);
    final result = await widget.vehicleController.updateVehicle(
      vehicleId: widget.bike['id'] as String,
      brand: _make,
      model: _modelController.text.trim(),
      color: _colorController.text.trim(),
      year: int.tryParse(_yearController.text.trim()),
    );
    if (!mounted) return;
    setState(() => _isSaving = false);
    result.when(
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Bike updated')),
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
    final canSave = _make != null && _modelController.text.trim().isNotEmpty;

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
                    'Edit bike details',
                    style: TextStyle(
                      fontSize: 26,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    'Update your bike information.',
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
                    value: _make,
                    hint: 'Select make',
                    items: _makes,
                    onChanged: (v) => setState(() => _make = v),
                  ),
                  const SizedBox(height: 20),

                  const _Label('Model'),
                  _Field(controller: _modelController, hint: 'e.g. Apache 160'),
                  const SizedBox(height: 20),

                  const _Label('Year'),
                  _Field(
                    controller: _yearController,
                    hint: 'e.g. 2022',
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                  ),
                  const SizedBox(height: 20),

                  const _Label('Color'),
                  _Field(
                    controller: _colorController,
                    hint: 'e.g. Black',
                    onChanged: (_) => setState(() {}),
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
                  onPressed: canSave && !_isSaving ? _save : null,
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
    required this.value,
    required this.hint,
    required this.items,
    required this.onChanged,
  });

  final String? value;
  final String hint;
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