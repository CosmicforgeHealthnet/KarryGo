import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/vehicle_controller.dart';
import 'add_bike_screen.dart';
import 'bike_details_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Bike / Vehicles list
//
// Backend sends verification_status: unverified | pending | verified |
// rejected | suspended. Routing by status:
//   pending   → BikePendingScreen
//   verified  → BikeApprovedScreen
//   otherwise → BikeDetailsScreen
// ═══════════════════════════════════════════════════════════════════════════

class BikeListScreen extends StatefulWidget {
  const BikeListScreen({super.key, required this.vehicleController});

  final VehicleController vehicleController;

  @override
  State<BikeListScreen> createState() => _BikeListScreenState();
}

class _BikeListScreenState extends State<BikeListScreen> {
  bool _isLoading = true;
  String? _loadError;
  List<Map<String, dynamic>> _bikes = [];

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _load();
    });
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result = await widget.vehicleController.listVehicles();
    if (!mounted) return;
    result.when(
      success: (list) => setState(() {
        _bikes = list;
        _isLoading = false;
      }),
      failure: (error) => setState(() {
        _loadError = error.message;
        _isLoading = false;
      }),
    );
  }

  Future<void> _openAdd() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) =>
            AddBikeScreen(vehicleController: widget.vehicleController),
      ),
    );
    if (!mounted) return;
    if (r == true) _load();
  }

  Future<void> _openBike(Map<String, dynamic> bike) async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => BikeDetailsScreen(
          vehicleController: widget.vehicleController,
          vehicleId: bike['id'] as String,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) _load();
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Vehicles',
    symbol: JourneySymbol.fleet,
    description: 'Your registered vehicles and their review status.',
    onRefresh: _load,
    bottom: FilledButton.icon(
      onPressed: _isLoading ? null : _openAdd,
      icon: const Icon(Icons.add_rounded),
      label: const Text('Add vehicle'),
    ),
    children: [
      if (_isLoading)
        const SizedBox(height: 220, child: ContentSkeleton(rows: 2))
      else if (_loadError != null)
        ActionNotice(message: _loadError!, isError: true, onRetry: _load)
      else if (_bikes.isEmpty)
        const DispatchEmptyState(
          story: EmptyStory.opportunities,
          title: 'Your next delivery starts here',
          message: 'Add your delivery vehicle and its registration details.',
        )
      else
        for (final bike in _bikes)
          Padding(
            padding: const EdgeInsets.only(bottom: 14),
            child: _BikeCard(bike: bike, onTap: () => _openBike(bike)),
          ),
    ],
  );
}

class _BikeCard extends StatelessWidget {
  const _BikeCard({required this.bike, required this.onTap});
  final Map<String, dynamic> bike;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    // Backend field is verification_status, values are:
    // unverified | pending | verified | rejected | suspended.
    final status = (bike['verification_status'] as String? ?? 'unverified')
        .toLowerCase();

    final brand = (bike['brand'] as String? ?? '').trim();
    final model = (bike['model'] as String? ?? '').trim();
    final name = [brand, model].where((s) => s.isNotEmpty).join(' ');

    final (label, fg, bg) = switch (status) {
      'verified' => ('Verified', c.success, c.successSoft),
      'pending' => ('Pending', c.warning, c.warningSoft),
      'rejected' => ('Rejected', c.danger, c.dangerSoft),
      'suspended' => ('Suspended', c.danger, c.dangerSoft),
      _ => ('Unverified', c.textMuted, c.navy10),
    };

    return DeliveryPanel(
      onTap: onTap,
      child: Row(
        children: [
          Container(
            width: 56,
            height: 56,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: c.teal10,
              borderRadius: BorderRadius.circular(14),
            ),
            child: JourneyIcon(
              JourneySymbol.fleet,
              size: 30,
              color: c.brandTeal600,
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  name.isEmpty ? '—' : name,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: c.text,
                    letterSpacing: -0.2,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  bike['plate_number'] as String? ?? '—',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    letterSpacing: 0.3,
                  ),
                ),
                const SizedBox(height: 8),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: bg,
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    label,
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                      color: fg,
                    ),
                  ),
                ),
              ],
            ),
          ),
          Icon(Icons.chevron_right_rounded, size: 22, color: c.textMuted),
        ],
      ),
    );
  }
}
