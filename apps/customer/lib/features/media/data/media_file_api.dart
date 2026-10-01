import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

import '../../auth/data/customer_session_store.dart';
import '../models/media_upload_result.dart';

class MediaFileApi {
  MediaFileApi({
    required ApiCoreConfig config,
    required CustomerSessionStore sessionStore,
    http.Client? client,
  }) : _config = config,
       _sessionStore = sessionStore,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final CustomerSessionStore _sessionStore;
  final http.Client _client;

  Future<MediaUploadResult> uploadFile({
    required String ownerId,
    required String purpose,
    required String filename,
    required List<int> bytes,
    required String contentType,
  }) async {
    try {
      if (purpose != 'profile_photo') {
        throw const ApiException(
          code: ApiErrorCode.unknown,
          message: 'Unsupported customer upload purpose.',
        );
      }
      final session = await _sessionStore.readSession();
      if (session == null ||
          session.customer.id != ownerId ||
          session.accessToken.isEmpty) {
        throw const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'Customer authentication is required.',
        );
      }
      final uri = _config.uri('/profile/photo');
      final request = http.MultipartRequest('POST', uri)
        ..headers['Authorization'] = 'Bearer ${session.accessToken}'
        ..files.add(
          http.MultipartFile.fromBytes(
            'photo',
            bytes,
            filename: filename,
            contentType: MediaType.parse(contentType),
          ),
        );

      final streamed = await _client.send(request);
      final response = await http.Response.fromStream(streamed);

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }

      final rawData = decoded['data'];
      if (rawData is Map) {
        final result = Map<String, dynamic>.from(rawData);
        final id = result['profile_photo_asset_id'];
        final url = result['profile_photo_url'];
        if (id is String && id.isNotEmpty && url is String && url.isNotEmpty) {
          return MediaUploadResult(id: id, url: url);
        }
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
      'error': {
        'code': ApiErrorCode.unknown,
        'message': 'Unexpected response.',
      },
    };
  }
}
