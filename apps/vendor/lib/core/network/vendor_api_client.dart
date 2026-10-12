import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;
import '../config/vendor_app_config.dart';
import '../contracts/vendor_contract.dart';
import '../../features/auth/data/vendor_session_store.dart';
import '../../features/auth/models/vendor_session.dart';
import 'vendor_command.dart';
import 'vendor_response.dart';

class VendorApiClient {
  VendorApiClient({
    required this.config,
    required this.sessions,
    http.Client? client,
    this.onSessionChanged,
    this.onForbidden,
  }) : _http = client ?? http.Client(),
       _ownsHttp = client == null;
  final VendorAppConfig config;
  final VendorSessionStore sessions;
  final http.Client _http;
  final bool _ownsHttp;
  final void Function(VendorSession?)? onSessionChanged;
  final void Function(String)? onForbidden;
  Future<VendorSession>? _refreshing;
  Future<void> _storeQueue = Future.value();
  int _sessionEpoch = 0;

  Future<void> _store(Future<void> Function() action) {
    final next = _storeQueue.then((_) => action());
    _storeQueue = next.catchError((Object _) {});
    return next;
  }

  Future<void> acceptSession(VendorSession session) async {
    _sessionEpoch++;
    await _store(() => sessions.save(session));
    onSessionChanged?.call(session);
  }

  Future<void> clearSession() async {
    _sessionEpoch++;
    await _store(sessions.clear);
    onSessionChanged?.call(null);
  }

  Future<VendorSession> refreshSession() async {
    final pending = _refreshing;
    if (pending != null) return pending;
    final future = _refresh();
    _refreshing = future;
    try {
      return await future;
    } finally {
      if (identical(_refreshing, future)) _refreshing = null;
    }
  }

  Future<VendorSession> _refresh() async {
    final epoch = _sessionEpoch;
    await _storeQueue;
    final current = await sessions.read();
    if (current == null) {
      throw const ApiException(
        code: ApiErrorCode.unauthorized,
        message: 'Sign in required',
        statusCode: 401,
      );
    }
    try {
      final result = await _send(
        'POST',
        '/api/v1/vendor/auth/refresh',
        body: {'refresh_token': current.refreshToken},
      );
      final session = VendorSession.fromJson(result.object);
      if (epoch != _sessionEpoch) {
        throw StateError('Session changed during refresh');
      }
      await _store(() async {
        if (epoch == _sessionEpoch) await sessions.save(session);
      });
      if (epoch != _sessionEpoch) {
        throw StateError('Session changed during refresh');
      }
      onSessionChanged?.call(session);
      return session;
    } on ApiException catch (e) {
      if ((e.isAuthFailure || e.statusCode == 403) && epoch == _sessionEpoch) {
        await clearSession();
      }
      rethrow;
    }
  }

