import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../models/verification_models.dart';

// ── Allowed extensions per upload type ───────────────────────────────────────
// Docs (govt ID, driver licence): jpg/jpeg/png/pdf
const _docExts = {'jpg', 'jpeg', 'png', 'pdf'};
// Photos (profile photo, selfie): jpg/jpeg/png only — no PDF
const _photoExts = {'jpg', 'jpeg', 'png'};

class VerificationApi {
  final ApiCoreConfig config;
  final http.Client _client;

  VerificationApi(this.config, {http.Client? client})
    : _client = client ?? http.Client();

  Map<String, String> _headers(String accessToken) {
    return {
      'Authorization': 'Bearer $accessToken',
      'Accept': 'application/json',
    };
  }

  // ── Pre-upload validators ─────────────────────────────────────────────────

  /// Returns an [ApiFailure] if the file path is empty, the file does not
  /// exist on disk, or the file extension is not in [allowedExts].
  /// Returns [null] if the file is valid and ready to upload.
  ApiFailure<Map<String, dynamic>>? _validateFile(
    String filePath,
    Set<String> allowedExts,
    String fieldLabel,
  ) {
    if (filePath.trim().isEmpty) {
      return ApiFailure(
        ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'No file selected for $fieldLabel. Please choose a file.',
        ),
      );
    }
    try {
      if (!File(filePath).existsSync()) {
        return ApiFailure(
          ApiException(
            code: ApiErrorCode.validationFailed,
            message:
                'The selected $fieldLabel no longer exists on this device. '
                'Please re-select the file.',
          ),
        );
      }
    } catch (_) {
      // If existsSync throws (e.g. permission), let the upload attempt proceed
      // and surface any OS-level error naturally.
    }
    final ext = filePath.split('.').last.toLowerCase();
    if (!allowedExts.contains(ext)) {
      final allowed = allowedExts.map((e) => e.toUpperCase()).join(', ');
      return ApiFailure(
        ApiException(
          code: ApiErrorCode.validationFailed,
          message:
              'Invalid file type ".$ext" for $fieldLabel. '
              'Allowed types: $allowed.',
        ),
      );
    }
    return null; // valid
  }

  /// Returns an [ApiFailure] if [value] is blank, [null] otherwise.
  ApiFailure<Map<String, dynamic>>? _validateField(
    String value,
    String fieldLabel,
  ) {
    if (value.trim().isEmpty) {
      return ApiFailure(
        ApiException(
          code: ApiErrorCode.validationFailed,
          message: '$fieldLabel is required and must not be empty.',
        ),
      );
    }
    return null;
  }

  /// Returns an [ApiFailure] if [value] is shorter than [min] characters.
  ApiFailure<Map<String, dynamic>>? _validateMinLength(
    String value,
    String fieldLabel, {
    int min = 5,
  }) {
    if (value.trim().length < min) {
      return ApiFailure(
        ApiException(
          code: ApiErrorCode.validationFailed,
          message: '$fieldLabel must be at least $min characters.',
        ),
      );
    }
    return null;
  }

  Future<ApiResult<Map<String, dynamic>>> submitIdentity({
    required String accessToken,
    required String govtIdType,
    required String govtIdNumber,
    required String govtIdFilePath,
    required String profilePhotoFilePath,
  }) async {
    final uri = config.uri('/api/v1/provider/verification/identity');

    if (kDebugMode) {
      debugPrint('[IDENTITY] POST $uri '
          'type=$govtIdType '
          'num_len=${govtIdNumber.length} '
          'id_file=${govtIdFilePath.split(RegExp(r'[/\\]')).last} '
          'photo=${profilePhotoFilePath.split(RegExp(r'[/\\]')).last}');
    }

    final typeErr = _validateField(govtIdType, 'Government ID type');
    if (typeErr != null) return typeErr;
    final numErr = _validateField(govtIdNumber, 'Government ID number');
    if (numErr != null) return numErr;
    final numLenErr = _validateMinLength(
      govtIdNumber,
      'Government ID number',
    );
    if (numLenErr != null) return numLenErr;
    final idFileErr = _validateFile(
      govtIdFilePath,
      _docExts,
      'Government ID file',
    );
    if (idFileErr != null) return idFileErr;
    final photoErr = _validateFile(
      profilePhotoFilePath,
      _photoExts,
      'Profile photo',
    );
    if (photoErr != null) return photoErr;

    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_headers(accessToken));
      request.fields['govt_id_type'] = govtIdType;
      request.fields['govt_id_number'] = govtIdNumber;

      request.files.add(
        await http.MultipartFile.fromPath('govt_id_file', govtIdFilePath),
      );
      request.files.add(
        await http.MultipartFile.fromPath(
          'profile_photo',
          profilePhotoFilePath,
        ),
      );

      final streamedResponse = await _client
          .send(request)
          .timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamedResponse);

      if (kDebugMode) {
        debugPrint('[IDENTITY] status=${response.statusCode}');
      }

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(body['data'] as Map<String, dynamic>? ?? {});
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      if (kDebugMode) debugPrint('[IDENTITY] error: $e');
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<Map<String, dynamic>>> submitFace({
    required String accessToken,
    required String selfieFilePath,
  }) async {
    final uri = config.uri('/api/v1/provider/verification/face');

    if (kDebugMode) {
      debugPrint('[FACE] POST $uri '
          'selfie=${selfieFilePath.split(RegExp(r'[/\\]')).last}');
    }

    final selfieErr = _validateFile(selfieFilePath, _photoExts, 'Selfie photo');
    if (selfieErr != null) return selfieErr;

    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_headers(accessToken));

      request.files.add(
        await http.MultipartFile.fromPath('selfie', selfieFilePath),
      );

      final streamedResponse = await _client
          .send(request)
          .timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamedResponse);

      if (kDebugMode) {
        debugPrint('[FACE] status=${response.statusCode}');
      }

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(body['data'] as Map<String, dynamic>? ?? {});
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      if (kDebugMode) debugPrint('[FACE] error: $e');
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<Map<String, dynamic>>> submitLicence({
    required String accessToken,
    required String licenceNumber,
    required String expiryYear,
    required String expiryMonth,
    required String licenceFilePath,
  }) async {
    final uri = config.uri('/api/v1/provider/verification/licence');

    if (kDebugMode) {
      debugPrint('[LICENCE] POST $uri '
          'num_len=${licenceNumber.length} '
          'expiry=$expiryYear/$expiryMonth '
          'file=${licenceFilePath.split(RegExp(r'[/\\]')).last}');
    }

    final licNumErr = _validateField(licenceNumber, 'Licence number');
    if (licNumErr != null) return licNumErr;
    final licLenErr = _validateMinLength(licenceNumber, 'Licence number');
    if (licLenErr != null) return licLenErr;
    final yearErr = _validateField(expiryYear, 'Expiry year');
    if (yearErr != null) return yearErr;
    final monthErr = _validateField(expiryMonth, 'Expiry month');
    if (monthErr != null) return monthErr;
    final licFileErr = _validateFile(
      licenceFilePath,
      _docExts,
      'Driver licence file',
    );
    if (licFileErr != null) return licFileErr;

    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_headers(accessToken));
      request.fields['licence_number'] = licenceNumber;
      request.fields['expiry_year'] = expiryYear;
      request.fields['expiry_month'] = expiryMonth;

      request.files.add(
        await http.MultipartFile.fromPath('licence_file', licenceFilePath),
      );

      final streamedResponse = await _client
          .send(request)
          .timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamedResponse);

      if (kDebugMode) {
        debugPrint('[LICENCE] status=${response.statusCode}');
      }

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        return ApiSuccess(body['data'] as Map<String, dynamic>? ?? {});
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      if (kDebugMode) debugPrint('[LICENCE] error: $e');
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<AllStatusResponse>> getVerificationStatus({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/verification/status');

    if (kDebugMode) debugPrint('[VERIFICATION] GET status $uri');

    try {
      final response = await _client
          .get(uri, headers: _headers(accessToken))
          .timeout(const Duration(seconds: 15));

      if (kDebugMode) {
        debugPrint('[VERIFICATION] status=${response.statusCode}');
      }

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 && body['success'] == true) {
        final data = body['data'] as Map<String, dynamic>;
        return ApiSuccess(AllStatusResponse.fromJson(data));
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      if (kDebugMode) debugPrint('[VERIFICATION] status error: $e');
      return ApiFailure(ApiException.network(e));
    }
  }
}
