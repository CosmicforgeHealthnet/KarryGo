import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';

class OnboardingScreen extends StatefulWidget {
  const OnboardingScreen({super.key, required this.onDone});
  final VoidCallback onDone;
  @override
  State<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends State<OnboardingScreen> {
  final _pages = PageController();
  int _page = 0;
  static const _stories = [
    (
      'Your next move.',
      'Starts here.',
      'Set up your rider profile and discover delivery opportunities on Karryam.',
      JourneySymbol.route,
      'Made for your journey',
    ),
    (
      'Every handover.',
      'Handled with care.',
      'Follow pickup instructions, confirm packages and keep delivery evidence together.',
      JourneySymbol.parcel,
      'Clear steps. Secure handovers.',
    ),
    (
      'Your work.',
      'All in one place.',
      'Follow your deliveries, review wallet activity and reach support when you need it.',
      JourneySymbol.wallet,
      'From pickup to payout',
    ),
  ];
  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  void _next() {
    if (_page == 2) {
      widget.onDone();
      return;
    }
    _pages.nextPage(
      duration: Duration(
        milliseconds: MediaQuery.disableAnimationsOf(context) ? 0 : 380,
      ),
      curve: Curves.easeOutCubic,
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 520),
            child: Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 12, 64, 0),
                  child: Row(
                    children: [
                      if (_page > 0)
                        DispatchBackButton(
                          onPressed: () => _pages.previousPage(
                            duration: const Duration(milliseconds: 300),
                            curve: Curves.easeOutCubic,
                          ),
                        )
                      else
                        Image.asset(
                          'assets/figma/splash_logo.png',
                          width: 36,
                          height: 36,
                          semanticLabel: 'Karryam',
                        ),
                      const SizedBox(width: 12),
                      Text(
                        'KARRYAM RIDER',
                        style: TextStyle(
                          fontSize: 10,
                          letterSpacing: 2,
                          fontWeight: FontWeight.w700,
                          color: c.textMuted,
                        ),
                      ),
                      const Spacer(),
                    ],
                  ),
                ),
                Expanded(
                  child: PageView.builder(
                    controller: _pages,
                    itemCount: 3,
                    onPageChanged: (value) => setState(() => _page = value),
                    itemBuilder: (context, index) {
                      final story = _stories[index];
                      return LayoutBuilder(
                        builder: (context, constraints) =>
                            SingleChildScrollView(
                              padding: const EdgeInsets.fromLTRB(
                                24,
                                16,
                                24,
                                12,
                              ),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  SizedBox(
                                    height: (constraints.maxHeight * .55).clamp(
                                      220.0,
                                      365.0,
                                    ),
                                    child: _JourneyScene(
                                      index: index,
                                      icon: story.$4,
                                      label: story.$5,
                                    ),
                                  ),
                                  const SizedBox(height: 24),
                                  Text(
                                    story.$1,
                                    style: Theme.of(context)
                                        .textTheme
                                        .headlineLarge
                                        ?.copyWith(
                                          fontSize:
                                              MediaQuery.sizeOf(context).width <
                                                  360
                                              ? 32
                                              : 38,
                                        ),
                                  ),
                                  Text(
                                    story.$2,
                                    style: Theme.of(context)
                                        .textTheme
                                        .headlineLarge
                                        ?.copyWith(
                                          fontSize:
                                              MediaQuery.sizeOf(context).width <
                                                  360
                                              ? 32
                                              : 38,
                                          color: c.brandTeal600,
                                        ),
                                  ),
                                  const SizedBox(height: 16),
                                  Text(
                                    story.$3,
                                    style: Theme.of(
                                      context,
                                    ).textTheme.bodyLarge,
                                  ),
                                ],
                              ),
                            ),
                      );
                    },
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 12, 24, 20),
                  child: Column(
                    children: [
                      Row(
                        children: [
                          for (var i = 0; i < 3; i++)
                            AnimatedContainer(
                              duration: const Duration(milliseconds: 260),
                              margin: const EdgeInsets.only(right: 6),
                              height: 4,
                              width: i == _page ? 30 : 10,
                              decoration: BoxDecoration(
                                color: i == _page ? c.brandTeal600 : c.divider,
                                borderRadius: BorderRadius.circular(9),
                              ),
                            ),
                          const Spacer(),
                          TextButton(
                            onPressed: widget.onDone,
                            child: const Text('Skip'),
                          ),
                        ],
                      ),
                      const SizedBox(height: 12),
                      SizedBox(
                        width: double.infinity,
                        child: FilledButton(
                          onPressed: _next,
                          child: Row(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Text(_page == 2 ? 'Get started' : 'Continue'),
                              const SizedBox(width: 12),
                              const Icon(Icons.arrow_forward_rounded, size: 19),
                            ],
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _JourneyScene extends StatelessWidget {
  const _JourneyScene({
    required this.index,
    required this.icon,
    required this.label,
  });
  final int index;
  final JourneySymbol icon;
  final String label;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Stack(
      clipBehavior: Clip.none,
      children: [
        Positioned.fill(
          child: Container(
            margin: const EdgeInsets.fromLTRB(12, 15, 12, 20),
            decoration: BoxDecoration(
              color: c.surfaceElevated,
              borderRadius: const BorderRadius.only(
                topLeft: Radius.circular(90),
                topRight: Radius.circular(32),
                bottomLeft: Radius.circular(32),
                bottomRight: Radius.circular(90),
              ),
            ),
          ),
        ),
        Positioned(
          right: 8,
          top: 5,
          child: Container(
            width: 100,
            height: 100,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(
                color: c.brandTeal500.withValues(alpha: .25),
                width: 18,
              ),
            ),
          ),
        ),
        Positioned.fill(
          child: Padding(
            padding: EdgeInsets.fromLTRB(
              index == 1 ? 26 : 20,
              index == 1 ? 30 : 0,
              index == 1 ? 26 : 0,
              24,
            ),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(index == 1 ? 32 : 0),
              child: Image.asset(
                index == 1
                    ? 'assets/figma/onboarding_rider.png'
                    : 'assets/figma/welcome_hero.png',
                fit: index == 1 ? BoxFit.cover : BoxFit.contain,
                alignment: Alignment.bottomCenter,
              ),
            ),
          ),
        ),
        if (index == 2)
          Positioned(
            right: 8,
            top: 30,
            child: Transform.rotate(
              angle: .1,
              child: Container(
                padding: const EdgeInsets.all(20),
                decoration: BoxDecoration(
                  color: c.brandNavy900,
                  borderRadius: BorderRadius.circular(24),
                ),
                child: const JourneyIcon(
                  JourneySymbol.wallet,
                  color: Color(0xFF00C2AB),
                  size: 54,
                ),
              ),
            ),
          ),
        Positioned(
          left: 0,
          right: 18,
          bottom: 0,
          child: Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: c.surface,
              border: Border.all(color: c.divider),
              borderRadius: BorderRadius.circular(22),
              boxShadow: [
                BoxShadow(
                  color: Colors.black.withValues(alpha: .06),
                  blurRadius: 24,
                  offset: const Offset(0, 8),
                ),
              ],
            ),
            child: Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(10),
                  decoration: BoxDecoration(
                    color: c.teal10,
                    borderRadius: BorderRadius.circular(14),
                  ),
                  child: JourneyIcon(icon, color: c.brandTeal600, size: 28),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    label,
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}
