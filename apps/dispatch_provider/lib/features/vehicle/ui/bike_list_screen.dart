import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/vehicle_controller.dart';
import 'add_bike_screen.dart';
import 'bike_approved_screen.dart';
import 'bike_details_screen.dart';
import 'bike_pending_screen.dart';

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
    _load();
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
        builder: (_) => AddBikeScreen(
          vehicleController: widget.vehicleController,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) _load();
  }

  Future<void> _openBike(Map<String, dynamic> bike) async {
    final status =
        (bike['verification_status'] as String? ?? 'unverified').toLowerCase();

    if (status == 'pending') {
      await Navigator.of(context).push(
        MaterialPageRoute(builder: (_) => const BikePendingScreen()),
      );
      if (mounted) _load();
      return;
    }

    if (status == 'verified') {
      await Navigator.of(context).push(
        MaterialPageRoute(builder: (_) => const BikeApprovedScreen()),
      );
      if (mounted) _load();
      return;
    }

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
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: _isLoading
                  ? Center(
                      child: CircularProgressIndicator(
                        color: c.brandTeal500,
                      ),
                    )
                  : _loadError != null
                      ? _ErrorView(message: _loadError!, onRetry: _load)
                      : ListView(
                          padding:
                              const EdgeInsets.fromLTRB(20, 8, 20, 24),
                          children: [
                            Text(
                              'Bike Vehicles',
                              style: TextStyle(
                                fontSize: 26,
                                fontWeight: FontWeight.w800,
                                color: c.text,
                                letterSpacing: -0.4,
                              ),
                            ),
                            const SizedBox(height: 6),
                            Text(
                              'Keep your vehicle information up to date and on the road.',
                              style: TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w500,
                                color: c.textMuted,
                                height: 1.45,
                              ),
                            ),
                            const SizedBox(height: 28),

                            if (_bikes.isEmpty)
                              Container(
                                padding: const EdgeInsets.all(24),
                                decoration: BoxDecoration(
                                  color: c.surface,
                                  borderRadius: BorderRadius.circular(16),
                                  border: Border.all(color: c.divider),
                                ),
                                child: Column(
                                  children: [
                                    Icon(
                                      Icons.two_wheeler_outlined,
                                      size: 40,
                                      color: c.navy40,
                                    ),
                                    const SizedBox(height: 12),
                                    Text(
                                      'No bikes added yet',
                                      style: TextStyle(
                                        fontSize: 15,
                                        fontWeight: FontWeight.w700,
                                        color: c.text,
                                      ),
                                    ),
                                    const SizedBox(height: 4),
                                    Text(
                                      'Add your first vehicle to start receiving dispatches.',
                                      textAlign: TextAlign.center,
                                      style: TextStyle(
                                        fontSize: 13,
                                        color: c.textMuted,
                                        height: 1.4,
                                      ),
                                    ),
                                  ],
                                ),
                              )
                            else
                              for (final bike in _bikes) ...[
                                _BikeCard(
                                  bike: bike,
                                  onTap: () => _openBike(bike),
                                ),
                                const SizedBox(height: 14),
                              ],

                            const SizedBox(height: 8),
                            SizedBox(
                              height: 52,
                              width: double.infinity,
                              child: OutlinedButton.icon(
                                onPressed: _openAdd,
                                icon: const Icon(
                                  Icons.add_rounded,
                                  size: 20,
                                ),
                                label: const Text('Add a new bike'),
                                style: OutlinedButton.styleFrom(
                                  foregroundColor: c.text,
                                  side: BorderSide(
                                    color: c.border,
                                    width: 1.2,
                                  ),
                                  shape: const StadiumBorder(),
                                  textStyle: const TextStyle(
                                    fontSize: 15,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                            ),
                          ],
                        ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Bike card
// ═══════════════════════════════════════════════════════════════════════════

class _BikeCard extends StatelessWidget {
  const _BikeCard({required this.bike, required this.onTap});
  final Map<String, dynamic> bike;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    // Backend field is verification_status, values are:
    // unverified | pending | verified | rejected | suspended.
    final status =
        (bike['verification_status'] as String? ?? 'unverified').toLowerCase();

    final brand = (bike['brand'] as String? ?? '').trim();
    final model = (bike['model'] as String? ?? '').trim();
    final name = [brand, model].where((s) => s.isNotEmpty).join(' ');

    final (label, fg, bg) = switch (status) {
      'verified' => ('Active', c.success, c.successSoft),
      'pending' => ('Pending', c.warning, c.warningSoft),
      'rejected' => ('Rejected', c.danger, c.dangerSoft),
      'suspended' => ('Suspended', c.danger, c.dangerSoft),
      _ => ('Unverified', c.textMuted, c.navy10),
    };

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: c.surface,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: c.divider),
        ),
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
              child: Icon(
                Icons.two_wheeler_rounded,
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
                      fontWeight: FontWeight.w800,
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
                        fontSize: 11.5,
                        fontWeight: FontWeight.w700,
                        color: fg,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right_rounded,
              size: 22,
              color: c.textMuted,
            ),
          ],
        ),
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.cloud_off_outlined, size: 48, color: c.textDisabled),
            const SizedBox(height: 16),
            Text(
              message,
              style: TextStyle(fontSize: 15, color: c.textMuted),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 20),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ],
        ),
      ),
    );
  }
}