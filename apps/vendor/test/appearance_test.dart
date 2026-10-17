import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/core/theme/vendor_appearance.dart';
import 'package:vendor/features/settings/ui/settings_design.dart';
import 'package:vendor/features/inventory/ui/stock_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('plugins.it_nomads.com/flutter_secure_storage');
  final values = <String, String>{};
  var failWrite = false;
  setUp(() {
    values.clear();
    failWrite = false;
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          final args = call.arguments as Map;
          if (call.method == 'read') return values[args['key']];
          if (call.method == 'write') {
            if (failWrite) throw PlatformException(code: 'WRITE_FAILED');
            values[args['key']] = args['value'];
          }
          return null;
        });
  });
  tearDown(
    () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null),
  );
  test(
    'appearance persists and failed writes preserve the prior preference',
    () async {
      final c = VendorAppearance();
      addTearDown(c.dispose);
      await c.load();
      // The app starts in Light until the vendor picks otherwise.
      expect(c.mode, ThemeMode.light);
      await c.select(ThemeMode.dark);
      expect(values['vendor.appearance'], 'dark');
      final restored = VendorAppearance();
      addTearDown(restored.dispose);
      await restored.load();
      expect(restored.mode, ThemeMode.dark);
      failWrite = true;
      await c.select(ThemeMode.light);
      expect(c.mode, ThemeMode.dark);
      expect(c.error, isNotNull);
    },
  );
  testWidgets('System follows the phone while an explicit theme overrides it', (
    tester,
  ) async {
    final c = VendorAppearance();
    addTearDown(c.dispose);
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
    addTearDown(tester.platformDispatcher.clearPlatformBrightnessTestValue);
    await tester.pumpWidget(
      AppearanceScope(
        controller: c,
        child: AnimatedBuilder(
          animation: c,
          builder: (context, _) => MaterialApp(
            theme: vendorTheme(),
            darkTheme: vendorTheme(brightness: Brightness.dark),
            themeMode: c.mode,
            home: const Scaffold(body: AppearancePicker()),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    // Light is the default, so a dark phone does not darken the app yet.
    expect(
      Theme.of(tester.element(find.byType(AppearancePicker))).brightness,
      Brightness.light,
    );
    await tester.tap(find.text('Dark'));
    await tester.pumpAndSettle();
    expect(c.mode, ThemeMode.dark);
    await tester.tap(find.text('Light'));
    await tester.pumpAndSettle();
    expect(c.mode, ThemeMode.light);
    expect(
      Theme.of(tester.element(find.byType(AppearancePicker))).brightness,
      Brightness.light,
    );
    await tester.tap(find.text('System'));
    await tester.pumpAndSettle();
    expect(
      Theme.of(tester.element(find.byType(AppearancePicker))).brightness,
      Brightness.dark,
    );
    expect(tester.takeException(), isNull);
  });
  testWidgets(
    'open appearance sheet switches theme without dismissing the route',
    (tester) async {
      final c = VendorAppearance();
      addTearDown(c.dispose);
      await tester.pumpWidget(
        AppearanceScope(
          controller: c,
          child: MaterialApp(
            theme: vendorTheme(),
            home: Scaffold(
              body: Builder(
                builder: (context) => TextButton(
                  onPressed: () => showAppearance(context),
                  child: const Text('Open appearance'),
                ),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('Open appearance'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Dark'));
      await tester.pumpAndSettle();
      expect(
        Theme.of(tester.element(find.byType(AppearancePicker))).brightness,
        Brightness.dark,
      );
      expect(find.text('Appearance'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'dark stock details use layered surfaces and readable semantic colors',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      late BuildContext scope;
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: Scaffold(
            body: Builder(
              builder: (context) {
                scope = context;
                return const SingleChildScrollView(
                  padding: EdgeInsets.all(22),
                  child: Column(
                    children: [
                      SettingsCard(
                        child: SettingsFact('Stock item', 'Demo product'),
                      ),
                      StockFigures(
                        record: {
                          'on_hand_quantity': 24,
                          'reserved_quantity': 3,
                          'quarantined_quantity': 2,
                          'in_transit_quantity': 0,
                          'backorder_quantity': 0,
                        },
                      ),
                    ],
                  ),
                );
              },
            ),
          ),
        ),
      );
      final card = tester.widget<Material>(
        find
            .descendant(
              of: find.byType(SettingsCard),
              matching: find.byType(Material),
            )
            .first,
      );
      expect(card.color, KarryamColors.darkSurface);
      double contrast(Color foreground, Color background) {
        final a = foreground.computeLuminance(),
            b = background.computeLuminance();
        return (a > b ? a + .05 : b + .05) / (a > b ? b + .05 : a + .05);
      }

      for (final color in [
        KarryamColors.navy,
        KarryamColors.indigo,
        KarryamColors.tealInk,
        KarryamColors.error,
        const Color(0xFF986013),
      ]) {
        expect(
          contrast(color.forInk(scope), KarryamColors.darkSurface),
          greaterThan(4.5),
        );
      }
      expect(tester.takeException(), isNull);
    },
  );
}
