import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

import '../models/provider_auth_models.dart';

/// Persists the signed-in driver's session across app launches.
abstract interface class ProviderSessionStore {
  Future<ProviderSession?> read();
  Future<void> write(ProviderSession session);
  Future<void> clear();
}

class SharedPrefsProviderSessionStore implements ProviderSessionStore {
  static const _key = 'taxi_provider.session';

  @override
  Future<ProviderSession?> read() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final raw = prefs.getString(_key);
      if (raw == null || raw.isEmpty) return null;
      final decoded = jsonDecode(raw);
      if (decoded is! Map) return null;
      return ProviderSession.fromJson(Map<String, dynamic>.from(decoded));
    } catch (_) {
      // A corrupt or unreadable store must not brick the app: fall back to
      // signed-out rather than crashing on launch.
      return null;
    }
  }

  @override
  Future<void> write(ProviderSession session) async {
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(
        _key,
        jsonEncode({
          'access_token': session.accessToken,
          'refresh_token': session.refreshToken,
          'provider': {
            'id': session.provider.id,
            'phone': session.provider.phone,
            'first_name': session.provider.firstName,
            'last_name': session.provider.lastName,
            'account_status': session.provider.accountStatus,
            'onboarding_status': session.provider.onboardingStatus,
            'accepts_parcels': session.provider.acceptsParcels,
            'email': session.provider.email,
            'rating': session.provider.rating,
            'total_trips': session.provider.totalTrips,
            'profile_photo_url': session.provider.profilePhotoUrl,
          },
        }),
      );
    } catch (_) {
      // Non-fatal: the driver stays signed in for this run and simply has to
      // log in again next launch.
    }
  }

  @override
  Future<void> clear() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.remove(_key);
    } catch (_) {
      // Nothing useful to do; the in-memory session is already cleared.
    }
  }
}

/// In-memory store for tests.
class MemoryProviderSessionStore implements ProviderSessionStore {
  ProviderSession? _session;

  @override
  Future<ProviderSession?> read() async => _session;

  @override
  Future<void> write(ProviderSession session) async => _session = session;

  @override
  Future<void> clear() async => _session = null;
}
