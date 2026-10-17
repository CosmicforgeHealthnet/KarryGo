import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/catalogue/state/catalogue_controller.dart';
import 'package:vendor/features/catalogue/state/catalogue_resource.dart';
import 'package:vendor/features/catalogue/ui/catalogue_editor.dart';
import 'package:vendor/features/catalogue/ui/catalogue_fields.dart';
import 'package:vendor/features/catalogue/ui/catalogue_screen.dart';
import 'package:vendor/app/vendor_app.dart';
import 'support/mock/mock_vendor_client.dart';
import 'support/mock/mock_onboarding_store.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  test(
    'modifier attachment, ordering and removal use the exact relationship contract',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final api = r.backend.foodCatalogueApi;
      const path = '/api/v1/vendor/food/items/{itemId}/modifier-groups';
      const pp = {'itemId': 'demo-food-0'};
      final second = await api.request(
        'POST',
        '/api/v1/vendor/food/modifier-groups',
        body: VendorPayload({
          'name': 'Choose a drink',
          'min_select': 0,
          'max_select': 1,
        }),
        command: VendorCommand.create(),
      );
      final ids = ['demo-group', second.object['id'].toString()];
      for (final id in ids) {
        await api.request(
          'PUT',
          '$path/{groupId}',
          pathParameters: {...pp, 'groupId': id},
          command: VendorCommand.create(),
        );
      }
      await api.request(
        'POST',
        '$path/reorder',
        pathParameters: pp,
        body: VendorPayload({'ordered_ids': ids.reversed.toList()}),
        command: VendorCommand.create(),
      );
      final groups =
          (await api.request(
                'GET',
                path,
                pathParameters: pp,
              )).object['modifier_groups']
              as List;
      expect(groups.first['modifier_group_id'], ids.last);
      expect(groups.first['group']['name'], 'Choose a drink');
      await api.request(
        'DELETE',
        '$path/{groupId}',
        pathParameters: {...pp, 'groupId': ids.first},
        command: VendorCommand.create(),
      );
      expect(
        (await api.request(
          'GET',
          path,
          pathParameters: pp,
        )).object['modifier_groups'],
        hasLength(1),
      );
      expect(
        (await api.getFoodModifierGroupsGroupId(
          groupId: ids.first,
        )).object['status'],
        'ACTIVE',
      );
    },
  );
  test(
    'successful catalogue write survives a failed refresh without allowing another write',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final probe = CatalogueProbe(failRefresh: true);
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://mock.karryam.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: probe,
      );
      addTearDown(app.dispose);
      addTearDown(probe.close);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = CatalogueController(
        app,
        const CatalogueResource(CatalogueKind.products),
      );
      addTearDown(c.dispose);
      await c.load();
      expect(
        await c.write(
          'PATCH',
          c.resource.detail,
          {'productId': 'demo-product'},
          {'name': 'Saved product', 'expected_version': 1},
        ),
        isTrue,
      );
      expect(c.changed, isTrue);
      expect(c.stale, isTrue);
      expect(c.feedback, startsWith('Saved.'));
      expect(c.allowed('PATCH', c.resource.detail), isFalse);
    },
  );
  test(
    'denied catalogue command disables that command while retaining readable data',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final probe = CatalogueProbe(denyWrite: true);
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://mock.karryam.invalid'),
        sessions: MemoryVendorSessionStore(),
        httpClient: probe,
      );
      addTearDown(app.dispose);
      addTearDown(probe.close);
      await app.client.acceptSession(r.backend.auth.session!);
      final c = CatalogueController(
        app,
        const CatalogueResource(CatalogueKind.products),
      );
      addTearDown(c.dispose);
      await c.load();
      expect(
        await c.write(
          'PATCH',
          c.resource.detail,
          {'productId': 'demo-product'},
          {'name': 'Denied product', 'expected_version': 1},
        ),
        isFalse,
      );
      expect(c.stale, isFalse);
      expect(c.allowed('PATCH', c.resource.detail), isFalse);
      expect(c.rows.single['name'], 'Everyday grocery pack');
    },
  );
  test('money entry converts exact minor units without rounding', () {
    expect(catalogueMinor('3500.25'), 350025);
    expect(catalogueMinor('-2.50'), -250);
    expect(catalogueMinor('0'), 0);
    expect(catalogueMinor('1.009'), isNull);
    expect(catalogueMinor('NaN'), isNull);
  });
  test(
    'food menu, section and item save through their real scoped routes',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final menus = CatalogueController(
        r.backend,
        const CatalogueResource(CatalogueKind.menus, branchId: 'mock-branch'),
      );
      addTearDown(menus.dispose);
      await menus.load();
      expect(menus.error, isNull);
      expect(menus.rows, hasLength(1));
      final choices = await catalogueChoices(r, menus.resource, null);
      expect(choices['capability_type']!.keys, contains('RESTAURANT'));
      expect(
        await menus.write('POST', menus.resource.collection, {}, {
          'branch_id': 'mock-branch',
          'capability_type': 'RESTAURANT',
          'name': 'Lunch menu',
          'description': 'Lunch favourites',
        }),
        isTrue,
      );
      final menu = menus.rows.firstWhere((m) => m['name'] == 'Lunch menu');
      expect(menu['status'], 'DRAFT');
      final sections = CatalogueController(
        r.backend,
        menus.resource.child(menu)!,
      );
      addTearDown(sections.dispose);
      await sections.load();
      expect(
        await sections.write(
          'POST',
          sections.resource.collection,
          sections.resource.parameters,
          {'name': 'Main meals', 'description': ''},
        ),
        isTrue,
      );
      final items = CatalogueController(
        r.backend,
        sections.resource.child(sections.rows.single)!,
      );
      addTearDown(items.dispose);
      await items.load();
      expect(
        await items.write(
          'POST',
          items.resource.collection,
          items.resource.parameters,
          {
            'name': 'Rice bowl',
            'base_price_minor': 350000,
            'dietary_codes': [],
            'allergen_codes': [],
          },
        ),
        isTrue,
      );
      expect(items.rows.single['base_price_minor'], 350000);
      expect(items.rows.single['status'], 'DRAFT');
      expect(
        await menus.write(
          'POST',
          '${menus.resource.detail}/activate',
          menus.resource.detailParameters(menu['id'].toString()),
          {'expected_version': 1},
        ),
        isTrue,
      );
      expect((await menus.read(menu))['status'], 'ACTIVE');
    },
  );
  test(
    'shop product, variant and branch listing persist with server field names',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final products = CatalogueController(
        r.backend,
        const CatalogueResource(CatalogueKind.products),
      );
      addTearDown(products.dispose);
      await products.load();
      expect(
        await products.write('POST', products.resource.collection, {}, {
          'category_id': 'demo-category',
          'name': 'Premium rice',
          'product_class': 'DRY_BULK',
          'operating_models': ['GROCERY_SHOP'],
        }),
        isTrue,
      );
      final product = products.rows.firstWhere(
        (p) => p['name'] == 'Premium rice',
      );
      expect(product['category_ref'], 'demo-category');
      expect(product['status'], 'DRAFT');
      final variants = CatalogueController(
        r.backend,
        products.resource.child(product)!,
      );
      addTearDown(variants.dispose);
      await variants.load();
      expect(
        await variants.write(
          'POST',
          variants.resource.collection,
          variants.resource.parameters,
          {
            'name': 'One bag',
            'package_level': 'BAG',
            'pack_quantity': 1,
            'retail_price_minor': 450000,
          },
        ),
        isTrue,
      );
      final variant = variants.rows.single;
      final listings = CatalogueController(
        r.backend,
        const CatalogueResource(
          CatalogueKind.listings,
          parameters: {'branchId': 'mock-branch-east'},
        ),
      );
      addTearDown(listings.dispose);
      await listings.load();
      expect(
        await listings.write(
          'POST',
          listings.resource.collection,
          listings.resource.parameters,
          {
            'variant_id': variant['id'],
            'retail_enabled': true,
            'wholesale_enabled': false,
            'retail_override': 475000,
          },
        ),
        isTrue,
      );
      final listing = listings.rows.firstWhere(
        (l) => l['variant_ref'] == variant['id'],
      );
      expect(listing['branch_ref'], 'mock-branch-east');
      expect(listing['retail_price_override_minor'], 475000);
      expect((await listings.read(listing))['version'], 1);
    },
  );
  test('stale catalogue versions cannot overwrite an accepted edit', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = CatalogueController(
      r.backend,
      const CatalogueResource(CatalogueKind.products),
    );
    addTearDown(c.dispose);
    await c.load();
    final row = await c.read(c.rows.single);
    expect(
      await c.write(
        'PATCH',
        c.resource.detail,
        {'productId': 'demo-product'},
        {'name': 'Updated grocery pack', 'expected_version': row['version']},
      ),
      isTrue,
    );
    expect(
      await c.write(
        'PATCH',
        c.resource.detail,
        {'productId': 'demo-product'},
        {'name': 'Old edit', 'expected_version': row['version']},
      ),
      isFalse,
    );
    expect(c.stale, isTrue);
    expect(c.error, contains('Refresh'));
    await c.load();
    expect(c.rows.single['name'], 'Updated grocery pack');
  });
  test('identical creation retry replays one record', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final api = r.backend.shopCatalogueApi;
    final command = VendorCommand.create();
    final body = VendorPayload({
      'name': 'New category',
      'slug': 'new-category',
    });
    final a = await api.request(
      'POST',
      '/api/v1/vendor/shop/categories',
      body: body,
      command: command,
    );
    final b = await api.request(
      'POST',
      '/api/v1/vendor/shop/categories',
      body: body,
      command: command,
    );
    expect(a.object['id'], b.object['id']);
    final c = CatalogueController(
      r.backend,
      const CatalogueResource(CatalogueKind.categories),
    );
    addTearDown(c.dispose);
    await c.load();
    expect(c.rows.where((row) => row['name'] == 'New category'), hasLength(1));
  });
  test('food reorder and archive retain canonical state', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = CatalogueController(
      r.backend,
      const CatalogueResource(
        CatalogueKind.sections,
        parameters: {'menuId': 'demo-menu-mock-branch'},
      ),
    );
    addTearDown(c.dispose);
    await c.load();
    expect(
      await c.write('POST', c.resource.collection, c.resource.parameters, {
        'name': 'Sides',
        'description': '',
      }),
      isTrue,
    );
    final ids = c.rows.reversed.map((v) => v['id']).toList();
    expect(
      await c.write(
        'POST',
        '${c.resource.collection}/reorder',
        c.resource.parameters,
        {'ordered_ids': ids},
      ),
      isTrue,
    );
    expect(c.rows.first['name'], 'Sides');
    final row = await c.read(c.rows.first);
    expect(
      await c.write(
        'POST',
        '${c.resource.detail}/archive',
        c.resource.detailParameters(row['id'].toString()),
        {'expected_version': row['version']},
      ),
      isTrue,
    );
    expect(c.rows, hasLength(1));
    c.archived = true;
    await c.load();
    expect(c.rows, hasLength(2));
  });
  testWidgets(
    'menu opens a full screen and section creation uses a focused sheet',
    (tester) async {
      final r = (await tester.runAsync(dashboardRuntime))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: CatalogueScreen(
            runtime: r,
            resource: const CatalogueResource(
              CatalogueKind.menus,
              branchId: 'mock-branch',
            ),
            currency: 'NGN',
          ),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('Everyday favourites'));
      await flush(tester);
      expect(find.byType(CatalogueDetailScreen), findsOneWidget);
      expect(find.byType(BottomSheet), findsNothing);
      await tester.tap(find.text('Manage menu sections'));
      await flush(tester);
      await tester.tap(find.widgetWithText(FilledButton, 'Create section'));
      await flush(tester);
      expect(find.byType(BottomSheet), findsOneWidget);
      expect(find.byType(CatalogueEditor), findsOneWidget);
      expect(find.text('Customer favourites'), findsOneWidget);
    },
  );
  testWidgets('product editor handles narrow phone, large text and keyboard', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final r = (await tester.runAsync(dashboardRuntime))!;
    addTearDown(r.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: MediaQuery(
          data: const MediaQueryData(
            size: Size(360, 800),
            textScaler: TextScaler.linear(1.5),
            viewInsets: EdgeInsets.only(bottom: 280),
            disableAnimations: true,
          ),
          child: CatalogueEditor(
            runtime: r,
            resource: const CatalogueResource(CatalogueKind.products),
            currency: 'NGN',
            choices: const {
              'category_id': {'demo-category': 'Pantry essentials'},
            },
            onSave: (_) async => true,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    final button = tester.widget<FilledButton>(
      find.widgetWithText(FilledButton, 'Create product'),
    );
    expect(button.onPressed, isNotNull);
  });
}

class CatalogueProbe extends http.BaseClient {
  CatalogueProbe({this.failRefresh = false, this.denyWrite = false});
  final bool failRefresh, denyWrite;
  final delegate = MockVendorClient(
    onboardingScenario: MockOnboardingScenario.activated,
    latency: Duration.zero,
  );
  bool wrote = false;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.method == 'PATCH' && denyWrite) {
      return _error(403, 'Catalogue management access is required.');
    }
    if (request.method == 'GET' &&
        request.url.path == '/api/v1/vendor/shop/products' &&
        wrote &&
        failRefresh) {
      return _error(503, 'Catalogue read unavailable.');
    }
    final response = await delegate.send(request);
    if (request.method == 'PATCH' && response.statusCode == 200) wrote = true;
    return response;
  }

  http.StreamedResponse _error(int status, String message) =>
      http.StreamedResponse(
        Stream.value(
          utf8.encode(
            jsonEncode({
              'success': false,
              'error': {
                'code': status == 403 ? 'FORBIDDEN' : 'UNAVAILABLE',
                'message': message,
              },
            }),
          ),
        ),
        status,
        headers: {'content-type': 'application/json'},
      );
  @override
  void close() {
    delegate.close();
    super.close();
  }
}
