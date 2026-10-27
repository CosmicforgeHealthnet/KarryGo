import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/referral_dashboard.dart';

/// Client for referral-service. It accepts the driver's own login token.
class ReferralApi {
  ReferralApi({required ApiCoreConfig config, http.Client? client})
      : _config = config,
        _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  Future<ReferralDashboard> getDashboard({required String accessToken}) async {
    final http.Response response;
    try {
      response = await _client.get(_config.uri('/me/dashboard'), headers: {
        'Accept': 'application/json',
        'Authorization': 'Bearer $accessToken',
      });
    } catch (e) {
      throw ApiException.network(e);
    }
    final decoded = response.body.isEmpty
        ? <String, dynamic>{}
        : Map<String, dynamic>.from(jsonDecode(response.body) as Map);
    if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
      throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
    }
    return ReferralDashboard.fromJson(Map<String, dynamic>.from(decoded['data'] as Map));
  }
}
