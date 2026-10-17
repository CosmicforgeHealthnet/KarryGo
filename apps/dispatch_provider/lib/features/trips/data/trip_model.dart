enum TripStatusCode {
  assigned,
  enRoutePickup,
  arrivedPickup,
  inProgress,
  proofSubmitted,
  completed,
  cancelled,
  failed;

  static TripStatusCode fromString(String s) => switch (s) {
    'assigned' => TripStatusCode.assigned,
    'en_route_pickup' => TripStatusCode.enRoutePickup,
    'arrived_pickup' => TripStatusCode.arrivedPickup,
    'in_progress' => TripStatusCode.inProgress,
    'proof_submitted' => TripStatusCode.proofSubmitted,
    'completed' => TripStatusCode.completed,
    'cancelled' => TripStatusCode.cancelled,
    'failed' => TripStatusCode.failed,
    _ => TripStatusCode.assigned,
  };

  bool get isActive =>
      this == assigned ||
      this == enRoutePickup ||
      this == arrivedPickup ||
      this == inProgress ||
      this == proofSubmitted;

  bool get isCompleted => this == completed;
  bool get isCancelled => this == cancelled;
  bool get isFailed => this == failed;

  /// Short label shown on card chips.
  String get shortLabel => switch (this) {
    assigned => 'Assigned',
    enRoutePickup => 'En route',
    arrivedPickup => 'At pickup',
    inProgress => 'In transit',
    proofSubmitted => 'Proof sent',
    completed => 'Delivered',
    cancelled => 'Cancelled',
    failed => 'Failed',
  };
}

class PickupCredential {
  const PickupCredential({
    required this.code,
    this.expiresAt,
    this.verified = false,
  });

  final String code;
  final DateTime? expiresAt;
  final bool verified;

  factory PickupCredential.fromJson(Map<String, dynamic> j) {
    final expiresRaw = j['expires_at'] as String?;
    return PickupCredential(
      code: j['code'] as String? ?? '',
      expiresAt: expiresRaw != null ? DateTime.tryParse(expiresRaw) : null,
      verified: j['verified'] as bool? ?? false,
    );
  }

  static const empty = PickupCredential(code: '');
}

const Map<String, String> tripCancelReasons = {
  'rider_unavailable': 'I am no longer able to complete this trip',
  'package_not_ready': 'Package was not ready at pickup',
  'wrong_address': 'Pickup or drop-off address is wrong',
  'customer_unreachable': 'Customer is unreachable',
  'safety_concern': 'Safety concern',
  'other': 'Other',
};

class TripModel {
  const TripModel({
    required this.id,
    required this.bookingId,
    required this.statusCode,
    required this.rawStatus,
    required this.pickupAddress,
    required this.dropoffAddress,
    required this.customerName,
    required this.receiverName,
    required this.receiverPhone,
    required this.distanceKm,
    required this.estimatedFareKobo,
    required this.createdAt,
    this.customerPhone,
    this.customerPhoto,
    this.notes,
    this.completedAt,
    this.proofUrl,
    this.proofStatus,
    // Extended fields (all optional — tolerate missing values from live backend)
    this.earningsKobo,
    this.itemSummary,
    this.customerRating,
    this.customerFeedback,
    this.cancelReason,
    this.failReason,
    this.refundStatus,
    this.proofPhotos = const [],
    this.proofSignatureUrl,
    this.proofReceiptUrl,
    this.pickupLat,
    this.pickupLng,
    this.dropoffLat,
    this.dropoffLng,
    this.startedAt,
    this.cancelledAt,
    this.failedAt,
    this.mission,
    this.moneyVisible = true,
    this.stateLog = const [],
    this.proofRecord,
    this.routePickupPolyline,
    this.routeDropoffPolyline,
  });

  final List<Map<String, dynamic>> stateLog;
  final Map<String, dynamic>? proofRecord;
  final String? routePickupPolyline;
  final String? routeDropoffPolyline;
  final Map<String, dynamic>? mission;
  final bool moneyVisible;
  final String id;
  final String bookingId;
  final TripStatusCode statusCode;
  final String rawStatus;
  final String pickupAddress;
  final String dropoffAddress;
  final String customerName;
  final String? customerPhone;
  final String? customerPhoto;
  final String receiverName;
  final String receiverPhone;
  final double distanceKm;
  final int estimatedFareKobo;
  final String? notes;
  final DateTime createdAt;
  final DateTime? completedAt;
  final String? proofUrl;
  final String? proofStatus;

  // Extended
  final int? earningsKobo;
  final String? itemSummary;
  final double? customerRating;
  final String? customerFeedback;
  final String? cancelReason;
  final String? failReason;
  final String? refundStatus;
  final List<String> proofPhotos;
  final String? proofSignatureUrl;
  final String? proofReceiptUrl;
  final double? pickupLat;
  final double? pickupLng;
  final double? dropoffLat;
  final double? dropoffLng;
  final DateTime? startedAt;
  final DateTime? cancelledAt;
  final DateTime? failedAt;

  // ── Derived ──────────────────────────────────────────────────────

