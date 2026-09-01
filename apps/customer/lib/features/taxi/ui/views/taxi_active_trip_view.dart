import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/taxi_models.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// The live ride: driver assigned through to arrival at the destination.
///
/// There is no live car marker here, unlike the dispatch flow — taxi-service
/// exposes no customer-facing driver-location endpoint, so the screen tracks
/// the ride through its status ladder instead of a map.
class TaxiActiveTripView extends StatelessWidget {
  const TaxiActiveTripView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  /// Cancelling is refused server-side once the trip is underway, so the
  /// button is hidden rather than offered and then rejected.
  bool get _canCancel {
    final status = state.activeBooking?.status;
    return status != null && status.isBeforePickup;
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
                'Cancel this ride?',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w800,
                  color: CustomerFigmaColors.text,
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'Your driver is already on the way.',
                textAlign: TextAlign.center,
                style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              ),
              const SizedBox(height: 18),
              FigmaPrimaryButton(
                label: 'Yes, cancel',
                onPressed: () => Navigator.of(sheetContext).pop(true),
              ),
              const SizedBox(height: 8),
              FigmaSecondaryButton(
                label: 'Keep my ride',
                onPressed: () => Navigator.of(sheetContext).pop(false),
              ),
            ],
          ),
        ),
      ),
    );
    if (confirmed == true) {
      await controller.cancelBooking();
    }
  }

  @override
  Widget build(BuildContext context) {
    final booking = state.activeBooking;
    final status = booking?.status ?? TaxiBookingStatus.accepted;
    final showCode =
        booking?.pickupVerification == PickupVerification.pending;
    final isParcel = booking?.isParcel ?? state.rideMode.isParcel;

    return taxiFlowScaffold(
      body: ListView(
        children: [
          const SizedBox(height: 6),
          Text(
            status.activeTripHeading,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 20,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            status.activeTripSubtitle(isParcel: isParcel),
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 13,
            ),
          ),
          const SizedBox(height: 18),

          // The handshake at the kerb: the passenger shows, the driver types.
          if (showCode) ...[
            CosmicforgeLogisticsVerificationCodeCard(
              title: 'Show this code to your driver',
              subtitle: isParcel
                  ? 'They will scan or enter it to confirm they collected the '
                        'right package.'
                  : 'They will scan or enter it to confirm they picked up the '
                        'right passenger.',
              code: state.pickupCredential?.code,
              expiresAt: state.pickupCredential?.expiresAt,
              onResend: controller.resendPickupCode,
              canResend: !state.isResendingCode,
            ),
            const SizedBox(height: 18),
          ] else if (booking?.pickupVerification ==
              PickupVerification.verified) ...[
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: CustomerFigmaColors.successSoft,
                borderRadius: BorderRadius.circular(12),
              ),
              child: const Row(
                children: [
                  Icon(
                    Icons.verified_rounded,
                    size: 18,
                    color: CustomerFigmaColors.success,
                  ),
                  SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Pickup confirmed by your driver.',
                      style: TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 12.5,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 18),
          ],

          // The receiver's drop-off code. Fetched only once the package is
          // actually moving, so its short life starts near drop-off.
          if (isParcel &&
              booking?.deliveryVerification == DeliveryVerification.pending) ...[
            CosmicforgeLogisticsVerificationCodeCard(
              title: 'Delivery code',
              subtitle:
                  'Give this to whoever receives the package — the driver '
                  'scans it at drop-off.',
              code: state.deliveryCredential?.code,
              expiresAt: state.deliveryCredential?.expiresAt,
              onResend: controller.resendDeliveryCode,
              canResend: !state.isResendingDeliveryCode,
            ),
            const SizedBox(height: 18),
          ],

          _StatusLadder(status: status, isParcel: isParcel),
          const SizedBox(height: 18),

          TaxiRouteSummary(
            pickupAddress: booking?.pickup?.address ?? state.pickupAddress,
            dropoffAddress: booking?.dropoff?.address ?? state.dropoffAddress,
          ),
          const SizedBox(height: 14),

          if (booking != null)
            Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: CustomerFigmaColors.field,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: CustomerFigmaColors.border),
              ),
              child: Column(
                children: [
                  _DetailRow(
                    label: 'Fare',
                    value: formatHaulingFare(booking.displayFareKobo),
                  ),
                  const SizedBox(height: 8),
                  const _DetailRow(label: 'Payment', value: 'Wallet'),
                  if (isParcel && booking.receiverName.isNotEmpty) ...[
                    const SizedBox(height: 8),
                    _DetailRow(label: 'Receiver', value: booking.receiverName),
                  ],
                ],
              ),
            ),

          if (state.error != null) ...[
            const SizedBox(height: 14),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: const Color(0xFFFDECEC),
                borderRadius: BorderRadius.circular(10),
                border: Border.all(color: const Color(0xFFF5B5B5)),
              ),
              child: Text(
                state.error!,
                style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
              ),
            ),
          ],
          const SizedBox(height: 10),
        ],
      ),
      bottom: _canCancel
          ? FigmaSecondaryButton(
              label: state.isLoading ? 'Cancelling...' : 'Cancel ride',
              onPressed: () {
                if (state.isLoading) return;
                _confirmCancel(context);
              },
            )
          : null,
    );
  }
}

/// The ride's progress through its status ladder.
class _StatusLadder extends StatelessWidget {
  const _StatusLadder({required this.status, this.isParcel = false});

  final TaxiBookingStatus status;
  final bool isParcel;

  List<(TaxiBookingStatus, String)> get _steps => [
    (TaxiBookingStatus.accepted, 'Driver assigned'),
    (
      TaxiBookingStatus.enRoutePickup,
      isParcel ? 'On the way to collect' : 'On the way to you',
    ),
    (TaxiBookingStatus.arrivedAtPickup, 'Arrived at pickup'),
    (
      TaxiBookingStatus.inProgress,
      isParcel ? 'Package on its way' : 'On the trip',
    ),
  ];

  int get _currentIndex {
    final i = _steps.indexWhere((s) => s.$1 == status);
    return i < 0 ? 0 : i;
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        for (var i = 0; i < _steps.length; i++)
          _Step(
            label: _steps[i].$2,
            done: i <= _currentIndex,
            isLast: i == _steps.length - 1,
          ),
      ],
    );
  }
}

class _Step extends StatelessWidget {
  const _Step({required this.label, required this.done, required this.isLast});

  final String label;
  final bool done;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final color =
        done ? CustomerFigmaColors.success : CustomerFigmaColors.primarySoft;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Column(
          children: [
            Icon(
              done ? Icons.check_circle_rounded : Icons.circle_outlined,
              size: 18,
              color: color,
            ),
            if (!isLast) Container(width: 2, height: 22, color: color),
          ],
        ),
        const SizedBox(width: 12),
        Padding(
          padding: const EdgeInsets.only(top: 1),
          child: Text(
            label,
            style: TextStyle(
              color: done
                  ? CustomerFigmaColors.text
                  : CustomerFigmaColors.muted,
              fontSize: 13.5,
              fontWeight: done ? FontWeight.w700 : FontWeight.w500,
            ),
          ),
        ),
      ],
    );
  }
}

class _DetailRow extends StatelessWidget {
  const _DetailRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.muted,
            fontSize: 13,
          ),
        ),
        Text(
          value,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13.5,
            fontWeight: FontWeight.w800,
          ),
        ),
      ],
    );
  }
}
