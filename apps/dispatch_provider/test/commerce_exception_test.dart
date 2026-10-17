import 'dart:convert';
import 'dart:io';
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
import 'package:dispatch_provider/features/missions/ui/commerce_exception_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Damage and discrepancy evidence stays bound to exact parcels and blocks pickup',
    () async {
      FlutterSecureStorage.setMockInitialValues({});
      await AppModeConfig.instance.setMode(AppMode.mock);
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        token: () => 'demo',
        client: ModeHttpClient(
          inner: MockClient(
            (_) async => throw StateError('Unexpected Live call'),
          ),
        ),
      );
      const mission =
          '/api/v1/provider/commerce-missions/demo-commerce-mission';
      const read =
          '/api/v1/provider/commerce-missions/by-trip/demo-commerce-trip';
      final initial = await api.read(read);
      final body = <String, dynamic>{
        'expected_version': initial['version'],
        'reason_code': 'PACKAGE_MISMATCH',
        'recovery_action': 'SUPPORT',
        'parcel_refs': ['demo-parcel-1'],
        'evidence': {'captured_at': DateTime.now().toUtc().toIso8601String()},
      };
      await expectLater(
        api.command('$mission/exceptions', body, operationRef: true),
        throwsA(isA<ApiException>()),
      );
      final directory = await Directory.systemTemp.createTemp(
        'dispatch-proof-',
      );
      try {
        final photo = await File('${directory.path}/photo.png').writeAsBytes(
          base64Decode(
            'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jT0cAAAAASUVORK5CYII=',
          ),
        );
        final media = await api.upload(
          '$mission/evidence-content',
          '',
          photo.path,
          fields: {
            'operation_ref': 'proof-upload-a',
            'parcel_refs': jsonEncode(['demo-parcel-1']),
          },
        );
        final evidence = {
          ...(body['evidence'] as Map),
          'media_refs': [media],
        };
        await expectLater(
          api.command('$mission/exceptions', {
            ...body,
            'parcel_refs': ['demo-parcel-2'],
            'evidence': evidence,
          }, operationRef: true),
          throwsA(isA<ApiException>()),
        );
        await api.command('$mission/exceptions', {
          ...body,
          'evidence': evidence,
        }, operationRef: true);
        final saved = await api.read(read);
        expect(saved['pickup_actions_allowed'], false);
        expect(
          (saved['exceptions'] as List).single['reason_code'],
          'PACKAGE_MISMATCH',
        );
        expect((saved['evidence'] as List).single['attachment_count'], 1);
        await expectLater(
          api.command('$mission/stops/demo-stop-1/arrive', {
            'expected_version': saved['version'],
          }, operationRef: true),
          throwsA(isA<ApiException>()),
        );
      } finally {
        await directory.delete(recursive: true);
        api.client.close();
      }
    },
  );
  testWidgets('Damage report remains disabled without the required photo', (
    tester,
  ) async {
    final api = ProviderOperationsApi(
      origin: 'https://dispatch.test',
      token: () => 'provider',
      client: MockClient((_) async => http.Response('{}', 500)),
    );
    final mission = <String, dynamic>{
      'mission_ref': 'm',
      'version': 1,
      'exception_options': [
        {
          'reason_code': 'DAMAGED_PACKAGE',
          'recovery_actions': ['SUPPORT'],
          'photo_required': true,
        },
      ],
      'parcels': [
        {'parcel_ref': 'parcel-a', 'package_count': 1},
      ],
    };
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.light,
        home: CommerceExceptionScreen(api: api, mission: mission),
      ),
    );
    await tester.tap(find.text('Problem'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Package damaged').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Recovery action'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Support review').last);
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byType(CheckboxListTile));
    await tester.tap(find.byType(CheckboxListTile));
    await tester.pump();
    expect(find.text('Add required photo'), findsOneWidget);
    final button = find.widgetWithText(FilledButton, 'Submit report');
    expect(tester.widget<FilledButton>(button).onPressed, isNull);
    expect(find.text('Report recorded'), findsNothing);
    await tester.pumpWidget(const SizedBox());
  });
}