  double get estimatedFareNgn => estimatedFareKobo / 100.0;
  String get fareDisplay => '₦${estimatedFareNgn.toStringAsFixed(2)}';

  double get _earningsNgn => (earningsKobo ?? estimatedFareKobo) / 100.0;

  String get earningsDisplay =>
      '₦${_groupThousands(_earningsNgn.round().toString())}';

  String get distanceDisplay => '${distanceKm.toStringAsFixed(1)} km';

  String get etaDisplay => '${(distanceKm * 2.5).round()} min';

  bool get hasMap =>
      pickupLat != null &&
      pickupLng != null &&
      dropoffLat != null &&
      dropoffLng != null;

  bool get hasProof =>
      proofRecord != null ||
      proofPhotos.isNotEmpty ||
      proofSignatureUrl != null ||
      proofReceiptUrl != null ||
      proofUrl != null;

  static String _groupThousands(String whole) {
    final buffer = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buffer.write(',');
      buffer.write(whole[i]);
    }
    return buffer.toString();
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'booking_id': bookingId,
    'status': rawStatus,
    'pickup_address': pickupAddress,
    'dropoff_address': dropoffAddress,
    'customer_name': customerName,
    'receiver_name': receiverName,
    'receiver_phone': receiverPhone,
    'distance_km': distanceKm,
    if (moneyVisible) 'fare_amount': estimatedFareKobo,
    'created_at': createdAt.toUtc().toIso8601String(),
    if (pickupLat != null) 'pickup_lat': pickupLat,
    if (pickupLng != null) 'pickup_lng': pickupLng,
    if (dropoffLat != null) 'dropoff_lat': dropoffLat,
    if (dropoffLng != null) 'dropoff_lng': dropoffLng,
    if (mission != null) 'mission': mission,
  };

  factory TripModel.fromJson(Map<String, dynamic> json) {
    final rawStatus = (json['status'] as String?) ?? '';
    return TripModel(
      stateLog:
          (json['state_log'] as List?)
              ?.whereType<Map>()
              .map((e) => Map<String, dynamic>.from(e))
              .toList() ??
          const [],
      proofRecord: json['proof'] is Map
          ? Map<String, dynamic>.from(json['proof'] as Map)
          : null,
      mission: json['mission'] is Map
          ? Map<String, dynamic>.from(json['mission'] as Map)
          : null,
      moneyVisible:
          json.containsKey('fare_amount') ||
          json.containsKey('estimated_fare_kobo'),
      id: (json['id'] as String?) ?? '',
      bookingId: (json['booking_id'] as String?) ?? '',
      statusCode: TripStatusCode.fromString(rawStatus),
      rawStatus: rawStatus,
      pickupAddress: (json['pickup_address'] as String?) ?? '',
      dropoffAddress: (json['dropoff_address'] as String?) ?? '',
      customerName: (json['customer_name'] as String?) ?? '',
      customerPhone: json['customer_phone'] as String?,
      customerPhoto: json['customer_photo'] as String?,
      receiverName: (json['receiver_name'] as String?) ?? '',
      receiverPhone: (json['receiver_phone'] as String?) ?? '',
      distanceKm: ((json['distance_km'] as num?) ?? 0).toDouble(),
      estimatedFareKobo:
          (json['fare_amount'] as num?)?.toInt() ??
          (json['estimated_fare_kobo'] as num?)?.toInt() ??
          0,
      notes: json['notes'] as String?,
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at'] as String) ?? DateTime.now()
          : DateTime.now(),
      completedAt: json['completed_at'] != null
          ? DateTime.tryParse(json['completed_at'] as String)
          : null,
      proofUrl: json['proof_url'] as String?,
      proofStatus: json['proof_status'] as String?,
      earningsKobo: (json['earnings_kobo'] as num?)?.toInt(),
      itemSummary: json['item_summary'] as String?,
      customerRating: (json['customer_rating'] as num?)?.toDouble(),
      customerFeedback: json['customer_feedback'] as String?,
      cancelReason: json['cancel_reason'] as String?,
      failReason: json['fail_reason'] as String?,
      refundStatus: json['refund_status'] as String?,
      proofPhotos:
          (json['proof_photos'] as List?)?.map((e) => e.toString()).toList() ??
          const [],
      proofSignatureUrl: json['proof_signature_url'] as String?,
      proofReceiptUrl: json['proof_receipt_url'] as String?,
      routePickupPolyline: json['route_pickup_polyline'] as String?,
      routeDropoffPolyline: json['route_dropoff_polyline'] as String?,
      pickupLat: (json['pickup_lat'] as num?)?.toDouble(),
      pickupLng: (json['pickup_lng'] as num?)?.toDouble(),
      dropoffLat: (json['dropoff_lat'] as num?)?.toDouble(),
      dropoffLng: (json['dropoff_lng'] as num?)?.toDouble(),
      startedAt: json['started_at'] != null
          ? DateTime.tryParse(json['started_at'] as String)
          : null,
      cancelledAt: json['cancelled_at'] != null
          ? DateTime.tryParse(json['cancelled_at'] as String)
          : null,
      failedAt: json['failed_at'] != null
          ? DateTime.tryParse(json['failed_at'] as String)
          : null,
    );
  }
}
