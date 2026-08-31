import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

import 'lat_lng_bridge.dart';

/// Keeps the camera on a moving vehicle without fighting the user.
///
/// The naive approach -- refit the bounds on every position update -- makes
/// the map lurch on every report and overrides any panning or zooming the
/// user has done. This instead does nothing while the vehicle stays
/// comfortably inside the viewport, and only recentres once it drifts toward
/// the edge. It also stands down entirely once the user has moved the map,
/// until they ask to follow again.
class LiveCameraController {
  LiveCameraController({
    this.edgeThreshold = 0.3,
    this.recentreCooldown = const Duration(seconds: 3),
  });

  /// How close to the viewport edge, as a fraction of the visible span, the
  /// vehicle may get before the camera recentres. 0.3 keeps it inside the
  /// middle 40% of the screen.
  final double edgeThreshold;

  /// Minimum gap between recentres, so a vehicle hovering at the threshold
  /// cannot trigger a camera animation on every frame.
  final Duration recentreCooldown;

  bool _following = true;
  DateTime? _lastRecentre;

  /// Whether the camera is currently following the vehicle.
  bool get isFollowing => _following;

  /// Stops following. Call when the user pans or zooms: the map is theirs
  /// while they are using it.
  void handleUserGesture() => _following = false;

  /// Resumes following and recentres immediately.
  Future<void> follow(GoogleMapController map, TrackLatLng vehicle) async {
    _following = true;
    _lastRecentre = null;
    await _recentre(map, vehicle);
  }

  /// Called on each position update. Moves the camera only if the vehicle is
  /// nearing the edge of the visible region.
  Future<void> onVehicleMoved(GoogleMapController map, TrackLatLng? vehicle) async {
    if (!_following || vehicle == null || !vehicle.hasFix) return;

    final last = _lastRecentre;
    if (last != null && DateTime.now().difference(last) < recentreCooldown) return;

    LatLngBounds bounds;
    try {
      bounds = await map.getVisibleRegion();
    } catch (_) {
      // The map may not be laid out yet; there is nothing to recentre onto.
      return;
    }

    if (_isComfortablyInside(bounds, vehicle)) return;
    await _recentre(map, vehicle);
  }

  Future<void> _recentre(GoogleMapController map, TrackLatLng vehicle) async {
    _lastRecentre = DateTime.now();
    await map.animateCamera(CameraUpdate.newLatLng(vehicle.toGoogleMaps()));
  }

  /// Whether the vehicle sits inside the inset region, away from the edges.
  bool _isComfortablyInside(LatLngBounds bounds, TrackLatLng vehicle) {
    final latSpan = bounds.northeast.latitude - bounds.southwest.latitude;
    final lngSpan = bounds.northeast.longitude - bounds.southwest.longitude;
    if (latSpan <= 0 || lngSpan <= 0) return true;

    final latInset = latSpan * edgeThreshold;
    final lngInset = lngSpan * edgeThreshold;

    return vehicle.lat >= bounds.southwest.latitude + latInset &&
        vehicle.lat <= bounds.northeast.latitude - latInset &&
        vehicle.lng >= bounds.southwest.longitude + lngInset &&
        vehicle.lng <= bounds.northeast.longitude - lngInset;
  }
}

/// Fits the camera to everything that matters for a trip: the vehicle, the
/// pickup and the drop-off.
///
/// Intended for one-off framing -- when a trip screen opens -- not for every
/// position update.
Future<void> fitTripBounds(
  GoogleMapController map, {
  TrackLatLng? vehicle,
  TrackLatLng? pickup,
  TrackLatLng? dropoff,
  double padding = 80,
}) async {
  final points = <TrackLatLng>[
    if (vehicle != null && vehicle.hasFix) vehicle,
    if (pickup != null && pickup.hasFix) pickup,
    if (dropoff != null && dropoff.hasFix) dropoff,
  ];
  if (points.isEmpty) return;

  if (points.length == 1) {
    await map.animateCamera(CameraUpdate.newLatLngZoom(points.first.toGoogleMaps(), 15));
    return;
  }

  var minLat = points.first.lat;
  var maxLat = points.first.lat;
  var minLng = points.first.lng;
  var maxLng = points.first.lng;
  for (final point in points) {
    if (point.lat < minLat) minLat = point.lat;
    if (point.lat > maxLat) maxLat = point.lat;
    if (point.lng < minLng) minLng = point.lng;
    if (point.lng > maxLng) maxLng = point.lng;
  }

  await map.animateCamera(
    CameraUpdate.newLatLngBounds(
      LatLngBounds(
        southwest: LatLng(minLat, minLng),
        northeast: LatLng(maxLat, maxLng),
      ),
      padding,
    ),
  );
}
