import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../data/trip_model.dart';

class CustomerRatingScreen extends StatelessWidget {
  const CustomerRatingScreen({super.key, required this.trip});
  final TripModel trip;
  @override
  Widget build(BuildContext context) {
    final rating = trip.customerRating;
    return Scaffold(
      backgroundColor: context.palette.background,
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Customer rating'),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 24, 20, 24),
        children: [
          Center(
            child: Container(
              padding: const EdgeInsets.all(22),
              decoration: BoxDecoration(
                color: context.palette.surfaceElevated,
                borderRadius: BorderRadius.circular(28),
              ),
              child: const JourneyIcon(JourneySymbol.profile, size: 48),
            ),
          ),
          const SizedBox(height: 24),
          Text(
            rating == null ? 'Not rated' : rating.toStringAsFixed(1),
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.headlineLarge,
          ),
          if (rating != null) ...[
            const SizedBox(height: 12),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                for (var i = 0; i < 5; i++)
                  Icon(
                    i < rating.floor()
                        ? Icons.star_rounded
                        : i < rating
                        ? Icons.star_half_rounded
                        : Icons.star_outline_rounded,
                    color: context.palette.warning,
                    size: 30,
                  ),
              ],
            ),
          ],
          const SizedBox(height: 24),
          DeliveryPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  trip.customerName.isNotEmpty ? trip.customerName : 'Customer',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 8),
                Text(
                  deliveryDate(trip.createdAt),
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
              ],
            ),
          ),
          if (trip.customerFeedback?.isNotEmpty ?? false) ...[
            const SizedBox(height: 16),
            DeliveryPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    'Feedback',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 12),
                  Text(
                    trip.customerFeedback!,
                    style: Theme.of(context).textTheme.bodyLarge,
                  ),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }
}
