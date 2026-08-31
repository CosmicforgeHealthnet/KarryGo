import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

/// Converts between the engine-agnostic [TrackLatLng] the tracking core speaks
/// and Google Maps' own [LatLng].
extension TrackLatLngGoogleMaps on TrackLatLng {
  LatLng toGoogleMaps() => LatLng(lat, lng);
}

extension GoogleMapsLatLngTrack on LatLng {
  TrackLatLng toTrackLatLng() => TrackLatLng(latitude, longitude);
}

/// Converts a decoded route into Google Maps points.
List<LatLng> toGoogleMapsPoints(List<TrackLatLng> points) =>
    points.map((point) => point.toGoogleMaps()).toList(growable: false);
