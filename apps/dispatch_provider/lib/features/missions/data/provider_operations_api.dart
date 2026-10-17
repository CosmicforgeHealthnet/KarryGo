import 'dart:convert';
import 'dart:async';
import 'dart:math';

import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

class ProviderOperationsApi {
  ProviderOperationsApi({
    required this.origin,
    required this.client,
    required this.token,
    this.onUncertainEvidence,
  });
  final String origin;
  final http.Client client;
  final String? Function() token;
  final Future<void> Function(List<String>, String)? onUncertainEvidence;
  final _pending = <String, String>{};

  static String newOperationKey() {
    final random = Random.secure();
    return List.generate(
      16,
      (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
    ).join();
  }

  Map<String, String> get _headers => {
    'Authorization': 'Bearer ${token() ?? ''}',
    'Accept': 'application/json',
  };

  Future<Map<String, dynamic>> readTrip(String tripId) async {
    final response = await client
        .get(
          Uri.parse(
            '$origin/api/v1/provider/trips/${Uri.encodeComponent(tripId)}',
          ),
          headers: _headers,
        )
        .timeout(const Duration(seconds: 25));
    return decode(response);
  }

  Future<Map<String, dynamic>> read(String path) async {
    return decode(
      await client
          .get(Uri.parse('$origin$path'), headers: _headers)
          .timeout(const Duration(seconds: 25)),
    );
  }

  Future<Map<String, dynamic>> command(
    String path,
    Map<String, dynamic> body, {
    bool operationRef = false,
  }) async {
    final fingerprint = '$path:${jsonEncode(body)}';
    final key = _pending.putIfAbsent(fingerprint, newOperationKey);
    try {
      final response = await client
          .post(
            Uri.parse('$origin$path'),
            headers: {
              ..._headers,
              'Content-Type': 'application/json',
              'Idempotency-Key': key,
            },
            body: jsonEncode({...body, if (operationRef) 'operation_ref': key}),
          )
          .timeout(const Duration(seconds: 35));
      final result = decode(response);
      _pending.remove(fingerprint);
      return result;
    } catch (error) {
      final uncertain =
          error is http.ClientException ||
          error is TimeoutException ||
          (error is ApiException &&
              (error.statusCode == null || error.statusCode! >= 500));
      if (uncertain && onUncertainEvidence != null) {
        final refs = <String>{};
        if (body['media_ref'] is String) refs.add(body['media_ref'] as String);
        for (final source in [
          body,
          if (body['evidence'] is Map) body['evidence'] as Map,
        ]) {
          for (final value in source['media_refs'] as List? ?? []) {
            if (value is String) refs.add(value);
          }
        }
        if (refs.isNotEmpty) {
          try {
            await onUncertainEvidence!(refs.toList(), path);
          } catch (_) {}
        }
      }
      rethrow;
    }
  }

  Future<String> upload(
    String path,
    String sessionRef,
    String photoPath, {
    Map<String, String> fields = const {},
  }) async {
    final request = http.MultipartRequest('POST', Uri.parse('$origin$path'));
    request.headers.addAll(_headers);
    if (sessionRef.isNotEmpty)
      request.fields['capture_session_ref'] = sessionRef;
    request.fields.addAll(fields);
    if (fields['operation_ref'] != null)
      request.headers['Idempotency-Key'] = fields['operation_ref']!;
    request.files.add(await http.MultipartFile.fromPath('photo', photoPath));
    final response = await http.Response.fromStream(
      await client.send(request).timeout(const Duration(seconds: 60)),
    );
    final data = decode(response);
    final ref = data['media_ref']?.toString() ?? '';
    if (ref.isEmpty)
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message: 'The photo upload could not be confirmed.',
      );
    return ref;
  }

  static Map<String, dynamic> decode(http.Response response) {
    Map<String, dynamic> envelope;
    try {
      envelope = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message: 'The server returned an unreadable response.',
      );
    }
    if (response.statusCode < 200 ||
        response.statusCode >= 300 ||
        envelope['success'] == false) {
      throw ApiException.fromErrorEnvelope(
        envelope,
        statusCode: response.statusCode,
      );
    }
    return Map<String, dynamic>.from(envelope['data'] as Map? ?? {});
  }
}
