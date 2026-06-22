import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/support_models.dart';

class SupportApi {
  SupportApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  })  : _config = config,
        _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Called whenever a request fails with an authentication error (401 /
  /// unauthorized), to drive a global logout on session expiry/revocation.
  final void Function()? onAuthFailure;

  Future<Complaint> createComplaint({
    required String accessToken,
    required String serviceType,
    required String subject,
    required String description,
    String? bookingReference,
  }) async {
    final data = await _sendJson(
      'POST',
      '/complaints',
      accessToken: accessToken,
      body: {
        'service_type': serviceType,
        'subject': subject,
        'description': description,
        if (bookingReference != null && bookingReference.isNotEmpty)
          'booking_reference': bookingReference,
      },
    );
    return Complaint.fromJson(data);
  }

  Future<List<Complaint>> listMyComplaints({
    required String accessToken,
    int limit = 50,
    int offset = 0,
  }) async {
    final data = await _sendJson(
      'GET',
      '/complaints?limit=$limit&offset=$offset',
      accessToken: accessToken,
    );
    final raw = data['complaints'];
    if (raw is! List) return [];
    return raw
        .map((e) => Complaint.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<Complaint> getComplaint({
    required String accessToken,
    required String id,
  }) async {
    final data = await _sendJson(
      'GET',
      '/complaints/$id',
      accessToken: accessToken,
    );
    return Complaint.fromJson(data);
  }

  Future<Complaint> startSupportChat({required String accessToken}) async {
    final data = await _sendJson(
      'POST',
      '/support-chat/start',
      accessToken: accessToken,
    );
    return Complaint.fromJson(data);
  }

  Future<ChatMessage> sendMessage({
    required String accessToken,
    required String complaintId,
    required String content,
  }) async {
    final data = await _sendJson(
      'POST',
      '/complaints/$complaintId/messages',
      accessToken: accessToken,
      body: {'content': content},
    );
    return ChatMessage.fromJson(data);
  }

  Future<List<ChatMessage>> listMessages({
    required String accessToken,
    required String complaintId,
    int limit = 50,
    int offset = 0,
  }) async {
    final data = await _sendJson(
      'GET',
      '/complaints/$complaintId/messages?limit=$limit&offset=$offset',
      accessToken: accessToken,
    );
    final raw = data['messages'];
    if (raw is! List) return [];
    return raw
        .map((e) => ChatMessage.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  void close() => _client.close();

  Future<Map<String, dynamic>> _sendJson(
    String method,
    String path, {
    required String accessToken,
    Map<String, dynamic>? body,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        'Authorization': 'Bearer $accessToken',
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
        throw ApiException.fromErrorEnvelope(decoded,
            statusCode: response.statusCode);
      }
      if (decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(decoded,
            statusCode: response.statusCode);
      }

      final rawData = decoded['data'];
      if (rawData is Map) return Map<String, dynamic>.from(rawData);
      return const {};
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, dynamic> _decodeResponse(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {
      'success': false,
      'error': {'code': 'unknown', 'message': 'Something went wrong.'}
    };
  }
}
