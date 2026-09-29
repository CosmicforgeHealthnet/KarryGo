import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart' hide Path;

import '../../../core/theme/theme_extensions.dart';
import '../data/trip_model.dart';
import 'proof_of_delivery_screen.dart';
import 'customer_rating_screen.dart';

class TripDetailScreen extends StatelessWidget {
  const TripDetailScreen({super.key, required this.trip});
  final TripModel trip;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final status = trip.statusCode;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    _buildHeroCard(context),
                    const SizedBox(height: 16),
                    if (status.isActive) _buildActiveBlock(context),
                    if (status.isCompleted) _buildCompletedBlock(context),
                    if (status.isCancelled) _buildCancelledBlock(context),
                    if (status.isFailed) _buildFailedBlock(context),
                    const SizedBox(height: 16),
                    _buildRouteCard(context),
                    const SizedBox(height: 16),
                    _buildTimeline(context),
                    const SizedBox(height: 16),
                    _buildRecipientCard(context),
                    if (trip.notes != null && trip.notes!.isNotEmpty) ...[
                      const SizedBox(height: 16),
                      _buildNotesCard(context),
                    ],
                    const SizedBox(height: 24),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            _titleFor(trip.statusCode),
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          Align(
            alignment: Alignment.centerRight,
            child: GestureDetector(
              onTap: () {},
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.more_horiz_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  static String _titleFor(TripStatusCode status) => switch (status) {
        TripStatusCode.completed => 'Completed delivery',
        TripStatusCode.cancelled => 'Cancelled delivery',
        TripStatusCode.failed => 'Failed delivery',
        _ => 'Active delivery',
      };

  // ── Hero card ─────────────────────────────────────────────────────

  Widget _buildHeroCard(BuildContext context) {
    final colors = context.palette;
    final status = trip.statusCode;

    final (Color fg, Color bg) = switch (status) {
      TripStatusCode.completed => (colors.success, colors.successSoft),
      TripStatusCode.inProgress => (colors.brandTeal600, colors.teal10),
      TripStatusCode.enRoutePickup => (colors.brandTeal600, colors.teal10),
      TripStatusCode.assigned => (colors.brandNavy900, colors.navy10),
      TripStatusCode.arrivedPickup => (colors.warning, colors.warningSoft),
      TripStatusCode.proofSubmitted => (colors.brandTeal600, colors.teal10),
      TripStatusCode.cancelled => (colors.danger, colors.dangerSoft),
      TripStatusCode.failed => (colors.danger, colors.dangerSoft),
    };

    final icon = switch (status) {
      TripStatusCode.completed => Icons.check_circle_rounded,
      TripStatusCode.inProgress => Icons.local_shipping_rounded,
      TripStatusCode.enRoutePickup => Icons.navigation_rounded, 
      TripStatusCode.assigned => Icons.assignment_turned_in_outlined,
      TripStatusCode.arrivedPickup => Icons.pin_drop_outlined,
      TripStatusCode.proofSubmitted => Icons.photo_camera_outlined,
      TripStatusCode.cancelled => Icons.cancel_outlined,
      TripStatusCode.failed => Icons.error_outline_rounded,
    };

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 10,
                  vertical: 6,
                ),
                decoration: BoxDecoration(
                  color: bg,
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(icon, size: 13, color: fg),
                    const SizedBox(width: 5),
                    Text(
                      status.shortLabel,
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: fg,
                      ),
                    ),
                  ],
                ),
              ),
              const Spacer(),
              Text(
                '#${trip.bookingId}',
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  color: colors.textMuted,
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              Text(
                trip.earningsDisplay,
                style: TextStyle(
                  fontSize: 26,
                  fontWeight: FontWeight.w800,
                  color: colors.text,
                  letterSpacing: -0.8,
                  height: 1.0,
                ),
              ),
              if (trip.itemSummary != null) ...[
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    trip.itemSummary!,
                    style: TextStyle(
                      fontSize: 13,
                      color: colors.textMuted,
                      height: 1.3,
                    ),
                  ),
                ),
              ],
            ],
          ),
          const SizedBox(height: 6),
          Text(
            _formatLongDate(
              trip.completedAt ??
                  trip.cancelledAt ??
                  trip.failedAt ??
                  trip.createdAt,
            ),
            style: TextStyle(
              fontSize: 12.5,
              color: colors.textMuted,
            ),
          ),
        ],
      ),
    );
  }

  // ── Active block (with live map) ──────────────────────────────────

  Widget _buildActiveBlock(BuildContext context) {
    final colors = context.palette;

    if (!trip.hasMap) {
      // No coords yet — fall back to the plain banner.
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: colors.brandTeal500,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Row(
          children: [
            const Icon(
              Icons.my_location_rounded,
              size: 20,
              color: Colors.white,
            ),
            const SizedBox(width: 10),
            const Expanded(
              child: Text(
                'Live tracking active',
                style: TextStyle(
                  fontSize: 14.5,
                  fontWeight: FontWeight.w700,
                  color: Colors.white,
                ),
              ),
            ),
            const Icon(
              Icons.arrow_forward_rounded,
              size: 18,
              color: Colors.white,
            ),
          ],
        ),
      );
    }

    final pickup = LatLng(trip.pickupLat!, trip.pickupLng!);
    final dropoff = LatLng(trip.dropoffLat!, trip.dropoffLng!);
    final center = LatLng(
      (pickup.latitude + dropoff.latitude) / 2,
      (pickup.longitude + dropoff.longitude) / 2,
    );

    return ClipRRect(
      borderRadius: BorderRadius.circular(16),
      child: SizedBox(
        height: 240,
        child: Stack(
          children: [
            FlutterMap(
              options: MapOptions(
                initialCenter: center,
                initialZoom: 12.5,
                interactionOptions: const InteractionOptions(
                  flags: InteractiveFlag.none,
                ),
              ),
              children: [
                TileLayer(
                  urlTemplate:
                      'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
                  userAgentPackageName:
                      'com.cosmicforge.logistics.dispatchprovider',
                ),
                PolylineLayer(
                  polylines: [
                    Polyline(
                      points: [pickup, dropoff],
                      color: colors.brandNavy900,
                      strokeWidth: 3.5,
                      pattern:
                          StrokePattern.dashed(segments: const [10, 6]),
                    ),
                  ],
                ),
                MarkerLayer(
                  markers: [
                    Marker(
                      point: pickup,
                      width: 32,
                      height: 32,
                      child: _DotMarker(
                        color: colors.brandNavy900,
                        icon: Icons.radio_button_checked,
                      ),
                    ),
                    Marker(
                      point: dropoff,
                      width: 32,
                      height: 32,
                      child: _DotMarker(
                        color: colors.brandTeal500,
                        icon: Icons.place_rounded,
                      ),
                    ),
                  ],
                ),
              ],
            ),
            // "Live" pill overlay
            Positioned(
              top: 12,
              left: 12,
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 10,
                  vertical: 6,
                ),
                decoration: BoxDecoration(
                  color: colors.surface,
                  borderRadius: BorderRadius.circular(999),
                  boxShadow: colors.cardShadow,
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Container(
                      width: 8,
                      height: 8,
                      decoration: BoxDecoration(
                        color: colors.success,
                        shape: BoxShape.circle,
                      ),
                    ),
                    const SizedBox(width: 6),
                    Text(
                      'Live',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: colors.text,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Completed block ───────────────────────────────────────────────

  Widget _buildCompletedBlock(BuildContext context) {
    return Column(
      children: [
        if (trip.customerRating != null) ...[
          _buildRatingRow(context),
          const SizedBox(height: 12),
        ],
        if (trip.hasProof) _buildProofRow(context),
      ],
    );
  }

  Widget _buildRatingRow(BuildContext context) {
    final colors = context.palette;
    final rating = trip.customerRating ?? 0;
    return GestureDetector(
      onTap: () => Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => CustomerRatingScreen(trip: trip),
        ),
      ),
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: colors.divider),
        ),
        child: Row(
          children: [
            Container(
              width: 40,
              height: 40,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: colors.warningSoft,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Icon(
                Icons.star_rounded,
                size: 22,
                color: colors.warning,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Customer rating',
                    style: TextStyle(
                      fontSize: 12.5,
                      fontWeight: FontWeight.w600,
                      color: colors.textMuted,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Row(
                    children: [
                      Text(
                        rating.toStringAsFixed(1),
                        style: TextStyle(
                          fontSize: 17,
                          fontWeight: FontWeight.w800,
                          color: colors.text,
                          letterSpacing: -0.3,
                        ),
                      ),
                      const SizedBox(width: 6),
                      ...List.generate(
                        5,
                        (i) => Icon(
                          i < rating.round()
                              ? Icons.star_rounded
                              : Icons.star_outline_rounded,
                          size: 14,
                          color: colors.warning,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right_rounded,
              size: 22,
              color: colors.navy40,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildProofRow(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: () => Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => ProofOfDeliveryScreen(trip: trip),
        ),
      ),
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: colors.divider),
        ),
        child: Row(
          children: [
            Container(
              width: 40,
              height: 40,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: colors.teal10,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Icon(
                Icons.photo_camera_outlined,
                size: 20,
                color: colors.brandTeal600,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Proof of delivery',
                    style: TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w700,
                      color: colors.text,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    _proofSummary(trip),
                    style: TextStyle(
                      fontSize: 12,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right_rounded,
              size: 22,
              color: colors.navy40,
            ),
          ],
        ),
      ),
    );
  }

  static String _proofSummary(TripModel trip) {
    final parts = <String>[];
    if (trip.proofPhotos.isNotEmpty) {
      parts.add(
        '${trip.proofPhotos.length} photo${trip.proofPhotos.length == 1 ? '' : 's'}',
      );
    }
    if (trip.proofSignatureUrl != null) parts.add('Signature');
    if (trip.proofReceiptUrl != null) parts.add('Receipt');
    return parts.isEmpty ? 'View proof' : parts.join(' · ');
  }

  // ── Cancelled block ───────────────────────────────────────────────

  Widget _buildCancelledBlock(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.dangerSoft,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.danger.withValues(alpha: 0.35)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                Icons.cancel_outlined,
                size: 18,
                color: colors.danger,
              ),
              const SizedBox(width: 6),
              Text(
                'Cancelled',
                style: TextStyle(
                  fontSize: 13.5,
                  fontWeight: FontWeight.w800,
                  color: colors.danger,
                ),
              ),
            ],
          ),
          if (trip.cancelReason != null) ...[
            const SizedBox(height: 8),
            Text(
              trip.cancelReason!,
              style: TextStyle(
                fontSize: 13,
                color: colors.danger,
                height: 1.4,
              ),
            ),
          ],
          const SizedBox(height: 12),
          Row(
            children: [
              Text(
                'Refund status:',
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w600,
                  color: colors.textMuted,
                ),
              ),
              const SizedBox(width: 6),
              Text(
                trip.refundStatus ?? 'N/A',
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  // ── Failed block ──────────────────────────────────────────────────

  Widget _buildFailedBlock(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.dangerSoft,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.danger.withValues(alpha: 0.35)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                Icons.error_outline_rounded,
                size: 18,
                color: colors.danger,
              ),
              const SizedBox(width: 6),
              Text(
                'Failed delivery attempt',
                style: TextStyle(
                  fontSize: 13.5,
                  fontWeight: FontWeight.w800,
                  color: colors.danger,
                ),
              ),
            ],
          ),
          if (trip.failReason != null) ...[
            const SizedBox(height: 8),
            Text(
              trip.failReason!,
              style: TextStyle(
                fontSize: 13,
                color: colors.danger,
                height: 1.4,
              ),
            ),
          ],
          if (trip.failedAt != null) ...[
            const SizedBox(height: 10),
            Row(
              children: [
                Text(
                  'Attempted:',
                  style: TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w600,
                    color: colors.textMuted,
                  ),
                ),
                const SizedBox(width: 6),
                Text(
                  _formatLongDate(trip.failedAt!),
                  style: TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }

  // ── Route card ────────────────────────────────────────────────────

  Widget _buildRouteCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        children: [
          _StopRow(
            label: 'Pick-up',
            address: trip.pickupAddress,
            color: colors.brandNavy900,
            icon: Icons.radio_button_checked,
          ),
          Padding(
            padding: const EdgeInsets.only(left: 9, top: 4, bottom: 4),
            child: Container(width: 2, height: 20, color: colors.divider),
          ),
          _StopRow(
            label: 'Drop-off',
            address: trip.dropoffAddress,
            color: colors.brandTeal500,
            icon: Icons.place_rounded,
          ),
        ],
      ),
    );
  }

  // ── Timeline ──────────────────────────────────────────────────────

  Widget _buildTimeline(BuildContext context) {
    final colors = context.palette;
    final status = trip.statusCode;

    if (status.isCancelled) {
      return const SizedBox.shrink();
    }

    final steps = <(String, String, bool)>[
      ('Assigned', _timeOrEmpty(trip.createdAt), true),
      (
        'At Pickup',
        trip.startedAt != null ? _timeOrEmpty(trip.startedAt!) : '',
        status != TripStatusCode.assigned,
      ),
      (
        'In progress',
        trip.startedAt != null ? _timeOrEmpty(trip.startedAt!) : '',
        status == TripStatusCode.inProgress ||
            status == TripStatusCode.proofSubmitted ||
            status == TripStatusCode.completed,
      ),
      (
        'Proof submitted',
        '',
        status == TripStatusCode.proofSubmitted ||
            status == TripStatusCode.completed,
      ),
      (
        status == TripStatusCode.failed ? 'Failed' : 'Completed',
        trip.completedAt != null
            ? _timeOrEmpty(trip.completedAt!)
            : trip.failedAt != null
                ? _timeOrEmpty(trip.failedAt!)
                : '',
        status == TripStatusCode.completed || status == TripStatusCode.failed,
      ),
    ];

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Trip status',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w800,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 14),
          for (int i = 0; i < steps.length; i++) ...[
            _TimelineRow(
              label: steps[i].$1,
              time: steps[i].$2,
              done: steps[i].$3,
              isLast: i == steps.length - 1,
              isCancelled: status.isFailed && i == steps.length - 1,
            ),
          ],
        ],
      ),
    );
  }

  // ── Recipient card ────────────────────────────────────────────────

  Widget _buildRecipientCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Recipient',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w800,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Container(
                width: 44,
                height: 44,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: colors.navy10,
                  shape: BoxShape.circle,
                ),
                child: Icon(
                  Icons.person_rounded,
                  size: 22,
                  color: colors.textMuted,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      trip.receiverName.isNotEmpty
                          ? trip.receiverName
                          : 'Not available',
                      style: TextStyle(
                        fontSize: 14.5,
                        fontWeight: FontWeight.w700,
                        color: colors.text,
                      ),
                    ),
                    if (trip.receiverPhone.isNotEmpty) ...[
                      const SizedBox(height: 2),
                      Text(
                        trip.receiverPhone,
                        style: TextStyle(
                          fontSize: 12.5,
                          color: colors.textMuted,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  // ── Notes card ────────────────────────────────────────────────────

  Widget _buildNotesCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Notes',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w800,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            trip.notes!,
            style: TextStyle(
              fontSize: 13.5,
              color: colors.textMuted,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }

  // ── Helpers ───────────────────────────────────────────────────────

  static String _timeOrEmpty(DateTime dt) {
    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    return '$h:$m';
  }

  static String _formatLongDate(DateTime dt) {
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    return '${dt.day.toString().padLeft(2, '0')} ${months[dt.month - 1]} ${dt.year}, $h:$m';
  }
}

// ── Small widgets ───────────────────────────────────────────────────

class _DotMarker extends StatelessWidget {
  const _DotMarker({required this.color, required this.icon});
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Container(
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        border: Border.all(color: Colors.white, width: 2.5),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.18),
            blurRadius: 6,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Icon(icon, size: 16, color: Colors.white),
    );
  }
}

class _StopRow extends StatelessWidget {
  const _StopRow({
    required this.label,
    required this.address,
    required this.color,
    required this.icon,
  });

  final String label;
  final String address;
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 1),
          child: Icon(icon, size: 18, color: color),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: colors.textMuted,
                  letterSpacing: 0.3,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                address,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                  height: 1.3,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _TimelineRow extends StatelessWidget {
  const _TimelineRow({
    required this.label,
    required this.time,
    required this.done,
    required this.isLast,
    this.isCancelled = false,
  });

  final String label;
  final String time;
  final bool done;
  final bool isLast;
  final bool isCancelled;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final dotColor = isCancelled
        ? colors.danger
        : done
            ? colors.success
            : colors.navy20;
    final textColor = done ? colors.text : colors.textMuted;

    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 20,
            child: Column(
              children: [
                Container(
                  width: 16,
                  height: 16,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: done || isCancelled ? dotColor : Colors.transparent,
                    shape: BoxShape.circle,
                    border: done || isCancelled
                        ? null
                        : Border.all(color: colors.navy20, width: 2),
                  ),
                  child: done && !isCancelled
                      ? const Icon(
                          Icons.check_rounded,
                          size: 10,
                          color: Colors.white,
                        )
                      : isCancelled
                          ? const Icon(
                              Icons.close_rounded,
                              size: 10,
                              color: Colors.white,
                            )
                          : null,
                ),
                if (!isLast)
                  Expanded(
                    child: Center(
                      child: Container(
                        width: 2,
                        color: done ? colors.success : colors.navy20,
                      ),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Padding(
              padding: EdgeInsets.only(bottom: isLast ? 0 : 16),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      label,
                      style: TextStyle(
                        fontSize: 13.5,
                        fontWeight: FontWeight.w700,
                        color: textColor,
                        letterSpacing: -0.1,
                      ),
                    ),
                  ),
                  if (time.isNotEmpty)
                    Text(
                      time,
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w500,
                        color: colors.textMuted,
                      ),
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}