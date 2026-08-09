enum TripStatusCode {
  assigned,
  arrivedPickup,
  inProgress,
  proofSubmitted,
  completed,
  cancelled;

  static TripStatusCode fromString(String s) => switch (s) {
    'assigned' => TripStatusCode.assigned,
    'arrived_pickup' => TripStatusCode.arrivedPickup,
    'in_progress' => TripStatusCode.inProgress,
    'proof_submitted' => TripStatusCode.proofSubmitted,
    'completed' => TripStatusCode.completed,
    'cancelled' => TripStatusCode.cancelled,
    _ => TripStatusCode.assigned,
  };

  bool get isActive =>
      this == assigned ||
      this == arrivedPickup ||
      this == inProgress ||
      this == proofSubmitted;
}

/// The rider's own pickup handover credential — displayed on this app and
/// verified by the customer. Never visible to the customer through the API.
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

/// Trip cancel reason codes accepted by the backend
/// (`internal/features/trip/model.go` `ValidCancelReasons`). A cancel request
/// without one of these codes is rejected with a 400.
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
  });

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

  double get estimatedFareNgn => estimatedFareKobo / 100.0;
  String get fareDisplay => '₦${estimatedFareNgn.toStringAsFixed(2)}';
  String get distanceDisplay => '${distanceKm.toStringAsFixed(1)} km';

  factory TripModel.fromJson(Map<String, dynamic> json) {
    final rawStatus = (json['status'] as String?) ?? '';
    return TripModel(
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
      // Backend `Trip` DTO emits the fare as `fare_amount`; keep the legacy
      // `estimated_fare_kobo` as a fallback.
      estimatedFareKobo: (json['fare_amount'] as num?)?.toInt() ??
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
    );
  }
}
