import 'package:shared_preferences/shared_preferences.dart';

import '../models/provider_auth_models.dart';

abstract interface class ProviderSessionStore {
  Future<void> saveSession(ProviderSession session);
  Future<ProviderSession?> loadSession();
  Future<void> clearSession();
}

class SharedPrefsProviderSessionStore implements ProviderSessionStore {
  static const _keyAccessToken = 'tp_access_token';
  static const _keyRefreshToken = 'tp_refresh_token';
  static const _keyProviderId = 'tp_provider_id';
  static const _keyPhone = 'tp_provider_phone';
  static const _keyOnboardingStatus = 'tp_onboarding_status';
  static const _keyFirstName = 'tp_first_name';
  static const _keyLastName = 'tp_last_name';
  static const _keyRating = 'tp_rating';
  static const _keyTotalTrips = 'tp_total_trips';

  @override
  Future<void> saveSession(ProviderSession session) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_keyAccessToken, session.accessToken);
    await prefs.setString(_keyRefreshToken, session.refreshToken);
    await prefs.setString(_keyProviderId, session.provider.id);
    await prefs.setString(_keyPhone, session.provider.phone);
    await prefs.setString(_keyOnboardingStatus, session.provider.onboardingStatus);
    await prefs.setString(_keyFirstName, session.provider.firstName);
    await prefs.setString(_keyLastName, session.provider.lastName);
    await prefs.setDouble(_keyRating, session.provider.rating);
    await prefs.setInt(_keyTotalTrips, session.provider.totalTrips);
  }

  @override
  Future<ProviderSession?> loadSession() async {
    final prefs = await SharedPreferences.getInstance();
    final accessToken = prefs.getString(_keyAccessToken);
    final refreshToken = prefs.getString(_keyRefreshToken);
    final id = prefs.getString(_keyProviderId);
    final phone = prefs.getString(_keyPhone);
    if (accessToken == null || refreshToken == null || id == null || phone == null) return null;

    return ProviderSession(
      accessToken: accessToken,
      refreshToken: refreshToken,
      provider: TruckProvider(
        id: id,
        phone: phone,
        onboardingStatus: prefs.getString(_keyOnboardingStatus) ?? 'profile_required',
        firstName: prefs.getString(_keyFirstName) ?? '',
        lastName: prefs.getString(_keyLastName) ?? '',
        rating: prefs.getDouble(_keyRating) ?? 0.0,
        totalTrips: prefs.getInt(_keyTotalTrips) ?? 0,
      ),
    );
  }

  @override
  Future<void> clearSession() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_keyAccessToken);
    await prefs.remove(_keyRefreshToken);
    await prefs.remove(_keyProviderId);
    await prefs.remove(_keyPhone);
    await prefs.remove(_keyOnboardingStatus);
    await prefs.remove(_keyFirstName);
    await prefs.remove(_keyLastName);
    await prefs.remove(_keyRating);
    await prefs.remove(_keyTotalTrips);
  }
}
