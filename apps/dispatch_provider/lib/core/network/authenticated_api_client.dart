import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

typedef RefreshSession = Future<bool> Function();
typedef SessionExpired = FutureOr<void> Function();

class AuthenticatedApiClient extends http.BaseClient {
  AuthenticatedApiClient({
    required this.getAccessToken,
    required this.refreshSession,
    this.onSessionExpired,
    this.requestTimeout = const Duration(seconds: 30),
    http.Client? inner,
  }) : _inner = inner ?? http.Client();

  final String? Function() getAccessToken;
  final RefreshSession refreshSession;
  final SessionExpired? onSessionExpired;
  final Duration requestTimeout;
  final http.Client _inner;

  Future<bool>? _refreshInFlight;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final bodyBytes = await request.finalize().toBytes();
    final details = _BufferedRequest.from(request, bodyBytes);

    final response = await _sendBuffered(details);
    if (response.statusCode != 401) {
      return response;
    }

    final responseBytes = await response.stream.toBytes();
    _debugLog('[API] 401 from ${details.method} ${details.path}; refreshing.');

    final refreshed = await _tryRefresh();
    if (!refreshed) {
      await onSessionExpired?.call();
      return _copyResponse(response, responseBytes);
    }

    final retryResponse = await _sendBuffered(details);
    if (retryResponse.statusCode == 401) {
      await onSessionExpired?.call();
    }
    return retryResponse;
  }

  Future<http.StreamedResponse> _sendBuffered(_BufferedRequest details) async {
    final replay = http.StreamedRequest(details.method, details.url)
      ..followRedirects = details.followRedirects
      ..maxRedirects = details.maxRedirects
      ..persistentConnection = details.persistentConnection
      ..contentLength = details.bodyBytes.length;

    replay.headers.addAll(details.headers);

    final token = getAccessToken()?.trim();
    if (token != null && token.isNotEmpty) {
      replay.headers['Authorization'] = 'Bearer $token';
    }

    _debugLog('[API] -> ${details.method} ${details.url}');
    try {
      replay.sink.add(details.bodyBytes);
      // Do NOT await sink.close() — the StreamController.done future requires
      // a subscriber, but the only subscriber is _inner.send(replay) below.
      // Awaiting here before send() deadlocks the request permanently.
      replay.sink.close(); // ignore: discarded_futures

      _debugLog('[API] sink closed, starting inner send');
      final response = await _inner.send(replay).timeout(requestTimeout);
      _debugLog(
        '[API] <- ${details.method} ${details.path} '
        'status=${response.statusCode}',
      );
      return response;
    } catch (error) {
      _debugLog(
        '[API] !! ${details.method} ${details.url}: ${_shortError(error)}',
      );
      rethrow;
    }
  }

  Future<bool> _tryRefresh() {
    final activeRefresh = _refreshInFlight;
    if (activeRefresh != null) {
      return activeRefresh;
    }

    final refresh = () async {
      try {
        return await refreshSession().timeout(requestTimeout);
      } catch (error) {
        _debugLog('[API] refresh failed: ${_shortError(error)}');
        return false;
      }
    }();

    _refreshInFlight = refresh;
    refresh.whenComplete(() => _refreshInFlight = null);
    return refresh;
  }

  static http.StreamedResponse _copyResponse(
    http.StreamedResponse response,
    List<int> bodyBytes,
  ) {
    return http.StreamedResponse(
      Stream<List<int>>.fromIterable([bodyBytes]),
      response.statusCode,
      contentLength: bodyBytes.length,
      request: response.request,
      headers: response.headers,
      isRedirect: response.isRedirect,
      persistentConnection: response.persistentConnection,
      reasonPhrase: response.reasonPhrase,
    );
  }

  static void _debugLog(String message) {
    if (kDebugMode) {
      debugPrint(message);
    }
  }

  static String _shortError(Object error) {
    final message = error.toString();
    return message.length > 160 ? '${message.substring(0, 160)}...' : message;
  }

  @override
  void close() {
    _inner.close();
    super.close();
  }
}

class _BufferedRequest {
  const _BufferedRequest({
    required this.method,
    required this.url,
    required this.headers,
    required this.bodyBytes,
    required this.followRedirects,
    required this.maxRedirects,
    required this.persistentConnection,
  });

  factory _BufferedRequest.from(http.BaseRequest request, List<int> bodyBytes) {
    return _BufferedRequest(
      method: request.method,
      url: request.url,
      headers: Map<String, String>.from(request.headers),
      bodyBytes: bodyBytes,
      followRedirects: request.followRedirects,
      maxRedirects: request.maxRedirects,
      persistentConnection: request.persistentConnection,
    );
  }

  final String method;
  final Uri url;
  final Map<String, String> headers;
  final List<int> bodyBytes;
  final bool followRedirects;
  final int maxRedirects;
  final bool persistentConnection;

  String get path => url.hasQuery ? '${url.path}?${url.query}' : url.path;
}
