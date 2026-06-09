import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:karrygo_api_core/karrygo_api_core.dart';
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
      return ApiFailure(ApiException(
        code: ApiErrorCode.validationFailed,
        message: 'No file selected for $fieldLabel. Please choose a file.',
      ));
    }
    try {
      if (!File(filePath).existsSync()) {
        return ApiFailure(ApiException(
          code: ApiErrorCode.validationFailed,
          message:
              'The selected $fieldLabel no longer exists on this device. '
              'Please re-select the file.',
        ));
      }
    } catch (_) {
      // If existsSync throws (e.g. permission), let the upload attempt proceed
      // and surface any OS-level error naturally.
    }
    final ext = filePath.split('.').last.toLowerCase();
    if (!allowedExts.contains(ext)) {
      final allowed = allowedExts.map((e) => e.toUpperCase()).join(', ');
      return ApiFailure(ApiException(
        code: ApiErrorCode.validationFailed,
        message:
            'Invalid file type ".$ext" for $fieldLabel. '
            'Allowed types: $allowed.',
      ));
    }
    return null; // valid
  }

  /// Returns an [ApiFailure] if [value] is blank, [null] otherwise.
  ApiFailure<Map<String, dynamic>>? _validateField(
    String value,
    String fieldLabel,
  ) {
    if (value.trim().isEmpty) {
      return ApiFailure(ApiException(
        code: ApiErrorCode.validationFailed,
        message: '$fieldLabel is required and must not be empty.',
      ));
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
    
    debugPrint('=== [VERIFICATION IDENTITY REQUEST] ===');
    debugPrint('URL: $uri');
    debugPrint('govt_id_type: $govtIdType');
    debugPrint('govt_id_number: $govtIdNumber');
    debugPrint('govt_id_file path: $govtIdFilePath');
    debugPrint('profile_photo path: $profilePhotoFilePath');

    // Validate required fields before sending
    final typeErr = _validateField(govtIdType, 'Government ID type');
    if (typeErr != null) {
      debugPrint('[IDENTITY] Validation failed: ${typeErr.error.message}');
      return typeErr;
    }
    final numErr = _validateField(govtIdNumber, 'Government ID number');
    if (numErr != null) {
      debugPrint('[IDENTITY] Validation failed: ${numErr.error.message}');
      return numErr;
    }
    final idFileErr = _validateFile(govtIdFilePath, _docExts, 'Government ID file');
    if (idFileErr != null) {
      debugPrint('[IDENTITY] Validation failed: ${idFileErr.error.message}');
      return idFileErr;
    }
    final photoErr = _validateFile(profilePhotoFilePath, _photoExts, 'Profile photo');
    if (photoErr != null) {
      debugPrint('[IDENTITY] Validation failed: ${photoErr.error.message}');
      return photoErr;
    }

    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_headers(accessToken));
      request.fields['govt_id_type'] = govtIdType;
      request.fields['govt_id_number'] = govtIdNumber;
      
      request.files.add(await http.MultipartFile.fromPath('govt_id_file', govtIdFilePath));
      request.files.add(await http.MultipartFile.fromPath('profile_photo', profilePhotoFilePath));

      final streamedResponse = await request.send().timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamedResponse);

      debugPrint('=== [VERIFICATION IDENTITY RESPONSE] ===');
      debugPrint('Status Code: ${response.statusCode}');
      debugPrint('Response Body: ${response.body}');

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(data);
        }
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e, stackTrace) {
      debugPrint('=== [VERIFICATION IDENTITY EXCEPTION] ===');
      debugPrint('Exception: $e');
      debugPrint('StackTrace: $stackTrace');
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<Map<String, dynamic>>> submitFace({
    required String accessToken,
    required String selfieFilePath,
  }) async {
    final uri = config.uri('/api/v1/provider/verification/face');
    
    debugPrint('=== [VERIFICATION FACE REQUEST] ===');
    debugPrint('URL: $uri');
    debugPrint('selfie path: $selfieFilePath');

    // Validate selfie file before sending
    final selfieErr = _validateFile(selfieFilePath, _photoExts, 'Selfie photo');
    if (selfieErr != null) {
      debugPrint('[FACE] Validation failed: ${selfieErr.error.message}');
      return selfieErr;
    }

    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_headers(accessToken));

      request.files.add(await http.MultipartFile.fromPath('selfie', selfieFilePath));

      final streamedResponse = await request.send().timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamedResponse);

      debugPrint('=== [VERIFICATION FACE RESPONSE] ===');
      debugPrint('Status Code: ${response.statusCode}');
      debugPrint('Response Body: ${response.body}');

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(data);
        }
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e, stackTrace) {
      debugPrint('=== [VERIFICATION FACE EXCEPTION] ===');
      debugPrint('Exception: $e');
      debugPrint('StackTrace: $stackTrace');
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
    
    debugPrint('=== [VERIFICATION LICENCE REQUEST] ===');
    debugPrint('URL: $uri');
    debugPrint('licence_number: $licenceNumber');
    debugPrint('expiry_year: $expiryYear');
    debugPrint('expiry_month: $expiryMonth');
    debugPrint('licence_file path: $licenceFilePath');

    // Validate required fields and file before sending
    final licNumErr = _validateField(licenceNumber, 'Licence number');
    if (licNumErr != null) {
      debugPrint('[LICENCE] Validation failed: ${licNumErr.error.message}');
      return licNumErr;
    }
    final yearErr = _validateField(expiryYear, 'Expiry year');
    if (yearErr != null) {
      debugPrint('[LICENCE] Validation failed: ${yearErr.error.message}');
      return yearErr;
    }
    final monthErr = _validateField(expiryMonth, 'Expiry month');
    if (monthErr != null) {
      debugPrint('[LICENCE] Validation failed: ${monthErr.error.message}');
      return monthErr;
    }
    final licFileErr = _validateFile(licenceFilePath, _docExts, 'Driver licence file');
    if (licFileErr != null) {
      debugPrint('[LICENCE] Validation failed: ${licFileErr.error.message}');
      return licFileErr;
    }

    try {
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_headers(accessToken));
      request.fields['licence_number'] = licenceNumber;
      request.fields['expiry_year'] = expiryYear;
      request.fields['expiry_month'] = expiryMonth;

      request.files.add(await http.MultipartFile.fromPath('licence_file', licenceFilePath));

      final streamedResponse = await request.send().timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamedResponse);

      debugPrint('=== [VERIFICATION LICENCE RESPONSE] ===');
      debugPrint('Status Code: ${response.statusCode}');
      debugPrint('Response Body: ${response.body}');

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(data);
        }
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e, stackTrace) {
      debugPrint('=== [VERIFICATION LICENCE EXCEPTION] ===');
      debugPrint('Exception: $e');
      debugPrint('StackTrace: $stackTrace');
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<AllStatusResponse>> getVerificationStatus({
    required String accessToken,
  }) async {
    final uri = config.uri('/api/v1/provider/verification/status');
    
    debugPrint('=== [GET VERIFICATION STATUS REQUEST] ===');
    debugPrint('URL: $uri');

    try {
      final response = await _client.get(
        uri,
        headers: _headers(accessToken),
      ).timeout(const Duration(seconds: 15));

      debugPrint('=== [GET VERIFICATION STATUS RESPONSE] ===');
      debugPrint('Status Code: ${response.statusCode}');
      debugPrint('Response Body: ${response.body}');

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(AllStatusResponse.fromJson(data));
        }
      }
      return ApiFailure(ApiException.fromErrorEnvelope(body, statusCode: response.statusCode));
    } catch (e, stackTrace) {
      debugPrint('=== [GET VERIFICATION STATUS EXCEPTION] ===');
      debugPrint('Exception: $e');
      debugPrint('StackTrace: $stackTrace');
      return ApiFailure(ApiException.network(e));
    }
  }
}
