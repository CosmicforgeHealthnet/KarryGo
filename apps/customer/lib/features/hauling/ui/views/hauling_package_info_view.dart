import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_flow_helpers.dart';

class HaulingPackageInfoView extends StatefulWidget {
  const HaulingPackageInfoView({super.key, required this.controller, required this.state});

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  State<HaulingPackageInfoView> createState() => _HaulingPackageInfoViewState();
}

class _HaulingPackageInfoViewState extends State<HaulingPackageInfoView> {
  final _receiverNameCtrl = TextEditingController();
  final _receiverPhoneCtrl = TextEditingController();
  final _contentCtrl = TextEditingController();
  final _sizeCtrl = TextEditingController();

  HaulingBookingController get _ctrl => widget.controller;
  HaulingBookingState get _state => widget.state;

  @override
  void initState() {
    super.initState();
    _receiverNameCtrl.text = _state.receiverName;
    _receiverPhoneCtrl.text = _state.receiverPhone;
    _contentCtrl.text = _state.packageContent;
    _sizeCtrl.text = _state.packageSize;
  }

  @override
  void dispose() {
    _receiverNameCtrl.dispose();
    _receiverPhoneCtrl.dispose();
    _contentCtrl.dispose();
    _sizeCtrl.dispose();
    super.dispose();
  }

  bool get _canContinue =>
      _receiverNameCtrl.text.trim().isNotEmpty &&
      _receiverPhoneCtrl.text.trim().isNotEmpty;

  @override
  Widget build(BuildContext context) {
    return haulingFlowScaffold(
      title: 'Package Information',
      onBack: _ctrl.backToDetails,
      body: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text(
              'Who is receiving this package?',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.5),
            ),
            const SizedBox(height: 20),

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
            const SizedBox(height: 24),

            const Divider(color: CustomerFigmaColors.border),
            const SizedBox(height: 16),

            const Text(
              'Tell us about the package',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.5),
            ),
            const SizedBox(height: 16),

            haulingSectionLabel('Package Content'),
            const SizedBox(height: 6),
            TextField(
              controller: _contentCtrl,
              onChanged: (v) { setState(() {}); _ctrl.setPackageContent(v); },
              decoration: _inputDecoration('e.g. Electronics, clothing, food items...'),
            ),
            const SizedBox(height: 16),

            haulingSectionLabel('Package Size'),
            const SizedBox(height: 6),
            TextField(
              controller: _sizeCtrl,
              onChanged: (v) { setState(() {}); _ctrl.setPackageSize(v); },
              decoration: _inputDecoration('e.g. Small box, large crate, pallet...'),
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
            onPressed: _canContinue ? _ctrl.proceedFromPackageInfoToPayment : null,
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

class _FragileOption extends StatelessWidget {
  const _FragileOption({required this.label, required this.selected, required this.onTap});

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
            color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.border,
            width: 1.5,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: selected ? CustomerFigmaColors.primary : CustomerFigmaColors.text,
            fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
            fontSize: 14,
          ),
        ),
      ),
    );
  }
}
