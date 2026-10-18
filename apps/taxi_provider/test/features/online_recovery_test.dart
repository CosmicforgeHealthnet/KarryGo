import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taxi_provider/features/auth/data/provider_auth_api.dart';
import 'package:taxi_provider/features/auth/data/provider_session_store.dart';
import 'package:taxi_provider/features/auth/models/provider_auth_models.dart';
import 'package:taxi_provider/features/auth/state/provider_auth_controller.dart';
import 'package:taxi_provider/features/home/data/provider_api.dart';
import 'package:taxi_provider/features/home/state/provider_home_controller.dart';

class _SignedInAuth extends ProviderAuthController {
  _SignedInAuth(ProviderAuthApi api)
    : super(api: api, sessionStore: MemoryProviderSessionStore());
  @override
  ProviderAuthState get state => const ProviderAuthState(
    status: ProviderAuthStatus.authenticated,
    session: ProviderSession(
      accessToken: 'test',
      refreshToken: 'test',
      provider: TaxiProvider(
        id: 'driver',
        phone: '+2348000000000',
        firstName: 'Ada',
        lastName: 'Driver',
        accountStatus: 'active',
        onboardingStatus: 'complete',
        acceptsParcels: false,
      ),
    ),
  );
}

http.Response _ok(Map<String, dynamic> data) =>
    http.Response(jsonEncode({'success': true, 'data': data}), 200);

http.Response _err(int status, String code, String message) => http.Response(
  jsonEncode({
    'success': false,
    'error': {'code': code, 'message': message, 'request_id': 'r'},
  }),
  status,
);

/// A fake taxi service whose online status can be expired by the test, the
/// way the real one expires it when heartbeats stop in the background.
class _FakeServer {
  bool online = false;
  String? refuseGoOnline;
  int goOnlineCalls = 0;

  Future<http.Response> handle(http.Request request) async {
    final path = request.url.path;
    if (path.endsWith('/provider/availability/heartbeat')) {
      return online
          ? _ok({'online': true})
          : _err(400, 'NOT_ONLINE', 'You are not currently online.');
    }
    if (path.endsWith('/provider/availability')) {
      if (request.method == 'GET') return _ok({'online': online});
      final wantOnline =
          (jsonDecode(request.body) as Map)['status'] == 'online';
      if (wantOnline) {
        goOnlineCalls++;
        if (refuseGoOnline != null) {
          return _err(400, 'outside_service_area', refuseGoOnline!);
        }
      }
      online = wantOnline;
      return _ok({'online': online});
    }
    return _ok({'bookings': []});
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const config = ApiCoreConfig(baseUrl: 'http://localhost/api/v1/taxi');

  late _FakeServer server;
  late ProviderAuthApi authApi;
  late ProviderApi api;
  late _SignedInAuth auth;
  late ProviderHomeController home;

  setUp(() {
    server = _FakeServer();
    final client = MockClient(server.handle);
    authApi = ProviderAuthApi(config: config, client: client);
    api = ProviderApi(config: config, client: client);
    auth = _SignedInAuth(authApi);
    home = ProviderHomeController(
      api: api,
      authApi: authApi,
      authController: auth,
    );
  });

  tearDown(() {
    home.dispose();
    auth.dispose();
    api.close();
    authApi.close();
  });

  test('returning to the app restores an expired online status', () async {
    await home.setOnline(true);
    expect(home.state.online, isTrue);
    expect(server.goOnlineCalls, 1);

    server.online = false; // expired while the app was in the background
    await home.resync();

    expect(server.online, isTrue, reason: 'the app must go online again');
    expect(server.goOnlineCalls, 2);
    expect(home.state.online, isTrue);
    expect(home.state.error, isNull);
  });

  test('shows offline with the reason when going back online is refused',
      () async {
    await home.setOnline(true);
    server.online = false;
    server.refuseGoOnline = 'We do not operate in this area yet.';

    await home.resync();

    expect(home.state.online, isFalse,
        reason: 'never show online while the server says offline');
    expect(home.state.error, contains('We do not operate in this area yet.'));
  });

  test('stays online without re-registering when the server agrees',
      () async {
    await home.setOnline(true);
    await home.resync();
    expect(server.goOnlineCalls, 1);
    expect(home.state.online, isTrue);
  });

  test('accepting an offer keeps its addresses on screen', () async {
    final booking = {
      'id': 'b1',
      'status': 'awaiting_acceptance',
      'stops': [
        {'sequence': 0, 'type': 'PICKUP', 'address': '12 Allen Ave'},
        {'sequence': 1, 'type': 'DROPOFF', 'address': 'Lekki Phase 1'},
      ],
    };
    final client = MockClient((request) async {
      if (request.url.path.endsWith('/accept')) {
        // Lifecycle responses carry only the booking row, no stops.
        return _ok({'id': 'b1', 'status': 'accepted'});
      }
      return _ok({
        'bookings': [booking],
      });
    });
    final tripApi = ProviderApi(config: config, client: client);
    final tripHome = ProviderHomeController(
      api: tripApi,
      authApi: authApi,
      authController: auth,
    );
    addTearDown(() {
      tripHome.dispose();
      tripApi.close();
    });

    await tripHome.refreshBookings();
    expect(tripHome.state.offers.single.pickup?.address, '12 Allen Ave');

    expect(await tripHome.acceptOffer('b1'), isTrue);
    final active = tripHome.state.activeBooking;
    expect(active, isNotNull);
    expect(active!.pickup?.address, '12 Allen Ave');
    expect(active.dropoff?.address, 'Lekki Phase 1');
  });
}
