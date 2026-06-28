import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/dispute_models.dart';
import '../state/provider_dispute_controller.dart';
import 'provider_dispute_details_screen.dart';
import 'widgets/dispute_widgets.dart';

/// Select Dispute Type (Figma 2267 / 2268): the chosen transaction + the type
/// picker, then submit to create the complaint.
class ProviderSelectDisputeTypeScreen extends StatelessWidget {
  const ProviderSelectDisputeTypeScreen({super.key, required this.controller});

  final ProviderDisputeController controller;

  Future<void> _confirm(BuildContext context) async {
    final complaint = await controller.submitDispute();
    if (complaint != null && context.mounted) {
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(
          builder: (_) => ProviderDisputeDetailsScreen(controller: controller, complaint: complaint),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: AnimatedBuilder(
        animation: controller,
        builder: (context, _) {
          final txn = controller.selectedTransaction;
          return SafeArea(
            child: Column(
              children: [
                const DisputeAppBar(title: 'Log Disputes'),
                const SizedBox(height: 16),
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    children: [
                      const Align(
                        alignment: Alignment.centerLeft,
                        child: Text(
                          'Select a Transaction',
                          style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
                        ),
                      ),
                      const SizedBox(height: 10),
                      if (txn != null)
                        DisputeTransactionCard(txn: txn, selected: true),
                      const SizedBox(height: 24),
                      const Align(
                        alignment: Alignment.centerLeft,
                        child: Text(
                          'Select Dispute Type',
                          style: TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
                        ),
                      ),
                      const SizedBox(height: 14),
                      for (final type in disputeTypes)
                        _DisputeTypeRow(
                          label: type,
                          selected: controller.selectedType == type,
                          onTap: () => controller.selectType(type),
                        ),
                      if (controller.submitError != null) ...[
                        const SizedBox(height: 12),
                        Text(
                          controller.submitError!,
                          style: const TextStyle(color: kProviderRejectText, fontSize: 13),
                        ),
                      ],
                    ],
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                  child: DisputePrimaryButton(
                    label: 'Confirm',
                    loading: controller.submitting,
                    onPressed: controller.selectedType == null ? null : () => _confirm(context),
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _DisputeTypeRow extends StatelessWidget {
  const _DisputeTypeRow({required this.label, required this.selected, required this.onTap});
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        decoration: BoxDecoration(
          color: selected ? kProviderGreenPale : kProviderSurface,
          borderRadius: BorderRadius.circular(30),
          border: selected ? Border.all(color: kProviderGreen) : null,
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  color: selected ? kProviderDarkGreen : kProviderText,
                  fontSize: 14,
                  fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                ),
              ),
            ),
            if (selected) const Icon(Icons.check_circle, color: kProviderGreen, size: 20),
          ],
        ),
      ),
    );
  }
}
