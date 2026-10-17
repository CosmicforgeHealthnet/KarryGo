import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../state/vehicle_controller.dart';

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
    'Honda',
    'TVS',
    'Bajaj',
    'Yamaha',
    'Suzuki',
    'Hero',
    'KTM',
    'Royal Enfield',
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
      success: (_) async {
        await showHomeFeedback(
          context,
          title: 'Vehicle added',
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
    title: 'Add vehicle',
    symbol: JourneySymbol.fleet,
    description: 'Keep your delivery vehicle details accurate.',
    bottom: FilledButton(
      onPressed: _isSaving || !_valid ? null : _onContinue,
      child: _isSaving
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Add vehicle'),
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
      const SizedBox(height: 18),
      TextField(
        controller: _plateController,
        enabled: !_isSaving,
        textCapitalization: TextCapitalization.characters,
        onChanged: (_) => setState(() {}),
        decoration: const InputDecoration(labelText: 'Plate number'),
      ),
    ],
  );
}
