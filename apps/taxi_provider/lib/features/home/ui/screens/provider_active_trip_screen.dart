import '../../../../core/widgets/driver_map.dart';
import '../../../../core/widgets/driver_state_art.dart';
import '../../../../core/theme/karryam_theme.dart';
import '../../../../core/widgets/driver_code_sheet.dart';
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
    this.onSos,
  });

  final ProviderHomeController controller;
  final ProviderBooking booking;

  /// Opens the emergency options; the SOS button is hidden when null.
  final VoidCallback? onSos;

  Future<void> _scanAndVerify(
    BuildContext context, {
    required bool delivery,
  }) async {
    final code = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      backgroundColor: KarryamColors.of(context).card,
      builder: (_) => DriverCodeSheet(
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
        duration: Duration(seconds: 2),
      ),
    );
  }

  Future<void> _complete(BuildContext context) async {
    final navigator = Navigator.of(context);
    final success = await controller.completeTrip();
    if (!success ||
        !navigator.mounted ||
        controller.state.activeBooking != null) {
      return;
    }
    await showModalBottomSheet<void>(
      context: navigator.context,
      isScrollControlled: true,
      builder: (sheetContext) => SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const DriverStateArt(kind: DriverArt.complete),
              Text(
                booking.isParcel ? 'Delivery complete' : 'Trip complete',
                style: const TextStyle(
                  fontSize: 28,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 12),
              const Text(
                'Another connection made. Ready for the next one?',
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 24),
              SizedBox(
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => Navigator.pop(sheetContext),
                  child: const Text('Back to Home'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _confirmCancel(BuildContext context) async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      backgroundColor: KarryamColors.of(context).card,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (sheetContext) => SafeArea(
        child: Padding(
          padding: EdgeInsets.all(20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                'Cancel this trip?',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w800,
                  color: KarryamColors.of(context).text,
                ),
              ),
              SizedBox(height: 8),
              Text(
                'The customer will be told, and it counts against your '
                'cancellation rate.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: KarryamColors.of(context).textMuted,
                  fontSize: 13,
                ),
              ),
              SizedBox(height: 18),
              PrimaryActionButton(
                label: 'Yes, cancel',
                color: KarryamColors.of(context).danger,
                onPressed: () => Navigator.of(sheetContext).pop(true),
              ),
              SizedBox(height: 8),
              TextButton(
                onPressed: () => Navigator.of(sheetContext).pop(false),
                child: Text('Keep the trip'),
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
            label: booking.isParcel
                ? 'Scan sender code'
                : 'Scan passenger code',
            onTap: () => _scanAndVerify(context, delivery: false),
            color: KarryamColors.of(context).success,
          );
        }
        return (label: 'Start trip', onTap: controller.startTrip, color: null);
      case BookingStatus.inProgress:
        if (booking.awaitingDeliveryVerification) {
          return (
            label: 'Scan receiver code',
            onTap: () => _scanAndVerify(context, delivery: true),
            color: KarryamColors.of(context).success,
          );
        }
        return (
          label: booking.isParcel ? 'Complete delivery' : 'End trip',
          onTap: () => _complete(context),
          color: KarryamColors.of(context).success,
        );
      default:
        return (label: 'Updating trip...', onTap: null, color: null);
    }
  }

  String get _heading => switch (booking.status) {
    BookingStatus.accepted => 'Trip accepted',
    BookingStatus.enRoutePickup => 'Heading to pickup',
    BookingStatus.arrivedAtPickup =>
      booking.awaitingPickupVerification
          ? 'Confirm before you set off'
          : 'Ready to start',
    BookingStatus.inProgress =>
      booking.awaitingDeliveryVerification
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
    BookingStatus.arrivedAtPickup =>
      booking.awaitingPickupVerification
          ? 'Scan the code on their screen to unlock the trip.'
          : 'You can set off now.',
    BookingStatus.inProgress =>
      booking.awaitingDeliveryVerification
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
      backgroundColor: KarryamColors.of(context).surface,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: EdgeInsets.fromLTRB(20, 20, 20, 12),
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          _heading,
                          style: TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w800,
                            color: KarryamColors.of(context).text,
                          ),
                        ),
                      ),
                      JobTypeBadge(isParcel: booking.isParcel),
                      if (onSos != null) ...[
                        SizedBox(width: 8),
                        OutlinedButton(
                          onPressed: onSos,
                          style: OutlinedButton.styleFrom(
                            foregroundColor: KarryamColors.of(context).danger,
                            side: BorderSide(
                              color: KarryamColors.of(context).danger,
                            ),
                            visualDensity: VisualDensity.compact,
                          ),
                          child: Text('SOS'),
                        ),
                      ],
                    ],
                  ),
                  SizedBox(height: 6),
                  Text(
                    _subheading,
                    style: TextStyle(
                      fontSize: 13,
                      color: KarryamColors.of(context).textMuted,
                    ),
                  ),

                  if (state.error != null) ...[
                    SizedBox(height: 16),
                    InlineBanner(
                      message: state.error!,
                      onDismiss: controller.clearError,
                    ),
                  ],

                  SizedBox(height: 20),
                  ClipRRect(
                    borderRadius: BorderRadius.circular(20),
                    child: SizedBox(
                      height: 220,
                      child: DriverMap(booking: booking),
                    ),
                  ),
                  SizedBox(height: 16),
                  if (booking.awaitingPickupVerification ||
                      booking.awaitingDeliveryVerification)
                    DriverStateArt(kind: DriverArt.verifying, height: 100),
                  _Card(child: RouteSummary(booking: booking)),
                  SizedBox(height: 12),
                  _Card(child: JobStatsRow(booking: booking)),

                  if (booking.isParcel) ...[
                    SizedBox(height: 12),
                    _Card(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Package',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w800,
                              color: KarryamColors.of(context).text,
                            ),
                          ),
                          SizedBox(height: 6),
                          _row(context, 'What', booking.packageDesc),
                          if (booking.packageSize.isNotEmpty)
                            _row(context, 'Size', booking.packageSize),
                          _row(context, 'Receiver', booking.receiverName),
                          _row(context, 'Phone', booking.receiverPhone),
                          if (booking.isFragile)
                            Padding(
                              padding: EdgeInsets.only(top: 6),
                              child: Text(
                                'Handle with care — marked fragile.',
                                style: TextStyle(
                                  color: KarryamColors.of(context).warning,
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
                    SizedBox(height: 12),
                    _Card(
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Icon(
                            Icons.sticky_note_2_rounded,
                            size: 16,
                            color: KarryamColors.of(context).textMuted,
                          ),
                          SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              booking.rideNotes,
                              style: TextStyle(
                                fontSize: 13,
                                color: KarryamColors.of(context).text,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],

                  SizedBox(height: 12),
                  _Card(
                    child: Row(
                      children: [
                        Text(
                          'Fare',
                          style: TextStyle(
                            color: KarryamColors.of(context).textMuted,
                            fontSize: 13,
                          ),
                        ),
                        Spacer(),
                        Text(
                          booking.displayFareKobo > 0
                              ? formatNaira(booking.displayFareKobo)
                              : '--',
                          style: TextStyle(
                            fontSize: 16,
                            fontWeight: FontWeight.w800,
                            color: KarryamColors.of(context).text,
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),

            Padding(
              padding: EdgeInsets.fromLTRB(20, 4, 20, 20),
              child: Column(
                children: [
                  PrimaryActionButton(
                    label: action.label,
                    onPressed: action.onTap,
                    isLoading: state.isActionLoading,
                    color: action.color,
                  ),
                  if (canCancel) ...[
                    SizedBox(height: 8),
                    TextButton(
                      onPressed: state.isActionLoading
                          ? null
                          : () => _confirmCancel(context),
                      child: Text(
                        'Cancel trip',
                        style: TextStyle(
                          color: KarryamColors.of(context).danger,
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

  Widget _row(BuildContext context, String label, String value) {
    if (value.trim().isEmpty) return SizedBox.shrink();
    return Padding(
      padding: EdgeInsets.only(top: 4),
      child: Row(
        children: [
          SizedBox(
            width: 78,
            child: Text(
              label,
              style: TextStyle(
                color: KarryamColors.of(context).textMuted,
                fontSize: 12.5,
              ),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: TextStyle(
                color: KarryamColors.of(context).text,
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
      padding: EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: KarryamColors.of(context).card,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: KarryamColors.of(context).border),
      ),
      child: child,
    );
  }
}
