import 'package:flutter/material.dart';

class OnboardingScreen extends StatefulWidget {
  const OnboardingScreen({super.key, required this.onDone});
  final VoidCallback onDone;

  @override
  State<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends State<OnboardingScreen> {
  final PageController _pageController = PageController();
  int _index = 0;

  static const _steps = [
    _OnboardingStep(
      image: 'assets/figma/onboarding_rider.png',
      title: 'Send or Deliver your packages with ease.',
      body:
          "Whether you're sending a package or delivering one, Cosmicforge Logistics connects you to the fastest, safest routes and helps you earn or get deliveries done on time.",
    ),
    _OnboardingStep(
      image: 'assets/figma/onboarding_car.png',
      title: 'Ride or Drive on your Terms!',
      body:
          'Book a ride instantly or pick up passengers nearby. Cosmicforge Logistics makes every trip simple, efficient, and rewarding for both riders and drivers.',
    ),
    _OnboardingStep(
      image: 'assets/figma/onboarding_truck.png',
      title: 'Move Big, Move Smart!',
      body:
          'Transport goods, materials, or equipment with confidence. Cosmicforge Logistics helps you find the right loads, optimize trips, and earn while delivering safely.',
      isLast: true,
    ),
  ];

  @override
  void dispose() {
    _pageController.dispose();
    super.dispose();
  }

  void _next() {
    if (_index < _steps.length - 1) {
      _pageController.nextPage(
        duration: const Duration(milliseconds: 300),
        curve: Curves.easeInOut,
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final size = MediaQuery.of(context).size;
    final imageHeight = size.height * 0.66;

    return Scaffold(
      backgroundColor: const Color(0xFF062E03),
      body: Container(
        width: double.infinity,
        height: double.infinity,
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.bottomCenter,
            end: Alignment.topCenter,
            stops: [0.0538, 0.9383],
            colors: [
              Color(0xFF062E03), // #062E03 at 5.38%
              Color(0x00212121), // rgba(33, 33, 33, 0) at 93.83%
            ],
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ── The Asymmetrical Visual Wave Mask ────────────────
            SizedBox(
              height: imageHeight,
              width: double.infinity,
              child: PageView.builder(
                controller: _pageController,
                onPageChanged: (val) => setState(() => _index = val),
                itemCount: _steps.length,
                itemBuilder: (context, i) {
                  return ClipPath(
                    clipper: _CosmicforgeLogisticsWaveClipper(),
                    child: Image.asset(
                      _steps[i].image,
                      width: double.infinity,
                      height: imageHeight,
                      fit: BoxFit.cover,
                    ),
                  );
                },
              ),
            ),

            // ── Information & Interaction Controls Area ──────────
            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(32, 12, 32, 40),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    // Uniform Page Indicator Dots (Centered)
                    Row(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: List.generate(_steps.length, (i) {
                        final isActive = i == _index;
                        return Container(
                          margin: const EdgeInsets.symmetric(horizontal: 4),
                          width: 7,
                          height: 7,
                          decoration: BoxDecoration(
                            shape: BoxShape.circle,
                            color: isActive
                                ? const Color(
                                    0xFF27A747,
                                  ) // Active indicator #27A747
                                : const Color(0xFF0D441D),
                          ),
                        );
                      }),
                    ),
                    const SizedBox(height: 32),

                    // Left-Aligned Title Text
                    ConstrainedBox(
                      constraints: BoxConstraints(maxWidth: size.width * 0.85),
                      child: Text(
                        _steps[_index].title,
                        softWrap: true,
                        style: const TextStyle(
                          fontFamily: 'Roboto',
                          fontSize: 24,
                          fontWeight: FontWeight.w700,
                          color: Colors.white,
                          height: 1.15,
                          letterSpacing: 0.0,
                        ),
                      ),
                    ),
                    const SizedBox(height: 16),

                    // Left-Aligned & Justified Body Text
                    Text(
                      _steps[_index].body,
                      textAlign: TextAlign.justify,
                      style: TextStyle(
                        fontFamily: 'Roboto',
                        fontSize: 12,
                        fontWeight: FontWeight.w400,
                        color: Colors.white.withValues(alpha: 0.75),
                        height: 1.25,
                        letterSpacing: 0.0,
                      ),
                    ),

                    const SizedBox(height: 32),

                    // Action Footers: Balanced Layout
                    if (_steps[_index].isLast)
                      // Wide Pill Button for the final step
                      SizedBox(
                        width: double.infinity,
                        height: 54,
                        child: FilledButton(
                          onPressed: widget.onDone,
                          style: FilledButton.styleFrom(
                            backgroundColor: const Color(0xFF27A747), // #27A747
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(27),
                            ),
                          ),
                          child: const Text(
                            'Get Started',
                            style: TextStyle(
                              fontFamily: 'Roboto',
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                            ),
                          ),
                        ),
                      )
                    else
                      // Centered Arrow Button & Far Right Skip Text
                      Stack(
                        alignment: Alignment.center,
                        children: [
                          Align(
                            alignment: Alignment.center,
                            child: SizedBox(
                              width: 64,
                              height: 64,
                              child: FilledButton(
                                onPressed: _next,
                                style: FilledButton.styleFrom(
                                  backgroundColor: const Color(
                                    0xFF27A747,
                                  ), // #27A747
                                  shape: const CircleBorder(),
                                  padding: EdgeInsets.zero,
                                ),
                                child: const Icon(
                                  Icons.arrow_forward,
                                  color: Colors.white,
                                  size: 26,
                                ),
                              ),
                            ),
                          ),
                          Align(
                            alignment: Alignment.centerRight,
                            child: TextButton(
                              onPressed: widget.onDone,
                              style: TextButton.styleFrom(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 8,
                                ),
                              ),
                              child: const Text(
                                'Skip',
                                style: TextStyle(
                                  fontFamily: 'Roboto',
                                  color: Colors.white,
                                  fontSize: 14,
                                  fontWeight: FontWeight.w500,
                                ),
                              ),
                            ),
                          ),
                        ],
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CosmicforgeLogisticsWaveClipper extends CustomClipper<Path> {
  @override
  Path getClip(Size size) {
    final path = Path();
    path.moveTo(0, 0);
    path.lineTo(0, size.height * 0.83);

    path.cubicTo(
      size.width * 0.30,
      size.height * 0.93,
      size.width * 0.70,
      size.height * 0.88,
      size.width,
      size.height * 0.54,
    );

    path.lineTo(size.width, 0);
    path.close();
    return path;
  }

  @override
  bool shouldReclip(_CosmicforgeLogisticsWaveClipper oldClipper) => false;
}

class _OnboardingStep {
  const _OnboardingStep({
    required this.image,
    required this.title,
    required this.body,
    this.isLast = false,
  });

  final String image;
  final String title;
  final String body;
  final bool isLast;
}
