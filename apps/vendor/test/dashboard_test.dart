import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/app/data/mock_vendor_client.dart';
import 'package:vendor/app/data/mock_onboarding_store.dart';
import 'package:vendor/app/data/mock_dashboard_data.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/app/vendor_runtime.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/dashboard/state/dashboard_controller.dart';
import 'package:vendor/features/dashboard/ui/vendor_dashboard_screen.dart';
import 'package:vendor/features/dashboard/ui/vendor_workspace_screen.dart';
import 'package:vendor/features/orders/ui/order_detail_screen.dart';

Future<VendorRuntime> dashboardRuntime({
  MockDashboardScenario scenario = MockDashboardScenario.populated,
  bool activated = true,
}) async {
  final r =
      VendorRuntime(
          preferences: VendorPreferences(inMemory: true),
          mockLatency: Duration.zero,
        )
        ..scenario = MockAuthScenario.singleVendor
        ..onboardingScenario = activated
            ? MockOnboardingScenario.activated
            : MockOnboardingScenario.draft
        ..dashboardScenario = scenario;
  await r.initialize();
  await r.start('owner@example.com', email: true);
  await r.verify(MockVendorClient.code);
  return r;
}

Future<void> flush(WidgetTester tester) async {
  for (var i = 0; i < 25; i++) {
    await tester.pump(const Duration(milliseconds: 10));
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 1)),
    );
  }
  await tester.pumpAndSettle();
}

void main() {
  test('wallet money preserves integer minor units', () {
    expect(dashboardMoney(28465000, 'NGN'), 'NGN 284,650.00');
    expect(dashboardMoney(-125, 'NGN'), '-NGN 1.25');
    expect(dashboardMoney(null, 'NGN'), 'Unavailable');
  });
  test('branch scope changes the order queue and clears catalogue', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = DashboardController(r.backend);
    addTearDown(c.dispose);
    await c.load();
    expect(c.errors, isEmpty);
    expect(c.wallet['available_kobo'], 28465000);
    expect(c.branches, hasLength(2));
    expect(c.orders.every((o) => o['branch_id'] == 'mock-branch'), isTrue);
    await c.loadCatalogue();
    expect(c.menus, hasLength(1));
    expect(c.products, hasLength(1));
    await c.selectBranch('mock-branch-east');
    expect(c.orders.every((o) => o['branch_id'] == 'mock-branch-east'), isTrue);
    expect(c.menus, isEmpty);
    expect(c.catalogueLoaded, isFalse);
    await c.selectBranch('foreign-branch');
    expect(c.branchId, 'mock-branch-east');
  });
  for (final scenario in [
    MockDashboardScenario.empty,
    MockDashboardScenario.restricted,
    MockDashboardScenario.unavailable,
  ]) {
    test(
      '${scenario.name} keeps independent dashboard resource states',
      () async {
        final r = await dashboardRuntime(scenario: scenario);
        addTearDown(r.dispose);
        final c = DashboardController(r.backend);
        addTearDown(c.dispose);
        await c.load();
        if (scenario == MockDashboardScenario.empty) {
          expect(c.orders, isEmpty);
          expect(c.wallet['available_kobo'], 0);
        } else if (scenario == MockDashboardScenario.restricted) {
          expect(c.errors['wallet'], isNotNull);
          expect(c.wallet, isEmpty);
          expect(c.orders, isNotEmpty);
        } else {
          expect(c.errors['orders'], isNotNull);
          expect(c.orders, isEmpty);
          expect(c.wallet, isNotEmpty);
        }
      },
    );
  }
  testWidgets('five tabs, branch sheet and order details fit a small phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: VendorWorkspaceScreen(runtime: r, onSettings: () {}),
      ),
    );
    await flush(tester);
    expect(find.byType(NavigationDestination), findsNWidgets(5));
    expect(find.text('NGN 284,650.00'), findsOneWidget);
    expect(tester.takeException(), isNull);
    Future<void> tab(String label) async {
      await tester.tap(
        find.descendant(
          of: find.byType(NavigationBar),
          matching: find.text(label),
        ),
      );
      await flush(tester);
    }

    await tester.tap(find.byTooltip('Hide balances'));
    await tester.pumpAndSettle();
    expect(find.text('NGN 284,650.00'), findsNothing);
    await tab('Finance');
    expect(find.text('NGN 284,650.00'), findsNothing);
    await tester.tap(find.byTooltip('Show balances'));
    await tester.pumpAndSettle();
    expect(find.text('NGN 284,650.00'), findsOneWidget);
    await tab('Home');
    expect(find.text('NGN 284,650.00'), findsOneWidget);

    await tab('Orders');
    await tester.tap(find.text('#demo-order-1048'));
    await flush(tester);
    expect(find.text('Order details'), findsOneWidget);
    expect(find.byType(BottomSheet), findsNothing);
    expect(find.text('Cancel order'), findsOneWidget);
    expect(find.text('Reject order'), findsOneWidget);
    expect(find.text('More actions'), findsNothing);
    await tester.scrollUntilVisible(
      find.text('Jollof rice meal'),
      180,
      scrollable: find
          .descendant(
            of: find.byType(OrderDetailScreen),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.pumpAndSettle();
    expect(find.text('Jollof rice meal'), findsOneWidget);
    Navigator.of(tester.element(find.text('Order details'))).pop();
    await tester.pumpAndSettle();
    await tab('Catalogue');
    expect(find.text('Food menus'), findsOneWidget);
    await tester.tap(find.text('Everyday favourites'));
    await flush(tester);
    expect(find.text('Manage menu sections'), findsOneWidget);
    expect(find.byType(BottomSheet), findsNothing);
    Navigator.of(tester.element(find.text('Manage menu sections'))).pop();
    await flush(tester);
    await tester.tap(find.text('Shop products'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Everyday grocery pack'));
    await flush(tester);
    expect(find.text('Manage variants'), findsOneWidget);
    expect(find.byType(BottomSheet), findsNothing);
    Navigator.of(tester.element(find.text('Manage variants'))).pop();
    await flush(tester);
    expect(tester.takeException(), isNull);
    await tab('Finance');
    expect(find.text('Finance'), findsWidgets);
    expect(tester.takeException(), isNull);
    await tab('More');
    expect(find.text('Business setup'), findsOneWidget);
    await tester.ensureVisible(find.text('Switch branch'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Switch branch'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Lekki branch'));
    await flush(tester);
    await tab('Home');
    expect(find.text('Business overview'), findsOneWidget);
    await tester.tap(find.byTooltip('Order activity'));
    await tester.pumpAndSettle();
    expect(find.text('Order activity'), findsOneWidget);
    expect(find.text('Lekki branch / Recent order states'), findsOneWidget);
    Navigator.of(tester.element(find.text('Order activity'))).pop();
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await flush(tester);
  });
  testWidgets('unactivated businesses remain in onboarding', (tester) async {
    final r = (await tester.runAsync(
      () => dashboardRuntime(activated: false),
    ))!;
    addTearDown(r.dispose);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: VendorWorkspaceScreen(runtime: r, onSettings: () {}),
      ),
    );
    await flush(tester);
    expect(find.text('Your next chapter starts here.'), findsOneWidget);
    expect(find.byType(NavigationBar), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
    await flush(tester);
  });
}
