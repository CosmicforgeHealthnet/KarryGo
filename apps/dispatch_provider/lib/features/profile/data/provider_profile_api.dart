import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../models/provider_profile_models.dart';

class ProviderProfileApi {
  final ApiCoreConfig config;
  final http.Client _client;

  ProviderProfileApi(this.config, {http.Client? client})
    : _client = client ?? http.Client();

  Map<String, String> _headers(String accessToken) {
    return {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer $accessToken',
    };
  }

  Future<ApiResult<ProviderProfile>> getMe(String accessToken) async {
    try {
      final uri = config.uri('/api/v1/provider/me');
      final response = await _client
          .get(uri, headers: _headers(accessToken))
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(ProviderProfile.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<ProviderProfile>> submitOnboarding({
    required String accessToken,
    required String fullName,
    String? email,
    required String state,
    required String city,
    required String operationType,
  }) async {
    try {
      final uri = config.uri('/api/v1/provider/onboarding');
      final requestBody = {
        'full_name': fullName,
        if (email != null && email.isNotEmpty) 'email': email,
        'state': state,
        'city': city,
        'operation_type': operationType,
      };

      final response = await _client
          .post(
            uri,
            headers: _headers(accessToken),
            body: jsonEncode(requestBody),
          )
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(ProviderProfile.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<ProviderProfile>> updateMe({
    required String accessToken,
    String? fullName,
    String? email,
    String? state,
    String? city,
    String? profilePhotoUrl,
  }) async {
    try {
      final uri = config.uri('/api/v1/provider/me');
      final requestBody = {
        'full_name': ?fullName,
        'email': ?email,
        'state': ?state,
        'city': ?city,
        'profile_photo_url': ?profilePhotoUrl,
      };

      final response = await _client
          .patch(
            uri,
            headers: _headers(accessToken),
            body: jsonEncode(requestBody),
          )
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(ProviderProfile.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<EmergencyContact>> upsertEmergencyContact({
    required String accessToken,
    required String fullName,
    required String phone,
    required String relationship,
  }) async {
    try {
      final uri = config.uri('/api/v1/provider/emergency-contact');
      final requestBody = {
        'full_name': fullName,
        'phone': phone,
        'relationship': relationship,
      };

      final response = await _client
          .post(
            uri,
            headers: _headers(accessToken),
            body: jsonEncode(requestBody),
          )
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(EmergencyContact.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<EmergencyContact>> getEmergencyContact(
    String accessToken,
  ) async {
    try {
      final uri = config.uri('/api/v1/provider/emergency-contact');
      final response = await _client
          .get(uri, headers: _headers(accessToken))
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(EmergencyContact.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<Guarantor>> upsertGuarantor({
    required String accessToken,
    required String fullName,
    required String phone,
  }) async {
    try {
      final uri = config.uri('/api/v1/provider/guarantor');
      final requestBody = {'full_name': fullName, 'phone': phone};

      final response = await _client
          .post(
            uri,
            headers: _headers(accessToken),
            body: jsonEncode(requestBody),
          )
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(Guarantor.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<Guarantor>> getGuarantor(String accessToken) async {
    try {
      final uri = config.uri('/api/v1/provider/guarantor');
      final response = await _client
          .get(uri, headers: _headers(accessToken))
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(Guarantor.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<ProviderStats>> getStats(String accessToken) async {
    try {
      final uri = config.uri('/api/v1/provider/stats');
      final response = await _client
          .get(uri, headers: _headers(accessToken))
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(ProviderStats.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<PublicProviderProfile>> getPublicProfile(
    String providerId,
  ) async {
    try {
      final uri = config.uri('/api/v1/provider/$providerId/public');
      final response = await _client
          .get(uri, headers: {'Content-Type': 'application/json'})
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(PublicProviderProfile.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<bool>> deleteAccount(String accessToken) async {
    try {
      final uri = config.uri('/api/v1/provider/me');
      final response = await _client
          .delete(uri, headers: _headers(accessToken))
          .timeout(const Duration(seconds: 15));

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          return const ApiSuccess(true);
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }

  Future<ApiResult<ProviderProfile>> uploadAvatar({
    required String accessToken,
    required String filePath,
  }) async {
    try {
      final uri = config.uri('/api/v1/provider/me/avatar');
      final request = http.MultipartRequest('POST', uri)
        ..headers['Authorization'] = 'Bearer $accessToken'
        ..files.add(await http.MultipartFile.fromPath('avatar', filePath));

      final streamed = await request.send().timeout(const Duration(seconds: 30));
      final response = await http.Response.fromStream(streamed);

      final Map<String, dynamic> body = jsonDecode(response.body);
      if (response.statusCode == 200) {
        if (body['success'] == true) {
          final data = body['data'] as Map<String, dynamic>;
          return ApiSuccess(ProviderProfile.fromJson(data));
        }
      }
      return ApiFailure(
        ApiException.fromErrorEnvelope(body, statusCode: response.statusCode),
      );
    } catch (e) {
      return ApiFailure(ApiException.network(e));
    }
  }
}
