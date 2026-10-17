import 'package:flutter/material.dart';
import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/delivery_route_preview.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../../home/ui/widgets/route_info.dart';
import '../../missions/ui/delivery_operations_screen.dart';
import '../data/trip_model.dart';
import '../state/trips_controller.dart';
import 'proof_of_delivery_screen.dart';
import 'customer_rating_screen.dart';

class TripDetailScreen extends StatefulWidget {
  const TripDetailScreen({super.key, required this.trip, this.controller});
  final TripModel trip;
  final TripsController? controller;
  @override
  State<TripDetailScreen> createState() => _TripDetailScreenState();
}

class _TripDetailScreenState extends State<TripDetailScreen> {
  late TripModel _trip = widget.trip;
  bool _loading = false;
  String? _error;
  @override
  void initState() {
    super.initState();
    if (!AppModeConfig.instance.isMock && widget.controller != null)
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _refresh();
      });
  }

  Future<void> _refresh() async {
    if (widget.controller == null || _loading || AppModeConfig.instance.isMock)
      return;
    setState(() {
      _loading = true;
      _error = null;
    });
    final result = await widget.controller!.loadTrip(_trip.id);
    if (!mounted) return;
    result.when(
      success: (trip) => _trip = trip,
      failure: (error) => _error = error.message,
    );
    setState(() => _loading = false);
  }

  @override
  Widget build(BuildContext context) {
    final t = _trip, c = context.palette;
    final reason = t.statusCode.isCancelled
        ? t.cancelReason
        : t.statusCode.isFailed
        ? t.failReason
        : null;
    return Scaffold(
      backgroundColor: c.background,
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Delivery details'),
        actions: [
          if (widget.controller != null)
            IconButton(
              tooltip: 'Refresh delivery',
              onPressed: _loading ? null : _refresh,
              icon: const Icon(Icons.refresh_rounded),
            ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _refresh,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
          children: [
            if (_loading)
              const Padding(
                padding: EdgeInsets.only(bottom: 12),
                child: Center(
                  child: SizedBox.square(
                    dimension: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              ),
            if (_error != null) ...[
              ActionNotice(
                message: 'Showing saved details. $_error',
                isError: true,
                onRetry: _refresh,
              ),
              const SizedBox(height: 16),
            ],
            DeliveryPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Row(
                    children: [
                      Container(
                        padding: const EdgeInsets.all(14),
                        decoration: BoxDecoration(
                          color: c.surfaceElevated,
                          borderRadius: BorderRadius.circular(18),
                        ),
                        child: JourneyIcon(
                          t.statusCode.isCompleted
                              ? JourneySymbol.shield
                              : JourneySymbol.parcel,
                          size: 34,
                          color: t.statusCode.isCompleted
                              ? c.success
                              : t.statusCode.isCancelled ||
                                    t.statusCode.isFailed
                              ? c.danger
                              : c.brandTeal600,
                        ),
                      ),
                      const Spacer(),
                      TripStatusBadge(status: t.statusCode),
                    ],
                  ),
                  const SizedBox(height: 20),
                  Text(
                    t.customerName.isNotEmpty
                        ? t.customerName
                        : 'Delivery record',
                    style: Theme.of(context).textTheme.headlineSmall,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    deliveryDate(t.createdAt, time: true),
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  if (t.moneyVisible) ...[
                    const SizedBox(height: 20),
                    Text(
                      t.earningsKobo != null
                          ? 'Reported earnings'
                          : 'Estimated fare',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                    const SizedBox(height: 5),
                    Text(
                      t.earningsKobo != null
                          ? t.earningsDisplay
                          : t.fareDisplay,
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                  ],
                  const SizedBox(height: 16),
                  Text(
                    'Reference',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 4),
                  SelectableText(
                    t.bookingId.isNotEmpty ? t.bookingId : t.id,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                ],
              ),
            ),
            if (reason != null && reason.isNotEmpty) ...[
              const SizedBox(height: 16),
              ActionNotice(message: reason, isError: true),
            ],
            if (t.refundStatus != null) ...[
              const SizedBox(height: 16),
              DeliveryPanel(
                child: Text(
                  'Refund status: ${t.refundStatus}',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
              ),
            ],
            if (t.hasMap) ...[
              const SizedBox(height: 20),
              Text(
                'Route overview',
                style: Theme.of(context).textTheme.titleMedium,
              ),
              const SizedBox(height: 12),
              DeliveryRoutePreview(
                pickupLat: t.pickupLat!,
                pickupLng: t.pickupLng!,
                dropoffLat: t.dropoffLat!,
                dropoffLng: t.dropoffLng!,
                polyline: t.routeDropoffPolyline,
              ),
            ],
            if (t.pickupAddress.isNotEmpty && t.dropoffAddress.isNotEmpty) ...[
              const SizedBox(height: 16),
              DeliveryPanel(
                child: RouteInfo(
                  pickup: t.pickupAddress,
                  dropoff: t.dropoffAddress,
                ),
              ),
            ],
            if ((t.itemSummary?.isNotEmpty ?? false) ||
                (t.notes?.isNotEmpty ?? false)) ...[
              const SizedBox(height: 16),
              DeliveryPanel(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      'Delivery instructions',
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    if (t.itemSummary?.isNotEmpty ?? false) ...[
                      const SizedBox(height: 10),
                      Text(
                        t.itemSummary!,
                        style: Theme.of(context).textTheme.bodyLarge,
                      ),
                    ],
                    if (t.notes?.isNotEmpty ?? false) ...[
                      const SizedBox(height: 10),
                      Text(
                        t.notes!,
                        style: Theme.of(context).textTheme.bodyLarge,
                      ),
                    ],
                  ],
                ),
              ),
            ],
            if (t.receiverName.isNotEmpty || t.receiverPhone.isNotEmpty) ...[
              const SizedBox(height: 16),
              DeliveryPanel(
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const JourneyIcon(JourneySymbol.contact, size: 28),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Recipient',
                            style: Theme.of(context).textTheme.titleSmall,
                          ),
                          if (t.receiverName.isNotEmpty) ...[
                            const SizedBox(height: 8),
                            Text(
                              t.receiverName,
                              style: Theme.of(context).textTheme.bodyLarge,
                            ),
                          ],
                          if (t.receiverPhone.isNotEmpty) ...[
                            const SizedBox(height: 4),
                            SelectableText(
                              t.receiverPhone,
                              style: Theme.of(context).textTheme.bodyMedium,
                            ),
                          ],
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ],
            const SizedBox(height: 16),
            _record(context, t),
            if (t.hasProof) ...[
              const SizedBox(height: 14),
              _link(
                context,
                'Proof of delivery',
                'Review saved handover evidence',
                JourneySymbol.photo,
                () => Navigator.push(
                  context,
                  MaterialPageRoute(
                    builder: (_) => ProofOfDeliveryScreen(trip: t),
                  ),
                ),
              ),
            ],
            if (t.customerRating != null) ...[
              const SizedBox(height: 14),
              _link(
                context,
                'Customer rating',
                'View the recorded rating and feedback',
                JourneySymbol.profile,
                () => Navigator.push(
                  context,
                  MaterialPageRoute(
                    builder: (_) => CustomerRatingScreen(trip: t),
                  ),
                ),
              ),
            ],
          ],
        ),
      ),
      bottomNavigationBar: t.statusCode.isActive
          ? SafeArea(
              top: false,
              child: Container(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
                color: c.surface,
                child: FilledButton.icon(
                  onPressed: () async {
                    await Navigator.push(
                      context,
                      MaterialPageRoute(
                        builder: (_) => DeliveryOperationsScreen(tripId: t.id),
                      ),
                    );
                    if (mounted) _refresh();
                  },
                  icon: const JourneyIcon(JourneySymbol.review, size: 22),
                  label: const Text('Continue delivery'),
                ),
              ),
            )
          : null,
    );
  }

  Widget _link(
    BuildContext context,
    String title,
    String subtitle,
    JourneySymbol symbol,
    VoidCallback onTap,
  ) => DeliveryPanel(
    padding: EdgeInsets.zero,
    child: ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 10),
      leading: JourneyIcon(symbol, size: 27),
      title: Text(title, style: Theme.of(context).textTheme.titleSmall),
      subtitle: Text(subtitle, style: Theme.of(context).textTheme.bodySmall),
      trailing: const Icon(Icons.chevron_right_rounded),
      onTap: onTap,
    ),
  );
  Widget _record(BuildContext context, TripModel trip) {
    final events = trip.stateLog
        .map(
          (entry) => (
            entry['to_status']?.toString().replaceAll('_', ' ') ??
                'Status update',
            DateTime.tryParse(entry['changed_at']?.toString() ?? ''),
          ),
        )
        .where((e) => e.$2 != null)
        .toList();
    if (events.isEmpty) {
      events.add(('Created', trip.createdAt));
      if (trip.startedAt != null) events.add(('Started', trip.startedAt));
      if (trip.completedAt != null) events.add(('Completed', trip.completedAt));
      if (trip.cancelledAt != null) events.add(('Cancelled', trip.cancelledAt));
      if (trip.failedAt != null) events.add(('Failed', trip.failedAt));
    }
    return DeliveryPanel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            'Delivery record',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 16),
          for (final event in events)
            Padding(
              padding: const EdgeInsets.only(bottom: 14),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const JourneyIcon(JourneySymbol.review, size: 20),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          event.$1,
                          style: Theme.of(context).textTheme.titleSmall,
                        ),
                        const SizedBox(height: 3),
                        Text(
                          deliveryDate(event.$2!, time: true),
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
        ],
      ),
    );
  }
}
