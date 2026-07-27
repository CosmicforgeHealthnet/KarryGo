class RequestModel {
  const RequestModel({
    required this.id,
    required this.bookingId,
    required this.pickupAddress,
    required this.dropoffAddress,
    required this.receiverName,
    required this.receiverPhone,
    required this.distanceKm,
    required this.estimatedFareKobo,
    required this.status,
    required this.createdAt,
    required this.customerName,
    this.expiresAt,
    this.customerPhoto,
    this.notes,
    this.serviceTier = 'standard',
    this.serviceTierLabel,
  });

  final String id;
  final String bookingId;
  final String pickupAddress;
  final String dropoffAddress;
  final String receiverName;
  final String receiverPhone;
  final double distanceKm;
  final int estimatedFareKobo;
  final String status;
  final DateTime createdAt;
  final String customerName;
  final DateTime? expiresAt;
  final String? customerPhoto;
  final String? notes;
  final String serviceTier;
  final String? serviceTierLabel;

  bool get isExpress => serviceTier == 'express';

  double get estimatedFareNgn => estimatedFareKobo / 100.0;
  String get fareDisplay => '₦${estimatedFareNgn.toStringAsFixed(2)}';
  String get distanceDisplay => '${distanceKm.toStringAsFixed(1)} km';

  factory RequestModel.fromJson(Map<String, dynamic> json) {
    // The backend request-inbox list item serializes the row id as `inbox_id`
    // (and the fare as `fare_amount`, the timestamp as `received_at`). Read the
    // real backend keys first, falling back to the alternates so this keeps
    // working if the contract is ever aligned or the object comes from a trip.
    final createdRaw =
        (json['received_at'] as String?) ?? (json['created_at'] as String?);
    return RequestModel(
      id: (json['inbox_id'] as String?) ?? (json['id'] as String?) ?? '',
      bookingId: (json['booking_id'] as String?) ?? '',
      pickupAddress: (json['pickup_address'] as String?) ?? '',
      dropoffAddress: (json['dropoff_address'] as String?) ?? '',
      receiverName: (json['receiver_name'] as String?) ?? '',
      receiverPhone: (json['receiver_phone'] as String?) ?? '',
      distanceKm: ((json['distance_km'] as num?) ?? 0).toDouble(),
      estimatedFareKobo: (json['fare_amount'] as num?)?.toInt() ??
          (json['estimated_fare_kobo'] as num?)?.toInt() ??
          0,
      status: (json['status'] as String?) ?? '',
      createdAt: createdRaw != null
          ? DateTime.tryParse(createdRaw) ?? DateTime.now()
          : DateTime.now(),
      customerName: (json['customer_name'] as String?) ?? '',
      expiresAt: json['expires_at'] != null
          ? DateTime.tryParse(json['expires_at'] as String)
          : null,
      customerPhoto: json['customer_photo'] as String?,
      notes: json['notes'] as String?,
      serviceTier: (json['service_tier'] as String?) ?? 'standard',
      serviceTierLabel: json['service_tier_label'] as String?,
    );
  }
}
