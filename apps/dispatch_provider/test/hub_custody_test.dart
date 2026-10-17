import 'dart:io';
import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/mode_http_client.dart';
import 'package:dispatch_provider/features/missions/data/provider_operations_api.dart';
import 'package:dispatch_provider/features/missions/ui/mission_photo_field.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Photo draft retains one upload per exact scope and preserves original capture time',
    () async {
      var uploads = 0;
      final client = MockClient((request) async {
        uploads++;
        expect(request.headers['Idempotency-Key'], isNotEmpty);
        expect(request.body, contains('manifest-one'));
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'media_ref': 'media-$uploads'},
          }),
          201,
        );
      });
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        client: client,
        token: () => 'provider',
      );
      final directory = await Directory.systemTemp.createTemp(
        'dispatch-photo-draft-',
      );
      try {
        final file = await File(
          '${directory.path}/proof.jpg',
        ).writeAsBytes([1, 2, 3]);
        final captured = DateTime.utc(2026, 10, 15, 12);
        final draft = MissionPhotoDraft()..select(file.path, captured);
        final first = await draft.attach(
          api,
          '/upload',
          ['a'],
          {},
          fields: {'manifest_ref': 'manifest-one'},
        );
        final second = await draft.attach(
          api,
          '/upload',
          ['a'],
          {},
          fields: {'manifest_ref': 'manifest-one'},
        );
        expect(uploads, 1);
        expect(first['media_refs'], second['media_refs']);
        expect(first['captured_at'], captured.toIso8601String());
        await draft.attach(
          api,
          '/upload',
          ['b'],
          {},
          fields: {'manifest_ref': 'manifest-one'},
        );
        expect(uploads, 2);
      } finally {
        await directory.delete(recursive: true);
        client.close();
      }
    },
  );
  test(
    'Hub checkpoints require the approved hold, exact scan set and photo',
    () async {
      FlutterSecureStorage.setMockInitialValues({});
      await AppModeConfig.instance.setMode(AppMode.mock);
      final api = ProviderOperationsApi(
        origin: 'https://dispatch.test',
        client: ModeHttpClient(
          inner: MockClient(
            (_) async => throw StateError('Unexpected network'),
          ),
        ),
        token: () => 'demo',
      );
      const path = '/api/v1/provider/trips/trip-active-1/hub-hold';
      await expectLater(api.read(path), throwsA(isA<ApiException>()));
      await api.command(
        '/api/v1/provider/trips/trip-active-1/mock-hub-hold',
        {},
      );
      final hub = await api.read(path);
      final directory = await Directory.systemTemp.createTemp(
        'dispatch-hub-proof-',
      );
      try {
        final file = await File(
          '${directory.path}/proof.png',
        ).writeAsBytes([1, 2, 3]);
        final draft = MissionPhotoDraft()
          ..select(file.path, DateTime.now().toUtc());
        final evidence = await draft.attach(
          api,
          '$path/evidence-content',
          (hub['parcel_refs'] as List).cast<String>(),
          {},
          fields: {'manifest_ref': hub['manifest_ref'] as String},
        );
        final body = {
          'stage': 'ARRIVAL',
          'parcel_codes': [...(hub['parcel_codes'] as List).take(1)],
          'media_ref': (evidence['media_refs'] as List).single,
        };
        await expectLater(
          api.command('$path/checkpoints', body),
          throwsA(isA<ApiException>()),
        );
        await expectLater(
          api.command('$path/checkpoints', {
            ...body,
            'parcel_codes': hub['parcel_codes'],
            'media_ref': 'unknown',
          }),
          throwsA(isA<ApiException>()),
        );
        await api.command('$path/checkpoints', {
          ...body,
          'parcel_codes': hub['parcel_codes'],
        });
        expect((await api.read(path))['state'], 'HELD');
        await api.command('$path/checkpoints', {
          ...body,
          'stage': 'RELEASE',
          'parcel_codes': hub['parcel_codes'],
        });
        expect((await api.read(path))['state'], 'RELEASED');
        expect((await api.read(path))['allowed_stage'], '');
      } finally {
        await directory.delete(recursive: true);
        api.client.close();
      }
    },
  );
}
