import 'track_geo.dart';

/// One position report from a vehicle.
///
/// Mirrors the `livetrack.Fix` envelope the services publish, so a websocket
/// frame and a REST snapshot parse into the same thing.
class TrackPoint {
  const TrackPoint({
    required this.position,
    this.headingDegrees,
    this.speedMetersPerSecond,
    this.accuracyMeters,
    this.recordedAt,
  });

  final TrackLatLng position;

  /// Compass bearing the device reported, or null when it did not. Null is
  /// meaningfully different from 0: a device that reports no heading should
  /// have its bearing derived from movement instead of being pointed north.
  final double? headingDegrees;

  final double? speedMetersPerSecond;
  final double? accuracyMeters;

  /// When the device took the fix.
  final DateTime? recordedAt;

  double get lat => position.lat;
  double get lng => position.lng;

  bool get hasFix => position.hasFix;

  /// Parses a tracking websocket frame or a REST location snapshot.
  ///
  /// The services emit `entity_id` alongside the older `provider_id`, and
  /// `updated_at` as either an ISO timestamp (websocket) or unix seconds
  /// (the hauling REST snapshot), so both shapes are accepted.
  static TrackPoint? fromJson(Map<String, dynamic> json) {
    final lat = _toDouble(json['lat']);
    final lng = _toDouble(json['lng']);
    if (lat == null || lng == null) return null;

    // The REST snapshots carry an explicit availability flag; a false one
    // means "no driver fix", which must not be drawn as a position.
    final available = json['available'];
    if (available is bool && !available) return null;

    final point = TrackPoint(
      position: TrackLatLng(lat, lng),
      headingDegrees: _toDouble(json['heading']),
      speedMetersPerSecond: _toDouble(json['speed']),
      accuracyMeters: _toDouble(json['accuracy']),
      recordedAt: _toDateTime(json['updated_at'] ?? json['recorded_at']),
    );
    return point.hasFix ? point : null;
  }

  TrackPoint copyWith({TrackLatLng? position, double? headingDegrees}) {
    return TrackPoint(
      position: position ?? this.position,
      headingDegrees: headingDegrees ?? this.headingDegrees,
      speedMetersPerSecond: speedMetersPerSecond,
      accuracyMeters: accuracyMeters,
      recordedAt: recordedAt,
    );
  }

  static double? _toDouble(Object? value) {
    if (value is num) return value.toDouble();
    if (value is String) return double.tryParse(value);
    return null;
  }

  static DateTime? _toDateTime(Object? value) {
    if (value is String) return DateTime.tryParse(value)?.toUtc();
    // Unix seconds, as the hauling REST snapshot reports them.
    if (value is num) {
      if (value <= 0) return null;
      return DateTime.fromMillisecondsSinceEpoch(value.toInt() * 1000, isUtc: true);
    }
    return null;
  }

  @override
  String toString() =>
      'TrackPoint(${position.lat}, ${position.lng}, heading: $headingDegrees)';
}
