/// Rider availability, aligned with the backend `AvailabilityStatusResponse`.
///
/// Backend shape (GET /api/v1/provider/availability):
///   {
///     "status": "online" | "offline" | "busy",
///     "verified_to_go_online": bool,
///     "session_start": ISO8601 | null,
///     "session_duration_minutes": int,
///     "hours_online_today": double,
///     "trips_today": int
///   }
///
/// `sessionId` and `onlineSince` are kept as legacy constructor params so
/// the mock path in the controller compiles unchanged. They are now derived
/// from `session_start` in the real backend response.
class ProviderAvailability {
  final String status; // "online", "offline", "busy"
  final bool verifiedToGoOnline;
  final DateTime? sessionStart;
  final int sessionDurationMinutes;
  final double hoursOnlineToday;
  final int tripsToday;

  /// Legacy — populated only by mock code paths.
  final String? sessionId;

  const ProviderAvailability({
    required this.status,
    this.verifiedToGoOnline = false,
    this.sessionStart,
    this.sessionDurationMinutes = 0,
    this.hoursOnlineToday = 0,
    this.tripsToday = 0,
    this.sessionId,
  });

  bool get isOnline => status == 'online' || status == 'busy';
  bool get isOffline => status == 'offline';

  /// Legacy alias.
  DateTime? get onlineSince => sessionStart;

  factory ProviderAvailability.fromJson(Map<String, dynamic> json) {
    final sessionStartRaw = json['session_start'] as String?;
    final sessionStart = sessionStartRaw != null
        ? DateTime.tryParse(sessionStartRaw)
        : null;

    return ProviderAvailability(
      status: json['status'] as String? ?? 'offline',
      verifiedToGoOnline: json['verified_to_go_online'] as bool? ?? false,
      sessionStart: sessionStart,
      sessionDurationMinutes:
          (json['session_duration_minutes'] as num?)?.toInt() ?? 0,
      hoursOnlineToday:
          (json['hours_online_today'] as num?)?.toDouble() ?? 0.0,
      tripsToday: (json['trips_today'] as num?)?.toInt() ?? 0,
    );
  }
}

/// Current availability session, aligned with the backend
/// `CurrentSessionResponse`.
///
/// Backend shape (GET /api/v1/provider/availability/session/current):
///   {
///     "provider_id": "...",
///     "status": "online" | "offline" | "busy",
///     "session": {
///       "id": "...",
///       "provider_id": "...",
///       "went_online_at": ISO8601,
///       "went_offline_at": ISO8601 | null,
///       "duration_minutes": int | null,
///       "trips_in_session": int,
///       "forced_offline": bool,
///       "created_at": ISO8601
///     }
///   }
///
/// `fromJson` accepts either the full envelope (with `session` sub-object)
/// or a bare session object, so both current and future payload shapes work.
class AvailabilitySession {
  final String sessionId;
  final String providerId;
  final String status;
  final DateTime startedAt;
  final DateTime? endedAt;

  const AvailabilitySession({
    required this.sessionId,
    required this.providerId,
    required this.status,
    required this.startedAt,
    this.endedAt,
  });

  factory AvailabilitySession.fromJson(Map<String, dynamic> json) {
    // Unwrap the `session` sub-object when present. Falls through to the
    // top-level JSON for a bare-session response.
    final src =
        (json['session'] as Map<String, dynamic>?) ?? json;

    // Accept both new backend names (went_online_at / went_offline_at) and
    // the previous frontend names (started_at / ended_at).
    final onlineRaw = (src['went_online_at'] as String?) ??
        (src['started_at'] as String?);
    final offlineRaw = (src['went_offline_at'] as String?) ??
        (src['ended_at'] as String?);

    return AvailabilitySession(
      sessionId: (src['id'] as String?) ??
          (src['session_id'] as String?) ??
          '',
      providerId: (src['provider_id'] as String?) ??
          (json['provider_id'] as String?) ??
          '',
      status: (json['status'] as String?) ?? 'offline',
      startedAt: onlineRaw != null
          ? DateTime.tryParse(onlineRaw) ?? DateTime.now()
          : DateTime.now(),
      endedAt: offlineRaw != null ? DateTime.tryParse(offlineRaw) : null,
    );
  }
}

class ProviderLocation {
  final double lat;
  final double lng;
  final double? heading;
  final double? speed;
  final double? accuracy;
  final DateTime? updatedAt;

  const ProviderLocation({
    required this.lat,
    required this.lng,
    this.heading,
    this.speed,
    this.accuracy,
    this.updatedAt,
  });

  factory ProviderLocation.fromJson(Map<String, dynamic> json) {
    return ProviderLocation(
      lat: (json['lat'] as num?)?.toDouble() ?? 0.0,
      lng: (json['lng'] as num?)?.toDouble() ?? 0.0,
      heading: (json['heading'] as num?)?.toDouble(),
      speed: (json['speed'] as num?)?.toDouble(),
      accuracy: (json['accuracy'] as num?)?.toDouble(),
      updatedAt: json['updated_at'] != null
          ? DateTime.tryParse(json['updated_at'] as String)
          : null,
    );
  }
}