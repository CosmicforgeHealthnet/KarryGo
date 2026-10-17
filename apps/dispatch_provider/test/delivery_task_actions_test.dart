import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/provider_session.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/missions/ui/delivery_operations_screen.dart';

void main() {
  testWidgets(
    'Tasks have one primary action and unavailable-recipient flow requires a decision',
    (tester) async {
      tester.view.physicalSize = const Size(320, 720);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      FlutterSecureStorage.setMockInitialValues({});
      await AppModeConfig.instance.setMode(AppMode.mock);
      var delivering = false;
      final writes = <String>[];
      final client = MockClient((request) async {
        if (request.method == 'POST') writes.add(request.url.path);
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {
              'status': delivering ? 'in_progress' : 'arrived_pickup',
              'pickup_address': 'Pickup address',
              'dropoff_address': 'Recipient address',
              'mission': {
                'state': delivering ? 'IN_TRANSIT' : 'AT_PICKUP',
                'allowed_actions': delivering
                    ? [
                        'VERIFY_DELIVERY',
                        'START_DELIVERY_ATTEMPT',
                        'REPORT_BREAKDOWN',
                      ]
                    : [
                        'READ_PICKUP_CREDENTIAL',
                        'SCAN_PARCELS',
                        'START_DELIVERY',
                        'REPORT_BREAKDOWN',
                      ],
                'manifest': {'reference': 'manifest', 'version': 1},
                'stops': [
                  {
                    'id': 'stop',
                    'stop_ref': 'stop',
                    'type': delivering ? 'DELIVERY' : 'PICKUP',
                    'sequence': 1,
                    'location': {'address': 'Current stop'},
                  },
                ],
                'handover': {
                  'status': 'ACTIVE',
                  'stage': delivering ? 'DELIVERY' : 'PICKUP',
                  'stop_ref': 'stop',
                },
              },
            },
          }),
          200,
        );
      });
      await tester.pumpWidget(
        ProviderSession(
          client: client,
          accessToken: () => 'token',
          refreshProfile: () async {},
          child: MaterialApp(
            theme: DispatchTheme.dark,
            home: const DeliveryOperationsScreen(tripId: 'trip'),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byType(FilledButton), findsOneWidget);
      expect(
        find.widgetWithText(FilledButton, 'Show pickup code'),
        findsOneWidget,
      );
      expect(find.text('Scan parcel'), findsOneWidget);
      expect(find.text('Start delivery'), findsOneWidget);
      expect(tester.takeException(), isNull);
      delivering = true;
      await tester.tap(find.byTooltip('Refresh delivery'));
      await tester.pumpAndSettle();
      expect(
        find.widgetWithText(FilledButton, 'Verify delivery code'),
        findsOneWidget,
      );
      await tester.ensureVisible(find.text('Recipient unavailable'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Recipient unavailable'));
      await tester.pumpAndSettle();
      expect(find.text('Recipient unavailable?'), findsOneWidget);
      expect(writes, isEmpty);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(writes, isEmpty);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      client.close();
    },
  );
}
