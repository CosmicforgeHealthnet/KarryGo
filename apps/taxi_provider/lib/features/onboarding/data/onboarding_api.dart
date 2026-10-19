import 'package:image_picker/image_picker.dart';

import '../../../core/data/driver_client.dart';
import '../../auth/models/provider_auth_models.dart';

class VehicleClassOption {
  const VehicleClassOption({
    required this.value,
    required this.label,
    required this.maxPassengers,
  });
  final String value;
  final String label;
  final int maxPassengers;
}

/// Account setup against the taxi service: documents, profile and vehicle.
class OnboardingApi {
  OnboardingApi(this._client);
  final DriverClient _client;

  Future<List<VehicleClassOption>> vehicleClasses() async =>
      asList(await _client.get('/vehicle-classes'), 'classes')
          .map(
            (j) => VehicleClassOption(
              value: (j['vehicle_class'] as String?) ?? '',
              label: (j['label'] as String?) ?? '',
              maxPassengers: (j['max_passengers'] as num?)?.toInt() ?? 4,
            ),
          )
          .where((c) => c.value.isNotEmpty)
          .toList();

  /// Uploads a picked photo through the taxi service and returns its URL.
  /// [purpose] is `document_file` or `profile_photo`.
  Future<({String id, String url})> uploadPhoto(
    XFile file,
    String purpose,
  ) async {
    final bytes = await file.readAsBytes();
    final name = file.name.toLowerCase();
    final type = name.endsWith('.png')
        ? 'image/png'
        : name.endsWith('.webp')
        ? 'image/webp'
        : 'image/jpeg';
    final data = asMap(
      await _client.upload(
        '/provider/uploads',
        fields: {'purpose': purpose},
        bytes: bytes,
        filename: file.name.isEmpty ? 'photo.jpg' : file.name,
        contentType: type,
      ),
    );
    return (
      id: (data['id'] as String?) ?? '',
      url: (data['url'] as String?) ?? '',
    );
  }

  /// Saves profile fields. Empty values leave the stored value unchanged.
  Future<TaxiProvider> saveProfile(Map<String, String> fields) async =>
      TaxiProvider.fromJson(
        asMap(await _client.put('/provider/profile', fields)),
      );

  Future<void> addVehicle({
    required String vehicleClass,
    required String plateNumber,
    required int seats,
    String make = '',
    String model = '',
    String color = '',
    int? year,
  }) => _client.post('/provider/vehicles', {
    'vehicle_class': vehicleClass,
    'plate_number': plateNumber,
    'seats': seats,
    if (make.isNotEmpty) 'make': make,
    if (model.isNotEmpty) 'model': model,
    if (color.isNotEmpty) 'color': color,
    'year': ?year,
  });
}
