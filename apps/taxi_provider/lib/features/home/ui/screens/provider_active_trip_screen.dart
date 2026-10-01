import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../models/provider_booking.dart';
import '../../state/provider_home_controller.dart';
import '../widgets/provider_widgets.dart';

/// The committed ride, from accept through to complete.
///
/// The two handovers are scans, not typing: the customer's app renders the
/// code as a QR precisely so the driver never has to read six digits aloud at
/// the kerb. Manual entry stays available in the same sheet as a fallback.
class ProviderActiveTripScreen extends StatelessWidget {
  const ProviderActiveTripScreen({
    super.key,
    required this.controller,
    required this.booking,
  });

  final ProviderHomeController controller;
  final ProviderBooking booking;

  Future<void> _scanAndVerify(
    BuildContext context, {
    required bool delivery,
  }) async {
    final code = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => CosmicforgeLogisticsCodeEntrySheet(
        title: delivery ? 'Scan the delivery code' : 'Scan the pickup code',
        subtitle: delivery
            ? 'Ask the person receiving the package to show their code.'
            : booking.isParcel
                  ? 'Ask the sender to show their code before you take the package.'
                  : 'Ask your passenger to show their code before you set off.',
      ),
    );
    if (code == null || code.isEmpty) return;

    final ok = delivery
        ? await controller.verifyDelivery(code)
        : await controller.verifyPickup(code);
    if (!ok || !context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(delivery ? 'Delivery confirmed.' : 'Pickup confirmed.'),
        duration: const Duration(seconds: 2),
      ),
    );
  }

  Future<void> _confirmCancel(BuildContext context) async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (sheetContext) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text(
                'Cancel this trip?',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w800,
                  color: CosmicforgeLogisticsColors.text,
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'The customer will be told, and it counts against your '
                'cancellation rate.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: CosmicforgeLogisticsColors.textMuted,
                  fontSize: 13,
                ),
              ),
              const SizedBox(height: 18),
              PrimaryActionButton(
                label: 'Yes, cancel',
                color: CosmicforgeLogisticsColors.danger,
                onPressed: () => Navigator.of(sheetContext).pop(true),
              ),
              const SizedBox(height: 8),
              TextButton(
                onPressed: () => Navigator.of(sheetContext).pop(false),
                child: const Text('Keep the trip'),
              ),
            ],
          ),
        ),
      ),
    );
    if (confirmed == true) await controller.cancelTrip();
  }

  /// The single next action for the current status. Returning a disabled
  /// placeholder for an unexpected status is deliberate: re-firing the first
  /// step would be worse than a driver waiting a beat for the poll.
  ({String label, VoidCallback? onTap, Color? color}) _action(
    BuildContext context,
    ProviderHomeState state,
  ) {
    switch (booking.status) {
      case BookingStatus.accepted:
        return (
          label: 'Start driving to pickup',
          onTap: controller.markEnRoutePickup,
          color: null,
        );
      case BookingStatus.enRoutePickup:
        return (
          label: "I've arrived",
          onTap: controller.markArrived,
          color: null,
        );
      case BookingStatus.arrivedAtPickup:
        if (booking.awaitingPickupVerification) {
          return (
            label: booking.isParcel ? 'Scan sender code' : 'Scan passenger code',
            onTap: () => _scanAndVerify(context, delivery: false),
            color: CosmicforgeLogisticsColors.success,
          );
        }
        return (label: 'Start trip', onTap: controller.startTrip, color: null);
      case BookingStatus.inProgress:
        if (booking.awaitingDeliveryVerification) {
          return (
            label: 'Scan receiver code',
            onTap: () => _scanAndVerify(context, delivery: true),
            color: CosmicforgeLogisticsColors.success,
          );
        }
        return (
          label: booking.isParcel ? 'Complete delivery' : 'End trip',
          onTap: controller.completeTrip,
          color: CosmicforgeLogisticsColors.success,
        );
      default:
        return (label: 'Updating trip...', onTap: null, color: null);
    }
  }

  String get _heading => switch (booking.status) {
    BookingStatus.accepted => 'Trip accepted',
    BookingStatus.enRoutePickup => 'Heading to pickup',
    BookingStatus.arrivedAtPickup => booking.awaitingPickupVerification
        ? 'Confirm before you set off'
        : 'Ready to start',
    BookingStatus.inProgress => booking.awaitingDeliveryVerification
        ? 'Confirm the hand-over'
        : (booking.isParcel ? 'Delivering' : 'Trip in progress'),
    _ => 'Trip',
  };

  String get _subheading => switch (booking.status) {
    BookingStatus.accepted =>
      booking.isParcel
          ? 'Head to the sender to collect the package.'
          : 'Head to your passenger.',
    BookingStatus.enRoutePickup => 'Let them know when you get there.',
    BookingStatus.arrivedAtPickup => booking.awaitingPickupVerification
        ? 'Scan the code on their screen to unlock the trip.'
        : 'You can set off now.',
    BookingStatus.inProgress => booking.awaitingDeliveryVerification
        ? 'Scan the receiver\'s code to complete the delivery.'
        : (booking.isParcel
              ? 'Take the package to the drop-off.'
              : 'Drive safely.'),
    _ => '',
  };

  @override
  Widget build(BuildContext context) {
    final state = controller.state;
    final action = _action(context, state);
    // Cancelling is refused server-side once a passenger is aboard, so the
    // option disappears rather than being offered and then rejected.
    final canCancel = booking.status != BookingStatus.inProgress;

    return Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 12),
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          _heading,
                          style: const TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w800,
                            color: CosmicforgeLogisticsColors.text,
                          ),
                        ),
                      ),
                      JobTypeBadge(isParcel: booking.isParcel),
                    ],
                  ),
                  const SizedBox(height: 6),
                  Text(
                    _subheading,
                    style: const TextStyle(
                      fontSize: 13,
                      color: CosmicforgeLogisticsColors.textMuted,
                    ),
                  ),

                  if (state.error != null) ...[
                    const SizedBox(height: 16),
                    InlineBanner(
                      message: state.error!,
                      onDismiss: controller.clearError,
                    ),
                  ],

                  const SizedBox(height: 20),
                  _Card(child: RouteSummary(booking: booking)),
                  const SizedBox(height: 12),
                  _Card(child: JobStatsRow(booking: booking)),

                  if (booking.isParcel) ...[
                    const SizedBox(height: 12),
                    _Card(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            'Package',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w800,
                              color: CosmicforgeLogisticsColors.text,
                            ),
                          ),
                          const SizedBox(height: 6),
                          _row('What', booking.packageDesc),
                          if (booking.packageSize.isNotEmpty)
                            _row('Size', booking.packageSize),
                          _row('Receiver', booking.receiverName),
                          _row('Phone', booking.receiverPhone),
                          if (booking.isFragile)
                            const Padding(
                              padding: EdgeInsets.only(top: 6),
                              child: Text(
                                'Handle with care — marked fragile.',
                                style: TextStyle(
                                  color: CosmicforgeLogisticsColors.warning,
                                  fontSize: 12,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
                  ],

                  if (booking.rideNotes.isNotEmpty) ...[
                    const SizedBox(height: 12),
                    _Card(
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Icon(
                            Icons.sticky_note_2_rounded,
                            size: 16,
                            color: CosmicforgeLogisticsColors.textMuted,
                          ),
                          const SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              booking.rideNotes,
                              style: const TextStyle(
                                fontSize: 13,
                                color: CosmicforgeLogisticsColors.text,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],

                  const SizedBox(height: 12),
                  _Card(
                    child: Row(
                      children: [
                        const Text(
                          'Fare',
                          style: TextStyle(
                            color: CosmicforgeLogisticsColors.textMuted,
                            fontSize: 13,
                          ),
                        ),
                        const Spacer(),
                        Text(
                          booking.displayFareKobo > 0
                              ? formatNaira(booking.displayFareKobo)
                              : '--',
                          style: const TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w800,
                            color: CosmicforgeLogisticsColors.text,
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 4, 20, 20),
              child: Column(
                children: [
                  PrimaryActionButton(
                    label: action.label,
                    onPressed: action.onTap,
                    isLoading: state.isActionLoading,
                    color: action.color,
                  ),
                  if (canCancel) ...[
                    const SizedBox(height: 8),
                    TextButton(
                      onPressed: state.isActionLoading
                          ? null
                          : () => _confirmCancel(context),
                      child: const Text(
                        'Cancel trip',
                        style: TextStyle(
                          color: CosmicforgeLogisticsColors.danger,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _row(String label, String value) {
    if (value.trim().isEmpty) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: Row(
        children: [
          SizedBox(
            width: 78,
            child: Text(
              label,
              style: const TextStyle(
                color: CosmicforgeLogisticsColors.textMuted,
                fontSize: 12.5,
              ),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: const TextStyle(
                color: CosmicforgeLogisticsColors.text,
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _Card extends StatelessWidget {
  const _Card({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.card,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: CosmicforgeLogisticsColors.border),
      ),
      child: child,
    );
  }
}
