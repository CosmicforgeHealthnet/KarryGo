import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../../../core/app_mode.dart';
import '../data/rider_notification_api.dart';
import '../mock/notifications_mock.dart';
import '../models/rider_notification.dart';

class RiderNotificationController extends ChangeNotifier {
  RiderNotificationController({
    required RiderNotificationApi api,
    required String? Function() accessToken,
    FlutterSecureStorage? storage,
  })  : _api = api,
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
      _unreadCutoff = DateTime.fromMillisecondsSinceEpoch(0);
    }
    await refresh();
  }

  Future<void> refresh() async {
    if (_disposed) return;

    _loading = true;
    _error = null;
    notifyListeners();

    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      if (_disposed) return;
      _notifications
        ..clear()
        ..addAll(NotificationsMock.seed());
      _loadedOnce = true;
      _loading = false;
      notifyListeners();
      return;
    }

    // ── LIVE ──
    final token = _accessToken();
    if (token == null) {
      _loading = false;
      notifyListeners();
      return;
    }
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

  void onRealtimeEvent(String eventType) {
    if (!_loadedOnce || _loading) return;
    refresh();
  }

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