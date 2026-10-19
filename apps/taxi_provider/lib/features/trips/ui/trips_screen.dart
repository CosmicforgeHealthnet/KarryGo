import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/models/provider_booking.dart';
import '../data/trips_api.dart';
import 'trip_detail_screen.dart';

/// The Trips tab: finished and cancelled rides from the last 90 days.
class TripsScreen extends StatefulWidget {
  const TripsScreen({super.key, required this.api});
  final TripsApi api;

  @override
  State<TripsScreen> createState() => TripsScreenState();
}

class TripsScreenState extends State<TripsScreen> {
  List<ProviderBooking>? _trips;
  String? _error;

  @override
  void initState() {
    super.initState();
    reload();
  }

  /// Called when the tab is opened, so a just-finished ride shows up.
  Future<void> reload() async {
    setState(() => _error = null);
    try {
      final trips = await widget.api.listHistory();
      if (mounted) setState(() => _trips = trips);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        backgroundColor: colors.surface,
        body: SafeArea(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Padding(
                padding: EdgeInsets.fromLTRB(20, 20, 20, 4),
                child: Text(
                  'Your trips',
                  style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                child: Text(
                  'Rides from the last 90 days.',
                  style: TextStyle(color: colors.textMuted),
                ),
              ),
              TabBar(
                labelColor: colors.text,
                indicatorColor: colors.primary,
                tabs: const [
                  Tab(text: 'Completed'),
                  Tab(text: 'Cancelled'),
                ],
              ),
              Expanded(
                child: TabBarView(
                  children: [
                    _list(BookingStatus.completed),
                    _list(BookingStatus.cancelled),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _list(BookingStatus status) {
    if (_error != null && _trips == null) {
      return KarryamPlaceholder.error(message: _error!, onRetry: reload);
    }
    final trips = _trips;
    if (trips == null) return const KarryamPlaceholder.loading();
    final shown = trips.where((t) {
      if (status == BookingStatus.cancelled) {
        return t.status == BookingStatus.cancelled ||
            t.status == BookingStatus.unmatched;
      }
      return t.status == status;
    }).toList();
    return RefreshIndicator(
      onRefresh: reload,
      child: shown.isEmpty
          ? ListView(
              children: [
                KarryamPlaceholder.empty(
                  icon: Icons.route_outlined,
                  message: status == BookingStatus.completed
                      ? 'No completed trips yet.'
                      : 'No cancelled trips.',
                ),
              ],
            )
          : ListView.separated(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
              itemCount: shown.length,
              separatorBuilder: (_, _) => const SizedBox(height: 12),
              itemBuilder: (context, i) => TripCard(
                trip: shown[i],
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) =>
                        TripDetailScreen(trip: shown[i], api: widget.api),
                  ),
                ),
              ),
            ),
    );
  }
}

class TripCard extends StatelessWidget {
  const TripCard({super.key, required this.trip, this.onTap});
  final ProviderBooking trip;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final completed = trip.status == BookingStatus.completed;
    return KarryamPanel(
      onTap: onTap,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                trip.isParcel
                    ? Icons.inventory_2_outlined
                    : Icons.local_taxi_outlined,
                size: 18,
                color: colors.textMuted,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  formatDateTime(trip.createdAt),
                  style: TextStyle(color: colors.textMuted, fontSize: 12),
                ),
              ),
              KarryamChip(
                completed ? 'Completed' : 'Cancelled',
                tone: completed ? KarryamTone.good : KarryamTone.bad,
              ),
            ],
          ),
          const SizedBox(height: 12),
          _stop(context, Icons.trip_origin, trip.pickup?.address),
          const SizedBox(height: 6),
          _stop(context, Icons.place_outlined, trip.dropoff?.address),
          if (completed) ...[
            const SizedBox(height: 12),
            Text(
              formatNaira(trip.displayFareKobo),
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
          ],
        ],
      ),
    );
  }

  Widget _stop(BuildContext context, IconData icon, String? address) => Row(
    children: [
      Icon(icon, size: 16, color: KarryamColors.of(context).primary),
      const SizedBox(width: 8),
      Expanded(
        child: Text(
          (address == null || address.isEmpty)
              ? 'Address not recorded'
              : address,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
      ),
    ],
  );
}
