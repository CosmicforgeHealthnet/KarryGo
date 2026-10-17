import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

enum AppMode { mock, live }

enum MockEntry { introduction, setup, dashboard }

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

  static const _entryKey = 'dispatch_mock_entry_v2';
  static const _journeyKey = 'dispatch_mock_journey_v2';
  MockEntry _mockEntry = MockEntry.introduction;
  MockEntry get mockEntry => _mockEntry;
  Map<String, dynamic> _mockJourney = {};
  Map<String, dynamic> get mockJourney => Map.unmodifiable(_mockJourney);
  int _previewRevision = 0;
  int get previewRevision => _previewRevision;
  Future<void> _pendingSave = Future.value();

  AppMode _current = kReleaseMode ? AppMode.live : AppMode.mock;
  AppMode get current => kReleaseMode ? AppMode.live : _current;
  bool get isMock => !kReleaseMode && current == AppMode.mock;
  bool get isLive => current == AppMode.live;

  /// Loads the persisted mode. Call this before runApp().
  Future<void> load() async {
    // Never restore a demo account or mocked services in release builds.
    if (kReleaseMode) {
      _current = AppMode.live;
      _mockJourney = {};
      return;
    }
    try {
      await _pendingSave;
      final entry = await _storage.read(key: _entryKey);
      _mockEntry = MockEntry.values.firstWhere(
        (e) => e.name == entry,
        orElse: () => MockEntry.introduction,
      );
      final journey = await _storage.read(key: _journeyKey);
      _mockJourney = journey == null
          ? {}
          : Map<String, dynamic>.from(jsonDecode(journey) as Map);
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

  Future<void> setMode(AppMode mode, {MockEntry? mockEntry}) async {
    if (kReleaseMode) return;
    final restartPreview = mode == AppMode.mock && mockEntry != null;
    if (_current == mode && !restartPreview) return;
    Future<void>? reset;
    if (restartPreview) {
      _mockEntry = mockEntry;
      reset = saveMockJourney({});
      _previewRevision++;
    }
    _current = mode;
    notifyListeners();
    try {
      await _storage.write(key: _storageKey, value: mode.name);
      await _storage.write(key: _entryKey, value: _mockEntry.name);
      await reset;
    } catch (_) {
      // Persist failure shouldn't break the UI — mode is already applied.
    }
  }

  Future<void> saveMockJourney(Map<String, dynamic> journey) {
    _mockJourney = Map.of(journey);
    final encoded = jsonEncode(journey);
    _pendingSave = _pendingSave.then((_) async {
      try {
        await _storage.write(key: _journeyKey, value: encoded);
      } catch (_) {}
    });
    return _pendingSave;
  }

  @visibleForTesting
  void resetMemoryForTesting() {
    _pendingSave = Future.value();
    _mockJourney = {};
    _mockEntry = MockEntry.introduction;
    _current = AppMode.mock;
    _previewRevision = 0;
  }

  void toggle() {
    if (kReleaseMode) return;
    setMode(isMock ? AppMode.live : AppMode.mock);
  }

  AppMode _parse(String? raw) => switch (raw) {
    'live' => AppMode.live,
    _ => AppMode.mock,
  };
}