  Future<VendorResponse> call(
    VendorOperation operation, {
    Map<String, String> pathParameters = const {},
    Map<String, String> query = const {},
    VendorPayload? body,
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    bool retryAuthentication = true,
  }) async {
    var path = operation.path;
    final expected = RegExp(
      r'\{([^}]+)\}',
    ).allMatches(path).map((m) => m.group(1)!).toSet();
    if (expected.length != pathParameters.length ||
        !expected.every(pathParameters.containsKey)) {
      throw const VendorContractException(
        'Provide exactly the backend path parameters',
      );
    }
    for (final entry in pathParameters.entries) {
      if (entry.value.isEmpty) {
        throw const VendorContractException('Path parameter cannot be empty');
      }
      path = path.replaceAll(
        '{${entry.key}}',
        Uri.encodeComponent(entry.value),
      );
    }
    if (!path.startsWith('/api/v1/vendor/') || path.contains('/internal/')) {
      throw const VendorContractException('Not a Vendor public operation');
    }
    if (body != null) {
      if (!operation.requestKnown) {
        throw VendorContractException(
          'Request schema unresolved: ${operation.source}',
        );
      }
      final schema = operation.requestSchema;
      if (schema == null) {
        throw const VendorContractException('Operation has no request body');
      }
      validateRequest(body.toJson(), schema);
    } else if (operation.requestRequired) {
      throw const VendorContractException('Backend request body required');
    }
    final headers = <String, String>{};
    final declaredHeaders = operation
        .parametersIn('header')
        .map((p) => (p['name'] as String).toLowerCase())
        .toSet();
    for (final e in contractHeaders.entries) {
      if (!declaredHeaders.contains(e.key.toLowerCase()) ||
          [
            'authorization',
            'x-service-signature',
          ].contains(e.key.toLowerCase())) {
        throw VendorContractException('Undeclared request header ${e.key}');
      }
      headers[e.key] = e.value;
    }
    if (command?.idempotencyKey != null) {
      if (!declaredHeaders.contains('idempotency-key')) {
        throw const VendorContractException(
          'This operation does not declare Idempotency-Key',
        );
      }
      headers['Idempotency-Key'] = command!.idempotencyKey!;
    }
    if (command?.ifMatch != null) {
      if (!declaredHeaders.contains('if-match')) {
        throw const VendorContractException(
          'This command does not declare If-Match',
        );
      }
      headers['If-Match'] = command!.ifMatch!;
    }
    for (final p in operation.parametersIn('header')) {
      if (p['required'] == true &&
          !headers.keys.any(
            (k) => k.toLowerCase() == (p['name'] as String).toLowerCase(),
          )) {
        throw VendorContractException('Required header ${p['name']}');
      }
    }
    final queryNames = operation
        .parametersIn('query')
        .map((p) => p['name'] as String)
        .toSet();
    for (final k in query.keys) {
      if (!queryNames.contains(k)) {
        throw VendorContractException('Undeclared backend query parameter $k');
      }
    }
    for (final p in operation.parametersIn('query')) {
      if (p['required'] == true && !query.containsKey(p['name'])) {
        throw VendorContractException('Required query ${p['name']}');
      }
    }
    await _storeQueue;
    final initialEpoch = _sessionEpoch;
    final session = operation.authenticated ? await sessions.read() : null;
    if (operation.authenticated && session == null) {
      throw const ApiException(
        code: ApiErrorCode.unauthorized,
        message: 'Sign in required',
        statusCode: 401,
      );
    }
    Future<VendorResponse> send(String? token) => _send(
      operation.method,
      path,
      query: query,
      body: body?.toJson(),
      headers: headers,
      token: token,
    );
    try {
      return await send(session?.accessToken);
    } on ApiException catch (e) {
      if (e.statusCode == 403) onForbidden?.call(operation.id);
      if (!retryAuthentication ||
          (operation.method == 'POST' &&
              operation.path == '/api/v1/vendor/finance/withdrawals') ||
          !e.isAuthFailure ||
          !operation.authenticated ||
          initialEpoch != _sessionEpoch) {
        rethrow;
      }
      await _storeQueue;
      final latest = await sessions.read();
      if (latest == null) rethrow;
      final renewed = latest.accessToken != session!.accessToken
          ? latest
          : await refreshSession();
      if (initialEpoch != _sessionEpoch) {
        throw StateError('Session scope changed');
      }
      try {
        return await send(renewed.accessToken);
      } on ApiException catch (retryError) {
        if (retryError.isAuthFailure) await clearSession();
        if (retryError.statusCode == 403) onForbidden?.call(operation.id);
        rethrow;
      }
    }
  }

  Future<VendorResponse> _send(
    String method,
    String path, {
    Map<String, String> query = const {},
    Object? body,
    Map<String, String> headers = const {},
    String? token,
  }) async {
    final request = http.Request(
      method,
      config.api.uri(path, query.isEmpty ? null : query),
    );
    request.headers.addAll({'Accept': 'application/json', ...headers});
    if (token != null) request.headers['Authorization'] = 'Bearer $token';
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    http.Response response;
    try {
      response = await (() async => http.Response.fromStream(
        await _http.send(request),
      ))().timeout(config.timeout);
    } on TimeoutException {
      throw const ApiException(
        code: ApiErrorCode.network,
        message: 'Request timed out; outcome is not confirmed',
      );
    } on http.ClientException {
      throw const ApiException(
        code: ApiErrorCode.network,
        message: 'Request failed; outcome is not confirmed',
      );
    }
    JsonObject? envelope;
    if (response.body.isNotEmpty) {
      try {
        envelope = jsonObject(jsonDecode(response.body));
      } on FormatException {
        throw ApiException(
          code: ApiErrorCode.unknown,
          message: response.statusCode == 404
              ? 'This feature is not available on the connected server.'
              : response.statusCode == 401
              ? 'Your session has expired. Please sign in again.'
              : response.statusCode == 403
              ? 'The server denied access to this action.'
              : response.statusCode >= 500
              ? 'The service is temporarily unavailable. Please try again.'
              : 'The server returned an unexpected response. Please try again.',
          statusCode: response.statusCode,
        );
      }
    }
    if (response.statusCode < 200 ||
        response.statusCode >= 300 ||
        envelope?['success'] == false) {
      final e = ApiException.fromErrorEnvelope(
        envelope ?? const {},
        statusCode: response.statusCode,
      );
      throw ApiException(
        code: e.code,
        message: e.message,
        statusCode: e.statusCode,
        requestId: e.requestId ?? response.headers['x-request-id'],
        fields: e.fields,
        details: {
          ...e.details,
          if (response.headers['retry-after'] != null)
            'retry_after': response.headers['retry-after'],
        },
      );
    }
    if (response.statusCode != 204 && envelope?['success'] != true) {
      throw ApiException(
        code: ApiErrorCode.unknown,
        message: 'Missing backend success envelope',
        statusCode: response.statusCode,
      );
    }
    return VendorResponse(
      statusCode: response.statusCode,
      headers: response.headers,
      data: envelope?['data'],
      envelope: envelope,
    );
  }

