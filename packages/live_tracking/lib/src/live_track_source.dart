import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'track_point.dart';

/// A stream of vehicle positions.
abstract class LiveTrackSource {
  /// Begins delivering positions to [onPoint]. Idempotent.
  Future<void> start(void Function(TrackPoint point) onPoint);

  /// Stops delivering positions and releases resources.
  void stop();
}

/// Streams positions over a websocket.
///
/// This is the fast path. It reproduces the reconnect behaviour the platform's
/// existing realtime listeners already rely on: exponential backoff, a hard
/// attempt cap after which it gives up rather than reconnecting forever, and a
/// latch that disables it permanently when the backend reports the route is
/// not configured, so a service without the feature does not cause a
/// reconnect storm.
class WebSocketTrackSource implements LiveTrackSource {
  WebSocketTrackSource({
    required this.url,
    this.maxReconnectAttempts = 6,
    this.onUnavailable,
    @visibleForTesting WebSocketChannel Function(Uri uri)? connect,
  }) : _connect = connect ?? WebSocketChannel.connect;

  /// The websocket URL, including any auth token in the query string. A
  /// websocket handshake cannot carry an Authorization header, which is why
  /// the services accept the token this way.
  final String url;

  final int maxReconnectAttempts;

  /// Called when the socket gives up, so a caller can lean on polling. Fires
  /// once per give-up, not once per failed attempt.
  final void Function()? onUnavailable;

  final WebSocketChannel Function(Uri uri) _connect;

  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _subscription;
  Timer? _reconnectTimer;
  void Function(TrackPoint point)? _onPoint;
  int _attempts = 0;
  bool _active = false;

  /// Whether the socket is currently delivering.
  bool get isConnected => _channel != null;

  @override
  Future<void> start(void Function(TrackPoint point) onPoint) async {
    if (_active) return;
    _active = true;
    _onPoint = onPoint;
    _attempts = 0;
    _connectNow();
  }

  @override
  void stop() {
    _active = false;
    _reconnectTimer?.cancel();
    _reconnectTimer = null;
    _subscription?.cancel();
    _subscription = null;
    _channel?.sink.close();
    _channel = null;
    _onPoint = null;
  }

  void _connectNow() {
    if (!_active) return;
    try {
      final channel = _connect(Uri.parse(url));
      _channel = channel;
      _subscription = channel.stream.listen(
        _onMessage,
        onError: (Object _) => _scheduleReconnect(),
        onDone: _scheduleReconnect,
        cancelOnError: true,
      );
      _attempts = 0;
    } catch (error) {
      if (kDebugMode) {
        debugPrint('live tracking socket failed to open: $error');
      }
      _scheduleReconnect();
    }
  }

  void _onMessage(dynamic message) {
    if (message is! String) return;
    try {
      final decoded = jsonDecode(message);
      if (decoded is! Map<String, dynamic>) return;

      // The services publish a type-tagged envelope; an offline frame is a
      // real event, but it carries no position to draw.
      final type = decoded['type'];
      if (type is String && type != 'location_update') return;

      final point = TrackPoint.fromJson(decoded);
      if (point != null) _onPoint?.call(point);
    } catch (error) {
      if (kDebugMode) {
        debugPrint('live tracking frame ignored: $error');
      }
    }
  }

  void _scheduleReconnect() {
    _subscription?.cancel();
    _subscription = null;
    _channel = null;
    if (!_active) return;

    _attempts++;
    if (_attempts > maxReconnectAttempts) {
      // Give up rather than hammering: the caller's polling fallback carries
      // the trip from here.
      _active = false;
      onUnavailable?.call();
      return;
    }

    final backoff = Duration(seconds: 1 << (_attempts - 1));
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(backoff, _connectNow);
  }
}

/// Polls a REST endpoint for the latest position.
///
/// This is the safety net. It is deliberately kept running alongside the
/// websocket rather than only started on failure, because a socket that is
/// open but silent looks identical to one that is working.
class PollingTrackSource implements LiveTrackSource {
  PollingTrackSource({
    required this.fetch,
    this.interval = const Duration(seconds: 8),
  });

  /// Fetches the current position, or null when there is none to report.
  final Future<TrackPoint?> Function() fetch;

  final Duration interval;

  Timer? _timer;
  bool _inFlight = false;
  bool _active = false;

  @override
  Future<void> start(void Function(TrackPoint point) onPoint) async {
    if (_active) return;
    _active = true;
    await _poll(onPoint);
    _timer = Timer.periodic(interval, (_) => _poll(onPoint));
  }

  @override
  void stop() {
    _active = false;
    _timer?.cancel();
    _timer = null;
  }

  Future<void> _poll(void Function(TrackPoint point) onPoint) async {
    // Guard against a slow response overlapping the next tick and delivering
    // positions out of order.
    if (_inFlight || !_active) return;
    _inFlight = true;
    try {
      final point = await fetch();
      if (point != null && _active) onPoint(point);
    } catch (error) {
      if (kDebugMode) {
        debugPrint('live tracking poll failed: $error');
      }
    } finally {
      _inFlight = false;
    }
  }
}

/// Runs a websocket and a poll together: the socket for responsiveness, the
/// poll so a dropped or unconfigured socket never strands the map.
///
/// The poll runs at a relaxed interval while the socket is healthy and speeds
/// up once the socket gives up.
class CompositeTrackSource implements LiveTrackSource {
  CompositeTrackSource({
    required WebSocketTrackSource Function(void Function() onUnavailable) socket,
    required PollingTrackSource Function(Duration interval) poll,
    this.pollWhileConnected = const Duration(seconds: 20),
    this.pollWhenSocketUnavailable = const Duration(seconds: 6),
  })  : _socketFactory = socket,
        _pollFactory = poll;

  final WebSocketTrackSource Function(void Function() onUnavailable) _socketFactory;
  final PollingTrackSource Function(Duration interval) _pollFactory;

  /// Poll cadence while the websocket is expected to carry updates.
  final Duration pollWhileConnected;

  /// Poll cadence once the websocket has given up.
  final Duration pollWhenSocketUnavailable;

  WebSocketTrackSource? _socket;
  PollingTrackSource? _poll;
  void Function(TrackPoint point)? _onPoint;
  bool _active = false;

  /// Whether the websocket gave up and polling is now the only source.
  bool get isPollingOnly => _socket == null && _poll != null;

  @override
  Future<void> start(void Function(TrackPoint point) onPoint) async {
    if (_active) return;
    _active = true;
    _onPoint = onPoint;

    _socket = _socketFactory(_onSocketUnavailable);
    await _socket!.start(onPoint);

    _poll = _pollFactory(pollWhileConnected);
    await _poll!.start(onPoint);
  }

  @override
  void stop() {
    _active = false;
    _socket?.stop();
    _socket = null;
    _poll?.stop();
    _poll = null;
    _onPoint = null;
  }

  void _onSocketUnavailable() {
    if (!_active) return;
    _socket = null;

    // Tighten the poll to cover for the socket.
    final onPoint = _onPoint;
    if (onPoint == null) return;
    _poll?.stop();
    _poll = _pollFactory(pollWhenSocketUnavailable);
    _poll!.start(onPoint);
  }
}
