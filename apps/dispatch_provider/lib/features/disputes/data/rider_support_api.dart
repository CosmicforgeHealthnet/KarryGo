import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/dispute_models.dart';

/// Client for the support-dispute-service dispatch-rider surface
/// (`/dispatch-provider/...`). Uses the rider's dispatch bearer token (the
/// service verifies role=dispatch_provider, service=dispatch).
class RiderSupportApi {
  RiderSupportApi({required ApiCoreConfig config, http.Client? client})
    : _config = config,
      _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  Future<List<Complaint>> listComplaints({required String accessToken}) async {
    final data = await _get(
      '/dispatch-provider/complaints',
      accessToken: accessToken,
    );
    final raw = data['complaints'];
    if (raw is List) {
      return raw
          .map((e) => Complaint.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList();
    }
    throw const ApiException(
      code: ApiErrorCode.unknown,
      message: 'Support returned an unreadable list. Please retry.',
    );
  }

  Future<Complaint> createComplaint({
    required String accessToken,
    required String subject,
    required String description,
    required String bookingReference,
    String? category,
  }) async {
    final data = await _post('/dispatch-provider/complaints', {
      'service_type': 'dispatch',
      'booking_reference': bookingReference,
      if (category != null) 'category': category,
      'subject': subject,
      'description': description,
    }, accessToken: accessToken);
    return _complaint(data);
  }

  Future<Complaint> getComplaint({
    required String accessToken,
    required String id,
  }) async {
    final data = await _get(
      '/dispatch-provider/complaints/$id',
      accessToken: accessToken,
    );
    return Complaint.fromJson(data);
  }

  Future<Complaint> startSupportChat({required String accessToken}) async {
    final data = await _post(
      '/dispatch-provider/support-chat/start',
      const {},
      accessToken: accessToken,
    );
    return _complaint(data);
  }

  Complaint _complaint(Map<String, dynamic> data) {
    final complaint = Complaint.fromJson(data);
    if (complaint.id.isEmpty)
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message:
            'The server did not confirm the support case. Refresh your cases before retrying.',
      );
    return complaint;
  }

  Future<List<DisputeMessage>> listMessages({
    required String accessToken,
    required String id,
  }) async {
    final data = await _get(
      '/dispatch-provider/complaints/$id/messages',
      accessToken: accessToken,
    );
    final raw = data['messages'];
    if (raw is List) {
      return raw
          .map(
            (e) => DisputeMessage.fromJson(Map<String, dynamic>.from(e as Map)),
          )
          .toList();
    }
    throw const ApiException(
      code: ApiErrorCode.unknown,
      message: 'Support returned an unreadable list. Please retry.',
    );
  }

  Future<DisputeMessage> sendMessage({
    required String accessToken,
    required String id,
    required String content,
  }) async {
    final data = await _post('/dispatch-provider/complaints/$id/messages', {
      'content': content,
    }, accessToken: accessToken);
    final message = DisputeMessage.fromJson(data);
    if (message.id.isEmpty)
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message:
            'Message delivery was not confirmed. Refresh the conversation before retrying.',
      );
    return message;
  }

  /// Marks the admin/agent messages on a complaint as read.
  Future<void> markMessagesRead({
    required String accessToken,
    required String id,
  }) async {
    await _post(
      '/dispatch-provider/complaints/$id/messages/read',
      const {},
      accessToken: accessToken,
    );
  }

  void close() => _client.close();

  // ─── HTTP helpers ─────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> _get(
    String path, {
    required String accessToken,
  }) async {
    try {
      final response = await _client
          .get(_config.uri(path), headers: _headers(accessToken))
          .timeout(const Duration(seconds: 20));
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body, {
    required String accessToken,
  }) async {
    try {
      final response = await _client
          .post(
            _config.uri(path),
            headers: _headers(accessToken),
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 20));
      return _unwrap(response);
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
  }
}
