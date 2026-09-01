import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// What is being sent, and who receives it. Parcel mode only.
class TaxiParcelDetailsView extends StatefulWidget {
  const TaxiParcelDetailsView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  State<TaxiParcelDetailsView> createState() => _TaxiParcelDetailsViewState();
}

class _TaxiParcelDetailsViewState extends State<TaxiParcelDetailsView> {
  late final TextEditingController _descCtrl;
  late final TextEditingController _typeCtrl;
  late final TextEditingController _receiverNameCtrl;
  late final TextEditingController _receiverPhoneCtrl;

  static const _sizes = ['small', 'medium', 'large'];

  @override
  void initState() {
    super.initState();
    _descCtrl = TextEditingController(text: widget.state.packageDesc);
    _typeCtrl = TextEditingController(text: widget.state.packageType);
    _receiverNameCtrl = TextEditingController(text: widget.state.receiverName);
    _receiverPhoneCtrl = TextEditingController(text: widget.state.receiverPhone);
  }

  @override
  void dispose() {
    _descCtrl.dispose();
    _typeCtrl.dispose();
    _receiverNameCtrl.dispose();
    _receiverPhoneCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final controller = widget.controller;

    return taxiFlowScaffold(
      title: 'Package details',
      onBack: controller.backToFareSelection,
      body: ListView(
        children: [
          taxiSectionLabel('What are you sending?'),
          FigmaTextField(
            controller: _descCtrl,
            label: '',
            hintText: 'e.g. A4 documents',
            variant: FigmaTextFieldVariant.grey,
            onSubmitted: controller.setPackageDesc,
          ),
          const SizedBox(height: 16),

          taxiSectionLabel('Type (optional)'),
          FigmaTextField(
            controller: _typeCtrl,
            label: '',
            hintText: 'e.g. documents, electronics',
            variant: FigmaTextFieldVariant.grey,
            onSubmitted: controller.setPackageType,
          ),
          const SizedBox(height: 16),

          taxiSectionLabel('Size'),
          Row(
            children: [
              for (final size in _sizes) ...[
                Expanded(
                  child: _SizeChip(
                    label: '${size[0].toUpperCase()}${size.substring(1)}',
                    selected: state.packageSize == size,
                    onTap: () => controller.setPackageSize(size),
                  ),
                ),
                if (size != _sizes.last) const SizedBox(width: 8),
              ],
            ],
          ),
          const SizedBox(height: 6),
          const Text(
            'Size helps the driver plan — it does not change the fare.',
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11.5),
          ),
          const SizedBox(height: 16),

          Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 4),
            decoration: BoxDecoration(
              color: CustomerFigmaColors.fieldMuted,
              borderRadius: BorderRadius.circular(12),
            ),
            child: SwitchListTile.adaptive(
              contentPadding: EdgeInsets.zero,
              value: state.isFragile,
              onChanged: controller.setIsFragile,
              activeThumbColor: CustomerFigmaColors.primary,
              title: const Text(
                'Fragile',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
              subtitle: const Text(
                'Ask the driver to handle it with care.',
                style: TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 11.5,
                ),
              ),
            ),
          ),
          const SizedBox(height: 20),

          taxiSectionLabel('Who is receiving it?'),
          FigmaTextField(
            controller: _receiverNameCtrl,
            label: '',
            hintText: 'Receiver name',
            variant: FigmaTextFieldVariant.grey,
            onSubmitted: controller.setReceiverName,
          ),
          const SizedBox(height: 10),
          FigmaTextField(
            controller: _receiverPhoneCtrl,
            label: '',
            hintText: '+234 801 234 5678',
            keyboardType: TextInputType.phone,
            variant: FigmaTextFieldVariant.grey,
            inputFormatters: [
              FilteringTextInputFormatter.allow(RegExp(r'[0-9+ ]')),
            ],
            onSubmitted: controller.setReceiverPhone,
          ),
          const SizedBox(height: 6),
          const Text(
            'They will show the driver a code at drop-off, so the package '
            'reaches the right person.',
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 11.5),
          ),

          if (state.error != null) ...[
            const SizedBox(height: 14),
            Text(
              state.error!,
              style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
            ),
          ],
          const SizedBox(height: 10),
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'Continue',
        onPressed: () {
          // Commit anything typed but never submitted, then let the
          // controller's own readiness check decide.
          controller.setPackageDesc(_descCtrl.text);
          controller.setPackageType(_typeCtrl.text);
          controller.setReceiverName(_receiverNameCtrl.text);
          controller.setReceiverPhone(_receiverPhoneCtrl.text);
          if (!controller.state.canSubmitParcelDetails) {
            controller.showParcelDetailsIncomplete();
            return;
          }
          controller.proceedFromParcelDetails();
        },
      ),
    );
  }
}

class _SizeChip extends StatelessWidget {
  const _SizeChip({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(12),
      child: Container(
        height: 44,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: selected
              ? CustomerFigmaColors.primary
              : CustomerFigmaColors.fieldMuted,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: selected ? Colors.white : CustomerFigmaColors.text,
            fontSize: 13.5,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}
