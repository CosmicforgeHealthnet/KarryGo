import 'package:flutter/foundation.dart';

enum RequestSource { broadcast, assignment, commerce }

/// Which backend endpoint this offer came from. Determines the accept /
/// decline URL and the request body shape.
enum RequestServerKind {
  /// `/api/v1/provider/requests` — legacy broadcast inbox.
  broadcast,

  /// `/api/v1/provider/assignment-offers` — sequential/org/admin assignment.
  assignmentOffer,

  /// `/api/v1/provider/organization-offers` — marketplace, delegated.
  organizationOffer,

  /// `/api/v1/provider/company-assignments` — marketplace, dispatcher-assigned.
  companyAssignment,
}

enum RequestLifecycleState { available, expired, taken, requoted }

extension RequestSourceX on RequestSource {
  String get label => switch (this) {
        RequestSource.broadcast => 'Broadcast',
        RequestSource.assignment => 'Assignment',
        RequestSource.commerce => 'Commerce',
      };
}

class RequestModel {
  const RequestModel({
    required this.id,
    required this.kind,
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
    this.source = RequestSource.broadcast,
    this.lifecycle = RequestLifecycleState.available,
    this.itemSummary,
    this.handlingTags = const [],
    this.sourceOrgName,
    this.previousFareKobo,
    this.offerUpdatedAt,
    this.pickupLat,
    this.pickupLng,
    this.dropoffLat,
    this.dropoffLng,
    this.version = 0,
    this.rank = 0,
  });

  final String id;
  final RequestServerKind kind;
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

  final RequestSource source;
  final RequestLifecycleState lifecycle;
  final String? itemSummary;
  final List<String> handlingTags;
  final String? sourceOrgName;
  final int? previousFareKobo;
  final DateTime? offerUpdatedAt;
  final double? pickupLat;
  final double? pickupLng;
  final double? dropoffLat;
  final double? dropoffLng;

  /// Backend-side version. Required by assignment and marketplace accept
  /// calls. Always 0 for broadcasts (they don't use optimistic concurrency).
  final int version;

  /// Rank within a sequential assignment offer list. 0 for non-assignments.
  final int rank;

  // ── Presence getters — the card uses these to decide what to render ──

  bool get hasFare => estimatedFareKobo > 0;
  bool get hasRoute => pickupAddress.isNotEmpty && dropoffAddress.isNotEmpty;
  bool get hasPackage => (itemSummary ?? '').isNotEmpty;
  bool get hasHandling => handlingTags.isNotEmpty;
  bool get hasSourceOrg => (sourceOrgName ?? '').isNotEmpty;
  bool get hasCustomer => customerName.isNotEmpty;
  bool get hasDistance => distanceKm > 0;
  bool get hasNotes => (notes ?? '').isNotEmpty;
  bool get hasMap =>
      pickupLat != null &&
      pickupLng != null &&
      dropoffLat != null &&
      dropoffLng != null;

  // ── Derived display helpers ──────────────────────────────────────────

  bool get isExpress => serviceTier == 'express';

  double get estimatedFareNgn => estimatedFareKobo / 100.0;
  String get fareDisplay => '₦${estimatedFareNgn.toStringAsFixed(2)}';
  String get distanceDisplay => '${distanceKm.toStringAsFixed(1)} km';
  String get fareDisplayGrouped =>
      '₦${_groupThousands(estimatedFareNgn.round().toString())}';

  String? get previousFareDisplayGrouped {
    if (previousFareKobo == null) return null;
    return '₦${_groupThousands((previousFareKobo! / 100.0).round().toString())}';
  }

  bool get isFareUpdate =>
      previousFareKobo != null && previousFareKobo != estimatedFareKobo;

  int? get minutesRemaining {
    if (expiresAt == null) return null;
    final m = expiresAt!.difference(DateTime.now()).inMinutes;
    return m < 0 ? 0 : m;
  }

  /// A revised offer may be accepted using its new backend version, but
  /// expiration always wins over a stale lifecycle label.
  bool get isAcceptable =>
      (lifecycle == RequestLifecycleState.available ||
       lifecycle == RequestLifecycleState.requoted) &&
      (expiresAt == null || expiresAt!.isAfter(DateTime.now()));

  static String _groupThousands(String whole) {
    final buf = StringBuffer();
    for (int i = 0; i < whole.length; i++) {
      if (i != 0 && (whole.length - i) % 3 == 0) buf.write(',');
      buf.write(whole[i]);
    }
    return buf.toString();
  }

  // ── Parsing ───────────────────────────────────────────────────────────

