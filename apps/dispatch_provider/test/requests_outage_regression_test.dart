import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/features/requests/data/requests_api.dart';
import 'package:dispatch_provider/features/requests/state/requests_controller.dart';

const _baseUrl = 'https://dispatch.example.test';

http.Response _ok([List<Object> data = const []]) =>
    http.Response(jsonEncode({'success': true, 'data': data}), 200);

void main() {
  test('503 from optional offer endpoint is an outage, not an empty success',
      () async {
    final api = RequestsApi(
      const ApiCoreConfig(baseUrl: _baseUrl),
      client: MockClient((_) async => http.Response('unavailable', 503)),
    );
    final result = await api.listOrganizationOffers(accessToken: 'test-token');
    expect(result.isFailure, isTrue);
    result.when(
      success: (_) => fail('HTTP 503 must not be treated as an empty list'),
      failure: (error) => expect(error.statusCode, 503),
    );
  });

  test('only optional route 404 means a disabled source', () async {
    final api = RequestsApi(
      const ApiCoreConfig(baseUrl: _baseUrl),
      client: MockClient((_) async => http.Response('not found', 404)),
    );
    final optional = await api.listAssignmentOffers(accessToken: 'test-token');
    expect(optional.isSuccess, isTrue);
    optional.when(
      success: (requests) => expect(requests, isEmpty),
      failure: (_) => fail('Optional 404 should be an empty source'),
    );
    final mandatory = await api.listBroadcasts(accessToken: 'test-token');
    expect(mandatory.isFailure, isTrue);
  });

  group('opportunities end-to-end controller safety', () {
    setUp(() async => AppModeConfig.instance.setMode(AppMode.live));
    tearDown(() async => AppModeConfig.instance.setMode(AppMode.mock));

    test('failed source retains last result but blocks accepting it', () async {
      var failOneSource = false;
      var postAttempts = 0;
      final api = RequestsApi(
        const ApiCoreConfig(baseUrl: _baseUrl),
        client: MockClient((request) async {
          if (request.method == 'POST') {
            postAttempts++;
            return _ok();
          }
          if (failOneSource &&
              request.url.path == '/api/v1/provider/organization-offers') {
            return http.Response('temporary outage', 503);
          }
          if (request.url.path == '/api/v1/provider/requests') {
            return _ok([
              {
                'inbox_id': 'offer-a',
                'booking_id': 'booking-a',
                'pickup_address': 'Pickup',
                'dropoff_address': 'Dropoff',
                'lifecycle': 'available',
              },
            ]);
          }
          return _ok();
        }),
      );
      final controller = RequestsController(
        api: api,
        getAccessToken: () => 'test-token',
      );
      addTearDown(controller.dispose);

      final first = await controller.loadRequests();
      expect(first.isSuccess, isTrue);
      expect(controller.requests, hasLength(1));
      expect(controller.canAcceptOffers, isTrue);

      failOneSource = true;
      final second = await controller.loadRequests();
      expect(second.isFailure, isTrue);
      expect(controller.requests, hasLength(1));
      expect(controller.error, isNotNull);
      expect(controller.canAcceptOffers, isFalse);

      final accepted = await controller.accept(controller.requests.single);
      expect(accepted.isFailure, isTrue);
      expect(postAttempts, 0);
    });

    test('expired offer is not acceptable even when list refreshed', () async {
      final api = RequestsApi(
        const ApiCoreConfig(baseUrl: _baseUrl),
        client: MockClient((request) async {
          if (request.url.path == '/api/v1/provider/requests') {
            return _ok([
              {
                'inbox_id': 'expired',
                'booking_id': 'booking-b',
                'lifecycle': 'available',
                'expires_at':
                    DateTime.now().subtract(const Duration(seconds: 30)).toUtc().toIso8601String(),
              },
            ]);
          }
          return _ok();
        }),
      );
      final controller = RequestsController(
        api: api,
        getAccessToken: () => 'test-token',
      );
      addTearDown(controller.dispose);
      await controller.loadRequests();
      expect(controller.requests.single.isAcceptable, isFalse);
      expect((await controller.accept(controller.requests.single)).isFailure,
          isTrue);
    });
  });
}
