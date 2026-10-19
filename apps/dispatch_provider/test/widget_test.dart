import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:dispatch_provider/app/dispatch_provider_app.dart';
import 'package:dispatch_provider/core/theme/theme_controller.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:dispatch_provider/features/auth/ui/splash_screen.dart';

void main() {
  testWidgets('App splash screen smoke test', (WidgetTester tester) async {
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.setMode(AppMode.mock);
    // Configure virtual screen size to avoid layout overflow issues in tests
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // Build our app and trigger a frame.
    final themeController = ThemeController();
    await themeController.load();
    addTearDown(themeController.dispose);
    await tester.pumpWidget(
      DispatchProviderApp(themeController: themeController),
    );

    expect(find.byType(SplashScreen), findsOneWidget);
    expect(find.text('Karryam'), findsOneWidget);

    // Let the splash timer finish to transition to onboarding
    await tester.pump(const Duration(seconds: 3));
    await tester.pumpAndSettle();
    expect(find.byType(SplashScreen), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
  });
}
