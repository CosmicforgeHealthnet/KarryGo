import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/customer_auth_models.dart';

class CustomerAuthApi {
  CustomerAuthApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Called when a *bearer-authenticated* request fails with an auth error
  /// (401 / unauthorized). Only fires for requests that carried an access
  /// token, so the unauthenticated auth flow (start/verify/refresh) — where a
  /// 401 is expected and handled by the controller — never triggers a global
  /// logout.
  final void Function()? onAuthFailure;

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

  Future<CustomerProfile> getProfile({required String accessToken}) async {
    final data = await _sendJson('GET', '/profile', accessToken: accessToken);
    return CustomerProfile.fromJson(data);
  }

  Future<CustomerProfile> updateProfile({
    required String accessToken,
    required String firstName,
    required String lastName,
  }) async {
    final data = await _sendJson(
      'PUT',
      '/profile',
      body: {'first_name': firstName, 'last_name': lastName},
      accessToken: accessToken,
    );
    return CustomerProfile.fromJson(data);
  }

  Future<void> saveProfilePhotoUrl({
    required String accessToken,
    required String photoUrl,
    required String assetId,
  }) async {
    await _sendJson(
      'PUT',
      '/profile/photo-url',
      body: {'photo_url': photoUrl, 'asset_id': assetId},
      accessToken: accessToken,
    );
  }

  Future<List<EmergencyContact>> getEmergencyContacts({required String accessToken}) async {
    final data = await _sendJson('GET', '/profile/emergency-contacts', accessToken: accessToken);
    final raw = data['contacts'];
    if (raw is! List) return [];
    return raw
        .map((e) => EmergencyContact.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<EmergencyContact> addEmergencyContact({
    required String accessToken,
    required String name,
    required String phone,
    required String relationship,
  }) async {
    final data = await _sendJson(
      'POST',
      '/profile/emergency-contacts',
      accessToken: accessToken,
      body: {'name': name, 'phone': phone, 'relationship': relationship},
    );
    return EmergencyContact.fromJson(data);
  }

  Future<void> deleteEmergencyContact({
    required String accessToken,
    required String id,
  }) async {
    await _sendJson('DELETE', '/profile/emergency-contacts/$id', accessToken: accessToken);
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
        'PUT' => await _client.put(
          uri,
          headers: headers,
          body: jsonEncode(body ?? const {}),
        ),
        'DELETE' => await _client.delete(uri, headers: headers),
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
    } on ApiException catch (error) {
      if (accessToken != null && error.isAuthFailure) onAuthFailure?.call();
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
