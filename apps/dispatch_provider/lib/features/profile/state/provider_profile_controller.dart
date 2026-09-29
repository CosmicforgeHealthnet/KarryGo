import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/app_mode.dart';
import '../data/provider_profile_api.dart';
import '../mock/profile_mock.dart';
import '../models/provider_profile_models.dart';

class ProviderProfileController extends ChangeNotifier {
  final ProviderProfileApi api;
  final String? Function() getAccessToken;

  ProviderProfileController({required this.api, required this.getAccessToken});

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

  void clearLocalState() {
    _profile = null;
    _emergencyContact = null;
    _guarantor = null;
    _stats = null;
    _isLoading = false;
    _errorMessage = null;
    notifyListeners();
  }

  // ── loadMe ────────────────────────────────────────────────────────────

  Future<ApiResult<ProviderProfile>> loadMe() async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      _profile = ProfileMock.profile;
      _setLoading(false);
      return ApiSuccess(_profile!);
    }

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

  // ── submitOnboarding ──────────────────────────────────────────────────

  Future<ApiResult<ProviderProfile>> submitOnboarding({
    required String fullName,
    String? email,
    required String state,
    required String city,
    required String operationType,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      ProfileMock.updateMe(
        fullName: fullName,
        email: email,
        state: state,
        city: city,
        operationType: operationType,
      );
      _profile = ProfileMock.completeOnboarding();
      _setLoading(false);
      return ApiSuccess(_profile!);
    }

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

  // ── updateMe ──────────────────────────────────────────────────────────

  Future<ApiResult<ProviderProfile>> updateMe({
    String? fullName,
    String? email,
    String? state,
    String? city,
    String? profilePhotoUrl,
    String? operationType,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      _profile = ProfileMock.updateMe(
        fullName: fullName,
        email: email,
        state: state,
        city: city,
        profilePhotoUrl: profilePhotoUrl,
        operationType: operationType,
      );
      _setLoading(false);
      return ApiSuccess(_profile!);
    }

    final result = await api.updateMe(
      accessToken: _token,
      fullName: fullName,
      email: email,
      state: state,
      city: city,
      profilePhotoUrl: profilePhotoUrl,
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

  // ── saveEmergencyContact ──────────────────────────────────────────────

  Future<ApiResult<EmergencyContact>> saveEmergencyContact({
    required String fullName,
    required String phone,
    required String relationship,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      _emergencyContact = ProfileMock.saveEmergencyContact(
        fullName: fullName,
        phone: phone,
        relationship: relationship,
      );
      _profile = ProfileMock.profile;
      _setLoading(false);
      return ApiSuccess(_emergencyContact!);
    }

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

  // ── loadEmergencyContact ──────────────────────────────────────────────

  Future<ApiResult<EmergencyContact>> loadEmergencyContact() async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      final ec = ProfileMock.emergencyContact;
      if (ec == null) {
        _setLoading(false);
        return ApiFailure(
          const ApiException(
            code: ApiErrorCode.validationFailed,
            message: 'No emergency contact on file.',
          ),
        );
      }
      _emergencyContact = ec;
      _setLoading(false);
      return ApiSuccess(ec);
    }

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

  // ── saveGuarantor ─────────────────────────────────────────────────────

  Future<ApiResult<Guarantor>> saveGuarantor({
    required String fullName,
    required String phone,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      _guarantor = ProfileMock.saveGuarantor(
        fullName: fullName,
        phone: phone,
      );
      _profile = ProfileMock.profile;
      _setLoading(false);
      return ApiSuccess(_guarantor!);
    }

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

  // ── loadGuarantor ─────────────────────────────────────────────────────

  Future<ApiResult<Guarantor>> loadGuarantor() async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      final g = ProfileMock.guarantor;
      if (g == null) {
        _setLoading(false);
        return ApiFailure(
          const ApiException(
            code: ApiErrorCode.validationFailed,
            message: 'No guarantor on file.',
          ),
        );
      }
      _guarantor = g;
      _setLoading(false);
      return ApiSuccess(g);
    }

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

  // ── loadStats ─────────────────────────────────────────────────────────

  Future<ApiResult<ProviderStats>> loadStats() async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      _stats = ProfileMock.stats;
      _setLoading(false);
      return ApiSuccess(_stats!);
    }

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

  // ── deleteAccount ─────────────────────────────────────────────────────

  Future<ApiResult<bool>> deleteAccount() async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 700));
      ProfileMock.deleteAccount();
      _setLoading(false);
      return const ApiSuccess(true);
    }

    final result = await api.deleteAccount(_token);
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

  // ── uploadAvatar ──────────────────────────────────────────────────────

  Future<ApiResult<ProviderProfile>> uploadAvatar(String filePath) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      _profile = ProfileMock.setAvatar(filePath);
      _setLoading(false);
      return ApiSuccess(_profile!);
    }

    final result = await api.uploadAvatar(
      accessToken: _token,
      filePath: filePath,
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
}