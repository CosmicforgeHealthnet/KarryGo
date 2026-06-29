import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../vehicle/state/vehicle_controller.dart';

class VehicleInformationScreen extends StatefulWidget {
  const VehicleInformationScreen({super.key, required this.vehicleController});

  final VehicleController vehicleController;

  @override
  State<VehicleInformationScreen> createState() =>
      _VehicleInformationScreenState();
}

class _VehicleInformationScreenState extends State<VehicleInformationScreen> {
  bool _isLoadingVehicle = true;
  bool _isSaving = false;
  bool _isEditing = false;
  String? _loadError;

  // Loaded vehicle data
  String? _vehicleId;
  String _bikeType = '';
  String _bikeBrand = '';
  String _bikeModel = '';
  String _color = '';
  String _plateNumber = '';
  String _verificationStatus = '';

  // Edit controllers
  late TextEditingController _brandController;
  late TextEditingController _modelController;
  late TextEditingController _colorController;
  late TextEditingController _plateController;
  String? _editBikeType;

  // Add-vehicle form state
  String? _addBikeType;
  late TextEditingController _addBrandController;
  late TextEditingController _addModelController;
  late TextEditingController _addYearController;
  late TextEditingController _addColorController;
  late TextEditingController _addPlateController;

  static const _bikeTypeOptions = [
    _BikeTypeOption(label: 'Motorcycle', value: 'motorcycle'),
    _BikeTypeOption(label: 'Scooter', value: 'scooter'),
    _BikeTypeOption(label: 'Tricycle', value: 'tricycle'),
    _BikeTypeOption(label: 'Bicycle', value: 'bicycle'),
    _BikeTypeOption(label: 'Electric Bike', value: 'electric_bike'),
    _BikeTypeOption(label: 'Dispatch Bike', value: 'dispatch_bike'),
  ];

  // Only suspended bikes are fully locked.
  bool get _isEditable => _verificationStatus != 'suspended';

  String get _bikeTypeLabel {
    for (final opt in _bikeTypeOptions) {
      if (opt.value == _bikeType) return opt.label;
    }
    return _bikeType;
  }

  bool get _canAdd =>
      _addBikeType != null &&
      _addBrandController.text.trim().isNotEmpty &&
      _addModelController.text.trim().isNotEmpty &&
      (int.tryParse(_addYearController.text.trim()) ?? 0) > 1900 &&
      _addColorController.text.trim().isNotEmpty &&
      _addPlateController.text.trim().isNotEmpty;

  @override
  void initState() {
    super.initState();
    _brandController = TextEditingController();
    _modelController = TextEditingController();
    _colorController = TextEditingController();
    _plateController = TextEditingController();
    _addBrandController = TextEditingController();
    _addModelController = TextEditingController();
    _addYearController = TextEditingController();
    _addColorController = TextEditingController();
    _addPlateController = TextEditingController();
    _loadVehicles();
  }

  @override
  void dispose() {
    _brandController.dispose();
    _modelController.dispose();
    _colorController.dispose();
    _plateController.dispose();
    _addBrandController.dispose();
    _addModelController.dispose();
    _addYearController.dispose();
    _addColorController.dispose();
    _addPlateController.dispose();
    super.dispose();
  }

  Future<void> _loadVehicles() async {
    setState(() {
      _isLoadingVehicle = true;
      _loadError = null;
    });
    final result = await widget.vehicleController.listVehicles();
    if (!mounted) return;
    result.when(
      success: (list) {
        if (list.isEmpty) {
          setState(() => _isLoadingVehicle = false);
          return;
        }
        final v = list.first;
        setState(() {
          _vehicleId = v['id'] as String? ?? v['vehicle_id'] as String? ?? '';
          _bikeType = v['bike_type'] as String? ?? '';
          _bikeBrand = v['brand'] as String? ?? '';
          _bikeModel = v['model'] as String? ?? '';
          _color = v['color'] as String? ?? '';
          _plateNumber = v['plate_number'] as String? ?? '';
          _verificationStatus = v['verification_status'] as String? ?? '';
          _isLoadingVehicle = false;
        });
      },
      failure: (error) => setState(() {
        _loadError = error.message;
        _isLoadingVehicle = false;
      }),
    );
  }

