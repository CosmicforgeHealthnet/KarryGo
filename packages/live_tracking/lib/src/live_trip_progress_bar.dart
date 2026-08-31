import 'package:flutter/material.dart';

/// The pickup-to-destination progress bar shown on a live trip.
///
/// Generalises the truck-only slider the customer app had: the vehicle sprite
/// is supplied by the caller, so hauling shows a truck and dispatch a bike,
/// and the fill animates rather than jumping between poll results.
class LiveTripProgressBar extends StatelessWidget {
  const LiveTripProgressBar({
    super.key,
    required this.progress,
    this.vehicleIcon,
    this.showVehicle = true,
    this.startLabel = 'Pickup',
    this.endLabel = 'Destination',
    this.fillColor,
    this.trackColor,
    this.labelStyle,
    this.height = 8,
    this.animationDuration = const Duration(milliseconds: 600),
  });

  /// Progress along the active leg, 0 to 1.
  final double progress;

  /// The sprite that rides the bar. Omit for a plain bar.
  final Widget? vehicleIcon;

  /// Whether to draw the sprite. Hidden before the trip starts moving.
  final bool showVehicle;

  final String startLabel;
  final String endLabel;

  final Color? fillColor;
  final Color? trackColor;
  final TextStyle? labelStyle;

  final double height;
  final Duration animationDuration;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final fill = fillColor ?? theme.colorScheme.primary;
    final track = trackColor ?? theme.colorScheme.surfaceContainerHighest;
    final labels = labelStyle ??
        theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.onSurfaceVariant);

    final clamped = progress.clamp(0.0, 1.0);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        LayoutBuilder(
          builder: (context, constraints) {
            final width = constraints.maxWidth;
            const spriteSize = 34.0;
            // Keep the sprite fully on the bar at both ends.
            final spriteLeft =
                (width * clamped - spriteSize / 2).clamp(0.0, width - spriteSize);

            return SizedBox(
              height: vehicleIcon != null && showVehicle ? spriteSize : height,
              child: Stack(
                alignment: Alignment.centerLeft,
                clipBehavior: Clip.none,
                children: [
                  Container(
                    height: height,
                    decoration: BoxDecoration(
                      color: track,
                      borderRadius: BorderRadius.circular(height),
                    ),
                  ),
                  AnimatedContainer(
                    duration: animationDuration,
                    curve: Curves.easeOut,
                    height: height,
                    width: width * clamped,
                    decoration: BoxDecoration(
                      color: fill,
                      borderRadius: BorderRadius.circular(height),
                    ),
                  ),
                  if (vehicleIcon != null && showVehicle)
                    AnimatedPositioned(
                      duration: animationDuration,
                      curve: Curves.easeOut,
                      left: spriteLeft,
                      width: spriteSize,
                      height: spriteSize,
                      child: Center(child: vehicleIcon),
                    ),
                ],
              ),
            );
          },
        ),
        const SizedBox(height: 8),
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(startLabel, style: labels),
            Text(endLabel, style: labels),
          ],
        ),
      ],
    );
  }
}
