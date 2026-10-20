import 'package:dispatch_provider/features/home/ui/widgets/route_geometry.dart';
import 'package:dispatch_provider/features/home/ui/sheets/incoming_requests_sheet.dart';
import 'package:dispatch_provider/features/trips/data/trip_model.dart';
import 'package:dispatch_provider/features/home/mock/dispatch_home_mock.dart';
import 'package:dispatch_provider/features/home/ui/widgets/request_card.dart';
import 'package:dispatch_provider/features/home/ui/sheets/offline_sheet.dart';
import 'package:dispatch_provider/features/home/ui/widgets/live_dispatch_map.dart';
import 'dart:async';
import 'package:dispatch_provider/features/home/ui/sheets/go_online_processing.dart';
import 'package:dispatch_provider/app/dispatch_provider_app.dart';
import 'package:dispatch_provider/core/theme/theme_controller.dart';
import 'package:dispatch_provider/features/home/ui/home_screen.dart';
import 'package:dispatch_provider/features/wallet/ui/wallet_screen.dart';
import 'package:dispatch_provider/features/profile/mock/profile_mock.dart';
import 'dart:io';
import 'dart:ui' as ui;
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/storage/token_storage.dart';
import 'package:dispatch_provider/core/theme/dispatch_theme.dart';
import 'package:dispatch_provider/features/onboarding/ui/onboarding_screen.dart';
import 'package:dispatch_provider/features/account_setup/ui/setup_hub_screen.dart';
import 'package:dispatch_provider/features/profile/data/provider_profile_api.dart';
import 'package:dispatch_provider/features/profile/state/provider_profile_controller.dart';
import 'package:dispatch_provider/shared/widgets/mock_badge.dart';

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
  setUp(() async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(
          const MethodChannel('plugins.flutter.io/path_provider'),
          (call) async => Directory.systemTemp.path,
        );
    DispatchHomeMock.reset();
    FlutterSecureStorage.setMockInitialValues({});
    AppModeConfig.instance.resetMemoryForTesting();
    await AppModeConfig.instance.setMode(AppMode.mock);
  });
  Future<void> capture(WidgetTester tester, GlobalKey key, String name) async {
    await tester.runAsync(() async {
      await Future.wait([
        precacheImage(
          const AssetImage('assets/figma/welcome_hero.png'),
          key.currentContext!,
        ),
        precacheImage(
          const AssetImage('assets/figma/splash_logo.png'),
          key.currentContext!,
        ),
      ]);
    });
    await tester.pumpAndSettle();
    final boundary =
        key.currentContext!.findRenderObject()! as RenderRepaintBoundary;
    await tester.runAsync(() async {
      final image = await boundary.toImage(pixelRatio: 1);
      final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
      final file = File('build/design-previews/$name.png');
      await file.parent.create(recursive: true);
      await file.writeAsBytes(bytes!.buffer.asUint8List());
      image.dispose();
    });
  }

  test(
    'Mock preview choice survives reload and demo profile is complete',
    () async {
      await AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.setup,
      );
      await AppModeConfig.instance.load();
      expect(AppModeConfig.instance.mockEntry, MockEntry.setup);
      await AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.dashboard,
      );
      await AppModeConfig.instance.load();
      expect(AppModeConfig.instance.mockEntry, MockEntry.dashboard);
      ProfileMock.reset();
      ProfileMock.seedDashboard();
      expect(ProfileMock.profile.onboardingComplete, isTrue);
      expect(ProfileMock.profile.hasEmergencyContact, isTrue);
      expect(ProfileMock.profile.hasGuarantor, isTrue);
      expect(ProfileMock.profile.fullName, 'Tobi Adeyemi');
      expect(ProfileMock.profile.verificationStatus, 'verified');
      ProfileMock.reset();
      expect(ProfileMock.profile.onboardingComplete, isFalse);
    },
  );

  test(
    'Polyline decoding and Mock trip transitions preserve route coordinates',
    () {
      final points = decodeRoutePolyline('_p~iF~ps|U_ulLnnqC_mqNvxq`@');
      expect(points.length, 3);
      expect(points.first.latitude, 38.5);
      expect(points.last.longitude, closeTo(-126.453, .00001));
      expect(decodeRoutePolyline('_'), isEmpty);
      final request = DispatchHomeMock.pendingRequests.first;
      final trip = DispatchHomeMock.tripFromRequest(request);
      final advanced = DispatchHomeMock.advanceTrip(
        trip,
        TripStatusCode.inProgress,
      );
      expect(advanced.pickupLat, request.pickupLat);
      expect(advanced.dropoffLng, request.dropoffLng);
    },
  );

  testWidgets('Home requests swipe without accepting or declining', (
    tester,
  ) async {
    var accepts = 0, declines = 0;
    final requests = DispatchHomeMock.pendingRequests.take(3).toList();
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.light,
        home: Scaffold(
          body: SingleChildScrollView(
            child: IncomingRequestsSheet(
              requests: requests,
              onAccept: (_) => accepts++,
              onReject: (_) => declines++,
            ),
          ),
        ),
      ),
    );
    await tester.drag(
      find.byKey(const Key('home-request-pager')),
      const Offset(-650, 0),
    );
    await tester.pumpAndSettle();
    expect(find.text('2/3'), findsOneWidget);
    expect(accepts, 0);
    expect(declines, 0);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'Going online waits for the actual result without a fake checklist',
    (tester) async {
      final completion = Completer<ApiResult<String>>();
      ApiResult<String>? received;
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.dark,
          home: Scaffold(
            body: Builder(
              builder: (context) => FilledButton(
                onPressed: () async {
                  received = await showGoOnlineProcessing(
                    context,
                    task: completion.future,
                  );
                },
                child: const Text('Start'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('Start'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text('Connecting you'), findsOneWidget);
      expect(find.text('Checking your profile'), findsNothing);
      expect(received, isNull);
      final failure = ApiFailure<String>(
        const ApiException(
          code: ApiErrorCode.network,
          message: 'Connection lost',
        ),
      );
      completion.complete(failure);
      await tester.pump();
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));
      expect(find.text('Connecting you'), findsNothing);
      expect(received, same(failure));
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('Home accepts a request and opens scrollable trip instructions', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(320, 640);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    DispatchHomeMock.isOnline = false;
    // Storage and persisted preview reset must finish outside fake widget time.
    await tester.runAsync(
      () => AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.dashboard,
      ),
    );
    final theme = ThemeController();
    final previewKey = GlobalKey();
    await tester.pumpWidget(
      RepaintBoundary(
        key: previewKey,
        child: DispatchProviderApp(themeController: theme),
      ),
    );
    Future<void> saveFrame(String name) async {
      final boundary =
          previewKey.currentContext!.findRenderObject()!
              as RenderRepaintBoundary;
      await tester.runAsync(() async {
        final frame = await boundary.toImage();
        final bytes = await frame.toByteData(format: ui.ImageByteFormat.png);
        final file = File('build/design-previews/$name.png');
        await file.parent.create(recursive: true);
        await file.writeAsBytes(bytes!.buffer.asUint8List());
        frame.dispose();
      });
    }

    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 400));
    await tester.tap(find.widgetWithText(FilledButton, 'Go online'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 3200));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text('Delivery opportunities'), findsOneWidget);
    await saveFrame('home-request-small');
    final accept = find.widgetWithText(FilledButton, 'Accept').hitTestable();
    await tester.ensureVisible(accept);
    await tester.tap(accept);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 600));
    expect(find.text('Delivery accepted'), findsOneWidget);
    await tester.tap(find.text('View pickup'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 600));
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text('Current delivery'), findsOneWidget);
    await saveFrame('home-pickup-small');
    final scrollable = find
        .descendant(
          of: find.byKey(const Key('trip-workspace-scroll')),
          matching: find.byType(Scrollable),
        )
        .first;
    await tester.scrollUntilVisible(
      find.text('Cancel Trip'),
      200,
      scrollable: scrollable,
    );
    expect(find.text('Cancel Trip').hitTestable(), findsOneWidget);
    await tester.scrollUntilVisible(
      find.text("I've Arrived"),
      -200,
      scrollable: scrollable,
    );
    await tester.tap(find.text("I've Arrived"));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 700));
    await tester.pump(const Duration(milliseconds: 400));
    await tester.tap(find.text('View pickup code'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    await tester.scrollUntilVisible(
      find.text('Simulate customer scan'),
      180,
      scrollable: scrollable,
    );
    await tester.tap(find.text('Simulate customer scan'));
    await tester.pump();
    expect(find.widgetWithText(FilledButton, 'Start Trip'), findsOneWidget);
    await tester.ensureVisible(find.widgetWithText(FilledButton, 'Start Trip'));
    await tester.tap(find.widgetWithText(FilledButton, 'Start Trip'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 800));
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text('Delivery started'), findsOneWidget);
    await tester.tap(find.text('View delivery').last);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text('On to the delivery'), findsOneWidget);
    await tester.ensureVisible(find.widgetWithText(TextButton, 'View map'));
    await tester.tap(find.widgetWithText(TextButton, 'View map'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    await saveFrame('home-active-trip-small');
    await tester.tap(find.text('View delivery'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    final handoverScroll = find
        .descendant(
          of: find.byKey(const Key('trip-workspace-scroll')),
          matching: find.byType(Scrollable),
        )
        .first;
    await tester.scrollUntilVisible(
      find.widgetWithText(FilledButton, 'Complete delivery'),
      180,
      scrollable: handoverScroll,
    );
    await tester.tap(find.widgetWithText(FilledButton, 'Complete delivery'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.text('Complete your delivery'), findsOneWidget);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('Complete your delivery'), findsOneWidget);
    expect(find.text('Handover'), findsOneWidget);

    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump(const Duration(seconds: 1));
    theme.dispose();
  });

  testWidgets('Mock dashboard starts on Home without mounting Wallet', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    // Storage and persisted preview reset must finish outside fake widget time.
    await tester.runAsync(
      () => AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.dashboard,
      ),
    );
    final theme = ThemeController();
    await tester.pumpWidget(DispatchProviderApp(themeController: theme));
    await tester.pump(const Duration(seconds: 2));
    await tester.pump(const Duration(milliseconds: 400));
    await tester.pump(const Duration(milliseconds: 400));
    expect(find.byType(HomeScreen), findsOneWidget);
    expect(find.byType(WalletScreen), findsNothing);
    final sheetRect = tester.getRect(find.byType(OfflineSheet));
    final mapRect = tester.getRect(find.byType(LiveDispatchMap));
    expect(sheetRect.height, lessThan(240));
    expect(mapRect.height, greaterThan(600));
    expect(sheetRect.top, greaterThan(mapRect.top + mapRect.height * .6));

    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump(const Duration(seconds: 1));
    theme.dispose();
  });

  testWidgets('Data source overlay works outside the screen Scaffold', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.light,
        home: Builder(
          builder: (context) => Stack(
            children: [
              OnboardingScreen(onDone: () {}),
              const Positioned(
                top: 8,
                right: 12,
                width: 48,
                height: 48,
                child: MockBadge(),
              ),
            ],
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(tester.getSize(find.byType(MockBadge)), const Size(48, 48));
    await tester.tap(find.byKey(const Key('data-mode-button')));
    await tester.pumpAndSettle();
    expect(find.text('Data source'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'Introduction progresses through all slides on small phone with large text',
    (tester) async {
      tester.view.physicalSize = const Size(320, 640);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var completed = 0;
      final key = GlobalKey();
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.light,
          builder: (context, child) => MediaQuery(
            data: MediaQuery.of(
              context,
            ).copyWith(textScaler: const TextScaler.linear(1.3)),
            child: child!,
          ),
          home: RepaintBoundary(
            key: key,
            child: OnboardingScreen(onDone: () => completed++),
          ),
        ),
      );
      await tester.pumpAndSettle();
      await capture(tester, key, 'introduction-light-small');
      await tester.tap(find.widgetWithText(FilledButton, 'Continue'));
      await tester.pumpAndSettle();
      expect(find.text('Handled with care.'), findsOneWidget);
      await tester.tap(find.widgetWithText(FilledButton, 'Continue'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Get started'));
      expect(completed, 1);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets(
    'Setup overview shows actual saved steps and remains usable in dark mode',
    (tester) async {
      await AppModeConfig.instance.setMode(AppMode.live);
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final profile = ProviderProfileController(
        api: ProviderProfileApi(
          ApiCoreConfig(baseUrl: 'https://dispatch.test'),
          client: MockClient(
            (_) async => http.Response(
              jsonEncode({
                'success': true,
                'data': {
                  'provider_id': 'rider',
                  'full_name': 'Rider',
                  'state': 'Lagos',
                  'city': 'Ikeja',
                  'has_emergency_contact': false,
                  'has_guarantor': false,
                },
              }),
              200,
            ),
          ),
        ),
        getAccessToken: () => 'token',
      );
      int? step;
      final key = GlobalKey();
      await tester.pumpWidget(
        MaterialApp(
          theme: DispatchTheme.dark,
          home: RepaintBoundary(
            key: key,
            child: SetupHubScreen(
              profileController: profile,
              onContinue: (value) => step = value,
              onBack: () {},
              selectedOperationType: 'individual',
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('2 of 6 setup details saved'), findsOneWidget);
      await capture(tester, key, 'setup-dark');
      await tester.tap(find.text('Continue setup'));
      expect(step, 3);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      profile.dispose();
    },
  );
  testWidgets('Data source waits for Apply before switching', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: DispatchTheme.dark,
        home: const Scaffold(body: MockBadge()),
      ),
    );
    await tester.tap(find.byTooltip('Data source: Mock'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Live'));
    await tester.pumpAndSettle();
    expect(AppModeConfig.instance.current, AppMode.mock);
    await tester.tap(find.text('Apply'));
    await tester.pumpAndSettle();
    expect(AppModeConfig.instance.current, AppMode.live);
    expect(tester.takeException(), isNull);
  });
  test(
    'Introduction memory stays separate from login credentials and data modes',
    () async {
      final storage = TokenStorage();
      expect(await storage.hasSeenIntroduction(), isFalse);
      await storage.markIntroductionSeen();
      await storage.clear();
      expect(await TokenStorage().hasSeenIntroduction(), isTrue);
      await AppModeConfig.instance.setMode(AppMode.live);
      expect(await TokenStorage().hasSeenIntroduction(), isFalse);
    },
  );
}
