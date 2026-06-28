import 'package:flutter/foundation.dart' show kDebugMode;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import 'onboarding_shared_widgets.dart';

/// Onboarding step 5: capture the provider's truck so they finish sign-up with
/// at least one active truck (required by the backend before they can go
/// online). Mirrors the fields/validation of the Profile-section truck form.
class TruckInfoScreen extends StatefulWidget {
  const TruckInfoScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<TruckInfoScreen> createState() => _TruckInfoScreenState();
}

class _TruckInfoScreenState extends State<TruckInfoScreen> {
  final _capacityCtrl = TextEditingController();
  final _brandCtrl = TextEditingController();
  final _modelCtrl = TextEditingController();
  final _colorCtrl = TextEditingController();
  final _plateCtrl = TextEditingController();
  final _axlesCtrl = TextEditingController();
  final _experienceCtrl = TextEditingController();

  String? _truckType;
  String? _licenseType;
  final Set<String> _goods = {};
  bool _hasInsurance = false;

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
    // Dev convenience: prefill so testing onboarding doesn't require retyping.
    // Never runs in release builds.
    if (kDebugMode) {
      _truckType = 'flatbed';
      _licenseType = 'Class D (Heavy Commercial)';
      _capacityCtrl.text = '10000';
      _brandCtrl.text = 'Mercedes-Benz';
      _modelCtrl.text = 'Actros';
      _colorCtrl.text = 'White';
      _plateCtrl.text = 'LAG123XY';
      _axlesCtrl.text = '3';
      _experienceCtrl.text = '5';
      _goods.add('General goods');
      _hasInsurance = true;
    }
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

  bool get _canContinue =>
      _truckType != null &&
      (int.tryParse(_capacityCtrl.text.trim()) ?? 0) > 0 &&
      _plateCtrl.text.trim().isNotEmpty;

  void _proceed() {
    widget.controller.saveTruckInfo(
      truckType: _truckType!,
      capacityKg: _capacityCtrl.text.trim(),
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
  }

  @override
  Widget build(BuildContext context) {
    return OnboardingScaffold(
      title: 'Truck Information',
      subtitle: 'Tell us about the truck you will haul with. You can add more trucks later.',
      step: 5,
      content: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const OnboardingSectionLabel('Truck Type'),
          DropdownButtonFormField<String>(
            initialValue: _truckType,
            isExpanded: true,
            hint: const Text('Select Truck Type', style: TextStyle(color: kProviderMuted, fontSize: 14)),
            icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
            decoration: onboardingFieldDecoration(''),
            items: providerTruckTypeOptions
                .map((o) => DropdownMenuItem(value: o.slug, child: Text(o.label)))
                .toList(),
            onChanged: (v) => setState(() => _truckType = v),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('License Type'),
          DropdownButtonFormField<String>(
            initialValue: _licenseType,
            isExpanded: true,
            hint: const Text('Select License Type', style: TextStyle(color: kProviderMuted, fontSize: 14)),
            icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
            decoration: onboardingFieldDecoration(''),
            items: _licenseTypes.map((l) => DropdownMenuItem(value: l, child: Text(l))).toList(),
            onChanged: (v) => setState(() => _licenseType = v),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Capacity'),
          TextField(
            controller: _capacityCtrl,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: onboardingFieldDecoration('e.g. 10000 (kg)'),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Brand'),
          TextField(
            controller: _brandCtrl,
            textCapitalization: TextCapitalization.words,
            decoration: onboardingFieldDecoration('e.g. Mercedes-Benz'),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Model'),
          TextField(
            controller: _modelCtrl,
            textCapitalization: TextCapitalization.words,
            decoration: onboardingFieldDecoration('e.g. Actros'),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Color'),
          TextField(
            controller: _colorCtrl,
            textCapitalization: TextCapitalization.words,
            decoration: onboardingFieldDecoration('Enter color'),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Plate Number'),
          TextField(
            controller: _plateCtrl,
            textCapitalization: TextCapitalization.characters,
            decoration: onboardingFieldDecoration('AS347654'),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Number of Axles'),
          TextField(
            controller: _axlesCtrl,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: onboardingFieldDecoration('e.g. 3'),
          ),
          const SizedBox(height: 16),

          const OnboardingSectionLabel('Years of Experience'),
          TextField(
            controller: _experienceCtrl,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: onboardingFieldDecoration('e.g. 5'),
          ),
          const SizedBox(height: 20),

          const Text(
            'Select types of goods you can carry',
            style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 12),
          for (final g in _goodsOptions)
            _GoodsRow(
              label: g,
              selected: _goods.contains(g),
              onTap: () => setState(() => _goods.contains(g) ? _goods.remove(g) : _goods.add(g)),
            ),
          const SizedBox(height: 12),

          const Text(
            'Do you have active vehicle insurance?',
            style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              _YesNo(label: 'Yes', selected: _hasInsurance, onTap: () => setState(() => _hasInsurance = true)),
              const SizedBox(width: 24),
              _YesNo(label: 'No', selected: !_hasInsurance, onTap: () => setState(() => _hasInsurance = false)),
            ],
          ),
          const SizedBox(height: 8),
        ],
      ),
      onContinue: _canContinue ? _proceed : null,
      continueLabel: 'Continue',
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
