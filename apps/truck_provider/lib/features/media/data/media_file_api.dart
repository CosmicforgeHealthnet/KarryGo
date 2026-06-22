import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

import '../models/media_upload_result.dart';

class MediaFileApi {
  MediaFileApi({
    required ApiCoreConfig config,
    required String serviceToken,
    required String ownerService,
    http.Client? client,
  })  : _config = config,
        _serviceToken = serviceToken,
        _ownerService = ownerService,
        _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final String _serviceToken;
  final String _ownerService;
  final http.Client _client;

  Future<MediaUploadResult> uploadFile({
    required String ownerId,
    required String purpose,
    required String filename,
    required List<int> bytes,
    required String contentType,
  }) async {
    try {
      final uri = _config.uri('/uploads');
      final request = http.MultipartRequest('POST', uri)
        ..headers['Authorization'] = 'Bearer $_serviceToken'
        ..headers['X-Karrygo-Service'] = _ownerService
        ..fields['owner_service'] = _ownerService
        ..fields['owner_id'] = ownerId
        ..fields['purpose'] = purpose
        ..files.add(http.MultipartFile.fromBytes(
          'file',
          bytes,
          filename: filename,
          contentType: MediaType.parse(contentType),
        ));

      final streamed = await _client.send(request);
      final response = await http.Response.fromStream(streamed);

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }

      final rawData = decoded['data'];
      if (rawData is Map) {
        final result = Map<String, dynamic>.from(rawData);
        if (result['url'] is String && !result['url'].startsWith('http')) {
          result['url'] = '${_config.baseUrl}${result['url']}';
        }
        return MediaUploadResult.fromJson(result);
      }
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message: 'Unexpected response from media service.',
      );
    } on ApiException {
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
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
