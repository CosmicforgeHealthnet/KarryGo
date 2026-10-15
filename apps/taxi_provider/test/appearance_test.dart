import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taxi_provider/core/theme/appearance_controller.dart';

void main() {
  test('defaults to system and restores the last selection', () async {
    SharedPreferences.setMockInitialValues({});
    final controller = AppearanceController();
    await controller.load();
    expect(controller.mode, ThemeMode.system);
    await Future.wait([
      controller.select(ThemeMode.dark),
      controller.select(ThemeMode.light),
    ]);
    final restored = AppearanceController();
    await restored.load();
    expect(restored.mode, ThemeMode.light);
    controller.dispose();
    restored.dispose();
  });
  testWidgets('appearance controls immediately update the app theme', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    final controller = AppearanceController();
    await tester.pumpWidget(
      AppearanceScope(
        controller: controller,
        child: AnimatedBuilder(
          animation: controller,
          builder: (_, _) => MaterialApp(
            theme: ThemeData.light(),
            darkTheme: ThemeData.dark(),
            themeMode: controller.mode,
            home: const Scaffold(body: AppearanceSetting()),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Dark'));
    await tester.pumpAndSettle();
    expect(
      Theme.of(tester.element(find.byType(AppearanceSetting))).brightness,
      Brightness.dark,
    );
    await tester.tap(find.text('Light'));
    await tester.pumpAndSettle();
    expect(
      Theme.of(tester.element(find.byType(AppearanceSetting))).brightness,
      Brightness.light,
    );
    await tester.tap(find.text('System'));
    await tester.pumpAndSettle();
    expect(controller.mode, ThemeMode.system);
    await tester.pumpWidget(const SizedBox());
    controller.dispose();
  });
}
