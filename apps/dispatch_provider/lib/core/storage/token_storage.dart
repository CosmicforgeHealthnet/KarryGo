import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Persists auth tokens to the device keychain / encrypted shared preferences.
/// Keys are namespaced to avoid collisions with other apps.
class TokenStorage {
  static const _accessTokenKey = 'dp_access_token';
  static const _refreshTokenKey = 'dp_refresh_token';
  static const _providerIdKey = 'dp_provider_id';

  final FlutterSecureStorage _storage;

  TokenStorage()
      : _storage = const FlutterSecureStorage(
          // Use encrypted shared preferences on Android (requires API 23+).
          aOptions: AndroidOptions(encryptedSharedPreferences: true),
        );

  /// Saves access token, refresh token, and provider ID atomically.
  Future<void> saveTokens({
    required String accessToken,
    required String refreshToken,
    required String providerId,
  }) async {
    await Future.wait([
      _storage.write(key: _accessTokenKey, value: accessToken),
      _storage.write(key: _refreshTokenKey, value: refreshToken),
      _storage.write(key: _providerIdKey, value: providerId),
    ]);
  }

  /// Reads saved tokens.  Any field may be null if it was never saved or was cleared.
  Future<SavedTokens> readTokens() async {
    final results = await Future.wait([
      _storage.read(key: _accessTokenKey),
      _storage.read(key: _refreshTokenKey),
      _storage.read(key: _providerIdKey),
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
      _storage.delete(key: _accessTokenKey),
      _storage.delete(key: _refreshTokenKey),
      _storage.delete(key: _providerIdKey),
    ]);
  }
}

class SavedTokens {
  final String? accessToken;
  final String? refreshToken;
  final String? providerId;

  const SavedTokens({
    this.accessToken,
    this.refreshToken,
    this.providerId,
  });

  bool get hasAny => accessToken != null || refreshToken != null;
  bool get isEmpty => !hasAny;
}
