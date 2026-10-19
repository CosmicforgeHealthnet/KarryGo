import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/models/provider_booking.dart';
import '../data/trips_api.dart';

/// One finished ride: route, fare and the passenger's review.
class TripDetailScreen extends StatefulWidget {
  const TripDetailScreen({super.key, required this.trip, required this.api});
  final ProviderBooking trip;
  final TripsApi api;

  @override
  State<TripDetailScreen> createState() => _TripDetailScreenState();
}

class _TripDetailScreenState extends State<TripDetailScreen> {
  TripReview? _review;
  bool _loadingReview = true;

  @override
  void initState() {
    super.initState();
    if (widget.trip.status == BookingStatus.completed) {
      widget.api
          .passengerReview(widget.trip.id)
          .then((r) => _review = r, onError: (_) => null)
          .whenComplete(() {
            if (mounted) setState(() => _loadingReview = false);
          });
    } else {
      _loadingReview = false;
    }
  }

  @override
  Widget build(BuildContext context) {
    final trip = widget.trip;
    final colors = KarryamColors.of(context);
    final completed = trip.status == BookingStatus.completed;
    return KarryamPage(
      title: 'Trip ${trip.shortId}',
      subtitle: formatDateTime(trip.createdAt),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          KarryamPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                KarryamChip(
                  completed ? 'Completed' : 'Cancelled',
                  tone: completed ? KarryamTone.good : KarryamTone.bad,
                ),
                const SizedBox(height: 16),
                for (final stop in trip.stops) ...[
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        stop.isPickup
                            ? Icons.trip_origin
                            : Icons.place_outlined,
                        size: 18,
                        color: colors.primary,
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              stop.isPickup ? 'Pick-up' : 'Drop-off',
                              style: TextStyle(
                                color: colors.textMuted,
                                fontSize: 12,
                              ),
                            ),
                            Text(
                              stop.address.isEmpty
                                  ? 'Address not recorded'
                                  : stop.address,
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 12),
                ],
              ],
            ),
          ),
          const KarryamSectionTitle('Trip details'),
          KarryamPanel(
            child: Column(
              children: [
                _row(
                  'Type',
                  trip.isParcel ? 'Parcel delivery' : 'Passenger ride',
                ),
                if (trip.vehicleClass.isNotEmpty)
                  _row('Vehicle class', humanize(trip.vehicleClass)),
                if (trip.distanceKm != null)
                  _row('Distance', '${trip.distanceKm!.toStringAsFixed(1)} km'),
                if (trip.durationMinutes != null)
                  _row('Duration', '${trip.durationMinutes} min'),
                if (completed)
                  _row('Fare', formatNaira(trip.displayFareKobo), bold: true),
              ],
            ),
          ),
          if (completed) ...[
            const KarryamSectionTitle('Passenger review'),
            KarryamPanel(
              child: _loadingReview
                  ? const Center(child: CircularProgressIndicator())
                  : _review == null
                  ? Text(
                      'The passenger has not reviewed this trip.',
                      style: TextStyle(color: colors.textMuted),
                    )
                  : Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            for (var i = 1; i <= 5; i++)
                              Icon(
                                i <= (_review!.rating ?? 0)
                                    ? Icons.star_rounded
                                    : Icons.star_outline_rounded,
                                color: const Color(0xFFF2B01E),
                              ),
                          ],
                        ),
                        if (_review!.text.isNotEmpty) ...[
                          const SizedBox(height: 8),
                          Text(_review!.text),
                        ],
                      ],
                    ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _row(String label, String value, {bool bold = false}) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 6),
    child: Row(
      children: [
        Expanded(
          child: Text(
            label,
            style: TextStyle(color: KarryamColors.of(context).textMuted),
          ),
        ),
        Text(
          value,
          style: TextStyle(
            fontWeight: bold ? FontWeight.w700 : FontWeight.w500,
          ),
        ),
      ],
    ),
  );
}
