import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../../auth/state/customer_auth_controller.dart';
import '../data/notification_api.dart';
import '../data/push_registration_service.dart';
import '../models/app_notification.dart';

/// Drives the in-app notification feed and the realtime websocket connection.
///
/// On [start] it loads the REST feed, mints a realtime token, and opens a
/// websocket to notification-service. Each pushed message is prepended to the
/// feed. A dropped socket reconnects with exponential backoff; the realtime
/// token is re-minted on each (re)connect, covering expiry. [stop] tears
/// everything down (call on logout).
///
/// When [fetchRealtimeToken] returns null (proxy endpoint returned 404 —
/// customer-service not configured with CUSTOMER_NOTIFICATION_BASE_URL),
/// [_connect] treats this as a permanent "unavailable" signal and returns
/// without scheduling a reconnect. The feed REST endpoint and the booking
/// poll remain the fallback.
class NotificationController extends ChangeNotifier {
  NotificationController({
    required NotificationApi api,
    required CustomerAuthController authController,
    required String wsUrl,
    PushRegistrationService? pushRegistration,
  }) : _api = api,
       _auth = authController,
       _wsUrl = wsUrl,
       _pushRegistration = pushRegistration;

  final NotificationApi _api;
  final CustomerAuthController _auth;
  final String _wsUrl;

  /// Optional FCM device-token registration. Null until Firebase is configured.
  final PushRegistrationService? _pushRegistration;

  final List<AppNotification> _notifications = [];
  List<AppNotification> get notifications => List.unmodifiable(_notifications);

  bool _loading = false;
  bool get isLoading => _loading;

  String? _error;
  String? get error => _error;

  int _unread = 0;
  int get unreadCount => _unread;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _socketSub;
  Timer? _reconnectTimer;
  int _reconnectAttempts = 0;
  bool _started = false;
  bool _disposed = false;

  String? _accessToken() => _auth.state.session?.accessToken;

  /// Loads the feed, registers for push, and opens the realtime connection.
  /// Idempotent.
  Future<void> start() async {
    if (_started) return;
    _started = true;
    await loadFeed();
    await _registerForPush();
    await _connect();
  }

  Future<void> _registerForPush() async {
    final registration = _pushRegistration;
    final token = _accessToken();
    if (registration == null || token == null) return;
    await registration.register(accessToken: token);
  }

  /// Tears down the realtime connection and clears state. Call on logout.
  /// Safe to call when already stopped (no-op), so it can be driven from a
  /// build method without emitting a notification mid-build.
  void stop() {
    if (!_started && _channel == null && _notifications.isEmpty) return;
    _started = false;
    _reconnectTimer?.cancel();
    _reconnectTimer = null;
    _socketSub?.cancel();
    _socketSub = null;
    _channel?.sink.close();
    _channel = null;
    _notifications.clear();
    _unread = 0;
    if (!_disposed) {
      // Defer to avoid notifying during a build (stop() may be called from a
      // build method when the user logs out).
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!_disposed) notifyListeners();
      });
    }
  }

  /// Reloads the REST feed.
  Future<void> loadFeed() async {
    final token = _accessToken();
    if (token == null) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      final items = await _api.listNotifications(accessToken: token);
      _notifications
        ..clear()
        ..addAll(items);
      _unread = 0;
    } catch (error) {
      _error = 'Could not load notifications.';
      if (kDebugMode) debugPrint('notification feed error: $error');
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Marks the feed as read (e.g. when the notifications tab is opened).
  void markAllRead() {
    if (_unread == 0) return;
    _unread = 0;
    notifyListeners();
  }

  // ─── Realtime ────────────────────────────────────────────────────────────

  Future<void> _connect() async {
    if (_disposed || !_started) return;
    final token = _accessToken();
    if (token == null) return;

    try {
      // fetchRealtimeToken returns null when the proxy endpoint returns 404
      // (customer-service not configured with CUSTOMER_NOTIFICATION_BASE_URL).
      // Treat null as a permanent signal — no reconnect; rely on polling.
      final realtime = await _api.fetchRealtimeToken(accessToken: token);
      if (realtime == null || !realtime.isValid || _disposed || !_started) return;

      final uri = Uri.parse('$_wsUrl?token=${Uri.encodeComponent(realtime.token)}');
      final channel = WebSocketChannel.connect(uri);
      _channel = channel;
      _socketSub = channel.stream.listen(
        _onSocketMessage,
        onDone: _scheduleReconnect,
        onError: (_) => _scheduleReconnect(),
        cancelOnError: true,
      );
      _reconnectAttempts = 0;
    } catch (error) {
      if (kDebugMode) debugPrint('notification socket connect error: $error');
      _scheduleReconnect();
    }
  }

  void _onSocketMessage(dynamic raw) {
    if (raw is! String) return;
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map) return;
      final message = AppNotification.fromJson(Map<String, dynamic>.from(decoded));
      if (message.title.isEmpty && message.body.isEmpty) return;
      _notifications.insert(0, message);
      _unread += 1;
      notifyListeners();
    } catch (error) {
      if (kDebugMode) debugPrint('notification socket decode error: $error');
    }
  }

  void _scheduleReconnect() {
    _socketSub?.cancel();
    _socketSub = null;
    _channel = null;
    if (_disposed || !_started) return;
    _reconnectAttempts = (_reconnectAttempts + 1).clamp(1, 6);
    final delaySeconds = 1 << (_reconnectAttempts - 1); // 1,2,4,8,16,32
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: delaySeconds), _connect);
  }

  @override
  void dispose() {
    _disposed = true;
    stop();
    super.dispose();
  }
}
