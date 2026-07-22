import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../data/rider_notification_api.dart';
import '../models/rider_notification.dart';

/// Drives the rider's in-app notification feed (the bell screen).
///
/// The feed is loaded from the dispatch-service notification proxy when the
/// screen opens and on pull-to-refresh. Live pushes arrive through the app's
/// realtime listener, which forwards every event to [onRealtimeEvent] so an
/// open feed refreshes without its own websocket.
///
/// Read styling: items newer than [unreadCutoff] (the last time the screen was
/// opened, persisted across restarts) render as unread. Opening the feed
/// advances the stored cutoff but keeps the current one in memory, so items
/// stay visibly "new" for the whole visit and read on the next. The marker is
/// stored in flutter_secure_storage (this app has no shared_preferences dep);
/// reads/writes are best-effort so plugin-less test runs degrade to
/// "all unread".
class RiderNotificationController extends ChangeNotifier {
  RiderNotificationController({
    required RiderNotificationApi api,
    required String? Function() accessToken,
    FlutterSecureStorage? storage,
  }) : _api = api,
       _accessToken = accessToken,
       _storage = storage ?? const FlutterSecureStorage();

  static const _lastSeenKey = 'dp_notifications_last_seen';

  final RiderNotificationApi _api;
  final String? Function() _accessToken;
  final FlutterSecureStorage _storage;

  final List<RiderNotification> _notifications = [];
  List<RiderNotification> get notifications =>
      List.unmodifiable(_notifications);

  bool _loading = false;
  bool get isLoading => _loading;

  String? _error;
  String? get error => _error;

  DateTime? _unreadCutoff;
  DateTime? get unreadCutoff => _unreadCutoff;

  bool _loadedOnce = false;
  bool _disposed = false;

  bool isUnread(RiderNotification item) {
    final cutoff = _unreadCutoff;
    return cutoff == null || item.createdAt.isAfter(cutoff);
  }

  /// Loads the feed for a screen visit: styles anything newer than the previous
  /// visit as unread, then advances the persisted last-seen marker.
  Future<void> openFeed() async {
    try {
      final storedMillis = int.tryParse(
        await _storage.read(key: _lastSeenKey) ?? '',
      );
      _unreadCutoff = storedMillis == null
          ? DateTime.fromMillisecondsSinceEpoch(0)
          : DateTime.fromMillisecondsSinceEpoch(storedMillis);
      await _storage.write(
        key: _lastSeenKey,
        value: '${DateTime.now().millisecondsSinceEpoch}',
      );
    } catch (_) {
      // Storage unavailable (tests) — everything renders unread.
      _unreadCutoff = DateTime.fromMillisecondsSinceEpoch(0);
    }
    await refresh();
  }

  /// Reloads the feed, keeping the current unread cutoff.
  Future<void> refresh() async {
    final token = _accessToken();
    if (token == null || _disposed) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      final items = await _api.listNotifications(accessToken: token);
      if (_disposed) return;
      _notifications
        ..clear()
        ..addAll(items);
      _loadedOnce = true;
    } catch (error) {
      _error = 'Could not load notifications.';
      if (kDebugMode) debugPrint('rider notification feed error: $error');
    } finally {
      if (!_disposed) {
        _loading = false;
        notifyListeners();
      }
    }
  }

  /// Fast path from the realtime listener: a pushed event means the feed
  /// changed. Only refreshes once the feed has been loaded at least once.
  void onRealtimeEvent(String eventType) {
    if (!_loadedOnce || _loading) return;
    refresh();
  }

  /// Clears feed state on logout.
  void clear() {
    _notifications.clear();
    _loadedOnce = false;
    _error = null;
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}
