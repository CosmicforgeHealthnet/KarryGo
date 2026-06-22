import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/provider_auth_models.dart';

class ProviderAuthApi {
  ProviderAuthApi({required ApiCoreConfig config, http.Client? client})
    : _config = config,
      _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  Future<OtpChallenge> startAuth({String phone = '', String email = ''}) async {
    final body = <String, dynamic>{'challenge_id': null};
    body.remove('challenge_id');
    if (phone.isNotEmpty) body['phone'] = phone;
    if (email.isNotEmpty) body['email'] = email;
    final data = await _post('/provider/auth/start', body);
    return OtpChallenge.fromJson(data);
  }

  Future<ProviderSession> verifyOtp({
    required String challengeId,
    String phone = '',
    String email = '',
    required String code,
  }) async {
    final body = <String, dynamic>{
      'challenge_id': challengeId,
      'otp': code,
    };
    if (phone.isNotEmpty) body['phone'] = phone;
    if (email.isNotEmpty) body['email'] = email;
    final data = await _post('/provider/auth/verify', body);
    return ProviderSession.fromJson(data);
  }

  Future<ProviderSession> refreshSession({required String refreshToken}) async {
    final data = await _post('/provider/auth/refresh', {'refresh_token': refreshToken});
    return ProviderSession.fromJson(data);
  }

  Future<void> logout({required String accessToken, required String refreshToken}) async {
    await _post('/provider/auth/logout', {'refresh_token': refreshToken}, accessToken: accessToken);
  }

  Future<TruckProvider> getMe({required String accessToken}) async {
    final data = await _get('/provider/me', accessToken: accessToken);
    return TruckProvider.fromJson(data);
  }

  Future<void> updateOnboardingProfile({
    required String accessToken,
    required String firstName,
    required String lastName,
    required String email,
    required String locationState,
    required String locationCity,
    required String operationMode,
    required String serviceType,
    required String govIdUrl,
    required String driverLicenseUrl,
    required String vehicleRegUrl,
    required String guarantorName,
    required String guarantorPhone,
    required String emergencyContactName,
    required String emergencyContactPhone,
    required String emergencyContactRelationship,
    required String profilePhotoUrl,
    required String photoAssetId,
  }) async {
    await _put('/provider/profile', {
      'first_name': firstName,
      'last_name': lastName,
      'email': email,
      'location_state': locationState,
      'location_city': locationCity,
      'operation_mode': operationMode,
      'service_type': serviceType,
      'gov_id_url': govIdUrl,
      'driver_license_url': driverLicenseUrl,
      'vehicle_reg_url': vehicleRegUrl,
      'guarantor_name': guarantorName,
      'guarantor_phone': guarantorPhone,
      'emergency_contact_name': emergencyContactName,
      'emergency_contact_phone': emergencyContactPhone,
      'emergency_contact_relationship': emergencyContactRelationship,
      'profile_photo_url': profilePhotoUrl,
      'photo_asset_id': photoAssetId,
      'submit_for_verification': true,
    }, accessToken: accessToken);
  }

  void close() => _client.close();

  Future<Map<String, dynamic>> _get(String path, {required String accessToken}) async {
    try {
      final response = await _client.get(
        _config.uri(path),
        headers: {
          'Authorization': 'Bearer $accessToken',
          'Accept': 'application/json',
        },
      );
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _post(String path, Map<String, dynamic> body, {String? accessToken}) async {
    try {
      final response = await _client.post(
        _config.uri(path),
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json',
          if (accessToken != null) 'Authorization': 'Bearer $accessToken',
        },
        body: jsonEncode(body),
      );
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _put(String path, Map<String, dynamic> body, {required String accessToken}) async {
    try {
      final response = await _client.put(
        _config.uri(path),
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'application/json',
          'Authorization': 'Bearer $accessToken',
        },
        body: jsonEncode(body),
      );
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, dynamic> _unwrap(http.Response response) {
    final decoded = response.body.isEmpty
        ? <String, dynamic>{'success': true, 'data': <String, dynamic>{}}
        : Map<String, dynamic>.from(jsonDecode(response.body) as Map);

    if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
      throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
    }
    final raw = decoded['data'];
    if (raw is Map) return Map<String, dynamic>.from(raw);
    return const {};
  }
}
