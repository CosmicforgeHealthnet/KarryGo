import 'package:flutter/material.dart';

import '../../../auth/models/provider_auth_models.dart';
import 'provider_app_colors.dart';

/// Request card shown on the home bottom sheet and requests list (Figma 2034 / 2049).
/// Avatar + customer name, km/min/fare metrics, Pick-up → Drop off route,
/// "Reject Request" + "Accept Request" buttons.
class ProviderRequestCard extends StatelessWidget {
  const ProviderRequestCard({
    super.key,
    required this.booking,
    required this.onReject,
    required this.onAccept,
    this.isLoading = false,
  });

  final ProviderBooking booking;
  final VoidCallback onReject;
  final VoidCallback onAccept;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    final distanceKm = booking.distanceKm;
    final estMinutes = distanceKm != null ? (distanceKm / 30 * 60).round() : null;

    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
        boxShadow: const [BoxShadow(color: Color(0x0F000000), blurRadius: 14, offset: Offset(0, 4))],
      ),
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // ─── Header: avatar + name + metrics ───────────────────
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ProviderAvatar(name: booking.displayName, size: 56),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      booking.displayName,
                      style: const TextStyle(
                        color: kProviderText,
                        fontWeight: FontWeight.w800,
                        fontSize: 24,
                        height: 1.1,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 6),
                    BookingMetricsRow(
                      distanceKm: distanceKm,
                      minutes: estMinutes,
                      fareNaira: booking.fareEstimateNaira,
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),

          // ─── Route ─────────────────────────────────────────────
          BookingRoute(
            pickupAddress: booking.pickupAddress,
            dropoffAddress: booking.dropoffAddress,
          ),
          const SizedBox(height: 16),

          // ─── Action buttons ────────────────────────────────────
          Row(
            children: [
              Expanded(
                child: RejectRequestButton(onPressed: isLoading ? null : onReject),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: AcceptRequestButton(onPressed: isLoading ? null : onAccept),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

// ─── Shared building blocks (reused on detail screen) ─────────────────────────

/// Circular avatar with initials fallback.
class ProviderAvatar extends StatelessWidget {
  const ProviderAvatar({super.key, required this.name, this.size = 48, this.photoUrl});

  final String name;
  final double size;
  final String? photoUrl;

  @override
  Widget build(BuildContext context) {
    if (photoUrl != null && photoUrl!.isNotEmpty) {
      return ClipOval(
        child: Image.network(photoUrl!, width: size, height: size, fit: BoxFit.cover),
      );
    }
    final initials = _initials(name);
    return Container(
      width: size,
      height: size,
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [kProviderGreen, Color(0xFF1E8F3E)],
        ),
        shape: BoxShape.circle,
      ),
      alignment: Alignment.center,
      child: Text(
        initials,
        style: TextStyle(
          color: Colors.white,
          fontWeight: FontWeight.w800,
          fontSize: size * 0.36,
        ),
      ),
    );
  }

  String _initials(String name) {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
    if (parts.isEmpty) return '?';
    if (parts.length == 1) return parts.first.substring(0, 1).toUpperCase();
    return (parts.first.substring(0, 1) + parts.last.substring(0, 1)).toUpperCase();
  }
}

/// "21 km · 8 min · ₦7.00" metric row.
class BookingMetricsRow extends StatelessWidget {
  const BookingMetricsRow({
    super.key,
    required this.distanceKm,
    required this.minutes,
    required this.fareNaira,
  });

  final double? distanceKm;
  final int? minutes;
  final double fareNaira;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        if (distanceKm != null) ...[
          _metric(Icons.my_location_rounded, '${distanceKm!.toStringAsFixed(0)} km'),
          const SizedBox(width: 14),
        ],
        if (minutes != null) ...[
          _metric(Icons.access_time_rounded, '$minutes min'),
          const SizedBox(width: 14),
        ],
        _metric(Icons.attach_money_rounded, '₦${fareNaira.toStringAsFixed(2)}', isFare: true),
      ],
    );
  }

  Widget _metric(IconData icon, String label, {bool isFare = false}) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 14, color: kProviderMuted),
        const SizedBox(width: 4),
        Text(
          label,
          style: TextStyle(
            color: isFare ? kProviderGreen : kProviderText,
            fontSize: 13,
            fontWeight: FontWeight.w700,
          ),
        ),
      ],
    );
  }
}

/// Pick-up → Drop off route with green connector.
class BookingRoute extends StatelessWidget {
  const BookingRoute({
    super.key,
    required this.pickupAddress,
    required this.dropoffAddress,
  });

  final String pickupAddress;
  final String dropoffAddress;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 4),
          child: Column(
            children: [
              Container(
                width: 14,
                height: 14,
                decoration: BoxDecoration(
                  color: kProviderGreen,
                  shape: BoxShape.circle,
                  border: Border.all(color: kProviderGreenPale, width: 3),
                ),
              ),
              Container(width: 2, height: 30, color: kProviderGreen),
              Container(
                width: 14,
                height: 14,
                decoration: BoxDecoration(
                  color: Colors.white,
                  shape: BoxShape.circle,
                  border: Border.all(color: kProviderGreen, width: 2.5),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Pick-up', style: TextStyle(color: kProviderMuted, fontSize: 12)),
              const SizedBox(height: 2),
              Text(
                pickupAddress,
                style: const TextStyle(
                  color: kProviderText,
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
              const SizedBox(height: 12),
              const Text('Drop off (optional)', style: TextStyle(color: kProviderMuted, fontSize: 12)),
              const SizedBox(height: 2),
              Text(
                dropoffAddress,
                style: const TextStyle(
                  color: kProviderText,
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ],
    );
  }
}

/// Pink "Reject Request" button.
class RejectRequestButton extends StatelessWidget {
  const RejectRequestButton({super.key, required this.onPressed, this.label = 'Reject Request'});

  final VoidCallback? onPressed;
  final String label;

  @override
  Widget build(BuildContext context) {
    return FilledButton(
      onPressed: onPressed,
      style: FilledButton.styleFrom(
        backgroundColor: kProviderRejectBg,
        foregroundColor: kProviderRejectText,
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        padding: const EdgeInsets.symmetric(vertical: 14),
      ),
      child: Text(label, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14)),
    );
  }
}

/// Green "Accept Request" button.
class AcceptRequestButton extends StatelessWidget {
  const AcceptRequestButton({super.key, required this.onPressed, this.label = 'Accept Request'});

  final VoidCallback? onPressed;
  final String label;

  @override
  Widget build(BuildContext context) {
    return FilledButton(
      onPressed: onPressed,
      style: FilledButton.styleFrom(
        backgroundColor: kProviderGreen,
        foregroundColor: Colors.white,
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        padding: const EdgeInsets.symmetric(vertical: 14),
      ),
      child: Text(label, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14)),
    );
  }
}
