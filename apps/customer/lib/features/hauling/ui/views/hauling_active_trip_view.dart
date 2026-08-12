import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_map_widget.dart';
import '../widgets/hauling_trip_sheet_widgets.dart';

/// Live trip screen (Figma 1772 "Driver Assigned…" / 1773 "Trip has started."):
/// map + bottom sheet with status heading, vehicle line, driver block, stats,
/// progress slider and phase-specific actions.
class HaulingActiveTripView extends StatelessWidget {
  const HaulingActiveTripView({super.key, required this.controller, required this.state});

  final HaulingBookingController controller;
  final HaulingBookingState state;

  bool get _tripStarted => [
        HaulingBookingStatus.pickedUp,
        HaulingBookingStatus.enRouteDelivery,
        HaulingBookingStatus.delivered,
      ].contains(state.activeBooking!.status);

  @override
  Widget build(BuildContext context) {
    final booking = state.activeBooking!;
    final pickupLatLng = LatLng(booking.pickupLat, booking.pickupLng);
    final dropoffLatLng = LatLng(booking.dropoffLat, booking.dropoffLng);
    final driverLoc = state.providerLocation;
    final driverLatLng = (driverLoc != null && driverLoc.hasFix)
        ? LatLng(driverLoc.lat, driverLoc.lng)
        : null;
    final canCancel = [
      HaulingBookingStatus.accepted,
      HaulingBookingStatus.enRoutePickup,
    ].contains(booking.status);

    return Scaffold(
      backgroundColor: Colors.white,
      bottomNavigationBar: const HaulingFlowBottomNav(),
      body: Stack(
        children: [
          Positioned.fill(
            child: HaulingMapWidget(
              pickupLatLng: pickupLatLng,
              dropoffLatLng: dropoffLatLng,
              driverLatLng: driverLatLng,
            ),
          ),

          Align(
            alignment: Alignment.bottomCenter,
            child: Container(
              width: double.infinity,
              constraints: BoxConstraints(
                maxHeight: MediaQuery.sizeOf(context).height * 0.72,
              ),
              decoration: const BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
              ),
              padding: const EdgeInsets.fromLTRB(20, 22, 20, 16),
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      _heading(booking),
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontWeight: FontWeight.w800,
                        fontSize: 18,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      _subtitle(booking.status),
                      style: const TextStyle(
                        color: CustomerFigmaColors.muted,
                        fontSize: 13,
                        height: 1.4,
                      ),
                    ),
                    if (state.truckSnapshot != null) ...[
                      const SizedBox(height: 10),
                      HaulingVehicleLine(truck: state.truckSnapshot!),
                    ],
                    const SizedBox(height: 16),

                    if (state.providerSnapshot != null)
                      HaulingDriverProfileBlock(provider: state.providerSnapshot!)
                    else
                      const SizedBox(height: 64),
                    const SizedBox(height: 18),

                    HaulingTripStatsRow(
                      distanceKm: booking.distanceKm,
                      fareKobo: booking.displayFareKobo > 0
                          ? booking.displayFareKobo
                          : null,
                    ),
                    const SizedBox(height: 18),

                    HaulingTripProgressSlider(
                      progress: state.tripProgress,
                      showTruck: _tripStarted,
                    ),
                    const SizedBox(height: 20),

                    if (booking.status == HaulingBookingStatus.arrivedAtPickup) ...[
                      _PickupVerificationSection(
                        controller: controller,
                        verified: state.pickupVerifiedLocally,
                        isLoading: state.isLoading,
                      ),
                      const SizedBox(height: 20),
                    ],

                    if (booking.status == HaulingBookingStatus.enRouteDelivery) ...[
                      CosmicforgeLogisticsVerificationCodeCard(
                        title: 'Delivery verification code',
                        subtitle: 'Share this code with your driver to confirm delivery.',
                        code: state.deliveryVerification?.code,
                        expiresAt: state.deliveryVerification?.expiresAt,
                        onResend: controller.resendDeliveryCode,
                        canResend: !state.isResendingCode,
                      ),
                      const SizedBox(height: 20),
                    ],

                    if (state.error != null) ...[
                      Text(
                        state.error!,
                        style: const TextStyle(color: Colors.red, fontSize: 12),
                        textAlign: TextAlign.center,
                      ),
                      const SizedBox(height: 10),
                    ],

                    if (_tripStarted)
                      _TripActionCircles(
                        onShare: () => _shareTrip(context, booking),
                        onReport: () => _report(context),
                        onSos: () => _sos(context),
                      )
                    else
                      Row(
                        children: [
                          Expanded(
                            child: HaulingPillButton(
                              label: 'Call Driver',
                              onPressed: () =>
                                  _callDriver(state.providerSnapshot?.phone),
                            ),
                          ),
                          if (canCancel) ...[
                            const SizedBox(width: 12),
                            Expanded(
                              child: HaulingPillButton(
                                label: 'Cancel Truck',
                                background: CustomerFigmaColors.primarySoft,
                                isLoading: state.isLoading,
                                onPressed: () => _confirmCancel(context),
                              ),
                            ),
                          ],
                        ],
                      ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ─── Copy ───────────────────────────────────────────────────────────────────

  String _heading(HaulageBooking booking) {
    switch (booking.status) {
      case HaulingBookingStatus.accepted:
      case HaulingBookingStatus.enRoutePickup:
        final eta = _etaToPickupMinutes(booking);
        if (eta != null) {
          return 'Driver Assigned, arriving in $eta minute${eta == 1 ? '' : 's'}';
        }
        return 'Driver Assigned, arriving soon';
      case HaulingBookingStatus.arrivedAtPickup:
        return 'Driver has arrived.';
      default:
        return 'Trip has started.';
    }
  }

  String _subtitle(HaulingBookingStatus status) => switch (status) {
        HaulingBookingStatus.accepted ||
        HaulingBookingStatus.enRoutePickup =>
          'The driver has accepted your request, they are on their way to you.',
        HaulingBookingStatus.arrivedAtPickup =>
          'Your driver is at the pickup location.',
        _ => 'Your trip has started, enjoy your ride.',
      };

  /// Minutes for the driver to reach pickup, from the live location fix.
  int? _etaToPickupMinutes(HaulageBooking booking) {
    final loc = state.providerLocation;
    if (loc == null || !loc.hasFix) return null;
    final km = haulingHaversineKm(
        loc.lat, loc.lng, booking.pickupLat, booking.pickupLng);
    return haulingEtaMinutes(km);
  }

  // ─── Actions ────────────────────────────────────────────────────────────────

  Future<void> _callDriver(String? phone) async {
    if (phone == null || phone.isEmpty) return;
    final uri = Uri.parse('tel:$phone');
    if (await canLaunchUrl(uri)) await launchUrl(uri);
  }

  Future<void> _shareTrip(BuildContext context, HaulageBooking booking) async {
    final driver = state.providerSnapshot?.displayName ?? 'a KarryGo driver';
    final plate = state.truckSnapshot?.plateNumber ?? '';
    await Clipboard.setData(ClipboardData(
      text: 'I am on a KarryGo truck trip with $driver'
          '${plate.isNotEmpty ? ' ($plate)' : ''}.\n'
          'From: ${booking.pickupAddress}\n'
          'To: ${booking.dropoffAddress}\n'
          'Trip ref: ${booking.id}',
    ));
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Trip details copied — share them with a contact.')),
      );
    }
  }

  void _report(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Reporting is coming soon. For urgent issues use SOS.')),
    );
  }

  void _sos(BuildContext context) {
    showModalBottomSheet(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => Padding(
        padding: const EdgeInsets.fromLTRB(24, 20, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Emergency SOS',
              style: TextStyle(color: CustomerFigmaColors.text, fontSize: 16, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Call the national emergency line (112)?',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FigmaPrimaryButton(
              label: 'Call 112',
              onPressed: () async {
                Navigator.of(context).pop();
                final uri = Uri.parse('tel:112');
                if (await canLaunchUrl(uri)) await launchUrl(uri);
              },
            ),
            const SizedBox(height: 10),
            FigmaSecondaryButton(
              label: 'Not now',
              onPressed: () => Navigator.of(context).pop(),
            ),
          ],
        ),
      ),
    );
  }

  void _confirmCancel(BuildContext context) {
    showModalBottomSheet(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => Padding(
        padding: const EdgeInsets.fromLTRB(24, 20, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Cancel booking?',
              style: TextStyle(color: CustomerFigmaColors.text, fontSize: 16, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 8),
            const Text(
              'Cancelling after assignment may incur a fee.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FigmaPrimaryButton(
              label: 'Yes, cancel',
              onPressed: () {
                Navigator.of(context).pop();
                controller.cancelBooking(reason: 'Customer cancelled active booking');
              },
            ),
            const SizedBox(height: 10),
            FigmaSecondaryButton(
              label: 'Keep booking',
              onPressed: () => Navigator.of(context).pop(),
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Pickup handover verification (customer verifies the driver) ─────────────

class _PickupVerificationSection extends StatelessWidget {
  const _PickupVerificationSection({
    required this.controller,
    required this.verified,
    required this.isLoading,
  });

  final HaulingBookingController controller;
  final bool verified;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    if (verified) {
      return Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: CustomerFigmaColors.primaryTint,
          borderRadius: BorderRadius.circular(12),
        ),
        child: const Row(
          children: [
            Icon(Icons.check_circle, color: CustomerFigmaColors.primary),
            SizedBox(width: 10),
            Expanded(
              child: Text(
                'Driver verified. Waiting for them to start the trip.',
                style: TextStyle(color: CustomerFigmaColors.text, fontSize: 13),
              ),
            ),
          ],
        ),
      );
    }
    return HaulingPillButton(
      label: 'Verify your driver',
      isLoading: isLoading,
      onPressed: () => _openVerifySheet(context),
    );
  }

  Future<void> _openVerifySheet(BuildContext context) async {
    final code = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => const CosmicforgeLogisticsCodeEntrySheet(
        title: 'Verify your driver',
        subtitle: 'Scan the QR code on your driver\'s screen, or enter the code they read out.',
      ),
    );
    if (code == null || code.isEmpty) return;
    final ok = await controller.verifyDriver(code);
    if (!ok && context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(controller.state.error ?? 'Incorrect code. Please try again.')),
      );
    }
  }
}

// ─── Share Trip / Report / SOS circles (Figma 1773) ───────────────────────────

class _TripActionCircles extends StatelessWidget {
  const _TripActionCircles({
    required this.onShare,
    required this.onReport,
    required this.onSos,
  });

  final VoidCallback onShare;
  final VoidCallback onReport;
  final VoidCallback onSos;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceAround,
      children: [
        _ActionCircle(
          icon: Icons.shortcut,
          iconColor: CustomerFigmaColors.text,
          label: 'Share Trip',
          onTap: onShare,
        ),
        _ActionCircle(
          icon: Icons.warning_amber_rounded,
          iconColor: CustomerFigmaColors.text,
          label: 'Report',
          onTap: onReport,
        ),
        _ActionCircle(
          icon: Icons.power_settings_new,
          iconColor: const Color(0xFFA81717),
          label: 'SOS',
          onTap: onSos,
        ),
      ],
    );
  }
}

class _ActionCircle extends StatelessWidget {
  const _ActionCircle({
    required this.icon,
    required this.iconColor,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final Color iconColor;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 60,
            height: 60,
            decoration: const BoxDecoration(
              color: kHaulingCircleGrey,
              shape: BoxShape.circle,
            ),
            child: Icon(icon, size: 26, color: iconColor),
          ),
          const SizedBox(height: 8),
          Text(
            label,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 13,
              fontWeight: FontWeight.w500,
            ),
          ),
        ],
      ),
    );
  }
}
