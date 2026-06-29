class ProviderAvailability {
  final String status; // "online", "offline", "busy"
  final String? sessionId;
  final DateTime? onlineSince;

  const ProviderAvailability({
    required this.status,
    this.sessionId,
    this.onlineSince,
  });

  bool get isOnline => status == 'online' || status == 'busy';
  bool get isOffline => status == 'offline';

  factory ProviderAvailability.fromJson(Map<String, dynamic> json) {
    return ProviderAvailability(
      status: json['status'] as String? ?? 'offline',
      sessionId: json['session_id'] as String?,
      onlineSince: json['online_since'] != null
          ? DateTime.tryParse(json['online_since'] as String)
          : null,
    );
  }
}

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
    return AvailabilitySession(
      sessionId: json['session_id'] as String? ?? json['id'] as String? ?? '',
      providerId: json['provider_id'] as String? ?? '',
      status: json['status'] as String? ?? 'offline',
      startedAt: json['started_at'] != null
          ? DateTime.tryParse(json['started_at'] as String) ?? DateTime.now()
          : DateTime.now(),
      endedAt: json['ended_at'] != null
          ? DateTime.tryParse(json['ended_at'] as String)
          : null,
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
