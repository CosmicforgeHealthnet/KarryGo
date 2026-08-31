import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import 'lat_lng_bridge.dart';

/// Builds the moving vehicle marker.
///
/// Three details are what make it read as a vehicle rather than a dropped pin:
/// it is `flat`, so it lies on the map and rotates with it; it is anchored at
/// its centre, so rotation spins in place instead of swinging around a pin
/// tip; and its rotation comes from the controller's smoothed bearing.
Marker buildVehicleMarker({
  required LiveTrackController controller,
  MarkerId markerId = const MarkerId('vehicle'),
  BitmapDescriptor? icon,
  double? hue,
}) {
  final position = controller.renderedPosition;
  return Marker(
    markerId: markerId,
    position: (position ?? const TrackLatLng(0, 0)).toGoogleMaps(),
    rotation: controller.renderedBearing,
    flat: true,
    anchor: const Offset(0.5, 0.5),
    // A stale vehicle is drawn faded rather than removed: disappearing reads
    // as "the driver is gone", when they have only stopped reporting.
    alpha: controller.isStale ? 0.55 : 1.0,
    zIndexInt: 2,
    icon: icon ?? BitmapDescriptor.defaultMarkerWithHue(hue ?? BitmapDescriptor.hueAzure),
  );
}

/// Builds a two-tone route: the part already covered and the part remaining.
///
/// Splitting at the vehicle gives the customer an at-a-glance sense of
/// progress that a single-colour line cannot.
Set<Polyline> buildRoutePolylines({
  required List<TrackLatLng> route,
  TrackLatLng? vehicle,
  Color travelledColor = const Color(0xFFBDBDBD),
  Color remainingColor = const Color(0xFF22A84A),
  int width = 5,
  String idPrefix = 'route',
}) {
  if (route.length < 2) return const {};

  if (vehicle == null || !vehicle.hasFix) {
    return {
      Polyline(
        polylineId: PolylineId('$idPrefix-remaining'),
        points: toGoogleMapsPoints(route),
        color: remainingColor,
        width: width,
      ),
    };
  }

  final (travelled, remaining) = TrackGeo.splitAt(vehicle, route);
  return {
    if (travelled.length >= 2)
      Polyline(
        polylineId: PolylineId('$idPrefix-travelled'),
        points: toGoogleMapsPoints(travelled),
        color: travelledColor,
        width: width,
      ),
    if (remaining.length >= 2)
      Polyline(
        polylineId: PolylineId('$idPrefix-remaining'),
        points: toGoogleMapsPoints(remaining),
        color: remainingColor,
        width: width,
      ),
  };
}

/// Builds the fixed pickup and drop-off pins.
Set<Marker> buildEndpointMarkers({
  TrackLatLng? pickup,
  TrackLatLng? dropoff,
  String pickupLabel = 'Pickup',
  String dropoffLabel = 'Drop-off',
}) {
  return {
    if (pickup != null && pickup.hasFix)
      Marker(
        markerId: const MarkerId('pickup'),
        position: pickup.toGoogleMaps(),
        icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueGreen),
        infoWindow: InfoWindow(title: pickupLabel),
      ),
    if (dropoff != null && dropoff.hasFix)
      Marker(
        markerId: const MarkerId('dropoff'),
        position: dropoff.toGoogleMaps(),
        icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueRed),
        infoWindow: InfoWindow(title: dropoffLabel),
      ),
  };
}
