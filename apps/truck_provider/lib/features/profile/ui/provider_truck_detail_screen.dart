import 'package:flutter/material.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'provider_truck_edit_screen.dart';
import 'widgets/provider_profile_widgets.dart';
import 'widgets/provider_truck_widgets.dart';

/// Read-only detail for a single truck (Figma 2162) with the Edit confirmation
/// dialog (Figma 2163). Opened from the truck list in [ProviderTruckInfoScreen].
class ProviderTruckDetailScreen extends StatelessWidget {
  const ProviderTruckDetailScreen({
    super.key,
    required this.profileController,
    required this.truck,
  });

  final ProviderProfileController profileController;
  final ProviderTruck truck;

  Future<void> _confirmEdit(BuildContext context) async {
    final ok = await showProviderConfirmDialog(
      context,
      icon: Icons.directions_car_filled_outlined,
      title: 'Edit Vehicle Details?',
      message: 'Editing vehicle details may require re-verification of your identity. Are you sure you want to continue?',
      confirmLabel: 'Yes',
      cancelLabel: 'No',
    );
    if (ok == true && context.mounted) {
      await Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => ProviderTruckEditScreen(
            profileController: profileController,
            truck: truck,
          ),
        ),
      );
      await profileController.loadTrucks();
      if (context.mounted) Navigator.of(context).pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: kProviderPageBg,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
              child: ProviderProfileHeader(
                title: 'Truck Information',
                subtitle: truck.displayType,
              ),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  _hero(),
                  const SizedBox(height: 18),
                  _sectionTitle('Vehicle Details'),
                  const SizedBox(height: 10),
                  _detailsCard(),
                  const SizedBox(height: 18),
                  _sectionTitle('Goods You Can Carry'),
                  const SizedBox(height: 10),
                  _goodsCard(),
                  const SizedBox(height: 18),
                  _insuranceCard(),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: ProviderPrimaryButton(
                label: 'Edit',
                onPressed: () => _confirmEdit(context),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Hero card: gradient banner with type, plate, capacity, status ──
  Widget _hero() {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        gradient: kProviderBalanceGradient,
        borderRadius: BorderRadius.circular(24),
        boxShadow: [
          BoxShadow(
            color: kProviderGreen.withValues(alpha: 0.28),
            blurRadius: 22,
            offset: const Offset(0, 12),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 54,
                height: 54,
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.18),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: const Icon(Icons.local_shipping_rounded, color: Colors.white, size: 28),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      truck.displayType.isEmpty ? 'Truck' : truck.displayType,
                      style: const TextStyle(color: Colors.white, fontSize: 20, fontWeight: FontWeight.w800),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      truck.plateNumber.isEmpty ? 'No plate number' : truck.plateNumber,
                      style: TextStyle(
                        color: Colors.white.withValues(alpha: 0.85),
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        letterSpacing: 0.4,
                      ),
                    ),
                  ],
                ),
              ),
              TruckStatusPill(active: truck.isActive, onDark: true),
            ],
          ),
          const SizedBox(height: 18),
          Row(
            children: [
              _heroStat('Capacity', formatTruckCapacity(truck.capacityKg)),
              Container(width: 1, height: 34, color: Colors.white.withValues(alpha: 0.22)),
              _heroStat('Colour', truck.color.isEmpty ? '—' : truck.color),
            ],
          ),
        ],
      ),
    );
  }

  Widget _heroStat(String label, String value) {
    return Expanded(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: TextStyle(color: Colors.white.withValues(alpha: 0.78), fontSize: 12)),
          const SizedBox(height: 4),
          Text(value, style: const TextStyle(color: Colors.white, fontSize: 16, fontWeight: FontWeight.w800)),
        ],
      ),
    );
  }

  // ── Vehicle details card: label/value rows with dividers ──
  Widget _detailsCard() {
    final rows = <(String, String)>[
      ('Truck Type', truck.displayType),
      ('Brand', truck.make),
      ('Model', truck.model),
      ('Colour', truck.color),
      ('Plate Number', truck.plateNumber),
      ('Number of Axles', truck.numberOfAxles),
      ('Years of Experience', truck.yearsOfExperience),
      ('License Type', truck.licenseType),
    ];
    return _card(
      child: Column(
        children: [
          for (var i = 0; i < rows.length; i++) ...[
            if (i > 0) const Divider(height: 22, color: kProviderBorder),
            _kvRow(rows[i].$1, rows[i].$2),
          ],
        ],
      ),
    );
  }

  Widget _kvRow(String label, String value) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          flex: 5,
          child: Text(label, style: const TextStyle(color: kProviderMuted, fontSize: 13.5)),
        ),
        const SizedBox(width: 12),
        Expanded(
          flex: 6,
          child: Text(
            value.isEmpty ? '—' : value,
            textAlign: TextAlign.right,
            style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
          ),
        ),
      ],
    );
  }

  // ── Goods chips ──
  Widget _goodsCard() {
    if (truck.goodsTypes.isEmpty) {
      return _card(
        child: const Text(
          'No goods types specified yet.',
          style: TextStyle(color: kProviderMuted, fontSize: 13.5),
        ),
      );
    }
    return _card(
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          for (final g in truck.goodsTypes)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              decoration: BoxDecoration(
                color: kProviderGreenTint,
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(g, style: const TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w600)),
            ),
        ],
      ),
    );
  }

  // ── Insurance row ──
  Widget _insuranceCard() {
    final has = truck.hasInsurance;
    return _card(
      child: Row(
        children: [
          Container(
            width: 42,
            height: 42,
            decoration: BoxDecoration(
              color: has ? kProviderGreenTint : kProviderRejectBg,
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(
              has ? Icons.verified_user_outlined : Icons.gpp_maybe_outlined,
              color: has ? kProviderGreen : kProviderRejectText,
              size: 22,
            ),
          ),
          const SizedBox(width: 14),
          const Expanded(
            child: Text(
              'Active vehicle insurance',
              style: TextStyle(color: kProviderText, fontSize: 14.5, fontWeight: FontWeight.w700),
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: has ? kProviderGreenTint : kProviderRejectBg,
              borderRadius: BorderRadius.circular(999),
            ),
            child: Text(
              has ? 'Yes' : 'No',
              style: TextStyle(
                color: has ? kProviderGreen : kProviderRejectText,
                fontSize: 13,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Shared section helpers ──
  Widget _sectionTitle(String text) => Padding(
        padding: const EdgeInsets.only(left: 4),
        child: Text(text, style: const TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800)),
      );

  Widget _card({required Widget child}) => Container(
        width: double.infinity,
        padding: const EdgeInsets.all(18),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(18),
          border: Border.all(color: kProviderBorder),
          boxShadow: [
            BoxShadow(
              color: const Color(0xFF1B3A24).withValues(alpha: 0.04),
              blurRadius: 16,
              offset: const Offset(0, 6),
            ),
          ],
        ),
        child: child,
      );
}
