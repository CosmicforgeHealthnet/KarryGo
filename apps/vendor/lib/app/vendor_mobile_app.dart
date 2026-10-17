import '../core/theme/vendor_appearance.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../core/theme/vendor_theme.dart';
import '../core/widgets/vendor_widgets.dart';
import '../features/auth/ui/vendor_auth_screens.dart';
import '../features/onboarding/ui/vendor_onboarding_screen.dart';
import '../features/dashboard/ui/vendor_workspace_screen.dart';
import 'vendor_runtime.dart';

class VendorMobileApp extends StatefulWidget {
  const VendorMobileApp({super.key, this.runtime, this.appearance});
  final VendorAppearance? appearance;
  final VendorRuntime? runtime;
  @override
  State<VendorMobileApp> createState() => _VendorMobileAppState();
}

class _VendorMobileAppState extends State<VendorMobileApp> {
  late final VendorAppearance _appearance =
      widget.appearance ?? VendorAppearance();
  late final VendorRuntime _runtime = widget.runtime ?? VendorRuntime();
  @override
  void initState() {
    super.initState();
    unawaited(_runtime.initialize());
    if (widget.appearance == null) unawaited(_appearance.load());
  }

  @override
  void dispose() {
    if (widget.runtime == null) _runtime.dispose();
    if (widget.appearance == null) _appearance.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AppearanceScope(
    controller: _appearance,
    child: AnimatedBuilder(
      animation: _appearance,
      builder: (context, _) => MaterialApp(
        title: 'Karryam',
        debugShowCheckedModeBanner: false,
        theme: vendorTheme(),
        darkTheme: vendorTheme(brightness: Brightness.dark),
        themeMode: _appearance.mode,
        builder: (context, child) => AnnotatedRegion<SystemUiOverlayStyle>(
          value:
              (Theme.of(context).brightness == Brightness.dark
                      ? SystemUiOverlayStyle.light
                      : SystemUiOverlayStyle.dark)
                  .copyWith(
                    statusBarColor: Colors.transparent,
                    systemNavigationBarColor: Theme.of(
                      context,
                    ).scaffoldBackgroundColor,
                    systemNavigationBarDividerColor: Colors.transparent,
                    systemNavigationBarIconBrightness:
                        Theme.of(context).brightness == Brightness.dark
                        ? Brightness.light
                        : Brightness.dark,
                  ),
          child: child ?? const SizedBox.shrink(),
        ),
        themeAnimationDuration: const Duration(milliseconds: 240),
        home: AnimatedBuilder(
          animation: _runtime,
          builder: (context, _) {
            void settings() => showAppearance(context);
            final screen = switch (_runtime.page) {
              VendorPage.splash => const _SplashScreen(),
              VendorPage.onboarding => VendorOnboardingScreen(
                onFinish: _runtime.finishIntro,
                onSignIn: () => _runtime.finishIntro(registering: false),
                onSettings: settings,
                modeLabel: _runtime.mode.name,
              ),
              VendorPage.signIn => VendorSignInScreen(
                runtime: _runtime,
                onSettings: settings,
              ),
              VendorPage.code => VendorCodeScreen(
                runtime: _runtime,
                onSettings: settings,
              ),
              VendorPage.vendorChoice => VendorChoiceScreen(
                runtime: _runtime,
                onSettings: settings,
              ),
              VendorPage.signedIn => VendorWorkspaceScreen(
                runtime: _runtime,
                onSettings: settings,
              ),
              VendorPage.connectionError => VendorConnectionScreen(
                runtime: _runtime,
                onSettings: settings,
              ),
              VendorPage.accessUnavailable => VendorSessionScreen(
                runtime: _runtime,
                onSettings: settings,
                denied: true,
              ),
            };
            return AnnotatedRegion<SystemUiOverlayStyle>(
              value: _runtime.page == VendorPage.splash
                  ? SystemUiOverlayStyle.light.copyWith(
                      statusBarColor: (Colors.transparent).forInk(context),
                    )
                  : (Theme.of(context).brightness == Brightness.dark
                            ? SystemUiOverlayStyle.light
                            : SystemUiOverlayStyle.dark)
                        .copyWith(
                          statusBarColor: (Colors.transparent).forInk(context),
                        ),
              child: KeyedSubtree(
                key: ValueKey((_runtime.revision, _runtime.page)),
                child: screen,
              ),
            );
          },
        ),
      ),
    ),
  );
}

class _SplashScreen extends StatelessWidget {
  const _SplashScreen();
  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: (KarryamColors.navy).forSurface(context),
    body: DecoratedBox(
      decoration: BoxDecoration(
        gradient: RadialGradient(
          center: Alignment(.9, .6),
          radius: .9,
          colors: [
            (Color(0xFF0A424F)).forSurface(context),
            (KarryamColors.navy).forSurface(context),
          ],
        ),
      ),
      child: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const KarryamLogo(width: 252),
                    const SizedBox(height: 25),
                    Text(
                      'FOR YOUR BUSINESS',
                      style: TextStyle(
                        fontSize: 12,
                        letterSpacing: 3,
                        color: (KarryamColors.border).forInk(context),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: (KarryamColors.teal).forInk(context),
                semanticsLabel: 'Starting Karryam',
              ),
            ),
            const SizedBox(height: 28),
            Text(
              'MOVEMENT INTELLIGENCE\nFOR A BRIGHTER TOMORROW',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 10,
                height: 1.7,
                letterSpacing: 1.4,
                color: (KarryamColors.border).forInk(context),
              ),
            ),
            const SizedBox(height: 18),
            Text(
              'A CosmicForge Brand',
              style: TextStyle(
                fontSize: 10,
                color: (KarryamColors.gray).forInk(context),
              ),
            ),
            const SizedBox(height: 36),
          ],
        ),
      ),
    ),
  );
}
