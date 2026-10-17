import 'dart:io';
import 'dart:ui' as ui;
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:dispatch_provider/app/dispatch_provider_app.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/theme/theme_controller.dart';
import 'package:dispatch_provider/features/home/mock/dispatch_home_mock.dart';
import 'package:dispatch_provider/features/profile/ui/profile_screen.dart';
import 'package:dispatch_provider/features/profile/ui/profile_info_screen.dart';
import 'package:dispatch_provider/features/profile/ui/guarantor_information_screen.dart';
import 'package:dispatch_provider/features/wallet/ui/wallet_screen.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUpAll(() async {
    final loader = FontLoader('Inter')
      ..addFont(rootBundle.load('assets/fonts/inter-regular.ttf'))
      ..addFont(rootBundle.load('assets/fonts/inter-bold.ttf'));
    await loader.load();
    final icons = FontLoader('MaterialIcons')
      ..addFont(rootBundle.load('fonts/MaterialIcons-Regular.otf'));
    await icons.load();
  });
  testWidgets(
    'Wallet and Account remain usable on a small phone in both themes',
    (tester) async {
      tester.view.physicalSize = const Size(320, 720);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(
            const MethodChannel('plugins.flutter.io/path_provider'),
            (call) async => Directory.systemTemp.path,
          );
      FlutterSecureStorage.setMockInitialValues({});
      DispatchHomeMock.reset();
      await AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.dashboard,
      );
      final theme = ThemeController();
      final boundaryKey = GlobalKey();
      await tester.pumpWidget(
        RepaintBoundary(
          key: boundaryKey,
          child: DispatchProviderApp(themeController: theme),
        ),
      );
      await tester.pump(const Duration(seconds: 2));
      await tester.pump(const Duration(milliseconds: 500));
      await tester.tap(find.text('Wallet').last);
      await tester.pump();
      await tester.pump(const Duration(seconds: 1));
      expect(find.byType(WalletScreen), findsOneWidget);
      expect(
        tester
            .widget<AnnotatedRegion<SystemUiOverlayStyle>>(
              find.byKey(const Key('dispatch-system-ui')),
            )
            .value
            .statusBarIconBrightness,
        Brightness.dark,
      );
      expect(find.text('Available balance'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.tap(find.byTooltip('Hide balance'));
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.byTooltip('Show balance'), findsOneWidget);
      await tester.runAsync(() async {
        final boundary =
            boundaryKey.currentContext!.findRenderObject()
                as RenderRepaintBoundary;
        final image = await boundary.toImage(pixelRatio: 2);
        final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
        final target = File('build/design-previews/wallet-small.png');
        await target.parent.create(recursive: true);
        await target.writeAsBytes(bytes!.buffer.asUint8List());
        image.dispose();
      });
      await tester.tap(find.text('Account').last);
      await tester.pump();
      await tester.pump(const Duration(seconds: 1));
      expect(find.byType(ProfileScreen), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.runAsync(() async {
        final boundary =
            boundaryKey.currentContext!.findRenderObject()
                as RenderRepaintBoundary;
        final image = await boundary.toImage(pixelRatio: 2);
        final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
        await File(
          'build/design-previews/account-small.png',
        ).writeAsBytes(bytes!.buffer.asUint8List());
        image.dispose();
      });
      await tester.ensureVisible(find.text('Profile details'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('Profile details'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.byType(ProfileInfoScreen), findsOneWidget);
      expect(find.text('no-email@karryam.com'), findsNothing);
      await tester.tap(find.byTooltip('Go back'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      await theme.setMode(ThemeMode.dark);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      final darkBars = tester
          .widget<AnnotatedRegion<SystemUiOverlayStyle>>(
            find.byKey(const Key('dispatch-system-ui')),
          )
          .value;
      expect(darkBars.statusBarIconBrightness, Brightness.light);
      expect(darkBars.statusBarBrightness, Brightness.dark);
      expect(darkBars.systemNavigationBarIconBrightness, Brightness.light);
      await tester.ensureVisible(find.text('Guarantor'));
      await tester.pump(const Duration(milliseconds: 400));
      await tester.tap(find.text('Guarantor'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      expect(find.byType(GuarantorInformationScreen), findsOneWidget);
      expect(find.text('Relationship'), findsNothing);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump(const Duration(seconds: 1));
      theme.dispose();
    },
  );
}
