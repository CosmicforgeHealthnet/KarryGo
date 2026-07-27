import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../hauling/ui/widgets/hauling_flow_helpers.dart';
import '../../models/dispatch_models.dart';
import '../../state/dispatch_booking_controller.dart';

class DispatchPackageInfoView extends StatefulWidget {
  const DispatchPackageInfoView({
    super.key,
    required this.controller,
    required this.state,
  });

  final DispatchBookingController controller;
  final DispatchBookingState state;

  @override
  State<DispatchPackageInfoView> createState() =>
      _DispatchPackageInfoViewState();
}

class _DispatchPackageInfoViewState extends State<DispatchPackageInfoView> {
  final _descCtrl = TextEditingController();
  final _typeCtrl = TextEditingController();
  final _receiverNameCtrl = TextEditingController();
  final _receiverPhoneCtrl = TextEditingController();

  DispatchBookingController get _ctrl => widget.controller;
  DispatchBookingState get _state => widget.state;

  @override
  void initState() {
    super.initState();
    _descCtrl.text = _state.packageDesc;
    _typeCtrl.text = _state.packageType;
    _receiverNameCtrl.text = _state.receiverName;
    _receiverPhoneCtrl.text = _state.receiverPhone;
  }

  @override
  void dispose() {
    _descCtrl.dispose();
    _typeCtrl.dispose();
    _receiverNameCtrl.dispose();
    _receiverPhoneCtrl.dispose();
    super.dispose();
  }

  bool get _canContinue =>
      _descCtrl.text.trim().isNotEmpty &&
      _state.packageSize != null &&
      _receiverNameCtrl.text.trim().isNotEmpty &&
      _receiverPhoneCtrl.text.trim().isNotEmpty;

  @override
  Widget build(BuildContext context) {
    return haulingFlowScaffold(
      title: 'Package Information',
      onBack: _ctrl.backToLocationEntry,
      body: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text(
              'Tell us about the package you are sending.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.5),
            ),
            const SizedBox(height: 20),

            haulingSectionLabel('What are you sending?'),
            const SizedBox(height: 6),
            TextField(
              controller: _descCtrl,
              onChanged: (v) { setState(() {}); _ctrl.setPackageDesc(v); },
              decoration: _inputDecoration('e.g. Documents, phone, food pack...'),
            ),
            const SizedBox(height: 16),

            haulingSectionLabel('Package Type (optional)'),
            const SizedBox(height: 6),
            TextField(
              controller: _typeCtrl,
              onChanged: (v) { setState(() {}); _ctrl.setPackageType(v); },
              decoration: _inputDecoration('e.g. Parcel, envelope, box...'),
            ),
            const SizedBox(height: 20),

            haulingSectionLabel('Package Size'),
            const SizedBox(height: 8),
            Row(
              children: [
                for (final size in PackageSizeOption.values) ...[
                  Expanded(
                    child: _SelectChip(
                      label: size.displayLabel,
                      sublabel: size.description,
                      selected: _state.packageSize == size,
                      onTap: () { setState(() {}); _ctrl.setPackageSize(size); },
                    ),
                  ),
                  if (size != PackageSizeOption.values.last)
                    const SizedBox(width: 10),
                ],
              ],
            ),
            const SizedBox(height: 20),

            haulingSectionLabel('Delivery Speed'),
            const SizedBox(height: 8),
            Row(
              children: [
                for (final tier in ServiceTierOption.values) ...[
                  Expanded(
                    child: _SelectChip(
                      label: tier.displayLabel,
                      sublabel: tier.description,
                      selected: _state.serviceTier == tier,
                      onTap: () { setState(() {}); _ctrl.setServiceTier(tier); },
                    ),
                  ),
                  if (tier != ServiceTierOption.values.last)
                    const SizedBox(width: 10),
                ],
              ],
            ),
            const SizedBox(height: 20),

            haulingSectionLabel('Is the package fragile?'),
            const SizedBox(height: 8),
            Row(
              children: [
                _FragileOption(
                  label: 'No',
                  selected: !_state.isFragile,
                  onTap: () { setState(() {}); _ctrl.setIsFragile(false); },
                ),
                const SizedBox(width: 12),
                _FragileOption(
                  label: 'Yes',
                  selected: _state.isFragile,
                  onTap: () { setState(() {}); _ctrl.setIsFragile(true); },
                ),
              ],
            ),
            const SizedBox(height: 24),

            const Divider(color: CustomerFigmaColors.border),
            const SizedBox(height: 16),

            const Text(
              'Who is receiving this package?',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.5),
            ),
            const SizedBox(height: 16),

            haulingSectionLabel("Receiver's Name"),
            const SizedBox(height: 6),
            TextField(
              controller: _receiverNameCtrl,
              onChanged: (v) { setState(() {}); _ctrl.setReceiverName(v); },
              decoration: _inputDecoration('Enter full name'),
            ),
            const SizedBox(height: 16),

            haulingSectionLabel("Receiver's Phone"),
            const SizedBox(height: 6),
            TextField(
              controller: _receiverPhoneCtrl,
              keyboardType: TextInputType.phone,
              onChanged: (v) { setState(() {}); _ctrl.setReceiverPhone(v); },
              decoration: _inputDecoration('e.g. 080 1234 5678'),
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
      bottom: Column(
        children: [
          if (_state.error != null) ...[
            Text(
              _state.error!,
              style: const TextStyle(color: Colors.red, fontSize: 12),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
          ],
          FigmaPrimaryButton(
            label: 'Continue',
            onPressed:
                _canContinue ? _ctrl.proceedFromPackageInfoToPayment : null,
          ),
        ],
      ),
    );
  }

  InputDecoration _inputDecoration(String hint) => InputDecoration(
    hintText: hint,
    hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
    filled: true,
    fillColor: Colors.white,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: const BorderSide(color: CustomerFigmaColors.border),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: const BorderSide(color: CustomerFigmaColors.border),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: const BorderSide(color: CustomerFigmaColors.primary),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
  );
}

class _SelectChip extends StatelessWidget {
  const _SelectChip({
    required this.label,
    required this.sublabel,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final String sublabel;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
        decoration: BoxDecoration(
          color: selected ? CustomerFigmaColors.primaryTint : Colors.white,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(
            color: selected
                ? CustomerFigmaColors.primary
                : CustomerFigmaColors.border,
            width: 1.5,
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              label,
              style: TextStyle(
                color: selected
                    ? CustomerFigmaColors.primary
                    : CustomerFigmaColors.text,
                fontWeight: FontWeight.w700,
                fontSize: 13,
              ),
            ),
            const SizedBox(height: 3),
            Text(
              sublabel,
              style: const TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 10.5,
                height: 1.3,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _FragileOption extends StatelessWidget {
  const _FragileOption({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 10),
        decoration: BoxDecoration(
          color: selected ? CustomerFigmaColors.primaryTint : Colors.white,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: selected
                ? CustomerFigmaColors.primary
                : CustomerFigmaColors.border,
            width: 1.5,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: selected
                ? CustomerFigmaColors.primary
                : CustomerFigmaColors.text,
            fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
            fontSize: 14,
          ),
        ),
      ),
    );
  }
}
