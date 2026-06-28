import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../models/hauling_models.dart';

/// Opens and maintains the customer's realtime notification websocket.
///
/// This is the **fast path** for booking-status updates: when a push arrives,
/// [onEvent] is invoked with the event type so the booking controller can
/// refresh immediately instead of waiting for the next poll. The poll remains
/// the safety net, so a dropped socket never strands the customer.
///
/// Mirrors the truck-provider app's ProviderRealtimeListener. The realtime token
/// is re-minted on each (re)connect, so token expiry is handled by reconnect.
class CustomerRealtimeListener {
  CustomerRealtimeListener({
    required Future<RealtimeToken?> Function(String accessToken) fetchToken,
    required String wsUrl,
    required this.accessToken,
    required this.onEvent,
  })  : _fetchToken = fetchToken,
        _wsUrl = wsUrl;

  /// Fetches a realtime token. Returns null when the proxy is not configured
  /// (hauling-service reported 404 for the realtime-token endpoint).
  final Future<RealtimeToken?> Function(String accessToken) _fetchToken;
  final String _wsUrl;

  /// Returns the current customer access token, or null if signed out.
  final String? Function() accessToken;

  /// Called with the event_type of each pushed notification.
  final void Function(String eventType) onEvent;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _sub;
  Timer? _reconnectTimer;
  int _attempts = 0;
  bool _active = false;
  // True when the backend returned 404 (proxy route not configured). We stop all
  // reconnect attempts and fall back to the booking controller's 5s poll instead.
  bool _disabled = false;

  // Maximum backoff steps before giving up and relying on polling.
  static const _maxReconnectAttempts = 6;

  /// Opens the connection. Idempotent.
  Future<void> start() async {
    if (_active) return;
    _active = true;
    await _connect();
  }

  /// Closes the connection and stops reconnecting.
  void stop() {
    _active = false;
    _disabled = false; // reset so a fresh login can try again
    _reconnectTimer?.cancel();
    _reconnectTimer = null;
    _sub?.cancel();
    _sub = null;
    _channel?.sink.close();
    _channel = null;
    _attempts = 0;
  }

  Future<void> _connect() async {
    if (!_active || _disabled) return;
    final token = accessToken();
    if (token == null) return;

    try {
      // fetchRealtimeToken returns null when the proxy endpoint returns 404
      // (hauling-service not configured with a notification URL). Treat null
      // as permanent "unavailable" and let the 5s booking poll carry updates.
      final realtime = await _fetchToken(token);
      if (realtime == null) {
        _disabled = true;
        if (kDebugMode) {
          debugPrint('customer realtime: disabled (proxy not configured), polling will handle updates');
        }
        return;
      }
      if (!realtime.isValid || !_active) return;

      final uri = Uri.parse('$_wsUrl?token=${Uri.encodeComponent(realtime.token)}');
      final channel = WebSocketChannel.connect(uri);
      _channel = channel;
      _sub = channel.stream.listen(
        _onMessage,
        onDone: _scheduleReconnect,
        onError: (_) => _scheduleReconnect(),
        cancelOnError: true,
      );
      _attempts = 0;
    } catch (error) {
      if (kDebugMode) debugPrint('customer realtime connect error: $error');
      _scheduleReconnect();
    }
  }

  void _onMessage(dynamic raw) {
    if (raw is! String) return;
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map) return;
      final eventType = (decoded['event_type'] as String?) ?? '';
      onEvent(eventType);
    } catch (error) {
      if (kDebugMode) debugPrint('customer realtime decode error: $error');
    }
  }

  void _scheduleReconnect() {
    _sub?.cancel();
    _sub = null;
    _channel = null;
    if (!_active || _disabled) return;

    _attempts = (_attempts + 1).clamp(1, _maxReconnectAttempts);
    // Once the ceiling is reached, stop reconnecting and rely on the 5s poll.
    if (_attempts >= _maxReconnectAttempts) {
      if (kDebugMode) {
        debugPrint('customer realtime: max reconnect attempts reached, relying on polling');
      }
      return;
    }
    final delaySeconds = 1 << (_attempts - 1); // 1, 2, 4, 8, 16s
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: delaySeconds), _connect);
  }
}
