import '../core/theme/vendor_appearance.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../core/theme/vendor_theme.dart';
import '../core/widgets/vendor_widgets.dart';
import '../features/auth/ui/vendor_auth_screens.dart';
import '../features/onboarding/ui/vendor_onboarding_screen.dart';
import 'data/mock_vendor_client.dart';
import 'data/mock_onboarding_store.dart';
import '../features/dashboard/ui/vendor_workspace_screen.dart';
import 'data/mock_dashboard_data.dart';
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
            void settings() => showModalBottomSheet<void>(
              context: context,
              isScrollControlled: true,
              useSafeArea: true,
              backgroundColor: (Colors.white).forSurface(context),
              shape: const RoundedRectangleBorder(
                borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
              ),
              builder: (_) => _DataSourceSheet(runtime: _runtime),
            );
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

class _DataSourceSheet extends StatefulWidget {
  const _DataSourceSheet({required this.runtime});
  final VendorRuntime runtime;
  @override
  State<_DataSourceSheet> createState() => _DataSourceSheetState();
}

class _DataSourceSheetState extends State<_DataSourceSheet> {
  late VendorDataMode _mode = widget.runtime.mode;
  late MockAuthScenario _scenario = widget.runtime.scenario;
  late MockOnboardingScenario _onboardingScenario =
      widget.runtime.onboardingScenario;
  late MockDashboardScenario _dashboardScenario =
      widget.runtime.dashboardScenario;
  late final _origin = TextEditingController(text: widget.runtime.liveOrigin);
  String? _error;
  bool _saving = false;
  @override
  void dispose() {
    _origin.dispose();
    super.dispose();
  }

  Future<void> _apply() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      if (_mode == VendorDataMode.mock) {
        widget.runtime.scenario = _scenario;
        widget.runtime.onboardingScenario = _onboardingScenario;
        widget.runtime.dashboardScenario = _dashboardScenario;
      }
      await widget.runtime.changeMode(_mode, origin: _origin.text);
      if (mounted) Navigator.of(context).popUntil((route) => route.isFirst);
    } on ArgumentError {
      if (mounted) {
        setState(
          () => _error =
              'Enter an HTTP(S) server origin without an API path, query or credentials.',
        );
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => _error = 'The data source could not be changed. Try again.',
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) => Padding(
    padding: EdgeInsets.fromLTRB(
      24,
      24,
      24,
      24 + MediaQuery.viewInsetsOf(context).bottom,
    ),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Flexible(
          child: SingleChildScrollView(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  'Data source',
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                    fontWeight: FontWeight.w700,
                  ),
                ),
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: const Icon(Icons.palette_outlined),
                  title: const Text('Appearance'),
                  trailing: const Icon(Icons.chevron_right_rounded),
                  onTap: () => showAppearance(context),
                ),
                const SizedBox(height: 10),
                const Text(
                  'Testing controls. Mock is the default every time the app starts. Changing the source restarts this flow.',
                ),
                const SizedBox(height: 24),
                SegmentedButton<VendorDataMode>(
                  segments: const [
                    ButtonSegment(
                      value: VendorDataMode.mock,
                      label: Text('Mock'),
                      icon: Icon(Icons.science_outlined),
                    ),
                    ButtonSegment(
                      value: VendorDataMode.live,
                      label: Text('Live'),
                      icon: Icon(Icons.cloud_outlined),
                    ),
                  ],
                  selected: {_mode},
                  onSelectionChanged: _saving
                      ? null
                      : (values) => setState(() => _mode = values.single),
                ),
                const SizedBox(height: 24),
                if (_mode == VendorDataMode.mock) ...[
                  DropdownButtonFormField<MockAuthScenario>(
                    initialValue: _scenario,
                    isExpanded: true,
                    decoration: const InputDecoration(
                      labelText: 'Mock scenario',
                    ),
                    items: [
                      for (final scenario in MockAuthScenario.values)
                        DropdownMenuItem(
                          value: scenario,
                          child: Text(scenario.label),
                        ),
                    ],
                    onChanged: _saving
                        ? null
                        : (value) {
                            if (value != null) {
                              setState(() => _scenario = value);
                            }
                          },
                  ),
                  const SizedBox(height: 16),
                  DropdownButtonFormField<MockOnboardingScenario>(
                    initialValue: _onboardingScenario,
                    isExpanded: true,
                    decoration: const InputDecoration(
                      labelText: 'Mock business state',
                    ),
                    items: [
                      for (final s in MockOnboardingScenario.values)
                        DropdownMenuItem(value: s, child: Text(s.label)),
                    ],
                    onChanged: _saving
                        ? null
                        : (v) {
                            if (v != null) {
                              setState(() => _onboardingScenario = v);
                            }
                          },
                  ),
                  const SizedBox(height: 16),
                  DropdownButtonFormField<MockDashboardScenario>(
                    initialValue: _dashboardScenario,
                    isExpanded: true,
                    decoration: const InputDecoration(
                      labelText: 'Mock dashboard state',
                    ),
                    items: [
                      for (final state in MockDashboardScenario.values)
                        DropdownMenuItem(
                          value: state,
                          child: Text(state.label),
                        ),
                    ],
                    onChanged: _saving
                        ? null
                        : (value) {
                            if (value != null) {
                              setState(() => _dashboardScenario = value);
                            }
                          },
                  ),
                  const SizedBox(height: 16),
                  const InlineNotice(
                    message:
                        'Use code ${MockVendorClient.code}.\nMock responses stay on this device and cannot reach the live server.',
                  ),
                ] else ...[
                  TextField(
                    key: const Key('live-origin'),
                    controller: _origin,
                    enabled: !_saving,
                    keyboardType: TextInputType.url,
                    autocorrect: false,
                    decoration: const InputDecoration(
                      labelText: 'Server origin',
                      hintText: 'https://your-vendor-server',
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    'Live mode uses the server and real vendor credentials. Mock and live sessions are kept separate.',
                    style: TextStyle(fontSize: 12),
                  ),
                ],
                if (_error != null) ...[
                  const SizedBox(height: 16),
                  InlineNotice(message: _error!, error: true),
                ],
                const SizedBox(height: 24),
              ],
            ),
          ),
        ),
        const SizedBox(height: 16),
        PrimaryAction(
          label: _saving ? 'Connecting…' : 'Apply data source',
          busy: _saving,
          onPressed: _saving ? null : _apply,
        ),
      ],
    ),
  );
}
