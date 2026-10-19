import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/auth/data/provider_auth_api.dart';
import 'package:truck_provider/features/auth/data/provider_session_store.dart';
import 'package:truck_provider/features/auth/models/provider_auth_models.dart';
import 'package:truck_provider/features/auth/state/provider_auth_controller.dart';
import 'package:truck_provider/features/home/data/provider_api.dart';
import 'package:truck_provider/features/home/state/provider_home_controller.dart';
import 'package:truck_provider/features/media/data/media_file_api.dart';
import 'package:truck_provider/features/media/data/media_upload_service.dart';

/// Provider access tokens last an hour; a shift lasts longer. Before this fix
/// every call made after the hour mark failed with "Your session has expired"
/// until the app was restarted — including Go Online and advancing a live trip,
/// which is exactly what blocked an end-to-end booking run.
class _MemoryStore implements ProviderSessionStore {
  ProviderSession? _session;
  @override
  Future<ProviderSession?> loadSession() async => _session;
  @override
  Future<void> saveSession(ProviderSession session) async => _session = session;
  @override
  Future<void> clearSession() async => _session = null;
}

void main() {
  const provider = TruckProvider(
    id: 'p1',
    phone: '+2348011111001',
    onboardingStatus: 'complete',
  );

  ProviderSession session(String accessToken) => ProviderSession(
        accessToken: accessToken,
        refreshToken: 'refresh-token',
        provider: provider,
      );

  http.Response ok(Map<String, dynamic> data) =>
      http.Response(jsonEncode({'success': true, 'data': data}), 200);

  http.Response unauthorized() => http.Response(
        jsonEncode({
          'success': false,
          'error': {
            'code': 'unauthorized',
            'message': 'Your session has expired. Please sign in again.',
          },
        }),
        401,
      );

  Map<String, dynamic> sessionJson(String accessToken) => {
        'access_token': accessToken,
        'refresh_token': 'refresh-token-2',
        'provider': {
          'id': 'p1',
          'phone': '+2348011111001',
          'onboarding_status': 'complete',
        },
      };

  /// Builds an authenticated controller pair sharing one mock transport.
  Future<(ProviderAuthController, ProviderHomeController)> build(
    MockClient client,
  ) async {
    final store = _MemoryStore();
    await store.saveSession(session('expired-access'));
    final auth = ProviderAuthController(
      api: ProviderAuthApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8104/api/v1/hauling',
        ),
        client: client,
      ),
      sessionStore: store,
      // Never exercised by these tests; pointed at a client that always fails
      // so an accidental upload would be loud rather than silent.
      mediaUploadService: MediaUploadService(
        api: MediaFileApi(
          config: const ApiCoreConfig(
            baseUrl: 'http://localhost:8104/api/v1/hauling',
          ),
          accessToken: () => null,
          client: MockClient((_) async => http.Response('{}', 500)),
        ),
      ),
    );
    await auth.initialize();
    final home = ProviderHomeController(
      api: ProviderApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8104/api/v1/hauling',
        ),
        client: client,
      ),
      authController: auth,
    );
    return (auth, home);
  }

  test('going online after the token expires refreshes and succeeds', () async {
    // `initialize()` refreshes once on startup, so the session begins on
    // "access-1". That token then ages out mid-shift — the case this covers.
    var refreshCalls = 0, onlineAttempts = 0;
    final (_, home) = await build(MockClient((request) async {
      final path = request.url.path;
      if (path.endsWith('/auth/refresh')) {
        refreshCalls++;
        return ok(sessionJson('access-$refreshCalls'));
      }
      if (path.endsWith('/provider/availability')) {
        onlineAttempts++;
        // The startup token is stale; only the one from the second refresh
        // (triggered by the 401) is accepted.
        final auth = request.headers['Authorization'] ?? '';
        if (!auth.contains('access-2')) return unauthorized();
        return ok({'online': true});
      }
      return ok({});
    }));

    await home.goOnline();

    // One refresh at startup, one triggered by the 401.
    expect(refreshCalls, 2, reason: 'should refresh in response to the 401');
    expect(onlineAttempts, 2, reason: 'should retry the call after refreshing');
    expect(
      home.state.isOnline,
      isTrue,
      reason: 'the provider should end up online, not stuck on an error',
    );
    expect(home.state.error, isNull);
  });

  test('surfaces the error when the refresh token is also rejected', () async {
    // Startup refresh succeeds (so we have a session); the later one, triggered
    // by the 401 during the shift, is rejected — a genuinely revoked session.
    var refreshCalls = 0;
    final (_, home) = await build(MockClient((request) async {
      if (request.url.path.endsWith('/auth/refresh')) {
        refreshCalls++;
        if (refreshCalls == 1) return ok(sessionJson('access-1'));
        return unauthorized();
      }
      if (request.url.path.endsWith('/provider/availability')) {
        return unauthorized();
      }
      return ok({});
    }));

    await home.goOnline();

    expect(home.state.isOnline, isFalse);
    expect(
      home.state.error,
      isNotNull,
      reason: 'a genuinely dead session must still surface an error',
    );
  });

  test('a non-auth error is not retried and is surfaced as-is', () async {
    var attempts = 0;
    final (_, home) = await build(MockClient((request) async {
      if (request.url.path.endsWith('/auth/refresh')) {
        return ok(sessionJson('access-1'));
      }
      if (request.url.path.endsWith('/provider/availability')) {
        attempts++;
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {'code': 'not_verified', 'message': 'Account not verified.'},
          }),
          403,
        );
      }
      return ok({});
    }));

    await home.goOnline();

    expect(attempts, 1, reason: 'only auth failures may trigger a retry');
    expect(home.state.error, contains('not verified'));
  });
}
