import 'package:flutter/foundation.dart';
import 'package:karrygo_api_core/karrygo_api_core.dart';
import '../data/provider_profile_api.dart';
import '../models/provider_profile_models.dart';

class ProviderProfileController extends ChangeNotifier {
  final ProviderProfileApi api;
  final String? Function() getAccessToken;

  ProviderProfileController({
    required this.api,
    required this.getAccessToken,
  });

  ProviderProfile? _profile;
  ProviderProfile? get profile => _profile;

  EmergencyContact? _emergencyContact;
  EmergencyContact? get emergencyContact => _emergencyContact;

  Guarantor? _guarantor;
  Guarantor? get guarantor => _guarantor;

  ProviderStats? _stats;
  ProviderStats? get stats => _stats;

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

  /// Clears all cached profile data from memory (called on logout so the next
  /// user who logs in starts with a clean slate).
  void clearLocalState() {
    _profile = null;
    _emergencyContact = null;
    _guarantor = null;
    _stats = null;
    _isLoading = false;
    _errorMessage = null;
    notifyListeners();
  }

  Future<ApiResult<ProviderProfile>> loadMe() async {
    _setLoading(true);
    _setError(null);

    final result = await api.getMe(_token);
    return result.when(
      success: (data) {
        _profile = data;
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

  Future<ApiResult<ProviderProfile>> submitOnboarding({
    required String fullName,
    String? email,
    required String state,
    required String city,
    required String operationType,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.submitOnboarding(
      accessToken: _token,
      fullName: fullName,
      email: email,
      state: state,
      city: city,
      operationType: operationType,
    );

    return result.when(
      success: (data) {
        _profile = data;
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

  Future<ApiResult<ProviderProfile>> updateMe({
    String? fullName,
    String? email,
    String? state,
    String? city,
    String? profilePhotoUrl,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.updateMe(
      accessToken: _token,
      fullName: fullName,
      email: email,
      state: state,
      city: city,
      profilePhotoUrl: profilePhotoUrl,
    );

    return result.when(
      success: (data) {
        _profile = data;
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

  Future<ApiResult<EmergencyContact>> saveEmergencyContact({
    required String fullName,
    required String phone,
    required String relationship,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.upsertEmergencyContact(
      accessToken: _token,
      fullName: fullName,
      phone: phone,
      relationship: relationship,
    );

    return result.when(
      success: (data) {
        _emergencyContact = data;
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

  Future<ApiResult<EmergencyContact>> loadEmergencyContact() async {
    _setLoading(true);
    _setError(null);

    final result = await api.getEmergencyContact(_token);
    return result.when(
      success: (data) {
        _emergencyContact = data;
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

  Future<ApiResult<Guarantor>> saveGuarantor({
    required String fullName,
    required String phone,
  }) async {
    _setLoading(true);
    _setError(null);

    final result = await api.upsertGuarantor(
      accessToken: _token,
      fullName: fullName,
      phone: phone,
    );

    return result.when(
      success: (data) {
        _guarantor = data;
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

  Future<ApiResult<Guarantor>> loadGuarantor() async {
    _setLoading(true);
    _setError(null);

    final result = await api.getGuarantor(_token);
    return result.when(
      success: (data) {
        _guarantor = data;
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

  Future<ApiResult<ProviderStats>> loadStats() async {
    _setLoading(true);
    _setError(null);

    final result = await api.getStats(_token);
    return result.when(
      success: (data) {
        _stats = data;
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
