import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/delivery/state/rider_workspace_controller.dart';
import 'package:vendor/features/delivery/state/rider_assignment_controller.dart';
import 'package:vendor/features/delivery/ui/rider_workspace_screen.dart';
import 'package:vendor/features/delivery/ui/rider_assignment_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime, FailingSupportStorage;
import 'dashboard_test.dart' show flush;

class RiderTransport extends FinanceTransport {
  bool lose = false;
  int commands = 0;
  final riderKeys = <String?>[];
  final riderBodies = <JsonObject>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final write =
        ['PUT', 'POST'].contains(request.method) &&
        (request.url.path.contains('/delivery/rider/') ||
            request.url.path.endsWith('/rider') ||
            request.url.path.contains('/fulfilment/units/') ||
            request.url.path.contains('/fulfilment/parcels/') ||
            request.url.path.endsWith('/fallbacks') ||
            request.url.path.endsWith('/policy'));
    if (!write) return super.send(request);
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    commands++;
    riderKeys.add(
      request.headers.entries
          .where((e) => e.key.toLowerCase() == 'idempotency-key')
          .firstOrNull
          ?.value,
    );
    riderBodies.add(jsonObject(jsonDecode(utf8.decode(bytes))));
    final response = await super.send(copy);
    if (lose) {
      lose = false;
      throw http.ClientException('Lost response');
    }
    return response;
  }
}

Future<void> toReceiver(RiderWorkspaceController c) async {
  for (final action in [
    'acknowledge',
    'start-pickup',
    'arrive-pickup',
    'complete-pickup',
    'start-receiver',
    'arrive-receiver',
  ]) {
    expect(await c.execute(action), true, reason: action);
  }
}

void main() {
  test(
    'assigned rider follows verified delivery sequence and availability',
    () async {
      final app = await financeApp(RiderTransport());
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      final c = RiderWorkspaceController(runtime, id: 'demo-vendor-mission-1');
      addTearDown(c.dispose);
      await c.load();
      expect(await c.execute('complete-delivery'), false);
      await toReceiver(c);
      expect(
        await c.execute(
          'complete-delivery',
          evidence: 'demo-handover-demo-vendor-mission-1-delivery',
          method: 'OTP',
          credential: 'wrong',
        ),
        false,
      );
      await c.load();
      expect(
        await c.execute(
          'complete-delivery',
          evidence: 'demo-handover-demo-vendor-mission-1-delivery',
          method: 'OTP',
          credential: '654321',
        ),
        true,
      );
      expect(c.record!['stage'], 'COMPLETED');
      expect(c.actions, isEmpty);
      final hub = RiderWorkspaceController(runtime);
      addTearDown(hub.dispose);
      await hub.load();
      expect(hub.availability!['state'], 'AVAILABLE');
      expect(await hub.setAvailability('mock-branch', 'OFFLINE'), true);
    },
  );
  test('receiver unavailable leads to verified return', () async {
    final app = await financeApp(RiderTransport());
    addTearDown(app.dispose);
    final c = RiderWorkspaceController(
      SupportRuntime(app),
      id: 'demo-vendor-mission-1',
    );
    addTearDown(c.dispose);
    await c.load();
    await toReceiver(c);
    expect(await c.execute('receiver-unavailable'), false);
    expect(
      await c.execute(
        'receiver-unavailable',
        reason: 'Receiver cannot be reached',
      ),
      true,
    );
    expect(c.actions, contains('return'));
    expect(
      await c.execute(
        'return',
        evidence: 'demo-handover-demo-vendor-mission-1-return',
        method: 'QR',
        credential: '654321',
      ),
      true,
    );
    expect(c.record!['mission']['status'], 'RETURNED');
  });
  test(
    'lost verification recovers with identical key and no stored credential',
    () async {
      final transport = RiderTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final storage = VendorPreferences(inMemory: true);
      final runtime = SupportRuntime(app, store: storage);
      final c = RiderWorkspaceController(runtime, id: 'demo-vendor-mission-1');
      await c.load();
      await toReceiver(c);
      transport.lose = true;
      expect(
        await c.execute(
          'complete-delivery',
          evidence: 'demo-handover-demo-vendor-mission-1-delivery',
          method: 'OTP',
          credential: '654321',
        ),
        false,
      );
      final original = transport.riderKeys.last;
      final saved = await storage.read(c.attempt.key);
      expect(saved, isNot(contains('654321')));
      expect(c.attempt.pending, isNotNull);
      c.dispose();
      final recovered = RiderWorkspaceController(
        runtime,
        id: 'demo-vendor-mission-1',
      );
      addTearDown(recovered.dispose);
      await recovered.load();
      expect(recovered.record!['stage'], 'COMPLETED');
      final calls = transport.commands;
      expect(
        await recovered.execute('complete-delivery', credential: 'different'),
        false,
      );
      expect(transport.commands, calls);
      expect(
        await recovered.execute('complete-delivery', credential: '654321'),
        true,
      );
      expect(transport.riderKeys.last, original);
      expect(
        transport.riderBodies.last,
        transport.riderBodies[transport.riderBodies.length - 2],
      );
      expect(recovered.attempt.pending, isNull);
    },
  );
  test(
    'rider assignment filters real role and recovers lost assignment',
    () async {
      final transport = RiderTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final runtime = SupportRuntime(app);
      final mission =
          (await app.deliveryApi.getFulfilmentVendorMissionsMissionId(
            missionId: 'demo-vendor-mission-4',
          )).object;
      final c = RiderAssignmentController(runtime, mission);
      await c.load();
      expect(c.riders.any((r) => r['id'] == 'mock-staff-2'), false);
      expect(await c.assign('mock-staff-2'), false);
      transport.lose = true;
      expect(await c.assign('mock-staff-free-rider'), false);
      final key = transport.riderKeys.last;
      c.dispose();
      final recovered = RiderAssignmentController(runtime, mission);
      addTearDown(recovered.dispose);
      await recovered.load();
      expect(await recovered.assign(null), true);
      expect(transport.riderKeys.last, key);
    },
  );
  test('delivery recovery storage failure prevents commands', () async {
    final transport = RiderTransport();
    final app = await financeApp(transport);
    addTearDown(app.dispose);
    final runtime = SupportRuntime(app, store: FailingSupportStorage());
    final c = RiderWorkspaceController(runtime, id: 'demo-vendor-mission-1');
    addTearDown(c.dispose);
    await c.load();
    expect(await c.execute('acknowledge'), false);
    expect(transport.commands, 0);
  });
  testWidgets('rider workspace and assignment fit dark phone width', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final app = (await tester.runAsync(() => financeApp(RiderTransport())))!;
    addTearDown(app.dispose);
    final runtime = SupportRuntime(app);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: RiderWorkspaceScreen(
          runtime: runtime,
          id: 'demo-vendor-mission-1',
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Accept assignment'), findsOneWidget);
    expect(tester.takeException(), isNull);
    final mission = (await tester.runAsync(
      () => app.deliveryApi.getFulfilmentVendorMissionsMissionId(
        missionId: 'demo-vendor-mission-4',
      ),
    ))!.object;
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: RiderAssignmentScreen(runtime: runtime, mission: mission),
      ),
    );
    await flush(tester);
    expect(find.text('mock-staff-free-rider'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
