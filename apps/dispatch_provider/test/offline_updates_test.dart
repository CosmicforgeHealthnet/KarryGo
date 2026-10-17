import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/network/mode_http_client.dart';
import 'package:dispatch_provider/features/offline/state/offline_updates_controller.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.mock);
  });
  test(
    'Retry survives restart with unchanged identity and account isolation',
    () async {
      final requests = <http.Request>[];
      var fail = true;
      final client = MockClient((request) async {
        requests.add(request);
        if (fail) throw http.ClientException('Offline');
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'state': 'ACCEPTED', 'command_type': 'LOCATION_FIX'},
          }),
          200,
        );
      });
      OfflineUpdatesController controller(String owner) =>
          OfflineUpdatesController(
            client: client,
            origin: 'https://dispatch.test',
            token: () => 'token',
            providerId: () => owner,
          );
      final first = controller('one');
      await first.saveLocation(
        latitude: 6.5,
        longitude: 3.4,
        heading: 0,
        speed: 1,
        accuracy: 10,
        observedAt: DateTime.now().toUtc(),
      );
      await first.sync(force: true);
      expect(first.pending, 1);
      first.dispose();
      final other = controller('two');
      await other.start();
      expect(other.records, isEmpty);
      other.dispose();
      final restored = controller('one');
      fail = false;
      await restored.sync(force: true);
      expect(restored.records.single['state'], 'ACCEPTED');
      expect(requests[0].body, requests[1].body);
      expect(
        requests[0].headers['Idempotency-Key'],
        requests[1].headers['Idempotency-Key'],
      );
      restored.dispose();
    },
  );
  test(
    'Mock evidence remains pending confirmation without a network call',
    () async {
      final client = ModeHttpClient(
        inner: MockClient((_) async => throw StateError('Network used')),
      );
      final queue = OfflineUpdatesController(
        client: client,
        origin: 'https://dispatch.test',
        token: () => 'token',
        providerId: () => 'one',
      );
      await queue.saveEvidence(['media-one'], '/delivery/evidence');
      await queue.sync(force: true);
      expect(queue.records.single['state'], 'PENDING_EVIDENCE_SYNC');
      expect(queue.needsReview, 1);
      queue.dispose();
      client.close();
    },
  );
}
