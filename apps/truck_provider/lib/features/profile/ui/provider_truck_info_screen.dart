import 'package:flutter/material.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'provider_truck_detail_screen.dart';
import 'provider_truck_edit_screen.dart';
import 'widgets/provider_profile_widgets.dart';
import 'widgets/provider_truck_widgets.dart';

/// Truck Information screen — lists all of the provider's trucks (Figma 2162).
/// Tap a card to view its full details (and edit); "Add Truck" creates a new one.
class ProviderTruckInfoScreen extends StatefulWidget {
  const ProviderTruckInfoScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProviderTruckInfoScreen> createState() => _ProviderTruckInfoScreenState();
}

class _ProviderTruckInfoScreenState extends State<ProviderTruckInfoScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.profileController.loadTrucks());
  }

  Future<void> _openDetail(ProviderTruck truck) async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderTruckDetailScreen(
          profileController: widget.profileController,
          truck: truck,
        ),
      ),
    );
    if (mounted) widget.profileController.loadTrucks();
  }

  Future<void> _addTruck() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderTruckEditScreen(
          profileController: widget.profileController,
          truck: null,
        ),
      ),
    );
    if (mounted) widget.profileController.loadTrucks();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: kProviderPageBg,
      body: SafeArea(
        child: AnimatedBuilder(
          animation: widget.profileController,
          builder: (context, _) {
            final trucks = widget.profileController.trucks;
            final loading = widget.profileController.trucksLoading && trucks.isEmpty;
            final activeCount = trucks.where((t) => t.isActive).length;

            return Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
                  child: ProviderProfileHeader(
                    title: 'Truck Information',
                    subtitle: trucks.isEmpty
                        ? 'Manage your fleet'
                        : '${trucks.length} truck${trucks.length == 1 ? '' : 's'} • $activeCount active',
                  ),
                ),
                Expanded(
                  child: loading
                      ? const Center(child: CircularProgressIndicator(color: kProviderGreen))
                      : trucks.isEmpty
                          ? _emptyState()
                          : ListView.separated(
                              padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                              itemCount: trucks.length,
                              separatorBuilder: (_, _) => const SizedBox(height: 14),
                              itemBuilder: (_, i) => _TruckCard(
                                truck: trucks[i],
                                onTap: () => _openDetail(trucks[i]),
                              ),
                            ),
                ),
                if (!loading)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                    child: ProviderPrimaryButton(
                      label: trucks.isEmpty ? 'Add Truck' : 'Add Another Truck',
                      onPressed: _addTruck,
                    ),
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _emptyState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const TruckGlyphTile(size: 96, iconSize: 46),
            const SizedBox(height: 22),
            const Text('No truck added yet',
                style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800)),
            const SizedBox(height: 8),
            const Text(
              'Add your truck details so customers can book your haulage service.',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
            ),
          ],
        ),
      ),
    );
  }
}

/// Rich summary card for one truck in the list.
class _TruckCard extends StatelessWidget {
  const _TruckCard({required this.truck, required this.onTap});

  final ProviderTruck truck;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final makeModel = [truck.make, truck.model].where((s) => s.isNotEmpty).join(' ');

    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(20),
      elevation: 0,
      shadowColor: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(20),
        child: Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: kProviderBorder),
            boxShadow: [
              BoxShadow(
                color: const Color(0xFF1B3A24).withValues(alpha: 0.05),
                blurRadius: 18,
                offset: const Offset(0, 8),
              ),
            ],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const TruckGlyphTile(),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          truck.displayType.isEmpty ? 'Truck' : truck.displayType,
                          style: const TextStyle(color: kProviderText, fontSize: 16.5, fontWeight: FontWeight.w800),
                        ),
                        const SizedBox(height: 3),
                        Text(
                          makeModel.isEmpty ? 'Tap to view details' : makeModel,
                          style: const TextStyle(color: kProviderMuted, fontSize: 13),
                        ),
                      ],
                    ),
                  ),
                  TruckStatusPill(active: truck.isActive),
                ],
              ),
              const SizedBox(height: 14),
              const Divider(height: 1, color: kProviderBorder),
              const SizedBox(height: 14),
              Row(
                children: [
                  Expanded(
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        TruckInfoChip(icon: Icons.scale_outlined, label: formatTruckCapacity(truck.capacityKg)),
                        if (truck.plateNumber.isNotEmpty)
                          TruckInfoChip(icon: Icons.confirmation_number_outlined, label: truck.plateNumber),
                        if (truck.color.isNotEmpty)
                          TruckInfoChip(label: truck.color, swatch: truckColorSwatch(truck.color)),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  const Icon(Icons.chevron_right_rounded, color: kProviderMuted),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
