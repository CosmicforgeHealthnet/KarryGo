/// Live vehicle tracking for Cosmicforge Logistics apps.
///
/// The core here is deliberately map-engine agnostic: it computes positions,
/// bearings and trip progress, and knows nothing about how they are drawn. The
/// customer and truck-provider apps bind it to `google_maps_flutter` and the
/// dispatch rider app to `flutter_map`, via the sibling adapter packages.
///
/// Typical use on a live trip:
///
/// ```dart
/// final controller = LiveTrackController(vsync: this)
///   ..setRoute(TrackGeo.decodePolyline(booking.routePickupPolyline));
/// final source = CompositeTrackSource(
///   socket: (onUnavailable) => WebSocketTrackSource(
///     url: trackUrl, onUnavailable: onUnavailable),
///   poll: (interval) => PollingTrackSource(fetch: fetchLocation, interval: interval),
/// );
/// await source.start(controller.ingest);
/// ```
library;

export 'src/live_track_controller.dart';
export 'src/live_track_source.dart';
export 'src/live_trip_progress_bar.dart';
export 'src/track_geo.dart';
export 'src/track_point.dart';
export 'src/trip_progress.dart';
