import 'package:flutter/foundation.dart';
import 'package:karrygo_api_core/karrygo_api_core.dart';
import '../data/vehicle_api.dart';

class VehicleController extends ChangeNotifier {
  final VehicleApi api;
  final String? Function() getAccessToken;

  VehicleController({
    required this.api,
    required this.getAccessToken,
  });

  bool _isLoading = false;
  bool get isLoading => _isLoading;

  String? _errorMessage;
  String? get errorMessage => _errorMessage;

  String get _token => getAccessToken() ?? '';

  void _setLoading(bool value) {
    _isLoading = value;
    notifyListeners();
  }

  void _setError(String? value) {
    _errorMessage = value;
    notifyListeners();
  }

  Future<ApiResult<String>> createVehicle({
    required String bikeType,
    required String brand,
    required String model,
    required int year,
    required String color,
    required String plateNumber,
  }) async {
    _setLoading(true);
    _setError(null);
    final result = await api.createVehicle(
      accessToken: _token,
      bikeType: bikeType,
      brand: brand,
      model: model,
      year: year,
      color: color,
      plateNumber: plateNumber,
    );
    return result.when(
      success: (id) {
        _setLoading(false);
        return ApiSuccess(id);
      },
      failure: (error) {
        _setError(error.message);
        _setLoading(false);
        return ApiFailure(error);
      },
    );
  }

  Future<ApiResult<Map<String, dynamic>>> uploadVehicleDocument({
    required String vehicleId,
    required String documentType,
    required String documentFilePath,
    String? expiryDate,
  }) async {
    _setLoading(true);
    _setError(null);
    final result = await api.uploadVehicleDocument(
      accessToken: _token,
      vehicleId: vehicleId,
      documentType: documentType,
      documentFilePath: documentFilePath,
      expiryDate: expiryDate,
    );
    return result.when(
      success: (data) {
        _setLoading(false);
        return ApiSuccess(data);
      },
      failure: (error) {
        _setError(error.message);
        _setLoading(false);
        return ApiFailure(error);
      },
    );
  }
}
