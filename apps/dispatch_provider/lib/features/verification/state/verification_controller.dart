import 'package:flutter/foundation.dart';
import 'package:karrygo_api_core/karrygo_api_core.dart';
import '../data/verification_api.dart';
import '../models/verification_models.dart';

class VerificationController extends ChangeNotifier {
  final VerificationApi api;
  final String? Function() getAccessToken;

  VerificationController({
    required this.api,
    required this.getAccessToken,
  });

  bool _isLoading = false;
  bool get isLoading => _isLoading;

  String? _errorMessage;
  String? get errorMessage => _errorMessage;

  AllStatusResponse? _latestStatus;
  AllStatusResponse? get latestStatus => _latestStatus;

  String get _token => getAccessToken() ?? '';

  void _setLoading(bool value) {
    _isLoading = value;
    notifyListeners();
  }

  void _setError(String? value) {
    _errorMessage = value;
    notifyListeners();
  }

  Future<ApiResult<Map<String, dynamic>>> submitIdentity({
    required String govtIdType,
    required String govtIdNumber,
    required String govtIdFilePath,
    required String profilePhotoFilePath,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.submitIdentity(
      accessToken: _token,
      govtIdType: govtIdType,
      govtIdNumber: govtIdNumber,
      govtIdFilePath: govtIdFilePath,
      profilePhotoFilePath: profilePhotoFilePath,
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

  Future<ApiResult<Map<String, dynamic>>> submitFace({
    required String selfieFilePath,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.submitFace(
      accessToken: _token,
      selfieFilePath: selfieFilePath,
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

  Future<ApiResult<Map<String, dynamic>>> submitLicence({
    required String licenceNumber,
    required String expiryYear,
    required String expiryMonth,
    required String licenceFilePath,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.submitLicence(
      accessToken: _token,
      licenceNumber: licenceNumber,
      expiryYear: expiryYear,
      expiryMonth: expiryMonth,
      licenceFilePath: licenceFilePath,
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

  Future<ApiResult<AllStatusResponse>> loadVerificationStatus() async {
    _setLoading(true);
    _setError(null);

    final result = await api.getVerificationStatus(
      accessToken: _token,
    );

    return result.when(
      success: (data) {
        _latestStatus = data;
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
