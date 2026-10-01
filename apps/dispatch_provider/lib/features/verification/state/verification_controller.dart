import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/app_mode.dart';
import '../data/verification_api.dart';
import '../mock/verification_mock.dart';
import '../models/verification_draft.dart';
import '../models/verification_models.dart';

class VerificationController extends ChangeNotifier {
  final VerificationApi api;
  final String? Function() getAccessToken;

  VerificationController({required this.api, required this.getAccessToken});

  bool _isLoading = false;
  bool get isLoading => _isLoading;

  String? _errorMessage;
  String? get errorMessage => _errorMessage;

  AllStatusResponse? _latestStatus;
  AllStatusResponse? get latestStatus => _latestStatus;

  /// The full draft — only populated in mock.
  VerificationDraft? get draft => VerificationMock.identityDraft;

  String get _token => getAccessToken() ?? '';

  void _setLoading(bool value) {
    _isLoading = value;
    notifyListeners();
  }

  void _setError(String? value) {
    _errorMessage = value;
    notifyListeners();
  }

  // ── Identity ───────────────────────────────────────────────────────────

  Future<ApiResult<Map<String, dynamic>>> submitIdentity({
    required String govtIdType,
    required String govtIdNumber,
    required String govtIdFilePath,
    required String profilePhotoFilePath,
    // ── Figma-only fields (mock keeps, live drops) ──
    String? fullName,
    DateTime? dateOfBirth,
    String? gender,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      VerificationMock.submitIdentity(
        govtIdType: govtIdType,
        govtIdNumber: govtIdNumber,
        govtIdFilePath: govtIdFilePath,
        profilePhotoFilePath: profilePhotoFilePath,
        fullName: fullName,
        dateOfBirth: dateOfBirth,
        gender: gender,
      );
      _latestStatus = VerificationMock.status();
      _setLoading(false);
      return const ApiSuccess({'mock': true});
    }

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

  // ── Licence ────────────────────────────────────────────────────────────

  Future<ApiResult<Map<String, dynamic>>> submitLicence({
    required String licenceNumber,
    required String expiryYear,
    required String expiryMonth,
    required String licenceFilePath,
    // ── Figma-only ──
    String? licenceType,
    String? issuingState,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      VerificationMock.submitLicence(
        licenceNumber: licenceNumber,
        expiryYear: expiryYear,
        expiryMonth: expiryMonth,
        licenceFilePath: licenceFilePath,
        licenceType: licenceType,
        issuingState: issuingState,
      );
      _latestStatus = VerificationMock.status();
      _setLoading(false);
      return const ApiSuccess({'mock': true});
    }

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

  // ── Face ───────────────────────────────────────────────────────────────

  Future<ApiResult<Map<String, dynamic>>> submitFace({
    required String selfieFilePath,
  }) async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      VerificationMock.submitFace(selfieFilePath: selfieFilePath);
      _latestStatus = VerificationMock.status();
      _setLoading(false);
      return const ApiSuccess({'mock': true});
    }

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

  // ── Status ─────────────────────────────────────────────────────────────

  void refreshStatus() {
    if (AppModeConfig.instance.isMock) {
      _latestStatus = VerificationMock.status();
      notifyListeners();
      return;
    }
    api.getVerificationStatus(accessToken: _token).then((result) {
      result.when(
        success: (data) {
          _latestStatus = data;
          notifyListeners();
        },
        failure: (_) {},
      );
    });
  }

  Future<ApiResult<AllStatusResponse>> loadVerificationStatus() async {
    _setLoading(true);
    _setError(null);

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      final data = VerificationMock.status();
      _latestStatus = data;
      _setLoading(false);
      return ApiSuccess(data);
    }

    final result = await api.getVerificationStatus(accessToken: _token);

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