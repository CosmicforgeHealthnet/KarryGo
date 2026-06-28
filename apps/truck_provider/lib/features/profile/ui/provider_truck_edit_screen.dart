import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'widgets/provider_profile_widgets.dart';

/// Truck Information edit form (Figma 2164). Creates or updates the provider's
/// truck record.
class ProviderTruckEditScreen extends StatefulWidget {
  const ProviderTruckEditScreen({
    super.key,
    required this.profileController,
    this.truck,
  });

  final ProviderProfileController profileController;
  final ProviderTruck? truck;

  @override
  State<ProviderTruckEditScreen> createState() => _ProviderTruckEditScreenState();
}

class _ProviderTruckEditScreenState extends State<ProviderTruckEditScreen> {
  late final TextEditingController _capacityCtrl;
  late final TextEditingController _brandCtrl;
  late final TextEditingController _modelCtrl;
  late final TextEditingController _colorCtrl;
  late final TextEditingController _plateCtrl;
  late final TextEditingController _axlesCtrl;
  late final TextEditingController _experienceCtrl;

  String? _truckType;
  String? _licenseType;
  final Set<String> _goods = {};
  bool _hasInsurance = false;
  bool _saving = false;

  static const _licenseTypes = [
    'Class A (Motorcycle)',
    'Class B (Light Vehicle)',
    'Class C (Light Commercial)',
    'Class D (Heavy Commercial)',
    'Class E (Articulated)',
    'Class F (Special)',
  ];
  static const _goodsOptions = ['General goods', 'Construction Materials', 'Furniture', 'Equipment'];

  @override
  void initState() {
    super.initState();
    final t = widget.truck;
    _capacityCtrl = TextEditingController(text: t != null && t.capacityKg > 0 ? '${t.capacityKg}' : '');
    _brandCtrl = TextEditingController(text: t?.make ?? '');
    _modelCtrl = TextEditingController(text: t?.model ?? '');
    _colorCtrl = TextEditingController(text: t?.color ?? '');
    _plateCtrl = TextEditingController(text: t?.plateNumber ?? '');
    _axlesCtrl = TextEditingController(text: t?.numberOfAxles ?? '');
    _experienceCtrl = TextEditingController(text: t?.yearsOfExperience ?? '');
    _truckType = t != null && t.truckType.isNotEmpty ? t.truckType : null;
    _licenseType = (t?.licenseType.isNotEmpty ?? false) && _licenseTypes.contains(t!.licenseType) ? t.licenseType : null;
    _goods.addAll(t?.goodsTypes ?? const []);
    _hasInsurance = t?.hasInsurance ?? false;
    for (final c in [_capacityCtrl, _plateCtrl]) {
      c.addListener(() => setState(() {}));
    }
  }

  @override
  void dispose() {
    _capacityCtrl.dispose();
    _brandCtrl.dispose();
    _modelCtrl.dispose();
    _colorCtrl.dispose();
    _plateCtrl.dispose();
    _axlesCtrl.dispose();
    _experienceCtrl.dispose();
    super.dispose();
  }

  bool get _canSave =>
      !_saving &&
      _truckType != null &&
      (int.tryParse(_capacityCtrl.text.trim()) ?? 0) > 0 &&
      _plateCtrl.text.trim().isNotEmpty;

