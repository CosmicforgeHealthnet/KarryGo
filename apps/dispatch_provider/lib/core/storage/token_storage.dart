import 'dart:convert';
import 'dart:math';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import '../app_mode.dart';

/// Persists auth tokens to the device keychain / encrypted shared preferences.
/// Keys are namespaced to avoid collisions with other apps.
class TokenStorage {
  static const _accessTokenKey = 'dp_access_token';
  static const _refreshTokenKey = 'dp_refresh_token';
  static const _providerIdKey = 'dp_provider_id';
  static const _deviceIdKey = 'dp_install_device_id';

  final FlutterSecureStorage _storage;
  final String _scope = AppModeConfig.instance.current.name;
  String _key(String key) => '${key}_$_scope';

  TokenStorage()
    : _storage = const FlutterSecureStorage(
        // Use encrypted shared preferences on Android (requires API 23+).
        aOptions: AndroidOptions(encryptedSharedPreferences: true),
      );

  Future<bool> hasSeenIntroduction() async =>
      await _storage.read(key: _key('dp_intro_seen')) == 'true';
  Future<void> markIntroductionSeen() =>
      _storage.write(key: _key('dp_intro_seen'), value: 'true');

  /// A non-hardware identifier, stable within the current installation
  /// and isolated from mock-mode credentials. Logout does not rotate it.
  Future<String> getOrCreateDeviceId() async {
    final key = _key(_deviceIdKey);
    final current = await _storage.read(key: key);
    if (current != null && current.isNotEmpty) return current;
    final secureRandom = Random.secure();
    final randomBytes = List<int>.generate(24, (_) => secureRandom.nextInt(256));
    final deviceId = 'dp-${base64UrlEncode(randomBytes).replaceAll('=', '')}';
    await _storage.write(key: key, value: deviceId);
    return deviceId;
  }

  /// Saves access token, refresh token, and provider ID atomically.
  Future<void> saveTokens({
    required String accessToken,
    required String refreshToken,
    required String providerId,
  }) async {
    await Future.wait([
      _storage.write(key: _key(_accessTokenKey), value: accessToken),
      _storage.write(key: _key(_refreshTokenKey), value: refreshToken),
      _storage.write(key: _key(_providerIdKey), value: providerId),
    ]);
  }

  /// Reads saved tokens.  Any field may be null if it was never saved or was cleared.
  Future<SavedTokens> readTokens() async {
    final results = await Future.wait([
      _storage.read(key: _key(_accessTokenKey)),
      _storage.read(key: _key(_refreshTokenKey)),
      _storage.read(key: _key(_providerIdKey)),
    ]);
    return SavedTokens(
      accessToken: results[0],
      refreshToken: results[1],
      providerId: results[2],
    );
  }

  /// Deletes all saved tokens (called on logout or when session is invalidated).
  Future<void> clear() async {
    await Future.wait([
      _storage.delete(key: _key(_accessTokenKey)),
      _storage.delete(key: _key(_refreshTokenKey)),
      _storage.delete(key: _key(_providerIdKey)),
    ]);
  }

  Future<void> saveOperationDraft(
    String providerId,
    String operationType,
  ) async {
    if (providerId.isEmpty) return;
    await _storage.write(
      key: 'dp_operation_${_scope}_$providerId',
      value: operationType,
    );
  }

  Future<String?> readOperationDraft(String providerId) async {
    if (providerId.isEmpty) return null;
    return _storage.read(key: 'dp_operation_${_scope}_$providerId');
  }
}

class SavedTokens {
  final String? accessToken;
  final String? refreshToken;
  final String? providerId;

  const SavedTokens({this.accessToken, this.refreshToken, this.providerId});

  bool get hasAny => accessToken != null || refreshToken != null;
  bool get isEmpty => !hasAny;
}