  factory RequestModel.fromJson(Map<String, dynamic> json) {
    final kind = _inferKind(json);

    // ID: whichever primary key this payload uses.
    final id = (json['inbox_id'] as String?) ??
        (json['offer_id'] as String?) ??
        (json['offerId'] as String?) ??
        (json['assignmentId'] as String?) ??
        '';

    // Booking/mission/request — same thing across all three sources.
    final bookingId = (json['booking_id'] as String?) ??
        (json['mission_id'] as String?) ??
        (json['requestId'] as String?) ??
        '';

    // Addresses: snake_case from broadcast, camelCase from marketplace.
    final pickup = (json['pickup_address'] as String?) ??
        (json['pickupAddress'] as String?) ??
        '';
    final dropoff = (json['dropoff_address'] as String?) ??
        (json['dropoffAddress'] as String?) ??
        '';

    // Timestamps: broadcast uses received_at/created_at, others use
    // offered_at/expiresAt.
    final createdRaw = (json['received_at'] as String?) ??
        (json['created_at'] as String?) ??
        (json['offered_at'] as String?) ??
        (json['offeredAt'] as String?);
    final expiresRaw = (json['expires_at'] as String?) ??
        (json['expiresAt'] as String?);

    // Numeric fields.
    final fareAmount = (json['fare_amount'] as num?)?.toInt() ??
        (json['estimated_fare_kobo'] as num?)?.toInt() ??
        0;
    final distanceKm = ((json['distance_km'] as num?) ?? 0).toDouble();

    return RequestModel(
      id: id,
      kind: kind,
      bookingId: bookingId,
      pickupAddress: pickup,
      dropoffAddress: dropoff,
      receiverName: (json['receiver_name'] as String?) ?? '',
      receiverPhone: (json['receiver_phone'] as String?) ?? '',
      distanceKm: distanceKm,
      estimatedFareKobo: fareAmount,
      status: (json['status'] as String?) ?? '',
      createdAt: createdRaw != null
          ? DateTime.tryParse(createdRaw) ?? DateTime.now()
          : DateTime.now(),
      customerName: (json['customer_name'] as String?) ?? '',
      expiresAt: expiresRaw != null ? DateTime.tryParse(expiresRaw) : null,
      customerPhoto: json['customer_photo'] as String?,
      notes: json['notes'] as String?,
      serviceTier: (json['service_tier'] as String?) ??
          (json['serviceType'] as String?) ??
          'standard',
      serviceTierLabel: json['service_tier_label'] as String?,
      source: _inferSource(kind),
      lifecycle: _parseLifecycle(json['lifecycle'] as String?),
      itemSummary: json['item_summary'] as String?,
      handlingTags: (json['handling_tags'] as List?)
              ?.map((e) => e.toString())
              .toList() ??
          const [],
      sourceOrgName: json['source_org_name'] as String?,
      previousFareKobo: (json['previous_fare_kobo'] as num?)?.toInt(),
      offerUpdatedAt: json['offer_updated_at'] != null
          ? DateTime.tryParse(json['offer_updated_at'] as String)
          : null,
      pickupLat: (json['pickup_lat'] as num?)?.toDouble(),
      pickupLng: (json['pickup_lng'] as num?)?.toDouble(),
      dropoffLat: (json['dropoff_lat'] as num?)?.toDouble(),
      dropoffLng: (json['dropoff_lng'] as num?)?.toDouble(),
      version: (json['version'] as num?)?.toInt() ?? 0,
      rank: (json['rank'] as num?)?.toInt() ?? 0,
    );
  }

  static RequestServerKind _inferKind(Map<String, dynamic> json) {
    if (json.containsKey('inbox_id')) return RequestServerKind.broadcast;
    if (json.containsKey('offer_id')) return RequestServerKind.assignmentOffer;
    if (json.containsKey('offerId')) return RequestServerKind.organizationOffer;
    if (json.containsKey('assignmentId')) {
      return RequestServerKind.companyAssignment;
    }
    // Fallback: treat as broadcast so at least the shape is sane.
    return RequestServerKind.broadcast;
  }

  static RequestSource _inferSource(RequestServerKind kind) => switch (kind) {
        RequestServerKind.broadcast => RequestSource.broadcast,
        RequestServerKind.assignmentOffer => RequestSource.assignment,
        RequestServerKind.organizationOffer => RequestSource.commerce,
        RequestServerKind.companyAssignment => RequestSource.commerce,
      };

  static RequestLifecycleState _parseLifecycle(String? raw) => switch (raw) {
        'expired' => RequestLifecycleState.expired,
        'taken' => RequestLifecycleState.taken,
        'requoted' => RequestLifecycleState.requoted,
        _ => RequestLifecycleState.available,
      };

  RequestModel copyWithSource(RequestSource source) {
    return RequestModel(
      id: id,
      kind: kind,
      bookingId: bookingId,
      pickupAddress: pickupAddress,
      dropoffAddress: dropoffAddress,
      receiverName: receiverName,
      receiverPhone: receiverPhone,
      distanceKm: distanceKm,
      estimatedFareKobo: estimatedFareKobo,
      status: status,
      createdAt: createdAt,
      customerName: customerName,
      expiresAt: expiresAt,
      customerPhoto: customerPhoto,
      notes: notes,
      serviceTier: serviceTier,
      serviceTierLabel: serviceTierLabel,
      source: source,
      lifecycle: lifecycle,
      itemSummary: itemSummary,
      handlingTags: handlingTags,
      sourceOrgName: sourceOrgName,
      previousFareKobo: previousFareKobo,
      offerUpdatedAt: offerUpdatedAt,
      pickupLat: pickupLat,
      pickupLng: pickupLng,
      dropoffLat: dropoffLat,
      dropoffLng: dropoffLng,
      version: version,
      rank: rank,
    );
  }

  @override
  bool operator ==(Object other) =>
      other is RequestModel && other.id == id && other.kind == kind;

  @override
  int get hashCode => Object.hash(id, kind);
}

// Keep debugPrint reachable for future tracing without a separate import.
// ignore: unused_element
void _debug(String msg) => debugPrint(msg);