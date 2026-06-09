import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:karrygo_api_core/karrygo_api_core.dart';

class VehicleApi {
  final ApiCoreConfig config;
  final http.Client _client;

  VehicleApi(this.config, {http.Client? client})
      : _client = client ?? http.Client();

  Map<String, String> _jsonHeaders(String accessToken) => {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer $accessToken',
      };

  Map<String, String> _multipartHeaders(String accessToken) => {
        'Authorization': 'Bearer $accessToken',
        'Accept': 'application/json',
      };

  // ── POST /api/v1/provider/vehicle ─────────────────────────────────────────

  /// Creates a new vehicle for the provider.
  /// Returns the vehicle ID on success.
  Future<ApiResult<String>> createVehicle({
    required String accessToken,
    required String bikeType,
    required String brand,
    required String model,
    required int year,
    required String color,
    required String plateNumber,
  }) async {
    debugPrint('=== [VEHICLE CREATE REQUEST] ===');
    debugPrint(
        'bike_type=$bikeType  brand=$brand  model=$model  year=$year  color=$color  plate_number=$plateNumber');
    try {
      final uri = config.uri('/api/v1/provider/vehicle');
      final response = await _client
          .post(
            uri,
            headers: _jsonHeaders(accessToken),
            body: jsonEncode({
              'bike_type': bikeType,
              'brand': brand,
              'model': model,
              'year': year,
              'color': color,
              'plate_number': plateNumber,
            }),
          )
          .timeout(const Duration(seconds: 20));

      debugPrint('=== [VEHICLE CREATE RESPONSE] ===');
      debugPrint('Status: ${response.statusCode}');
      debugPrint('Body: ${response.body}');

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 || response.statusCode == 201) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          final id = data['id'] as String? ??
              data['vehicle_id'] as String? ??
              '';
          return ApiSuccess(id);
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body,
            statusCode: response.statusCode),
      );
    } catch (e, st) {
      debugPrint('[VEHICLE CREATE] Exception: $e\n$st');
      return ApiFailure(ApiException.network(e));
    }
  }

  // ── POST /api/v1/provider/vehicle/:id/documents ───────────────────────────

  /// Uploads a document (e.g. registration sticker) for a vehicle.
  Future<ApiResult<Map<String, dynamic>>> uploadVehicleDocument({
    required String accessToken,
    required String vehicleId,
    required String documentType,
    required String documentFilePath,
    String? expiryDate,
  }) async {
    debugPrint('=== [VEHICLE DOC UPLOAD REQUEST] ===');
    debugPrint(
        'vehicleId=$vehicleId  documentType=$documentType  file=$documentFilePath');

    if (documentFilePath.trim().isEmpty) {
      return ApiFailure(ApiException(
        code: ApiErrorCode.validationFailed,
        message: 'No vehicle registration document selected.',
      ));
    }
    try {
      if (!File(documentFilePath).existsSync()) {
        return ApiFailure(ApiException(
          code: ApiErrorCode.validationFailed,
          message:
              'The selected vehicle registration document no longer exists '
              'on this device. Please re-select.',
        ));
      }
    } catch (_) {
      // existsSync may throw on some platforms — let the upload attempt proceed.
    }

    try {
      final uri =
          config.uri('/api/v1/provider/vehicle/$vehicleId/documents');
      final request = http.MultipartRequest('POST', uri);
      request.headers.addAll(_multipartHeaders(accessToken));
      request.fields['document_type'] = documentType;
      if (expiryDate != null && expiryDate.isNotEmpty) {
        request.fields['expiry_date'] = expiryDate;
      }
      request.files.add(
        await http.MultipartFile.fromPath(
            'document_file', documentFilePath),
      );

      final streamed =
          await request.send().timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamed);

      debugPrint('=== [VEHICLE DOC UPLOAD RESPONSE] ===');
      debugPrint('Status: ${response.statusCode}');
      debugPrint('Body: ${response.body}');

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200 || response.statusCode == 201) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>? ?? {};
          return ApiSuccess(data);
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body,
            statusCode: response.statusCode),
      );
    } catch (e, st) {
      debugPrint('[VEHICLE DOC UPLOAD] Exception: $e\n$st');
      return ApiFailure(ApiException.network(e));
    }
  }
}
