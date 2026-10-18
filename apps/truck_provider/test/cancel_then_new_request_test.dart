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

class _MemoryStore implements ProviderSessionStore {
  ProviderSession? _session;
  @override
  Future<ProviderSession?> loadSession() async => _session;
  @override
  Future<void> saveSession(ProviderSession session) async => _session = session;
  @override
  Future<void> clearSession() async => _session = null;
}

Map<String, dynamic> _booking(String id, String status) => {
  'id': id,
  'pickup_address': 'Ikeja',
  'pickup_lat': 6.6,
  'pickup_lng': 3.35,
  'dropoff_address': 'Lekki',
  'dropoff_lat': 6.44,
  'dropoff_lng': 3.47,
  'cargo_type': 'general',
  'cargo_weight_kg': 100,
  'status': status,
  'created_at': '2026-10-18T10:00:00Z',
};

/// After a customer cancels an accepted trip, the driver must go straight
/// back to receiving new requests without restarting the app.
void main() {
  http.Response ok(Object data) =>
      http.Response(jsonEncode({'success': true, 'data': data}), 200);

  late List<Map<String, dynamic>> bookings;
  Map<String, dynamic>? single;

  Future<ProviderHomeController> build() async {
    final client = MockClient((request) async {
      final path = request.url.path;
      if (path.endsWith('/auth/refresh')) {
        return ok({
          'access_token': 'access-1',
          'refresh_token': 'refresh',
          'provider': {
            'id': 'p1',
            'phone': '+2348011111001',
            'onboarding_status': 'complete',
          },
        });
      }
      if (path.endsWith('/provider/bookings')) return ok(bookings);
      if (path.contains('/provider/bookings/') && single != null) {
        return ok(single!);
      }
      if (path.endsWith('/provider/availability') && request.method == 'GET') {
        return ok({'status': 'online'});
      }
      return ok({});
    });
    final store = _MemoryStore();
    await store.saveSession(const ProviderSession(
      accessToken: 'access',
      refreshToken: 'refresh',
      provider: TruckProvider(
        id: 'p1',
        phone: '+2348011111001',
        onboardingStatus: 'complete',
      ),
    ));
    const config = ApiCoreConfig(baseUrl: 'http://localhost/api/v1/hauling');
    final auth = ProviderAuthController(
      api: ProviderAuthApi(config: config, client: client),
      sessionStore: store,
      mediaUploadService: MediaUploadService(
        api: MediaFileApi(
          config: const ApiCoreConfig(baseUrl: 'http://localhost:8109'),
          serviceToken: 'test-token',
          ownerService: 'hauling-service',
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

  Future<void> poll(ProviderHomeController home) async {
    home.pokeRefresh();
    await Future<void>.delayed(const Duration(milliseconds: 20));
  }

  test('a new request shows after the customer cancels the active trip',
      () async {
    bookings = [_booking('trip-1', 'accepted')];
    final home = await build();
    addTearDown(home.dispose);

    await home.goOnline();
    await poll(home);
    expect(home.state.status, ProviderHomeStatus.activeTrip);

    // The customer cancels.
    bookings = [_booking('trip-1', 'cancelled')];
    await poll(home);
    expect(home.state.status, ProviderHomeStatus.dashboard);
    expect(home.state.isOnline, isTrue);

    // A new job is offered to this driver.
    bookings = [
      _booking('trip-2', 'awaiting_acceptance'),
      _booking('trip-1', 'cancelled'),
    ];
    await poll(home);
    expect(home.state.pendingRequests.map((b) => b.id), ['trip-2']);
  });

  test('a cancelled trip missing from the list still ends the trip screen',
      () async {
    single = null;
    bookings = [_booking('trip-1', 'accepted')];
    final home = await build();
    addTearDown(home.dispose);

    await home.goOnline();
    await poll(home);
    expect(home.state.status, ProviderHomeStatus.activeTrip);

    // The cancelled trip is no longer on the list page; only a new offer is.
    bookings = [_booking('trip-2', 'awaiting_acceptance')];
    single = _booking('trip-1', 'cancelled');
    await poll(home);

    expect(home.state.status, ProviderHomeStatus.dashboard);
    expect(home.state.pendingRequests.map((b) => b.id), ['trip-2']);
  });
}
