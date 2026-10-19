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

/// The server drops a provider from the matching pool ~90s after their last
/// heartbeat. The app's heartbeat timer is suspended while backgrounded, so a
/// provider who checks their phone for a few minutes comes back to a toggle
/// that still reads "Online" while no request can reach them — they sit waiting
/// for work that will never arrive.
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
  http.Response ok(Map<String, dynamic> data) =>
      http.Response(jsonEncode({'success': true, 'data': data}), 200);

  Map<String, dynamic> sessionJson() => {
        'access_token': 'access-1',
        'refresh_token': 'refresh-token',
        'provider': {
          'id': 'p1',
          'phone': '+2348011111001',
          'onboarding_status': 'complete',
        },
      };

  Future<ProviderHomeController> build(MockClient client) async {
    final store = _MemoryStore();
    await store.saveSession(const ProviderSession(
      accessToken: 'access-0',
      refreshToken: 'refresh-token',
      provider: TruckProvider(
        id: 'p1',
        phone: '+2348011111001',
        onboardingStatus: 'complete',
      ),
    ));
    const config = ApiCoreConfig(
      baseUrl: 'http://localhost:8104/api/v1/hauling',
    );
    final auth = ProviderAuthController(
      api: ProviderAuthApi(config: config, client: client),
      sessionStore: store,
      mediaUploadService: MediaUploadService(
        api: MediaFileApi(
          config: const ApiCoreConfig(baseUrl: 'http://localhost:8104/api/v1/hauling'),
          accessToken: () => null,
          client: MockClient((_) async => http.Response('{}', 500)),
        ),
      ),
    );
    await auth.initialize();
    return ProviderHomeController(
      api: ProviderApi(config: config, client: client),
      authController: auth,
    );
  }

  test('resuming while the server says offline clears the stale toggle',
      () async {
    final home = await build(MockClient((request) async {
      if (request.url.path.endsWith('/auth/refresh')) return ok(sessionJson());
      if (request.url.path.endsWith('/provider/availability')) {
        // PUT = go online (succeeds); GET = the availability check on resume,
        // which reports that the server has already dropped this provider.
        if (request.method == 'GET') return ok({'status': 'offline'});
        return ok({});
      }
      return ok({});
    }));

    await home.goOnline();
    expect(home.state.isOnline, isTrue, reason: 'precondition: went online');

    // Simulates returning to the foreground after the online TTL lapsed.
    await home.syncOnlineStatus();

    expect(
      home.state.isOnline,
      isFalse,
      reason: 'the toggle must follow the server, not a stale local belief',
    );
    expect(
      home.state.error,
      isNotNull,
      reason: 'the provider needs to be told they dropped offline',
    );
  });

  test('resuming while the server still says online keeps the provider online',
      () async {
    final home = await build(MockClient((request) async {
      if (request.url.path.endsWith('/auth/refresh')) return ok(sessionJson());
      if (request.url.path.endsWith('/provider/availability')) {
        if (request.method == 'GET') return ok({'status': 'online'});
        return ok({});
      }
      return ok({});
    }));

    await home.goOnline();
    await home.syncOnlineStatus();

    expect(home.state.isOnline, isTrue);
    expect(
      home.state.error,
      isNull,
      reason: 'no false alarm when the session is genuinely still live',
    );
  });

  test('a failed availability check leaves the toggle untouched', () async {
    final home = await build(MockClient((request) async {
      if (request.url.path.endsWith('/auth/refresh')) return ok(sessionJson());
      if (request.url.path.endsWith('/provider/availability')) {
        if (request.method == 'GET') {
          return http.Response(jsonEncode({'success': false}), 500);
        }
        return ok({});
      }
      return ok({});
    }));

    await home.goOnline();
    await home.syncOnlineStatus();

    expect(
      home.state.isOnline,
      isTrue,
      reason: 'a transient network failure must not flip the provider offline',
    );
  });
}
