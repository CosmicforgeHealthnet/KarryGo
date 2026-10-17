import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Owns the app's [ThemeMode] and persists the user's choice.
///
/// Registered once in `main()` and handed to the root widget. Any widget
/// that needs to read or change the theme listens to this.
class ThemeController extends ChangeNotifier {
  ThemeController({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _storageKey = 'app_theme_mode_v1';

  final FlutterSecureStorage _storage;

  ThemeMode _mode = ThemeMode.light;
  ThemeMode get mode => _mode;

  bool get isDark => _mode == ThemeMode.dark;
  bool get isLight => _mode == ThemeMode.light;
  bool get isSystem => _mode == ThemeMode.system;

  /// Loads the persisted preference. Call this before `runApp`.
  Future<void> load() async {
    try {
      final raw = await _storage.read(key: _storageKey);
      final parsed = _parse(raw);
      if (parsed != _mode) {
        _mode = parsed;
        notifyListeners();
      }
    } catch (_) {
      // Storage unavailable — silently fall back to system default.
    }
  }

  Future<void> setMode(ThemeMode mode) async {
    if (_mode == mode) return;
    _mode = mode;
    notifyListeners();
    try {
      await _storage.write(key: _storageKey, value: mode.name);
    } catch (_) {
      // Persist failure shouldn't break the UI — mode is already applied.
    }
  }

  /// Cycles through the three modes: system → light → dark → system.
  Future<void> cycle() => switch (_mode) {
    ThemeMode.system => setMode(ThemeMode.light),
    ThemeMode.light => setMode(ThemeMode.dark),
    ThemeMode.dark => setMode(ThemeMode.system),
  };

  Future<void> toggleDark() =>
      setMode(_mode == ThemeMode.dark ? ThemeMode.light : ThemeMode.dark);

  ThemeMode _parse(String? raw) => switch (raw) {
    'light' => ThemeMode.light,
    'dark' => ThemeMode.dark,
    'system' => ThemeMode.system,
    _ => ThemeMode.light,
  };
}
