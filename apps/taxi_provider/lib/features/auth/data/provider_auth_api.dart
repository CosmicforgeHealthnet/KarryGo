import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/provider_auth_models.dart';

/// HTTP client for the taxi driver's own surface of driver-taxi-service
/// (base: /api/v1/taxi). Auth is phone-only OTP — the service issues no
/// email challenge for drivers.
class ProviderAuthApi {
  ProviderAuthApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;
  final void Function()? onAuthFailure;

  Future<OtpChallenge> startAuth({required String phone}) async {
    final data = await _sendJson('POST', '/provider/auth/start', body: {
      'phone': phone,
    });
    return OtpChallenge.fromJson(data);
  }

  Future<ProviderSession> verifyAuth({
    required String phone,
    required String challengeId,
    required String otp,
  }) async {
    final data = await _sendJson('POST', '/provider/auth/verify', body: {
      'phone': phone,
      'challenge_id': challengeId,
      'otp': otp,
    });
    return ProviderSession.fromJson(data);
  }

  Future<ProviderSession> refresh({required String refreshToken}) async {
    final data = await _sendJson('POST', '/provider/auth/refresh', body: {
      'refresh_token': refreshToken,
    });
    return ProviderSession.fromJson(data);
  }

  Future<void> logout({required String refreshToken}) async {
    await _sendJson('POST', '/provider/auth/logout', body: {
      'refresh_token': refreshToken,
    });
  }

  // ─── Profile + vehicles ────────────────────────────────────────────────────

  Future<TaxiProvider> getProfile({required String accessToken}) async {
    final data = await _sendJson(
      'GET',
      '/provider/profile',
      accessToken: accessToken,
    );
    return TaxiProvider.fromJson(data);
  }

  /// Toggles the driver's opt-in to carrying packages. Its own endpoint, not
  /// part of the profile update: that path preserves fields left empty, which
  /// cannot express "false" for a boolean.
  Future<TaxiProvider> setParcelPreference({
    required String accessToken,
    required bool acceptsParcels,
  }) async {
    final data = await _sendJson(
      'PUT',
      '/provider/parcel-preference',
      accessToken: accessToken,
      body: {'accepts_parcels': acceptsParcels},
    );
    return TaxiProvider.fromJson(data);
  }

  Future<List<ProviderVehicle>> listVehicles({
    required String accessToken,
  }) async {
    final data = await _sendJson(
      'GET',
      '/provider/vehicles',
      accessToken: accessToken,
    );
    final raw = data['vehicles'];
    if (raw is! List) return const [];
    return raw
        .map((e) => ProviderVehicle.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  void close() => _client.close();

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
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decode(response);
      if (response.statusCode < 200 ||
          response.statusCode >= 300 ||
          decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }
      final raw = decoded['data'];
      if (raw is Map) return Map<String, dynamic>.from(raw);
      return const {};
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Map<String, dynamic> _decode(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {'success': false};
  }
}
