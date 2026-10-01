import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

enum AppMode { mock, live }

/// Global runtime switch between MOCK and LIVE modes.
///
/// Persisted in [FlutterSecureStorage] so it survives hot restart, hot reload,
/// and cold app launch — matching the ThemeController pattern.
///
/// Load via [load] in main() before runApp(). Toggle via [toggle] or [setMode].
class AppModeConfig extends ChangeNotifier {
  AppModeConfig._();
  static final AppModeConfig instance = AppModeConfig._();

  static const _storageKey = 'app_mode_v1';

  final FlutterSecureStorage _storage = const FlutterSecureStorage();

  AppMode _current = AppMode.mock;
  AppMode get current => _current;
  bool get isMock => _current == AppMode.mock;
  bool get isLive => _current == AppMode.live;

  /// Loads the persisted mode. Call this before runApp().
  Future<void> load() async {
    try {
      final raw = await _storage.read(key: _storageKey);
      final parsed = _parse(raw);
      if (parsed != _current) {
        _current = parsed;
        notifyListeners();
      }
    } catch (_) {
      // Storage unavailable — keep the default (mock).
    }
  }

  Future<void> setMode(AppMode mode) async {
    if (_current == mode) return;
    _current = mode;
    notifyListeners();
    try {
      await _storage.write(key: _storageKey, value: mode.name);
    } catch (_) {
      // Persist failure shouldn't break the UI — mode is already applied.
    }
  }

  void toggle() {
    setMode(isMock ? AppMode.live : AppMode.mock);
  }

  AppMode _parse(String? raw) => switch (raw) {
        'live' => AppMode.live,
        _ => AppMode.mock,
      };
}