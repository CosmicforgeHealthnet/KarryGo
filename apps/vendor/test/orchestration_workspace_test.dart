import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/orchestration/state/orchestration_workspace_controller.dart';
import 'package:vendor/features/orchestration/ui/orchestration_workspace_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

class OrchestrationTransport extends FinanceTransport {
  bool loseOverride = false;
  final overrides = <JsonObject>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.method != 'POST' ||
        !request.url.path.endsWith('/authority-override')) {
      return super.send(request);
    }
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    overrides.add({
      'body': jsonDecode(utf8.decode(bytes)),
      'headers': {
        for (final e in request.headers.entries) e.key.toLowerCase(): e.value,
      },
    });
    final result = await super.send(copy);
    if (loseOverride) {
      loseOverride = false;
      await result.stream.drain<void>();
      throw http.ClientException('Lost accepted override');
    }
    return result;
  }
}

const overrideBody = {
  'authority': 'HAULING',
  'product_code': '',
  'reason': 'Reviewed transport requirements',
};

void main() {
  test(
    'lost override is confirmed by audit history after reopening without duplicate POST',
    () async {
      final transport = OrchestrationTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final c = OrchestrationWorkspaceController(
        runtime,
        'demo-fulfilment-group-4',
      );
      await c.load();
      expect(c.canOverride, isTrue);
      transport.loseOverride = true;
      expect(
        await c.submitOverride(overrideBody, c.record!['version']),
        isFalse,
      );
      expect(c.pending, isNotNull);
      c.dispose();
      final reopened = OrchestrationWorkspaceController(
        runtime,
        'demo-fulfilment-group-4',
      );
      await reopened.load();
      expect(reopened.pending, isNull);
      expect(reopened.record!['selected_authority'], 'HAULING');
      expect(reopened.record!['selection_source'], 'ADMIN_OVERRIDE');
      expect(reopened.feedback, contains('confirms'));
      expect(transport.overrides.length, 1);
      expect(transport.overrides.single['headers']['idempotency-key'], isNull);
      expect(transport.overrides.single['headers']['if-match'], isNull);
      reopened.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  test(
    'executing groups and changed review versions never submit an override',
    () async {
      final transport = OrchestrationTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final executing = OrchestrationWorkspaceController(
        runtime,
        'demo-fulfilment-group-1',
      );
      await executing.load();
      expect(executing.canOverride, isFalse);
      expect(await executing.submitOverride(overrideBody, 1), isFalse);
      expect(transport.overrides, isEmpty);
      final planned = OrchestrationWorkspaceController(
        runtime,
        'demo-fulfilment-group-4',
      );
      await planned.load();
      await runtime.backend.orchestrationApi.request(
        'POST',
        '$orchestrationBase/authority-override',
        pathParameters: {'groupId': planned.groupId},
        body: VendorPayload({...overrideBody, 'authority': 'TAXI'}),
      );
      expect(await planned.submitOverride(overrideBody, 1), isFalse);
      expect(transport.overrides.length, 1);
      expect(planned.record!['selected_authority'], 'TAXI');
      expect(planned.error, contains('changed'));
      executing.dispose();
      planned.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  test('missing orchestration is an explicit not-found state', () async {
    final runtime = SupportRuntime(await financeApp(OrchestrationTransport()));
    final c = OrchestrationWorkspaceController(runtime, 'unrelated-group');
    await c.load();
    expect(c.errorCode, 'ORCHESTRATION_NOT_FOUND');
    expect(c.record, isNull);
    expect(c.canOverride, isFalse);
    c.dispose();
    runtime.value.dispose();
    runtime.dispose();
  });

  testWidgets('decision detail and override sheet fit a small dark phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final app = (await tester.runAsync(
      () => financeApp(OrchestrationTransport()),
    ))!;
    final runtime = SupportRuntime(app);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: OrchestrationWorkspaceScreen(
          runtime: runtime,
          groupId: 'demo-fulfilment-group-4',
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Waiting Readiness'), findsWidgets);
    expect(tester.takeException(), isNull);
    await tester.scrollUntilVisible(
      find.text('Override authority'),
      400,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.drag(find.byType(ListView).first, const Offset(0, -300));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Override authority'));
    await tester.pumpAndSettle();
    expect(find.text('Override reason'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    app.dispose();
    runtime.dispose();
  });
}