  Future<void> _save() async {
    setState(() => _saving = true);
    final ok = await widget.profileController.saveTruck(
      truckId: widget.truck?.id,
      truckType: _truckType!,
      capacityKg: int.tryParse(_capacityCtrl.text.trim()) ?? 0,
      plateNumber: _plateCtrl.text.trim(),
      licenseType: _licenseType ?? '',
      make: _brandCtrl.text.trim(),
      model: _modelCtrl.text.trim(),
      color: _colorCtrl.text.trim(),
      numberOfAxles: _axlesCtrl.text.trim(),
      yearsOfExperience: _experienceCtrl.text.trim(),
      goodsTypes: _goods.toList(),
      hasInsurance: _hasInsurance,
    );
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) {
      Navigator.of(context).pop();
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(widget.profileController.error ?? 'Could not save truck.')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
              child: ProviderProfileHeader(
                title: 'Truck Information',
                subtitle: 'Provide your truck information below.',
              ),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  const ProviderFieldLabel('Truck Type'),
                  DropdownButtonFormField<String>(
                    initialValue: _truckType,
                    isExpanded: true,
                    hint: const Text('Select Truck Type', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                    icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
                    decoration: providerWhiteField(''),
                    items: providerTruckTypeOptions
                        .map((o) => DropdownMenuItem(value: o.slug, child: Text(o.label)))
                        .toList(),
                    onChanged: (v) => setState(() => _truckType = v),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('License Type'),
                  DropdownButtonFormField<String>(
                    initialValue: _licenseType,
                    isExpanded: true,
                    hint: const Text('Select License Type', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                    icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
                    decoration: providerWhiteField(''),
                    items: _licenseTypes.map((l) => DropdownMenuItem(value: l, child: Text(l))).toList(),
                    onChanged: (v) => setState(() => _licenseType = v),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Capacity'),
                  TextField(
                    controller: _capacityCtrl,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: providerWhiteField('e.g. 10000 (kg)'),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Brand'),
                  TextField(controller: _brandCtrl, textCapitalization: TextCapitalization.words, decoration: providerWhiteField('e.g. Mercedes-Benz')),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Model'),
                  TextField(controller: _modelCtrl, textCapitalization: TextCapitalization.words, decoration: providerWhiteField('e.g. Actros')),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Color'),
                  TextField(controller: _colorCtrl, textCapitalization: TextCapitalization.words, decoration: providerWhiteField('Enter color')),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Plate Number'),
                  TextField(
                    controller: _plateCtrl,
                    textCapitalization: TextCapitalization.characters,
                    decoration: providerWhiteField('AS347654'),
                  ),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Number of Axles'),
                  TextField(controller: _axlesCtrl, keyboardType: TextInputType.number, decoration: providerWhiteField('e.g. 3')),
                  const SizedBox(height: 16),

                  const ProviderFieldLabel('Years of Experience'),
                  TextField(controller: _experienceCtrl, keyboardType: TextInputType.number, decoration: providerWhiteField('e.g. 5')),
                  const SizedBox(height: 20),

                  const Text('Select types of goods you can carry',
                      style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800)),
                  const SizedBox(height: 12),
                  for (final g in _goodsOptions)
                    _GoodsRow(
                      label: g,
                      selected: _goods.contains(g),
                      onTap: () => setState(() => _goods.contains(g) ? _goods.remove(g) : _goods.add(g)),
                    ),
                  const SizedBox(height: 12),

                  const Text('Do you have active vehicle insurance?',
                      style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800)),
                  const SizedBox(height: 12),
                  Row(
                    children: [
                      _YesNo(label: 'Yes', selected: _hasInsurance, onTap: () => setState(() => _hasInsurance = true)),
                      const SizedBox(width: 24),
                      _YesNo(label: 'No', selected: !_hasInsurance, onTap: () => setState(() => _hasInsurance = false)),
                    ],
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: ProviderPrimaryButton(
                label: 'Save',
                isLoading: _saving,
                onPressed: _canSave ? _save : null,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _GoodsRow extends StatelessWidget {
  const _GoodsRow({required this.label, required this.selected, required this.onTap});
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          children: [
            Container(
              width: 22,
              height: 22,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(color: selected ? kProviderGreen : kProviderBorder, width: 2),
                color: selected ? kProviderGreen : Colors.transparent,
              ),
              child: selected ? const Icon(Icons.check_rounded, size: 14, color: Colors.white) : null,
            ),
            const SizedBox(width: 12),
            Text(label, style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600)),
          ],
        ),
      ),
    );
  }
}

class _YesNo extends StatelessWidget {
  const _YesNo({required this.label, required this.selected, required this.onTap});
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Row(
        children: [
          Container(
            width: 22,
            height: 22,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(color: selected ? kProviderGreen : kProviderBorder, width: 2),
            ),
            child: selected
                ? Center(
                    child: Container(
                      width: 12,
                      height: 12,
                      decoration: const BoxDecoration(shape: BoxShape.circle, color: kProviderGreen),
                    ),
                  )
                : null,
          ),
          const SizedBox(width: 8),
          Text(label, style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600)),
        ],
      ),
    );
  }
}