  Future<void> _onAddSave() async {
    final year = int.tryParse(_addYearController.text.trim());
    if (year == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Please enter a valid year.')),
      );
      return;
    }
    setState(() => _isSaving = true);
    final result = await widget.vehicleController.createVehicle(
      bikeType: _addBikeType!,
      brand: _addBrandController.text.trim(),
      model: _addModelController.text.trim(),
      year: year,
      color: _addColorController.text.trim(),
      plateNumber: _addPlateController.text.trim(),
    );
    if (!mounted) return;
    result.when(
      success: (id) {
        setState(() => _isSaving = false);
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Vehicle registered')),
        );
        Navigator.of(context).pop(true);
      },
      failure: (error) {
        setState(() => _isSaving = false);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      },
    );
  }

  Future<void> _onEditTap() async {
    final confirmed = await showDialog<bool>(
      context: context,
      barrierColor: Colors.black45,
      builder: (ctx) => const _EditConfirmDialog(),
    );
    if (confirmed == true && mounted) {
      setState(() {
        _isEditing = true;
        _editBikeType = _bikeType;
        _brandController.text = _bikeBrand;
        _modelController.text = _bikeModel;
        _colorController.text = _color;
        _plateController.text = _plateNumber;
      });
    }
  }

  Future<void> _onSave() async {
    if (_vehicleId == null || _vehicleId!.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No vehicle ID — cannot save.')),
      );
      return;
    }

    final newPlate = _plateController.text.trim().toUpperCase();
    final newType = _editBikeType ?? _bikeType;
    final plateChanged =
        newPlate.isNotEmpty && newPlate != _plateNumber.toUpperCase();
    final typeChanged = newType != _bikeType;

    // Warn when identity-affecting fields change.
    if (plateChanged || typeChanged) {
      final confirmed = await showDialog<bool>(
        context: context,
        barrierColor: Colors.black45,
        builder: (ctx) => AlertDialog(
          shape:
              RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
          title: const Text(
            'Re-verification Required',
            style: TextStyle(fontSize: 17, fontWeight: FontWeight.w800),
          ),
          content: const Text(
            'Changing your plate number or bike type will reset your vehicle verification. You will need to re-submit for admin review.',
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF888888),
              height: 1.5,
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(false),
              child: const Text(
                'Cancel',
                style: TextStyle(color: Color(0xFF888888)),
              ),
            ),
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(true),
              child: const Text(
                'Continue',
                style: TextStyle(
                  color: Color(0xFF4CAF50),
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ],
        ),
      );
      if (confirmed != true || !mounted) return;
    }

    setState(() => _isSaving = true);
    final result = await widget.vehicleController.updateVehicle(
      vehicleId: _vehicleId!,
      brand: _brandController.text.trim().isNotEmpty
          ? _brandController.text.trim()
          : null,
      model: _modelController.text.trim().isNotEmpty
          ? _modelController.text.trim()
          : null,
      color: _colorController.text.trim().isNotEmpty
          ? _colorController.text.trim()
          : null,
      bikeType: typeChanged ? newType : null,
      plateNumber: plateChanged ? newPlate : null,
    );
    if (!mounted) return;
    result.when(
      success: (data) {
        setState(() => _isSaving = false);
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Vehicle updated')),
        );
        Navigator.of(context).pop(true);
      },
      failure: (error) {
        setState(() => _isSaving = false);
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(error.message)));
      },
    );
  }

  Widget _buildBikeTypeDropdown({
    required String? value,
    required ValueChanged<String?> onChanged,
  }) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: const Color(0xFFF5F6F8),
        borderRadius: BorderRadius.circular(12),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value,
          isExpanded: true,
          hint: const Text(
            'Select bike type',
            style: TextStyle(color: Color(0xFFAAAAAA), fontSize: 14),
          ),
          icon: const Icon(
            Icons.keyboard_arrow_down,
            color: Color(0xFF888888),
          ),
          items: _bikeTypeOptions
              .map(
                (o) => DropdownMenuItem(
                  value: o.value,
                  child: Text(
                    o.label,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: Color(0xFF1A1A1A),
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

  Widget _buildAddForm() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const _FieldLabel('Bike Type'),
        const SizedBox(height: 8),
        _buildBikeTypeDropdown(
          value: _addBikeType,
          onChanged: (v) => setState(() => _addBikeType = v),
        ),
        const SizedBox(height: 18),
        const _FieldLabel('Bike Brand'),
        const SizedBox(height: 8),
        _TextInput(
          controller: _addBrandController,
          hint: 'e.g. Honda',
          onChanged: (_) => setState(() {}),
        ),
        const SizedBox(height: 18),
        const _FieldLabel('Model'),
        const SizedBox(height: 8),
        _TextInput(
          controller: _addModelController,
          hint: 'e.g. CB150',
          onChanged: (_) => setState(() {}),
        ),
        const SizedBox(height: 18),
        const _FieldLabel('Year'),
        const SizedBox(height: 8),
        _TextInput(
          controller: _addYearController,
          hint: 'e.g. 2022',
          keyboardType: TextInputType.number,
          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          onChanged: (_) => setState(() {}),
        ),
        const SizedBox(height: 18),
        const _FieldLabel('Color'),
        const SizedBox(height: 8),
        _TextInput(
          controller: _addColorController,
          hint: 'e.g. Black',
          onChanged: (_) => setState(() {}),
        ),
        const SizedBox(height: 18),
        const _FieldLabel('Plate Number'),
        const SizedBox(height: 8),
        _TextInput(
          controller: _addPlateController,
          hint: 'e.g. LND 123 AB',
          onChanged: (_) => setState(() {}),
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final bool hasVehicle = _vehicleId != null && _vehicleId!.isNotEmpty;

    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: _isLoadingVehicle
            ? const Center(
                child: CircularProgressIndicator(color: Color(0xFF4CAF50)),
              )
            : Column(
                children: [
                  Expanded(
                    child: ListView(
                      padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                      children: [
                        // ── Header ──────────────────────────────────────
                        GestureDetector(
                          behavior: HitTestBehavior.opaque,
                          onTap: () => Navigator.of(context).pop(),
                          child: const Align(
                            alignment: Alignment.centerLeft,
                            child: Icon(
                              Icons.arrow_back_ios_new,
                              size: 20,
                              color: Color(0xFF1A1A1A),
                            ),
                          ),
                        ),
                        const SizedBox(height: 16),
                        const Text(
                          'Bike Information',
                          style: TextStyle(
                            fontSize: 20,
                            fontWeight: FontWeight.w800,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          hasVehicle
                              ? 'Your registered bike details.'
                              : 'Register your bike to get started.',
                          style: const TextStyle(
                            fontSize: 13,
                            color: Color(0xFF888888),
                          ),
                        ),

                        const SizedBox(height: 28),

                        if (_loadError != null) ...[
                          Text(
                            _loadError!,
                            style: const TextStyle(
                              color: Color(0xFFE53935),
                              fontSize: 13,
                            ),
                          ),
                          const SizedBox(height: 12),
                          OutlinedButton(
                            onPressed: _loadVehicles,
                            child: const Text('Retry'),
                          ),
                        ] else if (!hasVehicle) ...[
                          // ── Add vehicle form ─────────────────────────
                          _buildAddForm(),
                        ] else if (_isEditing) ...[
                          // ── Edit mode ────────────────────────────────
                          const _FieldLabel('Bike Type'),
                          const SizedBox(height: 8),
                          _buildBikeTypeDropdown(
                            value: _editBikeType,
                            onChanged: (v) =>
                                setState(() => _editBikeType = v),
                          ),
                          const SizedBox(height: 18),
                          const _FieldLabel('Plate Number'),
                          const SizedBox(height: 8),
                          _TextInput(
                            controller: _plateController,
                            hint: 'e.g. LND 123 AB',
                          ),
                          const SizedBox(height: 18),
                          const _FieldLabel('Bike Brand'),
                          const SizedBox(height: 8),
                          _TextInput(
                            controller: _brandController,
                            hint: 'e.g. Honda',
                          ),
                          const SizedBox(height: 18),
                          const _FieldLabel('Model'),
                          const SizedBox(height: 8),
                          _TextInput(
                            controller: _modelController,
                            hint: 'e.g. CB150',
                          ),
                          const SizedBox(height: 18),
                          const _FieldLabel('Color'),
                          const SizedBox(height: 8),
                          _TextInput(
                            controller: _colorController,
                            hint: 'Enter color',
                          ),
                        ] else ...[
                          // ── View mode ────────────────────────────────
                          if (_verificationStatus.isNotEmpty)
                            _StatusBadge(status: _verificationStatus),
                          if (_verificationStatus.isNotEmpty)
                            const SizedBox(height: 16),
                          _ViewField(label: 'Bike Type', value: _bikeTypeLabel),
                          const SizedBox(height: 20),
                          _ViewField(label: 'Bike Brand', value: _bikeBrand),
                          const SizedBox(height: 20),
                          if (_bikeModel.isNotEmpty) ...[
                            _ViewField(label: 'Model', value: _bikeModel),
                            const SizedBox(height: 20),
                          ],
                          _ViewField(label: 'Color', value: _color),
                          const SizedBox(height: 20),
                          _ViewField(
                            label: 'Plate Number',
                            value: _plateNumber,
                          ),
                          if (!_isEditable) ...[
                            const SizedBox(height: 16),
                            Container(
                              padding: const EdgeInsets.all(12),
                              decoration: BoxDecoration(
                                color: const Color(0xFFF5F6F8),
                                borderRadius: BorderRadius.circular(10),
                              ),
                              child: const Row(
                                children: [
                                  Icon(
                                    Icons.lock_outline,
                                    size: 15,
                                    color: Color(0xFF888888),
                                  ),
                                  SizedBox(width: 8),
                                  Expanded(
                                    child: Text(
                                      'Your vehicle is suspended. Contact support for assistance.',
                                      style: TextStyle(
                                        fontSize: 12,
                                        color: Color(0xFF888888),
                                        height: 1.4,
                                      ),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ],
                      ],
                    ),
                  ),

                  // ── Bottom button ────────────────────────────────────
                  if (_loadError == null &&
                      (!hasVehicle || _isEditing || _isEditable))
                    Padding(
                      padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
                      child: SizedBox(
                        height: 52,
                        width: double.infinity,
                        child: FilledButton(
                          onPressed: _isSaving
                              ? null
                              : !hasVehicle
                              ? (_canAdd ? _onAddSave : null)
                              : _isEditing
                              ? _onSave
                              : _onEditTap,
                          style: FilledButton.styleFrom(
                            backgroundColor: const Color(0xFF4CAF50),
                            disabledBackgroundColor: const Color(
                              0xFF4CAF50,
                            ).withValues(alpha: 0.35),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(999),
                            ),
                          ),
                          child: _isSaving
                              ? const SizedBox(
                                  width: 20,
                                  height: 20,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : Text(
                                  !hasVehicle
                                      ? 'Register Vehicle'
                                      : _isEditing
                                      ? 'Save'
                                      : 'Edit',
                                  style: const TextStyle(
                                    fontSize: 16,
                                    fontWeight: FontWeight.w700,
                                    color: Colors.white,
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

// ── Verification status badge ─────────────────────────────────────────────────

class _StatusBadge extends StatelessWidget {
  const _StatusBadge({required this.status});
  final String status;

  @override
  Widget build(BuildContext context) {
    final (label, bg, fg) = switch (status) {
      'verified' => (
          'Verified',
          const Color(0xFFE8F5E9),
          const Color(0xFF2E7D32)
        ),
      'suspended' => (
          'Suspended',
          const Color(0xFFFFEBEE),
          const Color(0xFFC62828)
        ),
      'submitted' || 'pending_review' => (
          'Under Review',
          const Color(0xFFFFF8E1),
          const Color(0xFFF57F17)
        ),
      'rejected' => (
          'Rejected',
          const Color(0xFFFFEBEE),
          const Color(0xFFC62828)
        ),
      _ => ('Unverified', const Color(0xFFF5F6F8), const Color(0xFF888888)),
    };
    return Row(
      children: [
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
          decoration: BoxDecoration(
            color: bg,
            borderRadius: BorderRadius.circular(20),
          ),
          child: Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: fg,
            ),
          ),
        ),
      ],
    );
  }
}

// ── View mode field ───────────────────────────────────────────────────────────

class _ViewField extends StatelessWidget {
  const _ViewField({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(height: 4),
        Text(
          value.isEmpty ? '—' : value,
          style: const TextStyle(fontSize: 14, color: Color(0xFF444444)),
        ),
      ],
    );
  }
}

// ── Edit confirm dialog ───────────────────────────────────────────────────────

class _EditConfirmDialog extends StatelessWidget {
  const _EditConfirmDialog();

  @override
  Widget build(BuildContext context) {
    return Dialog(
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      insetPadding: const EdgeInsets.symmetric(horizontal: 24),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Image.asset(
              'assets/figma/profile_submitted.png',
              height: 140,
              fit: BoxFit.contain,
            ),
            const SizedBox(height: 16),
            const Text(
              'Edit Vehicle Details?',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 10),
            const Text(
              'Editing vehicle details may require re-verification. Are you sure you want to continue?',
              style: TextStyle(
                fontSize: 13,
                color: Color(0xFF888888),
                height: 1.5,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              height: 50,
              child: FilledButton(
                onPressed: () => Navigator.of(context).pop(true),
                style: FilledButton.styleFrom(
                  backgroundColor: const Color(0xFF4CAF50),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                child: const Text(
                  'Yes',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: Colors.white,
                  ),
                ),
              ),
            ),
            const SizedBox(height: 10),
            SizedBox(
              width: double.infinity,
              height: 50,
              child: FilledButton(
                onPressed: () => Navigator.of(context).pop(false),
                style: FilledButton.styleFrom(
                  backgroundColor:
                      const Color(0xFF4CAF50).withValues(alpha: 0.15),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(999),
                  ),
                ),
                child: const Text(
                  'No',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF4CAF50),
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

// ── Shared widgets ────────────────────────────────────────────────────────────

class _BikeTypeOption {
  const _BikeTypeOption({required this.label, required this.value});
  final String label;
  final String value;
}

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w700,
        color: Color(0xFF1A1A1A),
      ),
    );
  }
}

class _TextInput extends StatelessWidget {
  const _TextInput({
    required this.controller,
    this.hint,
    this.keyboardType,
    this.inputFormatters,
    this.onChanged,
  });

  final TextEditingController controller;
  final String? hint;
  final TextInputType? keyboardType;
  final List<TextInputFormatter>? inputFormatters;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      textCapitalization: TextCapitalization.words,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      onChanged: onChanged,
      style: const TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w500,
        color: Color(0xFF1A1A1A),
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(color: Color(0xFFAAAAAA), fontSize: 14),
        filled: true,
        fillColor: const Color(0xFFF5F6F8),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 16,
          vertical: 14,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide.none,
        ),
      ),
    );
  }
}
