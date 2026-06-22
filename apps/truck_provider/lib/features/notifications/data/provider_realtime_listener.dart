import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'provider_notification_api.dart';

/// Opens and maintains the provider's realtime notification websocket.
///
/// This is the **fast path** for incoming booking events: when a push arrives,
/// [onEvent] is invoked with the event type so the home controller can refresh
/// immediately instead of waiting for the next 4-second poll. The poll remains
/// the safety net, so a dropped socket never strands a provider.
///
/// The realtime token is re-minted on each (re)connect, so token expiry is
/// handled by the reconnect path.
class ProviderRealtimeListener {
  ProviderRealtimeListener({
    required ProviderNotificationApi api,
    required String wsUrl,
    required this.accessToken,
    required this.onEvent,
  }) : _api = api,
       _wsUrl = wsUrl;

  final ProviderNotificationApi _api;
  final String _wsUrl;

  /// Returns the current provider access token, or null if signed out.
  final String? Function() accessToken;

  /// Called with the event_type of each pushed notification.
  final void Function(String eventType) onEvent;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _sub;
  Timer? _reconnectTimer;
  int _attempts = 0;
  bool _active = false;

  /// Opens the connection. Idempotent.
  Future<void> start() async {
    if (_active) return;
    _active = true;
    await _connect();
  }

  /// Closes the connection and stops reconnecting.
  void stop() {
    _active = false;
    _reconnectTimer?.cancel();
    _reconnectTimer = null;
    _sub?.cancel();
    _sub = null;
    _channel?.sink.close();
    _channel = null;
    _attempts = 0;
  }

  Future<void> _connect() async {
    if (!_active) return;
    final token = accessToken();
    if (token == null) return;

    try {
      final realtime = await _api.fetchRealtimeToken(accessToken: token);
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
      if (kDebugMode) debugPrint('provider realtime connect error: $error');
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
      if (kDebugMode) debugPrint('provider realtime decode error: $error');
    }
  }

  void _scheduleReconnect() {
    _sub?.cancel();
    _sub = null;
    _channel = null;
    if (!_active) return;

    _attempts = (_attempts + 1).clamp(1, 6);
    final delaySeconds = 1 << (_attempts - 1); // 1,2,4,8,16,32
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: delaySeconds), _connect);
  }
}
