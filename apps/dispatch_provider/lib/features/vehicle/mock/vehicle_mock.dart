/// Stateful mock for the vehicle feature. Same pattern as TripsTabMock /
/// VerificationMock / ProfileMock. Two seeded bikes match Figma 01.
class VehicleMock {
  VehicleMock._();

  static final List<Map<String, dynamic>> _bikes = [
    {
      'id': 'mock-bike-1',
      'name': 'TVS Apache 160',
      'plate_number': 'LND 784 KJ',
      'status': 'active',
      'verification_status': 'verified',
      'bike_type': 'motorcycle',
      'brand': 'TVS',
      'model': 'Apache 160',
      'year': 2022,
      'color': 'Black',
      'mileage_km': 12500,
      'next_service_date': null,
      'photo_url': null,
    },
    {
      'id': 'mock-bike-2',
      'name': 'Bajaj Boxer 150',
      'plate_number': 'KJA 550 HK',
      'status': 'pending',
      'verification_status': 'submitted',
      'bike_type': 'motorcycle',
      'brand': 'Bajaj',
      'model': 'Boxer 150',
      'year': 2023,
      'color': 'Red',
      'mileage_km': 4200,
      'next_service_date': null,
      'photo_url': null,
    },
  ];

  // ── Bikes ──────────────────────────────────────────────────────────────

  static List<Map<String, dynamic>> bikes() =>
      _bikes.map((e) => Map<String, dynamic>.from(e)).toList();

  static Map<String, dynamic>? getBike(String id) {
    for (final b in _bikes) {
      if (b['id'] == id) return Map<String, dynamic>.from(b);
    }
    return null;
  }

  static Map<String, dynamic> addBike({
    required String bikeType,
    required String brand,
    required String model,
    required int year,
    required String color,
    required String plateNumber,
  }) {
    final id = 'mock-bike-${_bikes.length + 1}';
    final bike = {
      'id': id,
      'name': '$brand $model',
      'plate_number': plateNumber.toUpperCase(),
      'status': 'pending',
      'verification_status': 'submitted',
      'bike_type': bikeType,
      'brand': brand,
      'model': model,
      'year': year,
      'color': color,
      'mileage_km': 0,
      'next_service_date': null,
      'photo_url': null,
    };
    _bikes.add(bike);
    return Map<String, dynamic>.from(bike);
  }

  static Map<String, dynamic> updateBike({
    required String id,
    String? brand,
    String? model,
    String? color,
    int? year,
    String? bikeType,
    String? plateNumber,
  }) {
    final idx = _bikes.indexWhere((b) => b['id'] == id);
    if (idx < 0) return {};
    final b = _bikes[idx];
    if (brand != null) b['brand'] = brand;
    if (model != null) b['model'] = model;
    if (color != null) b['color'] = color;
    if (year != null) b['year'] = year;
    if (bikeType != null) b['bike_type'] = bikeType;
    if (plateNumber != null) b['plate_number'] = plateNumber;
    b['name'] = '${b['brand']} ${b['model']}';
    return Map<String, dynamic>.from(b);
  }

  // ── Documents ──────────────────────────────────────────────────────────

  /// Returns the 4 document slots Figma 06 shows. Statuses vary by bike.
  static List<Map<String, dynamic>> documents(String vehicleId) {
    final bike = getBike(vehicleId);
    final verified = bike?['verification_status'] == 'verified';
    return [
      {
        'id': 'doc-reg-$vehicleId',
        'vehicle_id': vehicleId,
        'document_type': 'registration_certificate',
        'label': 'Registration Certificate',
        'status': verified ? 'valid' : 'pending',
        'expiry_date': '2025-05-12',
        'file_url': null,
      },
      {
        'id': 'doc-ins-$vehicleId',
        'vehicle_id': vehicleId,
        'document_type': 'insurance_certificate',
        'label': 'Insurance Certificate',
        'status': verified ? 'valid' : 'pending',
        'expiry_date': null,
        'file_url': null,
      },
      {
        'id': 'doc-rw-$vehicleId',
        'vehicle_id': vehicleId,
        'document_type': 'road_worthiness',
        'label': 'Road Worthiness',
        'status': verified ? 'valid' : 'pending',
        'expiry_date': null,
        'file_url': null,
      },
      {
        'id': 'doc-photo-$vehicleId',
        'vehicle_id': vehicleId,
        'document_type': 'profile_photo',
        'label': 'Profile Photo',
        'status': verified ? 'valid' : 'pending',
        'expiry_date': null,
        'file_url': null,
      },
    ];
  }

  static Map<String, dynamic> uploadDocument({
    required String vehicleId,
    required String documentType,
    required String filePath,
  }) {
    return {
      'id': 'doc-$documentType-$vehicleId',
      'vehicle_id': vehicleId,
      'document_type': documentType,
      'status': 'pending',
      'file_url': filePath,
    };
  }

  static void reset() {
    _bikes.clear();
    _bikes.addAll([
      {
        'id': 'mock-bike-1',
        'name': 'TVS Apache 160',
        'plate_number': 'LND 784 KJ',
        'status': 'active',
        'verification_status': 'verified',
        'bike_type': 'motorcycle',
        'brand': 'TVS',
        'model': 'Apache 160',
        'year': 2022,
        'color': 'Black',
        'mileage_km': 12500,
        'next_service_date': null,
        'photo_url': null,
      },
      {
        'id': 'mock-bike-2',
        'name': 'Bajaj Boxer 150',
        'plate_number': 'KJA 550 HK',
        'status': 'pending',
        'verification_status': 'submitted',
        'bike_type': 'motorcycle',
        'brand': 'Bajaj',
        'model': 'Boxer 150',
        'year': 2023,
        'color': 'Red',
        'mileage_km': 4200,
        'next_service_date': null,
        'photo_url': null,
      },
    ]);
  }
}