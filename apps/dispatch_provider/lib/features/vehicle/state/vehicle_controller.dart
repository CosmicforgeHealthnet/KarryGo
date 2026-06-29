import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../data/vehicle_api.dart';

class VehicleController extends ChangeNotifier {
  final VehicleApi api;
  final String? Function() getAccessToken;

  VehicleController({required this.api, required this.getAccessToken});

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

  Future<ApiResult<List<Map<String, dynamic>>>> listVehicles() async {
    _setLoading(true);
    _setError(null);
    final result = await api.listVehicles(accessToken: _token);
    return result.when(
      success: (list) {
        _setLoading(false);
        return ApiSuccess(list);
      },
      failure: (error) {
        _setError(error.message);
        _setLoading(false);
        return ApiFailure(error);
      },
    );
  }

  Future<ApiResult<Map<String, dynamic>>> getVehicle(String vehicleId) async {
    _setLoading(true);
    _setError(null);
    final result = await api.getVehicle(
      accessToken: _token,
      vehicleId: vehicleId,
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

  Future<ApiResult<Map<String, dynamic>>> updateVehicle({
    required String vehicleId,
    String? brand,
    String? model,
    String? color,
    int? year,
    String? bikeType,
    String? plateNumber,
  }) async {
    _setLoading(true);
    _setError(null);
    final result = await api.updateVehicle(
      accessToken: _token,
      vehicleId: vehicleId,
      brand: brand,
      model: model,
      color: color,
      year: year,
      bikeType: bikeType,
      plateNumber: plateNumber,
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

  Future<ApiResult<List<Map<String, dynamic>>>> listVehicleDocuments(
    String vehicleId,
  ) async {
    _setLoading(true);
    _setError(null);
    final result = await api.listVehicleDocuments(
      accessToken: _token,
      vehicleId: vehicleId,
    );
    return result.when(
      success: (list) {
        _setLoading(false);
        return ApiSuccess(list);
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