  Future<Uint8List> crmExportFile(String id, String grant) async {
    if (id.isEmpty || grant.isEmpty) {
      throw const VendorContractException('Export verification is required.');
    }
    final epoch = _sessionEpoch;
    final key = VendorCommand.create().idempotencyKey!;
    Future<Uint8List> send(String token) async {
      final request = http.Request(
        'POST',
        Uri.parse(
          config.origin,
        ).resolve('/api/v1/vendor/crm/exports/${Uri.encodeComponent(id)}/file'),
      );
      request.followRedirects = false;
      request.headers.addAll({
        'Idempotency-Key': key,
        'Authorization': 'Bearer $token',
        'Content-Type': 'application/json',
        'Accept': 'text/csv, application/x-ndjson',
      });
      request.body = jsonEncode({'step_up_grant_id': grant});
      return (() async {
        final response = await _http.send(request);
        final builder = BytesBuilder(copy: false);
        await for (final part in response.stream) {
          if (builder.length + part.length > 20 * 1024 * 1024) {
            throw const VendorContractException(
              'Export exceeds the supported file size.',
            );
          }
          builder.add(part);
        }
        final bytes = builder.takeBytes();
        if (response.statusCode != 200) {
          JsonObject body = {};
          try {
            body = jsonObject(jsonDecode(utf8.decode(bytes)));
          } catch (_) {}
          throw ApiException.fromErrorEnvelope(
            body,
            statusCode: response.statusCode,
          );
        }
        final mime = response.headers['content-type']?.split(';').first.trim();
        if (!['text/csv', 'application/x-ndjson'].contains(mime)) {
          throw const VendorContractException('Unexpected export file type.');
        }
        if (epoch != _sessionEpoch) {
          throw const VendorContractException(
            'Your session changed. Verify the export again.',
          );
        }
        return bytes;
      })().timeout(config.timeout);
    }

    final session = await sessions.read();
    if (session == null) {
      throw const ApiException(
        code: ApiErrorCode.unauthorized,
        message: 'Sign in required',
        statusCode: 401,
      );
    }
    try {
      return await send(session.accessToken);
    } on ApiException catch (e) {
      if (e.statusCode != 401) rethrow;
      return send((await refreshSession()).accessToken);
    }
  }

  Future<Uint8List> mediaFile(
    String ref,
    String targetType,
    String targetRef,
  ) async {
    final epoch = _sessionEpoch;
    Future<Uint8List> send(String token) async {
      final uri = Uri.parse(config.origin)
          .resolve(
            '/api/v1/vendor/media/objects/${Uri.encodeComponent(ref)}/content',
          )
          .replace(
            queryParameters: {
              'target_type': targetType,
              'target_ref': targetRef,
            },
          );
      final request = http.Request('GET', uri)..followRedirects = false;
      request.headers['Authorization'] = 'Bearer $token';
      return (() async {
        final response = await _http.send(request);
        final builder = BytesBuilder(copy: false);
        await for (final chunk in response.stream) {
          if (builder.length + chunk.length > 10 * 1024 * 1024) {
            throw const VendorContractException('File exceeds 10 MB.');
          }
          builder.add(chunk);
        }
        final bytes = builder.takeBytes();
        if (response.statusCode != 200) {
          JsonObject body = {};
          try {
            body = jsonObject(jsonDecode(utf8.decode(bytes)));
          } catch (_) {}
          throw ApiException.fromErrorEnvelope(
            body,
            statusCode: response.statusCode,
          );
        }
        final mime = response.headers['content-type']?.split(';').first.trim();
        if (![
          'image/jpeg',
          'image/png',
          'image/webp',
          'application/pdf',
        ].contains(mime)) {
          throw const VendorContractException('Unsupported file type.');
        }
        if (epoch != _sessionEpoch) {
          throw const VendorContractException(
            'Your session changed. Open the file again.',
          );
        }
        return bytes;
      })().timeout(config.timeout);
    }

    final session = await sessions.read();
    if (session == null) {
      throw const VendorContractException('Sign in required.');
    }
    try {
      return await send(session.accessToken);
    } on ApiException catch (e) {
      if (e.statusCode != 401) rethrow;
      return send((await refreshSession()).accessToken);
    }
  }

  void close() {
    if (_ownsHttp) _http.close();
  }
}
