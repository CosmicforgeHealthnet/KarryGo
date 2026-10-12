import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/promotions/state/promotion_rules.dart';
import 'package:vendor/features/promotions/state/promotions_workspace_controller.dart';
import 'package:vendor/features/promotions/ui/promotion_editor.dart';
import 'package:vendor/features/promotions/ui/promotions_workspace_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

class PromotionTransport extends FinanceTransport {
  bool losePromotionResponse = false;
  final writes = <JsonObject>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    final write =
        request.url.path.startsWith(promotionBase) && request.method != 'GET';
    if (write) {
      writes.add({
        'method': request.method,
        'body': jsonDecode(utf8.decode(bytes)),
        'headers': {
          for (final entry in request.headers.entries)
            entry.key.toLowerCase(): entry.value,
        },
      });
    }
    final response = await super.send(copy);
    if (write && losePromotionResponse) {
      losePromotionResponse = false;
      await response.stream.drain<void>();
      throw http.ClientException('Lost accepted promotion response');
    }
    return response;
  }
}

JsonObject input(JsonObject record) => {
  for (final key in [
    'name',
    'promotion_type',
    'currency',
    'percentage_bps',
    'fixed_amount_minor',
    'max_discount_minor',
    'minimum_order_minor',
    'starts_at',
    'ends_at',
    'funding_sponsor',
    'vendor_funding_bps',
    'coupon_required',
    'global_usage_limit',
    'per_customer_usage_limit',
    'combinability',
    'priority',
    'scopes',
    'bundle_requirements',
    'quantity_tiers',
  ])
    key: record[key],
};

void main() {
  test(
    'duplicate normalized coupon code is rejected without blocking recovery',
    () async {
      final runtime = SupportRuntime(await financeApp(PromotionTransport()));
      final c = PromotionsWorkspaceController(runtime, id: 'mock-promotion-0');
      await c.load();
      expect(
        await c.save(
          method: 'POST',
          path: '$promotionBase/{promotionId}/coupons',
          promotionId: c.id,
          body: {'code': 'WELCOME'},
        ),
        isNotNull,
      );
      expect(
        await c.save(
          method: 'POST',
          path: '$promotionBase/{promotionId}/coupons',
          promotionId: c.id,
          body: {'code': ' welcome '},
        ),
        isNull,
      );
      expect(c.pending, isNull);
      expect(c.writable, isTrue);
      expect(c.error, contains('already in use'));
      c.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );
  test(
    'lost coupon confirmation survives reopening and retries identically',
    () async {
      final transport = PromotionTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final first = PromotionsWorkspaceController(
        runtime,
        id: 'mock-promotion-0',
      );
      await first.load();
      transport.losePromotionResponse = true;
      expect(
        await first.save(
          method: 'POST',
          path: '$promotionBase/{promotionId}/coupons',
          promotionId: first.id,
          body: {'code': 'WELCOMEBACK', 'usage_limit': 5},
        ),
        isNull,
      );
      expect(first.pending, isNotNull);
      first.dispose();
      final reopened = PromotionsWorkspaceController(runtime);
      await reopened.load();
      expect(reopened.writable, isFalse);
      final saved = await reopened.save();
      expect(saved!['coupon_id'], startsWith('mock-coupon-'));
      expect(reopened.pending, isNull);
      expect(transport.writes.length, 2);
      expect(transport.writes.first, transport.writes.last);
      expect(transport.writes.first['headers']['if-match'], isNull);
      reopened.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  test(
    'conflicting status blocks writes until a fresh version is loaded',
    () async {
      final transport = PromotionTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final c = PromotionsWorkspaceController(runtime, id: 'mock-promotion-0');
      await c.load();
      await runtime.backend.promotionsApi.request(
        'POST',
        '$promotionBase/{promotionId}/status',
        pathParameters: {'promotionId': c.id!},
        body: VendorPayload({'status': 'PAUSED'}),
        command: VendorCommand.create(version: 1),
      );
      expect(
        await c.save(
          method: 'POST',
          path: '$promotionBase/{promotionId}/status',
          promotionId: c.id,
          version: 1,
          body: {'status': 'ENDED'},
        ),
        isNull,
      );
      expect(c.stale, isTrue);
      expect(c.writable, isFalse);
      final count = transport.writes.length;
      expect(
        await c.save(
          method: 'POST',
          path: '$promotionBase/{promotionId}/status',
          promotionId: c.id,
          version: 1,
          body: {'status': 'ENDED'},
        ),
        isNull,
      );
      expect(transport.writes.length, count);
      await c.load();
      expect(c.record!['version'], 2);
      final saved = await c.save(
        method: 'POST',
        path: '$promotionBase/{promotionId}/status',
        promotionId: c.id,
        version: 2,
        body: {'status': 'ENDED'},
      );
      expect(saved!['status'], 'ENDED');
      expect(saved['version'], 3);
      c.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  test(
    'quantity tiers and definition edits preserve exact backend fields',
    () async {
      final transport = PromotionTransport();
      final runtime = SupportRuntime(await financeApp(transport));
      final c = PromotionsWorkspaceController(runtime);
      await c.load();
      final body = input(c.rows.first)
        ..addAll({
          'name': 'Quantity offer',
          'promotion_type': 'QUANTITY_BREAK',
          'percentage_bps': 0,
          'quantity_tiers': [
            {
              'minimum_quantity': 2,
              'maximum_quantity': 4,
              'percentage_bps': 1000,
              'fixed_amount_minor': 0,
            },
            {
              'minimum_quantity': 5,
              'maximum_quantity': null,
              'percentage_bps': 1500,
              'fixed_amount_minor': 0,
            },
          ],
        });
      final created = await c.save(
        method: 'POST',
        path: promotionBase,
        body: body,
      );
      expect(created!['status'], 'DRAFT');
      expect(created['quantity_tiers'], body['quantity_tiers']);
      await c.load();
      final edited = await c.save(
        method: 'PATCH',
        path: '$promotionBase/{promotionId}',
        promotionId: created['id'],
        version: 1,
        body: {...body, 'name': 'Volume savings'},
      );
      expect(edited!['name'], 'Volume savings');
      expect(edited['version'], 2);
      expect(transport.writes.last['headers']['if-match'], '1');
      c.dispose();
      runtime.value.dispose();
      runtime.dispose();
    },
  );

  testWidgets('offer list, details and editor fit a small dark phone', (
    tester,
  ) async {
    late SupportRuntime runtime;
    await tester.runAsync(() async {
      runtime = SupportRuntime(await financeApp(PromotionTransport()));
    });
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: PromotionsWorkspaceScreen(runtime: runtime),
      ),
    );
    await flush(tester);
    expect(find.text('Lunch favourites'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: PromotionsWorkspaceScreen(
          key: const ValueKey('detail'),
          runtime: runtime,
          id: 'mock-promotion-0',
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Reserved & used redemptions'), findsOneWidget);
    expect(tester.takeException(), isNull);
    final c = PromotionsWorkspaceController(runtime, id: 'mock-promotion-0');
    await tester.runAsync(c.load);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: PromotionEditor(
          runtime: runtime,
          controller: c,
          record: c.record,
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Review offer'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    c.dispose();
    runtime.value.dispose();
    runtime.dispose();
  });
}
