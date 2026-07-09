import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../data/provider_notification_api.dart';
import '../models/provider_notification.dart';

/// Drives the provider's in-app notification feed (the bell screen).
///
/// The feed is loaded from the hauling-service notification proxy when the
/// screen opens and on pull-to-refresh. Live pushes arrive through the home
/// controller's existing realtime listener, which forwards every event to
/// [onRealtimeEvent] so an open feed refreshes without its own websocket.
///
/// Read styling: items newer than [unreadCutoff] (the last time the screen was
/// opened, persisted across restarts) render as unread. Opening the feed
/// advances the stored cutoff but keeps the current one in memory, so items
/// stay visibly "new" for the whole visit and read on the next.
class ProviderNotificationController extends ChangeNotifier {
  ProviderNotificationController({
    required ProviderNotificationApi api,
    required String? Function() accessToken,
  }) : _api = api,
       _accessToken = accessToken;

  static const _lastSeenKey = 'provider.notifications.last_seen';

  final ProviderNotificationApi _api;
  final String? Function() _accessToken;

  final List<ProviderNotification> _notifications = [];
  List<ProviderNotification> get notifications => List.unmodifiable(_notifications);

  bool _loading = false;
  bool get isLoading => _loading;

  String? _error;
  String? get error => _error;

  DateTime? _unreadCutoff;
  DateTime? get unreadCutoff => _unreadCutoff;

  bool _loadedOnce = false;
  bool _disposed = false;

  bool isUnread(ProviderNotification item) {
    final cutoff = _unreadCutoff;
    return cutoff == null || item.createdAt.isAfter(cutoff);
  }

  /// Loads the feed for a screen visit: styles anything newer than the previous
  /// visit as unread, then advances the persisted last-seen marker.
  Future<void> openFeed() async {
    final prefs = await SharedPreferences.getInstance();
    final storedMillis = prefs.getInt(_lastSeenKey);
    _unreadCutoff = storedMillis == null
        ? DateTime.fromMillisecondsSinceEpoch(0)
        : DateTime.fromMillisecondsSinceEpoch(storedMillis);
    await prefs.setInt(_lastSeenKey, DateTime.now().millisecondsSinceEpoch);
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
      if (kDebugMode) debugPrint('provider notification feed error: $error');
    } finally {
      if (!_disposed) {
        _loading = false;
        notifyListeners();
      }
    }
  }

  /// Fast path from the home realtime listener: a pushed event means the feed
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
