import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/onboarding_api.dart';

/// Registers another car. Pops `true` when added.
class AddVehicleScreen extends StatefulWidget {
  const AddVehicleScreen({super.key, required this.api});
  final OnboardingApi api;

  @override
  State<AddVehicleScreen> createState() => _AddVehicleScreenState();
}

class _AddVehicleScreenState extends State<AddVehicleScreen> {
  List<VehicleClassOption> _classes = const [];
  VehicleClassOption? _class;
  final _plate = TextEditingController();
  final _make = TextEditingController();
  final _model = TextEditingController();
  final _color = TextEditingController();
  bool _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _plate.addListener(() => setState(() {}));
    widget.api.vehicleClasses().then(
      (c) => mounted ? setState(() => _classes = c) : null,
      onError: (e) => mounted
          ? setState(() => _error = e is ApiException ? e.message : '$e')
          : null,
    );
  }

  @override
  void dispose() {
    for (final c in [_plate, _make, _model, _color]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      await widget.api.addVehicle(
        vehicleClass: _class!.value,
        plateNumber: _plate.text.trim().toUpperCase(),
        seats: _class!.maxPassengers,
        make: _make.text.trim(),
        model: _model.text.trim(),
        color: _color.text.trim(),
      );
      if (mounted) Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return KarryamPage(
      title: 'Add a car',
      subtitle: 'New cars are reviewed before use',
      bottom: KarryamButton(
        label: 'Add car',
        busy: _saving,
        onPressed: _class == null || _plate.text.trim().isEmpty ? null : _save,
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          DropdownButtonFormField<VehicleClassOption>(
            initialValue: _class,
            isExpanded: true,
            decoration: InputDecoration(
              labelText: 'Vehicle class',
              filled: true,
              fillColor: colors.card,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
            items: [
              for (final c in _classes)
                DropdownMenuItem(
                  value: c,
                  child: Text(
                    '${c.label} (up to ${c.maxPassengers} passengers)',
                  ),
                ),
            ],
            onChanged: (c) => setState(() => _class = c),
          ),
          const SizedBox(height: 14),
          KarryamField(
            controller: _plate,
            label: 'Plate number',
            textCapitalization: TextCapitalization.characters,
          ),
          KarryamField(
            controller: _make,
            label: 'Make (optional)',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _model,
            label: 'Model (optional)',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _color,
            label: 'Colour (optional)',
            textCapitalization: TextCapitalization.words,
          ),
          if (_error != null)
            Text(_error!, style: TextStyle(color: colors.danger)),
        ],
      ),
    );
  }
}
