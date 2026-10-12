import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/core/ui/premium_loading.dart';
import 'package:vendor/core/ui/vendor_toast.dart';
import 'package:vendor/features/dashboard/ui/vendor_workspace_screen.dart';
import 'dashboard_test.dart' as fixture;

void main() {
  testWidgets(
    'dark workspace filters, finance links and success feedback stay dark',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final runtime = (await tester.runAsync(
        () => fixture.dashboardRuntime(),
      ))!;
      addTearDown(runtime.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: VendorWorkspaceScreen(runtime: runtime, onSettings: () {}),
        ),
      );
      await fixture.flush(tester);
      Future<void> tab(String label) async {
        await tester.tap(
          find.descendant(
            of: find.byType(NavigationBar),
            matching: find.text(label),
          ),
        );
        await fixture.flush(tester);
      }

      await tab('Orders');
      final selected = tester
          .widgetList<ChoiceChip>(find.byType(ChoiceChip))
          .firstWhere((chip) => chip.selected);
      expect(selected.selectedColor!.computeLuminance(), lessThan(.15));
      await tab('Finance');
      final surfaces = tester
          .widgetList<Material>(find.byType(Material))
          .where((m) => m.color != null && m.color!.a > .9);
      expect(surfaces.where((m) => m.color!.computeLuminance() > .5), isEmpty);
      final context = tester.element(find.byType(NavigationBar));
      showVendorToast(context, 'Changes saved');
      await tester.pump(const Duration(milliseconds: 150));
      expect(find.text('Changes saved'), findsOneWidget);
      await tester.pump(const Duration(milliseconds: 500));
      final snack = tester.widget<SnackBar>(find.byType(SnackBar));
      expect(snack.backgroundColor!.computeLuminance(), lessThan(.15));
      expect(snack.backgroundColor, KarryamColors.darkRaised);
      expect(tester.takeException(), isNull);
      await tester.pump(const Duration(seconds: 5));
      await tester.pumpAndSettle();
    },
  );

  testWidgets(
    'archive popup and adaptive colors follow dark surfaces and intermediate theme frames',
    (tester) async {
      late BuildContext scope;
      final light = vendorTheme(),
          dark = vendorTheme(brightness: Brightness.dark);
      await tester.pumpWidget(
        MaterialApp(
          theme: dark,
          home: Scaffold(
            body: Builder(
              builder: (context) {
                scope = context;
                return ArchiveFilter(selected: false, onChanged: (_) {});
              },
            ),
          ),
        ),
      );
      expect(
        tester
            .widget<PopupMenuButton<bool>>(find.byType(PopupMenuButton<bool>))
            .color,
        KarryamColors.darkSurface,
      );
      await tester.tap(find.byType(PopupMenuButton<bool>));
      await tester.pumpAndSettle();
      expect(find.text('Include archived'), findsOneWidget);
      expect(tester.takeException(), isNull);
      Navigator.of(scope).pop();
      await tester.pumpAndSettle();
      await tester.pumpWidget(
        MaterialApp(
          theme: ThemeData.lerp(light, dark, .5),
          home: Scaffold(
            body: Builder(
              builder: (context) {
                scope = context;
                return const SizedBox();
              },
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      final ink = KarryamColors.navy.forInk(scope);
      expect(ink, isNot(KarryamColors.navy));
      expect(ink, isNot(KarryamColors.darkText));
      expect(Colors.white.forSurface(scope), isNot(Colors.white));
      expect(Colors.white.forSurface(scope), isNot(KarryamColors.darkSurface));
    },
  );
}
