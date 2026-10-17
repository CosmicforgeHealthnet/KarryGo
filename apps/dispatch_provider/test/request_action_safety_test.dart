import 'dart:async';
import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/features/requests/data/request_model.dart';
import 'package:dispatch_provider/features/requests/data/requests_api.dart';
import 'package:dispatch_provider/features/requests/state/requests_controller.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

const _config = ApiCoreConfig(baseUrl: 'https://dispatch.example.test');
const _token = 'test-provider-token';

http.Response _ok(Object data) =>
    http.Response(jsonEncode({'success': true, 'data': data}), 200);

RequestModel _offer() => RequestModel.fromJson({
  'inbox_id': 'offer-1',
  'booking_id': 'booking-1',
  'pickup_address': 'Market Road',
  'dropoff_address': 'Airport Road',
  'lifecycle': 'available',
});

void main() {
  test('an async 202 is not a confirmed accepted mission', () async {
    final api = RequestsApi(_config, client: MockClient(
      (_) async => http.Response(jsonEncode({'success': true}), 202),
    ));
    final result = await api.accept(accessToken: _token, request: _offer());
    expect(result.isFailure, true);
    result.when(
      success: (_) => fail('202 is not a confirmed accepted mission'),
      failure: (error) => expect(error.statusCode, 202),
    );
  });

  test('invalid or negative HTTP 200 acknowledgements cannot accept an offer',
      () async {
    for (final response in [
      http.Response('', 200),
      http.Response('{invalid', 200),
      http.Response(jsonEncode({'success': false, 'data': {}}), 200),
    ]) {
      final api = RequestsApi(_config, client: MockClient((_) async => response));
      expect(
        (await api.accept(accessToken: _token, request: _offer())).isFailure,
        true,
      );
    }
  });

  group('rider request action safety', () {
    setUp(() async => AppModeConfig.instance.setMode(AppMode.live));
    tearDown(() async => AppModeConfig.instance.setMode(AppMode.mock));

    test('double-tap produces one accept POST', () async {
      final reply = Completer<http.Response>();
      var posts = 0;
      final api = RequestsApi(_config, client: MockClient((req) async {
        if (req.method == 'POST') {
          posts++;
          return reply.future;
        }
        if (req.url.path == '/api/v1/provider/requests') {
          return _ok([{
            'inbox_id': 'offer-1',
            'booking_id': 'booking-1',
            'lifecycle': 'available',
          }]);
        }
        return _ok([]);
      }));
      final controller = RequestsController(
        api: api, getAccessToken: () => _token,
      );
      addTearDown(controller.dispose);
      await controller.loadRequests();
      final offer = controller.requests.single;
      final first = controller.accept(offer);
      final duplicate = controller.accept(offer);
      expect((await duplicate).isFailure, true);
      expect(posts, 1);
      reply.complete(_ok({}));
      expect((await first).isSuccess, true);
      expect(controller.requests, isEmpty);
    });

    test('late acceptance after logout never activates prior rider request',
        () async {
      final reply = Completer<http.Response>();
      var token = 'provider-one';
      final api = RequestsApi(_config, client: MockClient((req) async {
        if (req.method == 'POST') return reply.future;
        if (req.url.path == '/api/v1/provider/requests') {
          return _ok([{
            'inbox_id': 'offer-1',
            'booking_id': 'booking-1',
            'lifecycle': 'available',
          }]);
        }
        return _ok([]);
      }));
      final controller = RequestsController(api: api, getAccessToken: () => token);
      addTearDown(controller.dispose);
      await controller.loadRequests();
      final started = controller.accept(controller.requests.single);
      controller.resetSession();
      token = 'provider-two';
      reply.complete(_ok({}));
      expect((await started).isFailure, true);
      expect(controller.activeRequest, isNull);
      expect(controller.requests, isEmpty);
    });
  });
}
