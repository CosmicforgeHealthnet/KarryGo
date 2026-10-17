import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/mode_http_client.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/missions/data/provider_operations_api.dart';
import 'package:dispatch_provider/features/missions/ui/commerce_recovery_screen.dart';

const readPath =
    '/api/v1/provider/commerce-missions/by-trip/demo-commerce-trip';
const missionPath = '/api/v1/provider/commerce-missions/demo-commerce-mission';
Future<ProviderOperationsApi> preparedDemo() async {
  FlutterSecureStorage.setMockInitialValues({});
  await AppModeConfig.instance.setMode(AppMode.mock);
  final api = ProviderOperationsApi(
    origin: 'https://dispatch.test',
    token: () => 'demo',
    client: ModeHttpClient(
      inner: MockClient((_) async => throw StateError('Mock reached network')),
    ),
  );
  for (final i in [1, 2]) {
    for (final action in ['arrive', 'mock-sender-confirmation', 'complete']) {
      final view = await api.read(readPath);
      await api.command('$missionPath/stops/demo-stop-$i/$action', {
        'expected_version': view['version'],
        if (action == 'complete') ...{
          'manifest_ref': 'demo-manifest-$i',
          'manifest_version': 1,
          'parcel_refs': ['demo-parcel-$i'],
          'package_count': i,
        },
      }, operationRef: true);
    }
  }
  return api;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Return survives reload and requires the exact original parcels to finish',
    () async {
      final api = await preparedDemo();
      var view = await api.read(readPath);
      final started = await api.command('$missionPath/returns', {
        'expected_version': view['version'],
        'product_class': 'SHOP_RETAIL',
        'disposition': 'RETURN_TO_VENDOR',
        'reason_code': 'RECEIVER_REFUSED',
        'parcel_refs': ['demo-parcel-1', 'demo-parcel-2'],
        'evidence': {'captured_at': DateTime.now().toUtc().toIso8601String()},
      }, operationRef: true);
      view = await api.read(readPath);
      expect((view['returns'] as List).single['state'], 'IN_RETURN');
      final path = '$missionPath/returns/${started['return_id']}/complete';
      final body = {
        'return_id': started['return_id'],
        'expected_version': view['version'],
        'parcel_refs': ['demo-parcel-1'],
        'evidence': {'captured_at': DateTime.now().toUtc().toIso8601String()},
      };
      await expectLater(
        api.command(path, body, operationRef: true),
        throwsA(isA<ApiException>()),
      );
      await api.command(path, {
        ...body,
        'parcel_refs': ['demo-parcel-1', 'demo-parcel-2'],
      }, operationRef: true);
      view = await api.read(readPath);
      expect((view['returns'] as List).single['state'], 'RETURNED');
      expect(view['return_parcel_refs'], isEmpty);
      api.client.close();
    },
  );
  test(
    'Only incoming rider accepts transfer, and incoming rider cannot read the code',
    () async {
      final api = await preparedDemo();
      var view = await api.read(readPath);
      await api.command('$missionPath/mock-replacement', {
        'expected_version': view['version'],
      }, operationRef: true);
      view = await api.read(readPath);
      await api.command('$missionPath/provider-transfers/start', {
        'expected_version': view['version'],
        'parcel_refs': ['demo-parcel-1', 'demo-parcel-2'],
        'package_count': 3,
        'latitude': 6.45,
        'longitude': 3.4,
      }, operationRef: true);
      view = await api.read(readPath);
      expect(
        (await api.read(
          '$missionPath/provider-transfers/demo-transfer/credential',
        ))['challenge_code'],
        '123456',
      );
      final command = {
        'expected_version': view['version'],
        'parcel_refs': ['demo-parcel-1', 'demo-parcel-2'],
        'package_count': 3,
        'latitude': 6.45,
        'longitude': 3.4,
        'code': '123456',
        'evidence': {'captured_at': DateTime.now().toUtc().toIso8601String()},
      };
      await expectLater(
        api.command(
          '$missionPath/provider-transfers/demo-transfer/complete',
          command,
          operationRef: true,
        ),
        throwsA(isA<ApiException>()),
      );
      await api.command('$missionPath/mock-incoming-provider', {
        'expected_version': view['version'],
      }, operationRef: true);
      await expectLater(
        api.read('$missionPath/provider-transfers/demo-transfer/credential'),
        throwsA(isA<ApiException>()),
      );
      view = await api.read(readPath);
      await expectLater(
        api.command(
          '$missionPath/provider-transfers/demo-transfer/complete',
          {...command, 'expected_version': view['version'], 'code': '000000'},
          operationRef: true,
        ),
        throwsA(isA<ApiException>()),
      );
      await api.command(
        '$missionPath/provider-transfers/demo-transfer/complete',
        {...command, 'expected_version': view['version']},
        operationRef: true,
      );
      expect((await api.read(readPath))['transfer']['state'], 'COMPLETED');
      api.client.close();
    },
  );
  testWidgets(
    'Incoming custody button stays disabled until code and parcel confirmation',
    (tester) async {
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        token: () => 'provider',
        client: MockClient(
          (_) async =>
              http.Response(jsonEncode({'success': true, 'data': {}}), 200),
        ),
      );
      final data = <String, dynamic>{
        'mission_ref': 'm',
        'version': 1,
        'state': 'TRANSFER_PENDING',
        'returns': [],
        'return_parcel_refs': [],
        'transfer': {
          'transfer_ref': 't',
          'state': 'CHALLENGE_ISSUED',
          'role': 'INCOMING',
          'package_count': 2,
          'parcel_refs': ['p'],
          'expires_at': DateTime.now()
              .add(const Duration(minutes: 5))
              .toUtc()
              .toIso8601String(),
        },
      };
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          home: CommerceRecoveryScreen(api: api, tripId: 'trip', initial: data),
        ),
      );
      final button = find.widgetWithText(FilledButton, 'Accept custody');
      expect(tester.widget<FilledButton>(button).onPressed, isNull);
      await tester.enterText(find.byType(TextField), '123456');
      await tester.pump();
      expect(tester.widget<FilledButton>(button).onPressed, isNull);
      await tester.tap(find.byType(CheckboxListTile));
      await tester.pump();
      expect(tester.widget<FilledButton>(button).onPressed, isNotNull);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
