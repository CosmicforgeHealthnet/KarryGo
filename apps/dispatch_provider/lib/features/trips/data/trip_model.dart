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
      estimatedFareKobo: (json['estimated_fare_kobo'] as int?) ?? 0,
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
