enum MissionStatus {
  assigned,
  inProgress,
  completed,
  cancelled,
  failed;

  bool get isActive =>
      this == assigned || this == inProgress;

  String get label => switch (this) {
    assigned => 'Assigned',
    inProgress => 'In progress',
    completed => 'Completed',
    cancelled => 'Cancelled',
    failed => 'Failed',
  };
}

enum MissionStopStatus {
  pending,
  arrived,
  processing,
  completed,
  skipped;

  bool get isDone => this == completed || this == skipped;

  String get label => switch (this) {
    pending => 'Pending',
    arrived => 'Arrived',
    processing => 'Processing',
    completed => 'Completed',
    skipped => 'Skipped',
  };
}

enum ParcelStatus {
  pending,
  scanned,
  delivered,
  failed;

  String get label => switch (this) {
    pending => 'Pending',
    scanned => 'Scanned',
    delivered => 'Delivered',
    failed => 'Failed',
  };
}

enum ParcelHandling {
  standard,
  fragile,
  chilled,
  medical,
  documents,
  food;

  String get label => switch (this) {
    standard => 'Standard',
    fragile => 'Fragile',
    chilled => 'Chilled',
    medical => 'Medical',
    documents => 'Documents',
    food => 'Food',
  };
}

// ── Parcel ──────────────────────────────────────────────────────────

class MissionParcel {
  const MissionParcel({
    required this.id,
    required this.handling,
    required this.weightKg,
    this.itemType,
    this.barcode,
    this.status = ParcelStatus.pending,
  });

  final String id;
  final ParcelHandling handling;
  final double weightKg;
  final String? itemType;
  final String? barcode;
  final ParcelStatus status;

  String get weightDisplay => '${weightKg.toStringAsFixed(1)} kg';

  MissionParcel copyWith({ParcelStatus? status}) => MissionParcel(
        id: id,
        handling: handling,
        weightKg: weightKg,
        itemType: itemType,
        barcode: barcode,
        status: status ?? this.status,
      );
}

// ── Stop ────────────────────────────────────────────────────────────

class MissionStop {
  const MissionStop({
    required this.id,
    required this.number,
    required this.name,
    required this.address,
    required this.lat,
    required this.lng,
    required this.parcels,
    this.arriveBy,
    this.distanceFromPreviousKm = 0,
    this.etaMinutes = 0,
    this.contactPhone,
    this.contactName,
    this.status = MissionStopStatus.pending,
    this.arrivedAt,
    this.completedAt,
    this.credentialCode,
    this.credentialExpiresAt,
    this.credentialVerified = false,
    this.evidencePhotos = const [],
    this.evidenceSignatureUrl,
    this.evidenceNotes,
    this.evidenceCaptured = false,
  });

  final String id;
  final int number;
  final String name;
  final String address;
  final double lat;
  final double lng;
  final List<MissionParcel> parcels;
  final DateTime? arriveBy;
  final double distanceFromPreviousKm;
  final int etaMinutes;
  final String? contactPhone;
  final String? contactName;
  final MissionStopStatus status;
  final DateTime? arrivedAt;
  final DateTime? completedAt;
  final String? credentialCode;
  final DateTime? credentialExpiresAt;
  final bool credentialVerified;
  final List<String> evidencePhotos;
  final String? evidenceSignatureUrl;
  final String? evidenceNotes;
  final bool evidenceCaptured;

  int get totalParcels => parcels.length;
  int get scannedParcels =>
      parcels.where((p) => p.status == ParcelStatus.scanned ||
              p.status == ParcelStatus.delivered)
          .length;
  int get pendingParcels =>
      parcels.where((p) => p.status == ParcelStatus.pending).length;

  bool get allParcelsScanned => scannedParcels == totalParcels && totalParcels > 0;

  MissionStop copyWith({
    MissionStopStatus? status,
    List<MissionParcel>? parcels,
    DateTime? arrivedAt,
    DateTime? completedAt,
    bool? credentialVerified,
    List<String>? evidencePhotos,
    String? evidenceSignatureUrl,
    String? evidenceNotes,
    bool? evidenceCaptured,
  }) =>
      MissionStop(
        id: id,
        number: number,
        name: name,
        address: address,
        lat: lat,
        lng: lng,
        parcels: parcels ?? this.parcels,
        arriveBy: arriveBy,
        distanceFromPreviousKm: distanceFromPreviousKm,
        etaMinutes: etaMinutes,
        contactPhone: contactPhone,
        contactName: contactName,
        status: status ?? this.status,
        arrivedAt: arrivedAt ?? this.arrivedAt,
        completedAt: completedAt ?? this.completedAt,
        credentialCode: credentialCode,
        credentialExpiresAt: credentialExpiresAt,
        credentialVerified: credentialVerified ?? this.credentialVerified,
        evidencePhotos: evidencePhotos ?? this.evidencePhotos,
        evidenceSignatureUrl:
            evidenceSignatureUrl ?? this.evidenceSignatureUrl,
        evidenceNotes: evidenceNotes ?? this.evidenceNotes,
        evidenceCaptured: evidenceCaptured ?? this.evidenceCaptured,
      );
}

// ── Mission ─────────────────────────────────────────────────────────

class Mission {
  const Mission({
    required this.id,
    required this.sourceOrgName,
    required this.stops,
    required this.createdAt,
    required this.totalDistanceKm,
    this.status = MissionStatus.assigned,
    this.startedAt,
    this.completedAt,
    this.customerNote,
  });

  final String id;
  final String sourceOrgName;
  final List<MissionStop> stops;
  final DateTime createdAt;
  final double totalDistanceKm;
  final MissionStatus status;
  final DateTime? startedAt;
  final DateTime? completedAt;
  final String? customerNote;

  int get totalStops => stops.length;
  int get completedStops => stops.where((s) => s.status.isDone).length;
  int get totalParcels =>
      stops.fold(0, (sum, s) => sum + s.totalParcels);
  int get scannedParcels =>
      stops.fold(0, (sum, s) => sum + s.scannedParcels);

  /// Index of the first stop that isn't completed.
  int get currentStopIndex {
    final i = stops.indexWhere((s) => !s.status.isDone);
    return i < 0 ? stops.length - 1 : i;
  }

  MissionStop? get currentStop {
    final i = currentStopIndex;
    if (i < 0 || i >= stops.length) return null;
    return stops[i];
  }

  MissionStop? get nextStop {
    final i = currentStopIndex + 1;
    if (i < 0 || i >= stops.length) return null;
    return stops[i];
  }

  bool get allStopsComplete => completedStops == totalStops;

  String get totalDistanceDisplay =>
      '${totalDistanceKm.toStringAsFixed(1)} km';

  String get progressDisplay => '$completedStops of $totalStops stops';

  Mission copyWith({
    MissionStatus? status,
    List<MissionStop>? stops,
    DateTime? startedAt,
    DateTime? completedAt,
  }) =>
      Mission(
        id: id,
        sourceOrgName: sourceOrgName,
        stops: stops ?? this.stops,
        createdAt: createdAt,
        totalDistanceKm: totalDistanceKm,
        status: status ?? this.status,
        startedAt: startedAt ?? this.startedAt,
        completedAt: completedAt ?? this.completedAt,
        customerNote: customerNote,
      );
}