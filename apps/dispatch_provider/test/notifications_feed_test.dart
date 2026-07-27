import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:dispatch_provider/features/notifications/data/rider_notification_api.dart';
import 'package:dispatch_provider/features/notifications/state/rider_notification_controller.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  RiderNotificationApi buildApi(MockClient client) {
    return RiderNotificationApi(
      config: ApiCoreConfig(baseUrl: 'http://dispatch.test'),
      client: client,
    );
  }

  test('listNotifications parses Go-cased feed items', () async {
    final api = buildApi(
      MockClient((request) async {
        expect(request.url.path, '/api/v1/provider/notifications');
        return http.Response(
          jsonEncode({
            'success': true,
            'data': [
              {
                'ID': 'n1',
                'EventType': 'booking.matched',
                'Title': 'New delivery request',
                'Body': 'A delivery is waiting for you.',
                'CreatedAt': '2026-07-18T10:00:00Z',
                'Data': {'booking_id': 'b1'},
              },
              {
                'id': 'n2',
                'event_type': 'booking.cancelled',
                'title': 'Delivery cancelled',
                'body': 'The customer cancelled.',
                'created_at': '2026-07-18T09:00:00Z',
              },
            ],
          }),
          200,
        );
      }),
    );

    final items = await api.listNotifications(accessToken: 'tok');
    expect(items, hasLength(2));
    expect(items[0].id, 'n1');
    expect(items[0].eventType, 'booking.matched');
    expect(items[0].data['booking_id'], 'b1');
    expect(items[1].id, 'n2');
    expect(items[1].title, 'Delivery cancelled');
  });

  test('fetchRealtimeToken parses token payload', () async {
    final api = buildApi(
      MockClient((request) async {
        expect(
          request.url.path,
          '/api/v1/provider/notifications/realtime-token',
        );
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'token': 'ws-token', 'expires_in': 60},
          }),
          200,
        );
      }),
    );

    final token = await api.fetchRealtimeToken(accessToken: 'tok');
    expect(token.isValid, isTrue);
    expect(token.token, 'ws-token');
  });

  test('controller refresh populates feed; failure sets error', () async {
    var fail = false;
    final controller = RiderNotificationController(
      api: buildApi(
        MockClient((request) async {
          if (fail) return http.Response('oops', 500);
          return http.Response(
            jsonEncode({
              'success': true,
              'data': [
                {
                  'id': 'n1',
                  'event_type': 'booking.matched',
                  'body': 'msg',
                  'created_at': '2026-07-18T10:00:00Z',
                },
              ],
            }),
            200,
          );
        }),
      ),
      accessToken: () => 'tok',
    );

    await controller.refresh();
    expect(controller.notifications, hasLength(1));
    expect(controller.error, isNull);

    fail = true;
    await controller.refresh();
    expect(controller.error, isNotNull);
    // Existing items are retained on failure.
    expect(controller.notifications, hasLength(1));
  });
}
