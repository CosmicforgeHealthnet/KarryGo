import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:dispatch_provider/core/app_mode.dart';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:dispatch_provider/features/disputes/data/rider_support_api.dart';
import 'package:dispatch_provider/features/disputes/state/rider_dispute_controller.dart';
import 'package:dispatch_provider/features/trips/data/trip_model.dart';

Map<String, dynamic> complaintJson({
  required String id,
  required String subject,
  String status = 'open',
}) => {
  'id': id,
  'service_type': 'dispatch',
  'booking_reference': 'book-1',
  'subject': subject,
  'description': 'desc',
  'status': status,
  'created_at': '2026-07-18T10:00:00Z',
};

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.live);
  });
  tearDown(() async {
    await AppModeConfig.instance.setMode(AppMode.mock);
  });
  RiderDisputeController buildController(MockClient client) {
    return RiderDisputeController(
      api: RiderSupportApi(
        config: ApiCoreConfig(
          baseUrl: 'http://support.test/api/v1/support-disputes',
        ),
        client: client,
      ),
      accessToken: () => 'tok',
    );
  }

  test(
    'submitDispute posts a complaint on the dispatch-provider group',
    () async {
      late Map<String, dynamic> sentBody;
      final controller = buildController(
        MockClient((request) async {
          expect(
            request.url.path,
            '/api/v1/support-disputes/dispatch-provider/complaints',
          );
          expect(request.method, 'POST');
          sentBody = jsonDecode(request.body) as Map<String, dynamic>;
          return http.Response(
            jsonEncode({
              'success': true,
              'data': complaintJson(id: 'c1', subject: 'Cancelled trip'),
            }),
            201,
          );
        }),
      );

      controller.startNewDispute();
      controller.selectTrip(
        TripModel(
          id: 'trip-1',
          bookingId: 'book-1',
          statusCode: TripStatusCode.completed,
          rawStatus: 'completed',
          pickupAddress: 'A',
          dropoffAddress: 'B',
          customerName: 'C',
          receiverName: 'R',
          receiverPhone: '+2348011111111',
          distanceKm: 3.2,
          estimatedFareKobo: 120000,
          createdAt: DateTime.utc(2026, 7, 18),
        ),
      );
      controller.selectType('Cancelled trip');

      final complaint = await controller.submitDispute();
      expect(complaint, isNotNull);
      expect(complaint!.id, 'c1');
      expect(sentBody['service_type'], 'dispatch');
      expect(sentBody['booking_reference'], 'book-1');
      expect(sentBody['subject'], 'Cancelled trip');
      expect(controller.complaints, hasLength(1));
    },
  );

  test(
    'support chat uses backend start without client-side complaint creation',
    () async {
      final controller = buildController(
        MockClient((request) async {
          expect(request.method, 'POST');
          expect(
            request.url.path,
            '/api/v1/support-disputes/dispatch-provider/support-chat/start',
          );
          expect(jsonDecode(request.body), isEmpty);
          return http.Response(
            jsonEncode({
              'success': true,
              'data': complaintJson(id: 'chat-1', subject: 'Support Chat'),
            }),
            200,
          );
        }),
      );
      expect((await controller.openSupportChat())?.id, 'chat-1');
      controller.dispose();
    },
  );

  test(
    'support start failure does not claim agent availability or create a replacement case',
    () async {
      var calls = 0;
      final controller = buildController(
        MockClient((request) async {
          calls++;
          return http.Response(
            jsonEncode({
              'success': false,
              'error': {
                'code': 'service_unavailable',
                'message': 'Support is unavailable.',
              },
            }),
            503,
          );
        }),
      );
      expect(await controller.openSupportChat(), isNull);
      expect(controller.supportChatError, isNotNull);
      expect(calls, 1);
      controller.dispose();
    },
  );

  test('messages: rider bubbles detected by sender_type', () async {
    final controller = buildController(
      MockClient((request) async {
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {
              'messages': [
                {
                  'id': 'm1',
                  'sender_type': 'dispatch_provider',
                  'content': 'hello',
                  'created_at': '2026-07-18T10:00:00Z',
                },
                {
                  'id': 'm2',
                  'sender_type': 'admin',
                  'content': 'hi, how can we help?',
                  'created_at': '2026-07-18T10:01:00Z',
                },
              ],
            },
          }),
          200,
        );
      }),
    );

    await controller.loadMessages('c1');
    expect(controller.messages, hasLength(2));
    expect(controller.messages[0].isMine, isTrue);
    expect(controller.messages[1].isMine, isFalse);
  });
}
