import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../models/provider_booking.dart';

/// The green/orange dotted route summary shared by the offer card and the
/// active-trip screen.
class RouteSummary extends StatelessWidget {
  const RouteSummary({super.key, required this.booking});

  final ProviderBooking booking;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Column(
          children: [
            const Icon(
              Icons.radio_button_checked,
              size: 16,
              color: CosmicforgeLogisticsColors.success,
            ),
            SizedBox(
              height: 26,
              child: CustomPaint(
                size: const Size(2, 26),
                painter: _DottedLinePainter(),
              ),
            ),
            const Icon(
              Icons.location_on,
              size: 16,
              color: CosmicforgeLogisticsColors.warning,
            ),
          ],
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _line(booking.pickup?.address ?? 'Pick-up'),
              const SizedBox(height: 22),
              _line(booking.dropoff?.address ?? 'Destination'),
            ],
          ),
        ),
      ],
    );
  }

  Widget _line(String text) => Text(
    text,
    maxLines: 2,
    overflow: TextOverflow.ellipsis,
    style: const TextStyle(
      color: CosmicforgeLogisticsColors.text,
      fontSize: 13,
      fontWeight: FontWeight.w600,
    ),
  );
}

class _DottedLinePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = CosmicforgeLogisticsColors.primarySoft
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    const dash = 3.0;
    const gap = 3.0;
    var y = 0.0;
    while (y < size.height) {
      canvas.drawLine(
        Offset(size.width / 2, y),
        Offset(size.width / 2, (y + dash).clamp(0, size.height)),
        paint,
      );
      y += dash + gap;
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

/// Distinguishes a package job from a passenger ride at a glance — the driver
/// needs to know before accepting, because the two are worked differently.
class JobTypeBadge extends StatelessWidget {
  const JobTypeBadge({super.key, required this.isParcel});

  final bool isParcel;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: isParcel
            ? CosmicforgeLogisticsColors.warningSoft
            : CosmicforgeLogisticsColors.primaryTint,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            isParcel ? Icons.inventory_2_rounded : Icons.person_rounded,
            size: 13,
            color: isParcel
                ? CosmicforgeLogisticsColors.warning
                : CosmicforgeLogisticsColors.primary,
          ),
          const SizedBox(width: 4),
          Text(
            isParcel ? 'Package' : 'Passenger',
            style: TextStyle(
              color: isParcel
                  ? CosmicforgeLogisticsColors.warning
                  : CosmicforgeLogisticsColors.primary,
              fontSize: 11,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
    );
  }
}

/// Distance / duration / fare, the three numbers a driver decides on.
class JobStatsRow extends StatelessWidget {
  const JobStatsRow({super.key, required this.booking});

  final ProviderBooking booking;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        _stat(
          Icons.route_rounded,
          booking.distanceKm == null
              ? '--'
              : '${booking.distanceKm!.toStringAsFixed(1)} km',
        ),
        _stat(
          Icons.schedule_rounded,
          booking.durationMinutes == null
              ? '--'
              : '${booking.durationMinutes} min',
        ),
        _stat(
          Icons.payments_rounded,
          booking.displayFareKobo > 0
              ? formatNaira(booking.displayFareKobo)
              : '--',
        ),
      ],
    );
  }

  Widget _stat(IconData icon, String label) => Expanded(
    child: Row(
      children: [
        Icon(icon, size: 15, color: CosmicforgeLogisticsColors.textMuted),
        const SizedBox(width: 5),
        Flexible(
          child: Text(
            label,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: CosmicforgeLogisticsColors.text,
              fontSize: 12.5,
              fontWeight: FontWeight.w700,
            ),
          ),
        ),
      ],
    ),
  );
}

/// A full-width action button in the app's primary style.
class PrimaryActionButton extends StatelessWidget {
  const PrimaryActionButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.isLoading = false,
    this.color,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool isLoading;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 52,
      width: double.infinity,
      child: FilledButton(
        onPressed: isLoading ? null : onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: color ?? CosmicforgeLogisticsColors.primary,
          disabledBackgroundColor: CosmicforgeLogisticsColors.primarySoft,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(26),
          ),
        ),
        child: isLoading
            ? const SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: Colors.white,
                ),
              )
            : Text(
                label,
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                ),
              ),
      ),
    );
  }
}

/// Inline banner used for both refusals (compliance gates, expired offers)
/// and neutral notices.
class InlineBanner extends StatelessWidget {
  const InlineBanner({
    super.key,
    required this.message,
    this.isError = true,
    this.onDismiss,
  });

  final String message;
  final bool isError;
  final VoidCallback? onDismiss;

  @override
  Widget build(BuildContext context) {
    final fg = isError
        ? CosmicforgeLogisticsColors.danger
        : CosmicforgeLogisticsColors.info;
    final bg = isError
        ? CosmicforgeLogisticsColors.dangerSoft
        : CosmicforgeLogisticsColors.infoSoft;

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(10)),
      child: Row(
        children: [
          Icon(
            isError ? Icons.error_outline_rounded : Icons.info_outline_rounded,
            size: 18,
            color: fg,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: TextStyle(color: fg, fontSize: 12.5),
            ),
          ),
          if (onDismiss != null)
            InkWell(
              onTap: onDismiss,
              child: Icon(Icons.close_rounded, size: 16, color: fg),
            ),
        ],
      ),
    );
  }
}
