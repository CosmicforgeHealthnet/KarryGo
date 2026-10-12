import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/app/data/mock_vendor_client.dart';
import 'package:vendor/app/data/mock_onboarding_store.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/features/catalogue/state/catalogue_tools_controller.dart';
import 'package:vendor/features/catalogue/ui/catalogue_tools_screen.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  test(
    'food preview applies absolute variant precedence and attached modifier rules',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.foodCatalogueApi;
      await api.request(
        'PUT',
        '/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}',
        pathParameters: {'itemId': 'demo-food-0', 'groupId': 'demo-group'},
        command: VendorCommand.create(),
      );
      await api.request(
        'PUT',
        '/api/v1/vendor/food/items/{itemId}/branch-override',
        pathParameters: {'itemId': 'demo-food-0'},
        body: VendorPayload({
          'base_price_override_minor': 400000,
          'expected_version': 0,
        }),
        command: VendorCommand.create(),
      );
      final variant = (await api.request(
        'POST',
        '/api/v1/vendor/food/items/{itemId}/variants',
        pathParameters: {'itemId': 'demo-food-0'},
        body: VendorPayload({'name': 'Large', 'price_override_minor': 600000}),
        command: VendorCommand.create(),
      )).object;
      final c = CatalogueToolsController(
        r.backend,
        CatalogueTool.foodPrice,
        record: {'id': 'demo-food-0'},
      );
      addTearDown(c.dispose);
      await c.prepare();
      expect(c.error, isNull);
      expect(c.groups, hasLength(1));
      await c.preview();
      expect(c.result['base_source'], 'BRANCH_OVERRIDE');
      expect(c.result['effective_unit_price_minor'], 400000);
      c.selectVariant(variant['id']);
      expect(c.result, isEmpty);
      c.toggleModifier('demo-modifier', true);
      await c.preview();
      expect(c.error, isNull);
      expect(c.result['base_source'], 'VARIANT_OVERRIDE');
      expect(c.result['base_price_minor'], 600000);
      expect(c.result['modifier_total_minor'], 50000);
      expect(c.result['effective_unit_price_minor'], 650000);
      final item = (await api.request(
        'GET',
        '/api/v1/vendor/food/items/{itemId}',
        pathParameters: {'itemId': 'demo-food-0'},
      )).object;
      expect(item['version'], 1);
    },
  );
  test(
    'required food choices block incomplete previews and a removed attachment is rejected',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.foodCatalogueApi;
      await api.request(
        'PATCH',
        '/api/v1/vendor/food/modifier-groups/{groupId}',
        pathParameters: {'groupId': 'demo-group'},
        body: VendorPayload({'min_select': 1, 'expected_version': 1}),
        command: VendorCommand.create(),
      );
      await api.request(
        'PUT',
        '/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}',
        pathParameters: {'itemId': 'demo-food-0', 'groupId': 'demo-group'},
        command: VendorCommand.create(),
      );
      final c = CatalogueToolsController(
        r.backend,
        CatalogueTool.foodPrice,
        record: {'id': 'demo-food-0'},
      );
      addTearDown(c.dispose);
      await c.prepare();
      expect(c.selectionValid, false);
      await c.preview();
      expect(c.result, isEmpty);
      expect(c.error, contains('required choices'));
      c.toggleModifier('demo-modifier', true);
      await c.preview();
      expect(c.error, isNull);
      expect(c.result, isNotEmpty);
      await api.request(
        'DELETE',
        '/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}',
        pathParameters: {'itemId': 'demo-food-0', 'groupId': 'demo-group'},
        command: VendorCommand.create(),
      );
      await c.preview();
      expect(c.result, isEmpty);
      expect(c.error, isNotNull);
    },
  );
  test(
    'shop preview handles quantity rules, disabled channels and authoritative unpriced values',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = CatalogueToolsController(
        r.backend,
        CatalogueTool.shopPrice,
        record: {'id': 'demo-listing'},
      );
      addTearDown(c.dispose);
      await c.prepare();
      expect(c.error, isNull);
      expect(c.units, hasLength(1));
      await c.preview(quantity: 3);
      expect(c.result['total_price_minor'], 825000);
      expect(c.result['order_rules_satisfied'], true);
      expect(c.result['availability'], 'UNAVAILABLE');
      c.selectUnit('demo-sell-unit');
      await c.preview(quantity: 2);
      expect(c.result['price_source'], 'SELL_UNIT_PRICE');
      c.selectChannel('wholesale');
      expect(c.result, isEmpty);
      await c.preview(quantity: 2);
      expect(c.error, isNull);
      expect(c.result['unit_price_minor'], isNull);
      expect(c.result['total_price_minor'], isNull);
      expect(c.result['order_rules_satisfied'], false);
      await c.preview(quantity: 0);
      expect(c.error, contains('quantity'));
    },
  );
  test(
    'branch unit override and wholesale quantity band resolve from their real scopes',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.shopCatalogueApi;
      await api.request(
        'PATCH',
        '/api/v1/vendor/shop/listings/{listingId}',
        pathParameters: {'listingId': 'demo-listing'},
        body: VendorPayload({
          'wholesale_enabled': true,
          'minimum_order_qty': 2,
          'order_multiple': 2,
          'expected_version': 1,
        }),
        command: VendorCommand.create(),
      );
      await api.request(
        'PATCH',
        '/api/v1/vendor/shop/products/{productId}/sell-units/{sellUnitId}',
        pathParameters: {
          'productId': 'demo-product',
          'sellUnitId': 'demo-sell-unit',
        },
        body: VendorPayload({
          'wholesale_enabled': true,
          'wholesale_price_minor': 250000,
          'expected_version': 1,
        }),
        command: VendorCommand.create(),
      );
      final listing = (await api.request(
        'GET',
        '/api/v1/vendor/shop/listings/{listingId}',
        pathParameters: {'listingId': 'demo-listing'},
      )).object;
      final row = (listing['sell_units'] as List).single;
      await api.request(
        'PUT',
        '/api/v1/vendor/shop/listings/{listingId}/sell-units/{listingUnitId}',
        pathParameters: {
          'listingId': 'demo-listing',
          'listingUnitId': row['id'],
        },
        body: VendorPayload({
          'price_override': 260000,
          'expected_version': row['version'],
        }),
        command: VendorCommand.create(),
      );
      await api.request(
        'POST',
        '/api/v1/vendor/shop/listings/{listingId}/quantity-breaks',
        pathParameters: {'listingId': 'demo-listing'},
        body: VendorPayload({
          'sell_unit_id': 'demo-sell-unit',
          'min_quantity': 6,
          'unit_price_minor': 220000,
          'wholesale_only': true,
        }),
        command: VendorCommand.create(),
      );
      final c = CatalogueToolsController(
        r.backend,
        CatalogueTool.shopPrice,
        record: {'id': 'demo-listing'},
      );
      addTearDown(c.dispose);
      await c.prepare();
      c.selectUnit('demo-sell-unit');
      await c.preview(quantity: 3);
      expect(c.result['unit_price_minor'], 260000);
      expect(c.result['price_source'], 'LISTING_SELL_UNIT_OVERRIDE');
      expect(c.result['order_rules_satisfied'], false);
      c.selectChannel('wholesale');
      await c.preview(quantity: 6);
      expect(c.result['price_source'], 'QUANTITY_BREAK');
      expect(c.result['total_price_minor'], 1320000);
      expect(c.result['order_rules_satisfied'], true);
      await c.preview(quantity: 4);
      expect(c.result['price_source'], 'SELL_UNIT_PRICE');
      expect(c.result['unit_price_minor'], 250000);
    },
  );
  test(
    'lookup is case insensitive, type scoped and respects identifier archiving',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = CatalogueToolsController(r.backend, CatalogueTool.lookup);
      addTearDown(c.dispose);
      await c.lookup(' demo-pack-01 ');
      expect(c.error, isNull);
      expect(c.result['product']['id'], 'demo-product');
      expect(c.result['variant']['id'], 'demo-variant');
      expect(c.result['identifier']['externally_verified'], false);
      await c.lookup('DEMO-PACK-01', type: 'BARCODE');
      expect(c.notFound, true);
      expect(c.result, isEmpty);
      await c.lookup('DEMO-PACK-01', type: 'INTERNAL_SKU');
      final identifier = c.result['identifier'];
      await r.backend.shopCatalogueApi.request(
        'POST',
        '/api/v1/vendor/shop/variants/{variantId}/identifiers/{identifierId}/archive',
        pathParameters: {
          'variantId': 'demo-variant',
          'identifierId': identifier['id'],
        },
        body: VendorPayload({'expected_version': identifier['version']}),
        command: VendorCommand.create(),
      );
      await c.lookup('DEMO-PACK-01');
      expect(c.notFound, true);
      expect(c.error, isNull);
    },
  );
  test(
    'read-only price POST has no idempotency key and a server error never produces demo pricing',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final client = ToolsProbe();
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://server.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: client,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = CatalogueToolsController(
        app,
        CatalogueTool.foodPrice,
        record: {'id': 'demo-food-0'},
      );
      addTearDown(c.dispose);
      await c.prepare();
      expect(c.error, isNull);
      await c.preview();
      expect(c.result, isNotEmpty);
      final request = client.calls.last;
      expect(request.method, 'POST');
      expect(
        request.headers.keys.any((k) => k.toLowerCase() == 'idempotency-key'),
        false,
      );
      expect(jsonDecode(request.body), {'modifier_refs': []});
      client.fail = true;
      await c.preview();
      expect(c.result, isEmpty);
      expect(c.error, isNotNull);
    },
  );
  test(
    'generic missing route is an error rather than a no-match lookup state',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final client = ToolsProbe()..missingRoute = true;
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://server.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: client,
      );
      addTearDown(app.dispose);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = CatalogueToolsController(app, CatalogueTool.lookup);
      addTearDown(c.dispose);
      await c.lookup('DEMO-PACK-01');
      expect(c.notFound, false);
      expect(c.error, isNotNull);
      expect(c.result, isEmpty);
    },
  );
  testWidgets(
    'food preview shows actual choices and the returned price breakdown',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.runAsync(
        () => r.backend.foodCatalogueApi.request(
          'PUT',
          '/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}',
          pathParameters: {'itemId': 'demo-food-0', 'groupId': 'demo-group'},
          command: VendorCommand.create(),
        ),
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: CataloguePriceScreen(
            runtime: r,
            record: const {'id': 'demo-food-0'},
            food: true,
          ),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('Plantain'));
      await tester.pumpAndSettle();
      await tester.runAsync(() async {
        await tester.tap(find.text('Check price'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      await tester.scrollUntilVisible(
        find.text('NGN 4,000.00'),
        180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      expect(find.text('NGN 4,000.00'), findsOneWidget);
      expect(find.text('Extras total'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'shop price preview fits a narrow phone and clears after quantity changes',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: CataloguePriceScreen(
            runtime: r,
            record: const {'id': 'demo-listing'},
            food: false,
          ),
        ),
      );
      await flush(tester);
      expect(tester.takeException(), isNull);
      await tester.runAsync(() async {
        await tester.tap(find.text('Check price'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      await tester.scrollUntilVisible(
        find.text('NGN 2,750.00').first,
        180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      expect(find.text('NGN 2,750.00'), findsWidgets);
      expect(tester.takeException(), isNull);
      await tester.scrollUntilVisible(
        find.byType(TextField),
        -180,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.enterText(find.byType(TextField), '2');
      await tester.pumpAndSettle();
      expect(find.text('NGN 2,750.00'), findsNothing);
    },
  );
  testWidgets(
    'lookup can open the actual product and supports large text and keyboard',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.view.resetViewInsets);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          builder: (context, child) => MediaQuery(
            data: MediaQuery.of(
              context,
            ).copyWith(textScaler: const TextScaler.linear(1.4)),
            child: child!,
          ),
          home: CatalogueLookupScreen(runtime: r, currency: 'NGN'),
        ),
      );
      await tester.pumpAndSettle();
      tester.view.viewInsets = const FakeViewPadding(bottom: 300);
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      tester.view.viewInsets = const FakeViewPadding();
      await tester.pumpAndSettle();
      await tester.runAsync(() async {
        await tester.tap(find.text('Find product'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      await tester.scrollUntilVisible(
        find.text('Open product'),
        220,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      expect(find.text('Everyday grocery pack'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.runAsync(() async {
        await tester.tap(find.text('Open product'));
        await Future<void>.delayed(const Duration(milliseconds: 40));
      });
      await flush(tester);
      expect(find.text('Selling settings'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}

class ToolsProbe extends http.BaseClient {
  final delegate = MockVendorClient(
    onboardingScenario: MockOnboardingScenario.activated,
    latency: Duration.zero,
  );
  final calls = <http.Request>[];
  bool fail = false, missingRoute = false;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    calls.add(request as http.Request);
    if (fail && request.url.path.endsWith('/pricing/resolve') ||
        missingRoute && request.url.path.endsWith('/identifier-lookup')) {
      return http.StreamedResponse(
        Stream.value(
          utf8.encode(
            jsonEncode({
              'success': false,
              'error': {
                'code': missingRoute ? 'NOT_FOUND' : 'SERVICE_UNAVAILABLE',
                'message': 'Server unavailable',
              },
            }),
          ),
        ),
        missingRoute ? 404 : 503,
        headers: {'content-type': 'application/json'},
      );
    }
    return delegate.send(request);
  }

  @override
  void close() {
    delegate.close();
    super.close();
  }
}
