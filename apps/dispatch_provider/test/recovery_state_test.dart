import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/authenticated_api_client.dart';
import 'package:dispatch_provider/features/auth/data/dispatch_auth_api.dart';
import 'package:dispatch_provider/features/auth/state/dispatch_auth_controller.dart';
import 'package:dispatch_provider/features/requests/data/requests_api.dart';
import 'package:dispatch_provider/features/requests/data/request_model.dart';
import 'package:dispatch_provider/features/requests/state/requests_controller.dart';
import 'package:dispatch_provider/features/trips/data/trips_api.dart';
import 'package:dispatch_provider/features/trips/state/trips_controller.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.live);
  });
  tearDown(() async {
    await AppModeConfig.instance.setMode(AppMode.mock);
  });
  http.Response response(dynamic data) =>
      http.Response(jsonEncode({'success': true, 'data': data}), 200);
  test(
    'Temporary refresh failure preserves session but confirmed rejection clears it',
    () async {
      var reject = false;
      final auth = DispatchAuthController(
        api: DispatchAuthApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: MockClient((_) async {
            if (!reject) throw http.ClientException('Offline');
            return http.Response(
              jsonEncode({
                'success': false,
                'error': {'code': 'unauthorized', 'message': 'Session expired'},
              }),
              401,
            );
          }),
        ),
      );
      auth.restoreTokens(
        accessToken: 'access',
        refreshToken: 'refresh',
        providerId: 'rider',
      );
      expect(await auth.refreshSession(), isA<ApiFailure>());
      expect(auth.accessToken, 'access');
      expect(auth.refreshToken, 'refresh');
      expect(auth.providerId, 'rider');
      reject = true;
      await auth.refreshSession();
      expect(auth.accessToken, isNull);
      expect(auth.refreshToken, isNull);
      auth.dispose();
    },
  );
  test(
    'Protected request refresh outage never invokes session-expired callback',
    () async {
      var expired = 0;
      final client = AuthenticatedApiClient(
        getAccessToken: () => 'access',
        refreshSession: () async =>
            throw http.ClientException('Refresh unavailable'),
        onSessionExpired: () {
          expired++;
        },
        inner: MockClient((_) async => http.Response('', 401)),
      );
      await expectLater(
        client.get(Uri.parse('https://dispatch.test/account')),
        throwsA(isA<http.ClientException>()),
      );
      expect(expired, 0);
      client.close();
    },
  );
  test(
    'Accepted offers never reappear from an outdated source or its outage cache',
    () async {
      var outage = false;
      final client = MockClient((request) async {
        if (request.method == 'POST') return response({});
        if (request.url.path.endsWith('/requests')) {
          if (outage) return http.Response('down', 503);
          return response([
            {
              'inbox_id': 'offer',
              'broadcast_id': 'broadcast',
              'booking_id': 'booking',
              'expires_at': DateTime.now()
                  .add(const Duration(minutes: 5))
                  .toUtc()
                  .toIso8601String(),
            },
          ]);
        }
        return response([]);
      });
      final c = RequestsController(
        api: RequestsApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: client,
        ),
        getAccessToken: () => 'token',
      );
      await c.loadRequests();
      final offer = c.requests.single;
      await c.accept(offer);
      await c.loadRequests();
      expect(c.requests, isEmpty);
      outage = true;
      await c.loadRequests();
      expect(c.requests, isEmpty);
      c.dispose();
      client.close();
    },
  );
  test(
    'Expired request is rejected locally without sending acceptance',
    () async {
      var sent = 0;
      final c = RequestsController(
        api: RequestsApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: MockClient((_) async {
            sent++;
            return response({});
          }),
        ),
        getAccessToken: () => 'token',
      );
      final offer = RequestModel.fromJson({
        'id': 'expired',
        'expires_at': DateTime.now()
            .subtract(const Duration(seconds: 1))
            .toUtc()
            .toIso8601String(),
      });
      expect(offer.isAcceptable, isFalse);
      expect(await c.accept(offer), isA<ApiFailure>());
      expect(sent, 0);
      c.dispose();
    },
  );
  test(
    'Active delivery refresh failure preserves cache and exposes recovery state',
    () async {
      var outage = false;
      final c = TripsController(
        api: TripsApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: MockClient(
            (_) async => outage
                ? http.Response('down', 503)
                : response({'id': 'trip', 'status': 'in_progress'}),
          ),
        ),
        getAccessToken: () => 'token',
      );
      await c.loadActiveTrip();
      expect(c.activeTrip?.id, 'trip');
      outage = true;
      await c.loadActiveTrip();
      expect(c.activeTrip?.id, 'trip');
      expect(c.activeTripError, isNotNull);
      outage = false;
      await c.loadActiveTrip();
      expect(c.activeTripError, isNull);
      c.dispose();
    },
  );
}
