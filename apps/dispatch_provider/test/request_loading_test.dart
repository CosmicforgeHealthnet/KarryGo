import 'dart:convert';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/features/requests/data/requests_api.dart';
import 'package:dispatch_provider/features/requests/state/requests_controller.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.live);
  });

  test('A source outage remains an error instead of an empty inbox', () async {
    final controller = RequestsController(
      api: RequestsApi(
        ApiCoreConfig(baseUrl: 'https://dispatch.test'),
        client: MockClient(
          (_) async => http.Response(
            jsonEncode({
              'success': false,
              'error': {
                'code': 'service_unavailable',
                'message': 'Request service is unavailable.',
              },
            }),
            503,
          ),
        ),
      ),
      getAccessToken: () => 'test-token',
    );
    final result = await controller.loadRequests();
    expect(result, isA<ApiFailure>());
    expect(controller.error, 'Request service is unavailable.');
    controller.dispose();
  });

  test(
    'A failed source retains its cache while another source refreshes',
    () async {
      var outage = false;
      final controller = RequestsController(
        api: RequestsApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: MockClient((request) async {
            if (request.url.path.endsWith('/requests')) {
              if (outage)
                return http.Response(
                  jsonEncode({
                    'success': false,
                    'error': {
                      'code': 'service_unavailable',
                      'message': 'Unavailable',
                    },
                  }),
                  503,
                );
              return http.Response(
                jsonEncode({
                  'success': true,
                  'data': [
                    {
                      'inbox_id': 'inbox-a',
                      'broadcast_id': 'broadcast-a',
                      'booking_id': 'booking-a',
                      'pickup_address': 'Pickup',
                      'dropoff_address': 'Dropoff',
                      'fare_amount': 10000,
                    },
                  ],
                }),
                200,
              );
            }
            return http.Response(
              jsonEncode({'success': true, 'data': []}),
              200,
            );
          }),
        ),
        getAccessToken: () => 'test-token',
      );
      await controller.loadRequests();
      expect(controller.requests.single.id, 'inbox-a');
      outage = true;
      await controller.loadRequests();
      expect(controller.requests.single.id, 'inbox-a');
      expect(controller.error, contains('last available'));
      controller.dispose();
    },
  );
}
