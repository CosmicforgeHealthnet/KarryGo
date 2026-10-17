import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';
import '../../core/theme/theme_extensions.dart';
import '../../features/home/ui/widgets/route_geometry.dart';
import 'journey_icon.dart';

class DeliveryRoutePreview extends StatelessWidget {
  const DeliveryRoutePreview({
    super.key,
    required this.pickupLat,
    required this.pickupLng,
    required this.dropoffLat,
    required this.dropoffLng,
    this.polyline,
  });
  final double pickupLat, pickupLng, dropoffLat, dropoffLng;
  final String? polyline;
  @override
  Widget build(BuildContext context) {
    final pickup = LatLng(pickupLat, pickupLng),
        dropoff = LatLng(dropoffLat, dropoffLng);
    final route = decodeRoutePolyline(polyline);
    return ClipRRect(
      borderRadius: BorderRadius.circular(24),
      child: SizedBox(
        height: 210,
        child: FlutterMap(
          options: MapOptions(
            initialCameraFit: CameraFit.coordinates(
              coordinates: [pickup, dropoff],
              padding: const EdgeInsets.all(40),
              maxZoom: 15,
            ),
            backgroundColor: context.palette.surfaceElevated,
            interactionOptions: const InteractionOptions(
              flags: InteractiveFlag.none,
            ),
          ),
          children: [
            TileLayer(
              urlTemplate: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
              userAgentPackageName:
                  'com.cosmicforge.logistics.dispatchprovider',
              tileBuilder: context.isDark ? darkModeTileBuilder : null,
            ),
            if (route.isNotEmpty)
              PolylineLayer(
                polylines: [
                  Polyline(
                    points: route,
                    color: context.palette.brandTeal600,
                    strokeWidth: 4,
                  ),
                ],
              ),
            MarkerLayer(
              markers: [
                for (final entry in [
                  (pickup, JourneySymbol.parcel),
                  (dropoff, JourneySymbol.route),
                ])
                  Marker(
                    point: entry.$1,
                    width: 40,
                    height: 40,
                    child: Container(
                      decoration: BoxDecoration(
                        color: context.palette.surface,
                        borderRadius: BorderRadius.circular(13),
                        border: Border.all(color: context.palette.brandTeal600),
                      ),
                      child: Center(child: JourneyIcon(entry.$2, size: 24)),
                    ),
                  ),
              ],
            ),
            const RichAttributionWidget(
              attributions: [
                TextSourceAttribution('OpenStreetMap contributors'),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
