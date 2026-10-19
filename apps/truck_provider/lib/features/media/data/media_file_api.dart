import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

import '../models/media_upload_result.dart';

/// Uploads the driver's documents and photos through hauling-service
/// (`POST /provider/uploads`), signed in as the driver. Hauling checks the
/// file and stores it with media-file-service under its own credentials, so
/// the app carries no media credential and the file's owner is always the
/// signed-in driver.
class MediaFileApi {
  MediaFileApi({
    required ApiCoreConfig config,
    required String? Function() accessToken,
    Future<bool> Function()? refreshSession,
    http.Client? client,
  })  : _config = config,
        _accessToken = accessToken,
        _refreshSession = refreshSession,
        _client = client ?? http.Client();

  /// The hauling API base, e.g. https://.../api/v1/hauling.
  final ApiCoreConfig _config;
  final String? Function() _accessToken;
  final Future<bool> Function()? _refreshSession;
  final http.Client _client;

  /// [ownerId] is accepted for callers' convenience but not sent: the
  /// server takes the owner from the driver's sign-in.
  Future<MediaUploadResult> uploadFile({
    required String ownerId,
    required String purpose,
    required String filename,
    required List<int> bytes,
    required String contentType,
  }) async {
    try {
      var response = await _send(purpose, filename, bytes, contentType);
      // Access tokens are short-lived; onboarding can outlast one.
      if (response.statusCode == 401 &&
          _refreshSession != null &&
          await _refreshSession()) {
        response = await _send(purpose, filename, bytes, contentType);
      }

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final rawData = decoded['data'];
      if (rawData is Map) {
        return MediaUploadResult.fromJson(Map<String, dynamic>.from(rawData));
      }
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message: 'Unexpected response from the upload service.',
      );
    } on ApiException {
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Future<http.Response> _send(
    String purpose,
    String filename,
    List<int> bytes,
    String contentType,
  ) async {
    final token = _accessToken();
    if (token == null || token.isEmpty) {
      throw const ApiException(
        code: ApiErrorCode.unauthorized,
        message: 'Sign in again to upload files.',
        statusCode: 401,
      );
    }
    final request = http.MultipartRequest('POST', _config.uri('/provider/uploads'))
      ..headers['Authorization'] = 'Bearer $token'
      ..fields['purpose'] = purpose
      ..files.add(http.MultipartFile.fromBytes(
        'file',
        bytes,
        filename: filename,
        contentType: MediaType.parse(contentType),
      ));
    return http.Response.fromStream(await _client.send(request));
  }

  void close() => _client.close();

  Map<String, dynamic> _decodeResponse(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {
      'success': false,
      'error': {'code': ApiErrorCode.unknown, 'message': 'Unexpected response.'},
    };
  }
}
