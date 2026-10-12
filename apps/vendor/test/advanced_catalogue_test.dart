import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/catalogue/state/advanced_controller.dart';
import 'package:vendor/features/catalogue/state/advanced_resource.dart';
import 'package:vendor/features/catalogue/ui/advanced_screen.dart';
import 'package:vendor/features/catalogue/ui/advanced_fields.dart';
import 'package:vendor/features/catalogue/ui/catalogue_editor.dart';
import 'package:vendor/features/catalogue/state/catalogue_resource.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  test(
    'food profiles have independent versions, clear to inheritance and never publish an item',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final availability = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.availability,
          parameters: {'itemId': 'demo-food-0'},
        ),
      );
      final prep = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.prep,
          parameters: {'itemId': 'demo-food-0'},
        ),
      );
      final effective = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.effective,
          parameters: {'itemId': 'demo-food-0'},
        ),
      );
      addTearDown(availability.dispose);
      addTearDown(prep.dispose);
      addTearDown(effective.dispose);
      await availability.load();
      await prep.load();
      expect(availability.error, isNull);
      expect(prep.error, isNull);
      expect(prep.metadata['configured'], false);
      expect(
        await prep.mutate('PUT', prep.resource.path, {
          'prep_min_minutes': 8,
          'prep_max_minutes': 16,
          'expected_version': 0,
        }, parameters: prep.resource.parameters),
        true,
      );
      expect(prep.rows.single['version'], 1);
      expect(
        await availability.mutate(
          'PUT',
          availability.resource.path,
          {
            'status': 'SOLD_OUT',
            'reason': 'Today sold out',
            'expected_version': 0,
          },
          parameters: availability.resource.parameters,
        ),
        true,
      );
      await effective.load();
      expect(effective.error, isNull);
      expect(effective.rows.single['sellability']['sellable'], false);
      expect(effective.rows.single['prep']['prep_min_minutes'], 8);
      expect(
        await prep.mutate('DELETE', prep.resource.path, {
          'expected_version': 1,
        }, parameters: prep.resource.parameters),
        true,
      );
      expect(prep.metadata['configured'], false);
      expect(availability.rows.single['status'], 'SOLD_OUT');
    },
  );
  test(
    'menu schedules preserve overnight windows and clear optional dates',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final c = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.schedules,
          parameters: {'menuId': 'demo-menu-mock-branch'},
        ),
      );
      addTearDown(c.dispose);
      await c.load();
      expect(c.error, isNull);
      expect(c.metadata['timezone'], 'Africa/Lagos');
      expect(
        await c.mutate('POST', c.resource.path, {
          'label': 'DINNER',
          'days_of_week': [5, 6],
          'start_local_time': '20:00',
          'end_local_time': '02:00',
          'effective_start_date': '2026-01-01',
        }, parameters: c.resource.parameters),
        true,
      );
      final row = c.rows.single;
      expect(row['overnight'], true);
      expect(
        await c.mutate('PATCH', c.resource.detail, {
          'clear_effective_start_date': true,
          'expected_version': row['version'],
        }, parameters: c.resource.params(row)),
        true,
      );
      expect(c.rows.single['effective_start_date'], isNull);
      expect(
        await c.mutate(
          'POST',
          c.resource.removePath,
          {'expected_version': c.rows.single['version']},
          parameters: c.resource.params(c.rows.single),
        ),
        true,
      );
      expect(c.rows, isEmpty);
    },
  );
  test(
    'new sell units require explicit branch selection before unit-specific quantity pricing',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final units = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.sellUnits,
          parameters: {'productId': 'demo-product'},
        ),
      );
      final listing = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.listingUnits,
          parameters: {'listingId': 'demo-listing'},
        ),
      );
      final tiers = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.quantityBreaks,
          parameters: {'listingId': 'demo-listing'},
        ),
      );
      addTearDown(units.dispose);
      addTearDown(listing.dispose);
      addTearDown(tiers.dispose);
      await units.load();
      await listing.load();
      await tiers.load();
      expect(units.error, isNull);
      expect(listing.error, isNull);
      expect(tiers.error, isNull);
      expect(
        await units.mutate('POST', units.resource.path, {
          'variant_id': 'demo-variant',
          'measure_id': 'demo-measure-unit',
          'fraction_numerator': 1,
          'fraction_denominator': 1,
          'display_name': 'Single pack',
          'measurement_mode': 'FIXED',
          'price_minor': 250000,
          'retail_enabled': true,
          'wholesale_enabled': false,
        }, parameters: units.resource.parameters),
        true,
      );
      final id = units.rows.last['id'];
      await listing.load();
      expect(listing.rows.any((u) => u['sell_unit_ref'] == id), false);
      expect(
        await tiers.mutate('POST', tiers.resource.path, {
          'sell_unit_id': id,
          'min_quantity': 6,
          'unit_price_minor': 225000,
        }, parameters: tiers.resource.parameters),
        false,
      );
      await tiers.load();
      expect(
        await listing.mutate('PATCH', listing.resource.path, {
          'sell_unit_ids': [id],
          'expected_version': listing.metadata['listing']['version'],
        }, parameters: listing.resource.parameters),
        true,
      );
      expect(
        listing.rows.where((u) => u['enabled'] == true).single['sell_unit_ref'],
        id,
      );
      expect(
        await tiers.mutate('POST', tiers.resource.path, {
          'sell_unit_id': id,
          'min_quantity': 6,
          'unit_price_minor': 225000,
        }, parameters: tiers.resource.parameters),
        true,
      );
      expect(
        await tiers.mutate('POST', tiers.resource.path, {
          'sell_unit_id': id,
          'min_quantity': 8,
          'unit_price_minor': 215000,
        }, parameters: tiers.resource.parameters),
        false,
      );
    },
  );
  test(
    'platform measures are immutable but business aliases can be added and removed',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final c = AdvancedController(
        runtime.backend,
        const AdvancedResource(AdvancedKind.measures),
      );
      addTearDown(c.dispose);
      await c.load();
      expect(c.error, isNull);
      final unit = c.rows.firstWhere((r) => r['id'] == 'demo-measure-unit');
      expect(
        await c.mutate('PATCH', c.resource.detail, {
          'display_name': 'Changed',
          'expected_version': unit['version'],
        }, parameters: c.resource.params(unit)),
        false,
      );
      final a = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.aliases,
          parameters: {'measureId': 'demo-measure-unit'},
        ),
      );
      addTearDown(a.dispose);
      await a.load();
      expect(a.error, isNull);
      expect(
        await a.mutate('POST', a.resource.path, {
          'alias': 'Each',
          'region_code': 'NG',
        }, parameters: a.resource.parameters),
        true,
      );
      expect(a.rows.single['scope'], 'BUSINESS');
      expect(
        await a.mutate(
          'DELETE',
          a.resource.removePath,
          {},
          parameters: a.resource.params(a.rows.single),
        ),
        true,
      );
      expect(a.rows, isEmpty);
    },
  );
  test(
    'fresh options follow product class and archived attributes restore through upsert',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final c = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.fresh,
          parameters: {'productId': 'demo-product'},
        ),
      );
      addTearDown(c.dispose);
      await c.load();
      expect(c.error, isNull);
      expect(
        await c.mutate('PUT', c.resource.path, {
          'attribute_type': 'GUTTING',
          'options': [
            {'value': 'GUTTED', 'display_name': 'Gutted', 'is_default': true},
          ],
          'expected_version': 0,
        }, parameters: c.resource.parameters),
        false,
      );
      await c.load();
      expect(
        await c.mutate('PUT', c.resource.path, {
          'attribute_type': 'PACKAGING_PREFERENCE',
          'options': [
            {'value': 'BAGGED', 'display_name': 'Bag', 'is_default': true},
          ],
          'expected_version': 0,
        }, parameters: c.resource.parameters),
        true,
      );
      final row = c.rows.single;
      expect(
        await c.mutate('POST', c.resource.removePath, {
          'expected_version': row['version'],
        }, parameters: c.resource.params(row)),
        true,
      );
      expect(c.rows, isEmpty);
      c.archived = true;
      await c.load();
      final archived = c.rows.single;
      expect(
        await c.mutate('PUT', c.resource.path, {
          'attribute_type': 'PACKAGING_PREFERENCE',
          'options': [
            {
              'value': 'BAGGED',
              'display_name': 'Carry bag',
              'is_default': true,
            },
          ],
          'expected_version': archived['version'],
        }, parameters: c.resource.parameters),
        true,
      );
      expect(c.rows.single['status'], 'ACTIVE');
      expect(c.rows.single['options'].single['display_name'], 'Carry bag');
    },
  );
  test(
    'every advanced action resolves to an existing generated backend operation',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      for (final kind in AdvancedKind.values) {
        final resource = AdvancedResource(kind);
        final c = AdvancedController(runtime.backend, resource);
        addTearDown(c.dispose);
        expect(
          c.api.operation('GET', resource.path),
          isNotNull,
          reason: kind.name,
        );
        if (!resource.readOnly &&
            !resource.singleton &&
            kind != AdvancedKind.listingUnits) {
          expect(
            c.api.operation(resource.saveMethod, resource.path),
            isNotNull,
            reason: kind.name,
          );
        }
        if (resource.canEdit) {
          expect(
            c.api.operation(resource.editMethod, resource.editPath),
            isNotNull,
            reason: kind.name,
          );
        }
        if (resource.canRemove) {
          expect(
            c.api.operation(resource.removeMethod, resource.removePath),
            isNotNull,
            reason: kind.name,
          );
        }
      }
    },
  );
  test(
    'overnight menu windows use the opening day in the branch timezone',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final c = AdvancedController(
        runtime.backend,
        const AdvancedResource(
          AdvancedKind.schedules,
          parameters: {'menuId': 'demo-menu-mock-branch'},
        ),
      );
      addTearDown(c.dispose);
      await c.load();
      expect(
        await c.mutate('POST', c.resource.path, {
          'label': 'DINNER',
          'days_of_week': [5],
          'start_local_time': '22:00',
          'end_local_time': '02:00',
        }, parameters: c.resource.parameters),
        true,
      );
      const path = '/api/v1/vendor/food/items/{itemId}/availability/effective';
      final inside = await runtime.backend.foodCatalogueApi.request(
        'GET',
        path,
        pathParameters: {'itemId': 'demo-food-0'},
        query: {'at': '2026-10-09T23:30:00Z'},
      );
      final outside = await runtime.backend.foodCatalogueApi.request(
        'GET',
        path,
        pathParameters: {'itemId': 'demo-food-0'},
        query: {'at': '2026-10-10T01:30:00Z'},
      );
      expect(inside.object['sellability']['sellable'], true);
      expect(
        outside.object['sellability']['reason_code'],
        'OUTSIDE_MENU_SCHEDULE',
      );
    },
  );
  testWidgets('schedule setup fits a phone and opens a native editor', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final runtime = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(runtime.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: AdvancedCatalogueScreen(
          runtime: runtime,
          resource: const AdvancedResource(
            AdvancedKind.schedules,
            parameters: {'menuId': 'demo-menu-mock-branch'},
          ),
          currency: 'NGN',
        ),
      ),
    );
    await flush(tester);
    expect(tester.takeException(), isNull);
    final button = find.text('Add schedule');
    expect(button, findsOneWidget);
    await tester.runAsync(() async {
      await tester.tap(button);
      await Future<void>.delayed(const Duration(milliseconds: 50));
    });
    await flush(tester);
    expect(find.text('Days'), findsOneWidget);
    expect(find.text('Mon'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
  testWidgets('a preparation sheet saves before showing success feedback', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final runtime = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(runtime.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: AdvancedCatalogueScreen(
          runtime: runtime,
          resource: const AdvancedResource(
            AdvancedKind.prep,
            parameters: {'itemId': 'demo-food-0'},
          ),
          currency: 'NGN',
        ),
      ),
    );
    await flush(tester);
    await tester.ensureVisible(find.text('Configure'));
    await tester.pumpAndSettle();
    await tester.runAsync(() async {
      await tester.tap(find.text('Configure'));
      await Future<void>.delayed(const Duration(milliseconds: 50));
    });
    await flush(tester);
    expect(find.byType(BottomSheet), findsOneWidget);
    final save = find.widgetWithText(FilledButton, 'Add preparation time');
    expect(save, findsOneWidget);
    await tester.runAsync(() async {
      await tester.tap(save);
      await Future<void>.delayed(const Duration(milliseconds: 50));
    });
    await flush(tester);
    expect(find.text('Preparation time saved'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
  testWidgets(
    'fresh choices use a fixed backend type and fit large text with keyboard',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;

      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.view.resetViewInsets);
      final runtime = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(runtime.dispose);
      const resource = AdvancedResource(AdvancedKind.fresh);
      Map<String, dynamic>? submitted;
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          builder: (context, child) => MediaQuery(
            data: MediaQuery.of(
              context,
            ).copyWith(textScaler: const TextScaler.linear(1.4)),
            child: child!,
          ),
          home: CatalogueEditor(
            runtime: runtime,
            resource: const CatalogueResource(CatalogueKind.products),
            currency: 'NGN',
            entityLabel: 'fresh attribute',
            fieldsOverride: advancedFields(
              resource,
              freshType: 'PACKAGING_PREFERENCE',
            ),
            presets: advancedPresets(
              resource,
              freshType: 'PACKAGING_PREFERENCE',
            ),
            onSave: (body) async {
              submitted = body;
              return false;
            },
          ),
        ),
      );
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(
        find.byType(DropdownButtonFormField<String>),
        150,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.pumpAndSettle();
      final type = tester.widget<DropdownButtonFormField<String>>(
        find.byType(DropdownButtonFormField<String>),
      );
      expect(type.onChanged, isNull);
      expect(tester.takeException(), isNull);
      tester.view.viewInsets = const FakeViewPadding(bottom: 300);
      await tester.pumpAndSettle();
      await tester.tap(
        find.widgetWithText(FilledButton, 'Add fresh attribute'),
      );
      await tester.pumpAndSettle();
      expect(submitted?['attribute_type'], 'PACKAGING_PREFERENCE');
      expect(submitted?['options'].single['value'], 'BAGGED');
      expect(submitted?['options'].single.containsKey('option_value'), false);
      expect(tester.takeException(), isNull);
    },
  );
}
