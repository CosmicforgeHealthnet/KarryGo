import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../../auth/models/provider_auth_models.dart';

/// HTTP client for the provider profile, verification, truck, and phone-change
/// endpoints on the hauling-service (all bearer-protected, role=truck_provider).
class ProviderProfileApi {
  ProviderProfileApi({required ApiCoreConfig config, http.Client? client})
      : _config = config,
        _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  // ─── Profile ────────────────────────────────────────────────────────────

  Future<TruckProvider> getProfile({required String accessToken}) async {
    final data = await _get('/provider/profile', accessToken: accessToken);
    return TruckProvider.fromJson(data);
  }

  /// Full profile update. Empty strings for url/photo fields are ignored by the
  /// backend (kept at their current value), so callers only send what changed.
  Future<TruckProvider> updateProfile({
    required String accessToken,
    String firstName = '',
    String lastName = '',
    String email = '',
    String locationState = '',
    String locationCity = '',
    String language = '',
    String driverLicenseNumber = '',
    String licenseExpiryYear = '',
    String licenseExpiryDate = '',
    String govIdUrl = '',
    String driverLicenseUrl = '',
    String vehicleRegUrl = '',
    String profilePhotoUrl = '',
    String photoAssetId = '',
    bool submitForVerification = false,
  }) async {
    final data = await _put('/provider/profile', {
      'first_name': firstName,
      'last_name': lastName,
      'email': email,
      'location_state': locationState,
      'location_city': locationCity,
      'language': language,
      'driver_license_number': driverLicenseNumber,
      'license_expiry_year': licenseExpiryYear,
      'license_expiry_date': licenseExpiryDate,
      'gov_id_url': govIdUrl,
      'driver_license_url': driverLicenseUrl,
      'vehicle_reg_url': vehicleRegUrl,
      'profile_photo_url': profilePhotoUrl,
      'photo_asset_id': photoAssetId,
      'submit_for_verification': submitForVerification,
    }, accessToken: accessToken);
    return TruckProvider.fromJson(data);
  }

  // ─── Phone change ──────────────────────────────────────────────────────────

  Future<OtpChallenge> changePhoneStart({
    required String accessToken,
    required String phone,
  }) async {
    final data = await _post('/provider/phone/change/start', {'phone': phone}, accessToken: accessToken);
    return OtpChallenge.fromJson(data);
  }

  Future<TruckProvider> changePhoneVerify({
    required String accessToken,
    required String phone,
    required String otp,
    required String challengeId,
  }) async {
    final data = await _post('/provider/phone/change/verify', {
      'phone': phone,
      'otp': otp,
      'challenge_id': challengeId,
    }, accessToken: accessToken);
    return TruckProvider.fromJson(data);
  }

  // ─── Trucks ─────────────────────────────────────────────────────────────

  Future<List<ProviderTruck>> listTrucks({required String accessToken}) async {
    final list = await _getList('/provider/trucks', accessToken: accessToken);
    return list.map((e) => ProviderTruck.fromJson(Map<String, dynamic>.from(e as Map))).toList();
  }

  Future<ProviderTruck> createTruck({
    required String accessToken,
    required Map<String, dynamic> body,
  }) async {
    final data = await _post('/provider/trucks', body, accessToken: accessToken);
    return ProviderTruck.fromJson(data);
  }

  Future<ProviderTruck> updateTruck({
    required String accessToken,
    required String id,
    required Map<String, dynamic> body,
  }) async {
    final data = await _put('/provider/trucks/$id', body, accessToken: accessToken);
    return ProviderTruck.fromJson(data);
  }

  void close() => _client.close();

  // ─── HTTP helpers ─────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> _get(String path, {required String accessToken}) async {
    try {
      final response = await _client.get(_config.uri(path), headers: _headers(accessToken));
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _post(String path, Map<String, dynamic> body, {required String accessToken}) async {
    try {
      final response = await _client.post(_config.uri(path), headers: _headers(accessToken), body: jsonEncode(body));
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _put(String path, Map<String, dynamic> body, {required String accessToken}) async {
    try {
      final response = await _client.put(_config.uri(path), headers: _headers(accessToken), body: jsonEncode(body));
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<List<dynamic>> _getList(String path, {required String accessToken}) async {
    try {
      final response = await _client.get(_config.uri(path), headers: _headers(accessToken));
      final decoded = response.body.isEmpty
          ? <String, dynamic>{'success': true, 'data': const []}
          : Map<String, dynamic>.from(jsonDecode(response.body) as Map);
      if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final raw = decoded['data'];
      if (raw is List) return raw;
      return const [];
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, String> _headers(String accessToken) => {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Authorization': 'Bearer $accessToken',
      };

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
