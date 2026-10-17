import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

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
  late final _modelController = TextEditingController(
    text: widget.bike['model'] as String? ?? '',
  );
  late final _yearController = TextEditingController(
    text: '${widget.bike['year'] ?? ''}',
  );
  late final _colorController = TextEditingController(
    text: widget.bike['color'] as String? ?? '',
  );
  String? _make;
  bool _isSaving = false;

  static const _makes = [
    'Honda',
    'TVS',
    'Bajaj',
    'Yamaha',
    'Suzuki',
    'Hero',
    'KTM',
    'Royal Enfield',
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
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Vehicle updated',
          message: 'Your vehicle details have been saved.',
        );
        if (mounted) Navigator.of(context).pop(true);
      },
      failure: (error) async {
        await showHomeFeedback(
          context,
          title: 'Could not save vehicle',
          message: error.message,
          error: true,
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Edit vehicle',
    symbol: JourneySymbol.fleet,
    description: 'Keep your delivery vehicle details accurate.',
    bottom: FilledButton(
      onPressed: _isSaving ? null : _save,
      child: _isSaving
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Save changes'),
    ),
    children: [
      DropdownButtonFormField<String>(
        initialValue: _make,
        isExpanded: true,
        decoration: const InputDecoration(labelText: 'Make'),
        items: _makes
            .map((m) => DropdownMenuItem(value: m, child: Text(m)))
            .toList(),
        onChanged: _isSaving ? null : (v) => setState(() => _make = v),
      ),
      const SizedBox(height: 18),
      TextField(
        controller: _modelController,
        enabled: !_isSaving,
        onChanged: (_) => setState(() {}),
        decoration: const InputDecoration(labelText: 'Model'),
      ),
      const SizedBox(height: 18),
      TextField(
        controller: _yearController,
        enabled: !_isSaving,
        keyboardType: TextInputType.number,
        inputFormatters: [
          FilteringTextInputFormatter.digitsOnly,
          LengthLimitingTextInputFormatter(4),
        ],
        onChanged: (_) => setState(() {}),
        decoration: const InputDecoration(labelText: 'Year'),
      ),
      const SizedBox(height: 18),
      TextField(
        controller: _colorController,
        enabled: !_isSaving,
        onChanged: (_) => setState(() {}),
        decoration: const InputDecoration(labelText: 'Colour'),
      ),
    ],
  );
}
