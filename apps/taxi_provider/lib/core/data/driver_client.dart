import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

import '../../features/auth/state/provider_auth_controller.dart';

/// JSON over HTTP as the signed-in driver, for one service's API base.
///
/// Sends the current access token, refreshes the session once on a 401 and
/// retries, and only hands an unrecoverable 401 to the auth controller (which
/// signs the driver out). Responses use the platform envelope; [send]
/// returns its `data`, which may be a map or a list.
class DriverClient {
  DriverClient({
    required String baseUrl,
    required ProviderAuthController auth,
    http.Client? client,
  }) : _config = ApiCoreConfig(baseUrl: baseUrl),
       _auth = auth,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final ProviderAuthController _auth;
  final http.Client _client;

  Future<Object?> get(String path, {Map<String, String>? query}) =>
      send('GET', path, query: query);
  Future<Object?> post(String path, [Map<String, dynamic>? body]) =>
      send('POST', path, body: body ?? const {});
  Future<Object?> put(String path, [Map<String, dynamic>? body]) =>
      send('PUT', path, body: body ?? const {});
  Future<Object?> delete(String path) => send('DELETE', path);

  /// [get] for an endpoint whose `data` is an object.
  Future<Map<String, dynamic>> getMap(
    String path, {
    Map<String, String>? query,
  }) async => asMap(await get(path, query: query));

  Future<Object?> send(
    String method,
    String path, {
    Map<String, dynamic>? body,
    Map<String, String>? query,
  }) {
    return _authorized((token) async {
      var uri = _config.uri(path);
      if (query != null && query.isNotEmpty) {
        uri = uri.replace(queryParameters: {...uri.queryParameters, ...query});
      }
      final request = http.Request(method, uri)
        ..headers.addAll({
          'Accept': 'application/json',
          'Authorization': 'Bearer $token',
        });
      if (body != null) {
        request.headers['Content-Type'] = 'application/json';
        request.body = jsonEncode(body);
      }
      return http.Response.fromStream(await _client.send(request));
    });
  }

  /// Multipart upload of one file plus text fields.
  Future<Object?> upload(
    String path, {
    required Map<String, String> fields,
    required List<int> bytes,
    required String filename,
    required String contentType,
  }) {
    return _authorized((token) async {
      final request = http.MultipartRequest('POST', _config.uri(path))
        ..headers['Authorization'] = 'Bearer $token'
        ..fields.addAll(fields)
        ..files.add(
          http.MultipartFile.fromBytes(
            'file',
            bytes,
            filename: filename,
            contentType: MediaType.parse(contentType),
          ),
        );
      return http.Response.fromStream(await _client.send(request));
    });
  }

  Future<Object?> _authorized(
    Future<http.Response> Function(String token) call,
  ) async {
    try {
      var token = _auth.state.accessToken;
      if (token == null) {
        throw const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'Please sign in again.',
          statusCode: 401,
        );
      }
      var response = await call(token);
      if (response.statusCode == 401 && await _auth.tryRefreshSession()) {
        token = _auth.state.accessToken;
        if (token != null) response = await call(token);
      }
      final decoded = _decode(response);
      if (response.statusCode < 200 ||
          response.statusCode >= 300 ||
          decoded['success'] == false) {
        final error = ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
        if (response.statusCode == 401) await _auth.handleAuthFailure();
        throw error;
      }
      return decoded['data'];
    } on ApiException {
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Map<String, dynamic> _decode(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    try {
      final decoded = jsonDecode(response.body);
      if (decoded is Map) return Map<String, dynamic>.from(decoded);
    } on FormatException {
      // Fall through to the generic error below.
    }
    return {
      'success': false,
      'error': {
        'code': ApiErrorCode.unknown,
        'message': 'Unexpected response from the server.',
      },
    };
  }

  void close() => _client.close();
}

/// `data` as a map, or empty.
Map<String, dynamic> asMap(Object? data) =>
    data is Map ? Map<String, dynamic>.from(data) : <String, dynamic>{};

/// `data[key]` (or `data` itself) as a list of maps.
List<Map<String, dynamic>> asList(Object? data, [String? key]) {
  final raw = key != null && data is Map ? data[key] : data;
  if (raw is! List) return const [];
  return [
    for (final item in raw)
      if (item is Map) Map<String, dynamic>.from(item),
  ];
}
