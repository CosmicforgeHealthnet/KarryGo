import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/customer_auth_models.dart';

class CustomerAuthApi {
  CustomerAuthApi({required ApiCoreConfig config, http.Client? client})
    : _config = config,
      _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  Future<StartAuthResult> startAuth({String? phone, String? email}) async {
    final data = await _sendJson(
      'POST',
      '/auth/start',
      body: _identifierBody(phone: phone, email: email),
    );
    return StartAuthResult.fromJson(data);
  }

  Future<AuthTokenResult> verifyAuth({
    String? phone,
    String? email,
    required String otp,
    required String challengeId,
    required String deviceId,
  }) async {
    final body = _identifierBody(phone: phone, email: email)
      ..addAll({
        'otp': otp,
        'challenge_id': challengeId,
        'device_id': deviceId,
      });
    final data = await _sendJson(
      'POST',
      '/auth/verify',
      body: body,
    );
    return AuthTokenResult.fromJson(data);
  }

  Future<AuthTokenResult> refresh({
    required String refreshToken,
    required String deviceId,
  }) async {
    final data = await _sendJson(
      'POST',
      '/auth/refresh',
      body: {'refresh_token': refreshToken, 'device_id': deviceId},
    );
    return AuthTokenResult.fromJson(data);
  }

  Future<void> logout({required String refreshToken}) async {
    await _sendJson(
      'POST',
      '/auth/logout',
      body: {'refresh_token': refreshToken},
    );
  }

  Future<CustomerProfile> me({required String accessToken}) async {
    final data = await _sendJson('GET', '/me', accessToken: accessToken);
    return CustomerProfile.fromJson(data);
  }

  void close() {
    _client.close();
  }

  Future<Map<String, dynamic>> _sendJson(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? accessToken,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        if (accessToken != null) 'Authorization': 'Bearer $accessToken',
      };

      final response = switch (method) {
        'GET' => await _client.get(uri, headers: headers),
        'POST' => await _client.post(
          uri,
          headers: headers,
          body: jsonEncode(body ?? const {}),
        ),
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }

      if (decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }

      final rawData = decoded['data'];
      if (rawData is Map) {
        return Map<String, dynamic>.from(rawData);
      }
      return const {};
    } on ApiException {
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Map<String, dynamic> _decodeResponse(http.Response response) {
    if (response.body.isEmpty) {
      return const {'success': true, 'data': {}};
    }

    final decoded = jsonDecode(response.body);
    if (decoded is Map) {
      return Map<String, dynamic>.from(decoded);
    }

    return const {
      'success': false,
      'error': {
        'code': ApiErrorCode.unknown,
        'message': 'Something went wrong. Please try again.',
      },
    };
  }
}

Map<String, dynamic> _identifierBody({String? phone, String? email}) {
  final body = <String, dynamic>{};
  if (phone != null && phone.trim().isNotEmpty) {
    body['phone'] = phone;
  }
  if (email != null && email.trim().isNotEmpty) {
    body['email'] = email;
  }
  return body;
}
