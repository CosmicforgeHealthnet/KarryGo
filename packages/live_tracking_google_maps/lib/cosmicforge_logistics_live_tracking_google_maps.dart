/// Google Maps bindings for live vehicle tracking.
///
/// The tracking core is map-engine agnostic; this package is the thin layer
/// that turns its output into `google_maps_flutter` markers, polylines and
/// camera moves. The dispatch rider app uses `flutter_map` and has its own
/// equivalent, so neither app carries a plugin it does not use.
library;

export 'src/live_camera.dart';
export 'src/live_map_layers.dart';
export 'src/lat_lng_bridge.dart';
