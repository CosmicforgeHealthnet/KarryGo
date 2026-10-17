import 'dart:convert';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/mode_http_client.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/missions/data/provider_operations_api.dart';
import 'package:dispatch_provider/features/missions/ui/commerce_pickup_screen.dart';

Map<String, dynamic> mission({bool verified = false}) => {
  'mission_ref': 'mission-a',
  'trip_ref': 'trip-a',
  'state': 'PICKING_UP',
  'version': 8,
  'pickup_actions_allowed': true,
  'stops': [
    {
      'stop_ref': 'stop-a',
      'sequence': 1,
      'manifest_ref': 'manifest-a',
      'manifest_version': 2,
      'status': verified ? 'READY_FOR_HANDOVER' : 'AWAITING_PICKUP',
      'handover_status': verified ? 'VERIFIED' : 'PENDING',
      'location': {'address': 'Test Kitchen'},
    },
  ],
  'parcels': [
    {'parcel_ref': 'parcel-a', 'pickup_stop_ref': 'stop-a', 'package_count': 2},
  ],
};
void main() {
  test(
    'Mock commerce enforces stop order, sender approval and exact parcel collection',
    () async {
      TestWidgetsFlutterBinding.ensureInitialized();
      FlutterSecureStorage.setMockInitialValues({});
      await AppModeConfig.instance.setMode(AppMode.mock);
      final client = ModeHttpClient(
        inner: MockClient((_) async => throw StateError('Unexpected network')),
      );
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        client: client,
        token: () => 'demo',
      );
      const read =
          '/api/v1/provider/commerce-missions/by-trip/demo-commerce-trip';
      const base =
          '/api/v1/provider/commerce-missions/demo-commerce-mission/stops';
      await expectLater(
        api.command('$base/demo-stop-2/arrive', {
          'expected_version': 1,
        }, operationRef: true),
        throwsA(isA<ApiException>()),
      );
      for (final i in [1, 2]) {
        var current = await api.read(read);
        await api.command('$base/demo-stop-$i/arrive', {
          'expected_version': current['version'],
        }, operationRef: true);
        current = await api.read(read);
        final body = {
          'expected_version': current['version'],
          'manifest_ref': 'demo-manifest-$i',
          'manifest_version': 1,
          'parcel_refs': ['demo-parcel-$i'],
          'package_count': i,
        };
        await expectLater(
          api.command('$base/demo-stop-$i/complete', body, operationRef: true),
          throwsA(isA<ApiException>()),
        );
        await api.command('$base/demo-stop-$i/mock-sender-confirmation', {
          'expected_version': current['version'],
        }, operationRef: true);
        current = await api.read(read);
        await api.command('$base/demo-stop-$i/complete', {
          ...body,
          'expected_version': current['version'],
        }, operationRef: true);
      }
      expect((await api.read(read))['state'], 'IN_TRANSIT');
      client.close();
    },
  );

  testWidgets(
    'Commerce arrival sends the operation reference and shows the code for the sender to verify',
    (tester) async {
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        token: () => 'provider',
        client: MockClient((request) async {
          if (request.method == 'POST') {
            final body = jsonDecode(request.body);
            expect(body['operation_ref'], request.headers['Idempotency-Key']);
            expect(body['expected_version'], 8);
            return http.Response(
              jsonEncode({
                'success': true,
                'data': {
                  'stop_ref': 'stop-a',
                  'code': '456789',
                  'expires_at': DateTime.now()
                      .add(const Duration(minutes: 5))
                      .toUtc()
                      .toIso8601String(),
                },
              }),
              200,
            );
          }
          return http.Response(
            jsonEncode({'success': true, 'data': mission()}),
            200,
          );
        }),
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          home: CommercePickupScreen(
            api: api,
            tripId: 'trip-a',
            initial: mission(),
          ),
        ),
      );
      await tester.tap(find.text('I am at this pickup'));
      await tester.pumpAndSettle();
      expect(find.text('456789'), findsOneWidget);
      expect(find.textContaining('sender verifies'), findsOneWidget);
      expect(find.text('Confirm collection'), findsNothing);
      await tester.pumpWidget(const SizedBox());
    },
  );

  testWidgets(
    'Commerce collection requires parcel confirmation and exact server manifest',
    (tester) async {
      Map<String, dynamic>? sent;
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        token: () => 'provider',
        client: MockClient((request) async {
          if (request.method == 'POST') {
            sent = jsonDecode(request.body);
            expect(sent!['operation_ref'], request.headers['Idempotency-Key']);
          }
          return http.Response(
            jsonEncode({'success': true, 'data': mission(verified: true)}),
            200,
          );
        }),
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          home: CommercePickupScreen(
            api: api,
            tripId: 'trip-a',
            initial: mission(verified: true),
          ),
        ),
      );
      final button = find.widgetWithText(FilledButton, 'Confirm collection');
      expect(tester.widget<FilledButton>(button).onPressed, isNull);
      await tester.tap(find.byType(CheckboxListTile));
      await tester.pump();
      await tester.ensureVisible(button);
      await tester.tap(button);
      await tester.pumpAndSettle();
      await tester.tap(find.text('I have collected these parcels'));
      await tester.pumpAndSettle();
      expect(sent!['manifest_ref'], 'manifest-a');
      expect(sent!['manifest_version'], 2);
      expect(sent!['parcel_refs'], ['parcel-a']);
      expect(sent!['package_count'], 2);
      expect(sent!['expected_version'], 8);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
