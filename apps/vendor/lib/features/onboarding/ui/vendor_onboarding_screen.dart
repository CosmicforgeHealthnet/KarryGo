import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';

class VendorOnboardingScreen extends StatefulWidget {
  const VendorOnboardingScreen({
    super.key,
    required this.onFinish,
    required this.onSettings,
    required this.modeLabel,
  });
  final VoidCallback onFinish, onSettings;
  final String modeLabel;
  @override
  State<VendorOnboardingScreen> createState() => _VendorOnboardingScreenState();
}

class _VendorOnboardingScreenState extends State<VendorOnboardingScreen> {
  final _pages = PageController();
  int _index = 0;
  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  void _next() {
    if (_index == 2) {
      widget.onFinish();
    } else if (MediaQuery.disableAnimationsOf(context)) {
      _pages.jumpToPage(_index + 1);
    } else {
      _pages.nextPage(
        duration: const Duration(milliseconds: 380),
        curve: Curves.easeInOutCubic,
      );
    }
  }

  @override
  Widget build(BuildContext context) => AnnotatedRegion<SystemUiOverlayStyle>(
    value: SystemUiOverlayStyle.dark,
    child: VendorScreenFrame(
      leading: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (_index == 0) ...[
            const KarryamMark(size: 28),
            const SizedBox(width: 9),
          ],
          const Text(
            'FOR VENDORS',
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.8,
            ),
          ),
        ],
      ),
      modeLabel: widget.modeLabel,
      onSettings: widget.onSettings,
      onSkip: widget.onFinish,
      body: PageView(
        controller: _pages,
        onPageChanged: (value) => setState(() => _index = value),
        children: List.generate(3, (index) => _IntroPage(index: index)),
      ),
      footer: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              Semantics(
                label: 'Onboarding step ${_index + 1} of 3',
                child: Row(
                  children: List.generate(
                    3,
                    (n) => AnimatedContainer(
                      duration: MediaQuery.disableAnimationsOf(context)
                          ? Duration.zero
                          : const Duration(milliseconds: 240),
                      margin: const EdgeInsets.only(right: 6),
                      width: n == _index ? 30 : 7,
                      height: 5,
                      decoration: BoxDecoration(
                        color:
                            (n == _index
                                    ? KarryamColors.tealInk
                                    : const Color(0xFFD7DFD9))
                                .forSurface(context),
                        borderRadius: BorderRadius.circular(4),
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 18),
          PrimaryAction(
            label: [
              'Explore Karryam',
              'Continue',
              'Sign in to your business',
            ][_index],
            onPressed: _next,
          ),
          const SizedBox(height: 8),
          Wrap(
            alignment: WrapAlignment.center,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              const Text(
                'Already part of Karryam?',
                style: TextStyle(fontSize: 12),
              ),
              TextButton(
                onPressed: widget.onFinish,
                child: const Text('Sign in'),
              ),
            ],
          ),
        ],
      ),
    ),
  );
}

class _IntroPage extends StatelessWidget {
  const _IntroPage({required this.index});
  final int index;
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) {
      final titles = ['Your business,', 'Every detail,', 'Good things,'];
      final accents = [
        'in full bloom.',
        'beautifully in sync.',
        'on the move.',
      ];
      final descriptions = [
        'Your food or shop business. Connected orders and fulfilment. A world of possibilities.',
        'Bring your food menus or shop catalogue together. Prices and availability, all in one place.',
        'From receiving an order to preparing it and confirming pickup. Follow every available step.',
      ];
      final artHeight = (c.maxHeight - 224).clamp(255.0, 350.0);
      return SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text.rich(
              TextSpan(
                children: [
                  TextSpan(text: '${titles[index]}\n'),
                  TextSpan(
                    text: accents[index],
                    style: TextStyle(
                      color: KarryamColors.tealInk.forInk(context),
                    ),
                  ),
                ],
              ),
              style: Theme.of(context).textTheme.headlineLarge!.copyWith(
                fontSize: index == 1 ? 37 : 42,
                letterSpacing: -2.2,
              ),
            ),
            const SizedBox(height: 6),
            SizedBox(
              height: artHeight,
              width: double.infinity,
              child: _SceneEntrance(
                child: switch (index) {
                  0 => const _BusinessScene(),
                  1 => const _CatalogueScene(),
                  _ => const _FulfilmentScene(),
                },
              ),
            ),
            const SizedBox(height: 10),
            Text(
              descriptions[index],
              style: TextStyle(
                fontSize: 14,
                height: 1.65,
                color: KarryamColors.indigo.forInk(context),
              ),
            ),
          ],
        ),
      );
    },
  );
}

class _SceneEntrance extends StatelessWidget {
  const _SceneEntrance({required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) {
    if (MediaQuery.disableAnimationsOf(context)) return child;
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0, end: 1),
      duration: const Duration(milliseconds: 650),
      curve: Curves.easeOutCubic,
      builder: (context, value, child) => Opacity(
        opacity: value,
        child: Transform.translate(
          offset: Offset(0, 18 * (1 - value)),
          child: child,
        ),
      ),
      child: child,
    );
  }
}

class _BusinessScene extends StatelessWidget {
  const _BusinessScene();
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) => Stack(
      clipBehavior: Clip.none,
      children: [
        Positioned.fill(
          child: CustomPaint(
            painter: _OrbitPainter(
              surface: const Color(0xFFE7EEE5).forSurface(context),
              ink: KarryamColors.tealInk.forInk(context),
            ),
          ),
        ),
        Positioned(
          right: 9,
          top: 24,
          bottom: 18,
          width: c.maxWidth * .69,
          child: Transform.rotate(
            angle: .035,
            child: Container(
              padding: const EdgeInsets.all(5),
              decoration: BoxDecoration(
                color: Colors.white.forSurface(context),
                borderRadius: const BorderRadius.vertical(
                  top: Radius.circular(130),
                  bottom: Radius.circular(30),
                ),
                boxShadow: [
                  BoxShadow(
                    color: (KarryamColors.navy.withValues(
                      alpha: .10,
                    )).forInk(context),
                    blurRadius: 30,
                    offset: const Offset(0, 14),
                  ),
                ],
              ),
              child: ClipRRect(
                borderRadius: const BorderRadius.vertical(
                  top: Radius.circular(126),
                  bottom: Radius.circular(26),
                ),
                child: Image.asset(
                  'assets/onboarding/vendor-welcome.png',
                  fit: BoxFit.cover,
                  alignment: const Alignment(0, -.45),
                  semanticLabel: 'A Nigerian shop owner, ready for business',
                ),
              ),
            ),
          ),
        ),
        Positioned(
          left: 0,
          top: 35,
          child: Transform.rotate(
            angle: -.10,
            child: const _SceneLabel(
              icon: Icons.storefront_outlined,
              title: 'Local business.',
              subtitle: 'Bigger possibilities.',
            ),
          ),
        ),
        Positioned(
          left: 5,
          bottom: 18,
          child: Transform.rotate(
            angle: -.10,
            child: Container(
              width: c.maxWidth * .35,
              height: c.maxWidth * .35,
              padding: const EdgeInsets.all(5),
              decoration: BoxDecoration(
                color: Colors.white.forSurface(context),
                shape: BoxShape.circle,
                boxShadow: [
                  BoxShadow(
                    color: (KarryamColors.navy.withValues(
                      alpha: .1,
                    )).forInk(context),
                    blurRadius: 22,
                    offset: const Offset(0, 8),
                  ),
                ],
              ),
              child: ClipOval(
                child: Image.asset(
                  'assets/onboarding/food-editorial.png',
                  fit: BoxFit.cover,
                  excludeFromSemantics: true,
                ),
              ),
            ),
          ),
        ),
        Positioned(
          right: 0,
          bottom: 1,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
            decoration: BoxDecoration(
              color: KarryamColors.teal.forSurface(context),
              borderRadius: BorderRadius.circular(30),
            ),
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.restaurant_rounded, size: 15),
                SizedBox(width: 6),
                Text(
                  'FOOD',
                  style: TextStyle(
                    fontSize: 9,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1,
                  ),
                ),
                SizedBox(width: 12),
                Icon(Icons.shopping_bag_outlined, size: 15),
                SizedBox(width: 6),
                Text(
                  'SHOP',
                  style: TextStyle(
                    fontSize: 9,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1,
                  ),
                ),
              ],
            ),
          ),
        ),
        const Positioned(right: 1, top: 5, child: _Spark(size: 33)),
      ],
    ),
  );
}

class _CatalogueScene extends StatelessWidget {
  const _CatalogueScene();
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) => Stack(
      children: [
        Positioned.fill(
          child: CustomPaint(
            painter: _OrbitPainter(
              surface: const Color(0xFFE7EEE5).forSurface(context),
              ink: KarryamColors.tealInk.forInk(context),
            ),
          ),
        ),
        Positioned(
          left: 2,
          top: 27,
          width: c.maxWidth * .54,
          child: Transform.rotate(
            angle: -.12,
            child: const _ProductCard(
              asset: 'food-editorial',
              title: 'Food menus',
              caption: 'Made with care',
              icon: Icons.restaurant_rounded,
            ),
          ),
        ),
        Positioned(
          right: 3,
          top: 78,
          width: c.maxWidth * .54,
          child: Transform.rotate(
            angle: .10,
            child: const _ProductCard(
              asset: 'shop-editorial',
              title: 'Shop products',
              caption: 'Everyday favourites',
              icon: Icons.shopping_bag_outlined,
            ),
          ),
        ),
        const Positioned(right: 30, top: 13, child: _Spark(size: 35)),
        Positioned(
          left: 12,
          bottom: 7,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
            decoration: BoxDecoration(
              color: KarryamColors.navy.forSurface(context),
              borderRadius: BorderRadius.circular(24),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.tune_rounded,
                  color: KarryamColors.teal.forInk(context),
                  size: 16,
                ),
                SizedBox(width: 8),
                Text(
                  'Prices & availability',
                  style: TextStyle(
                    fontSize: 11,
                    color: Colors.white.forInk(context),
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    ),
  );
}

class _ProductCard extends StatelessWidget {
  const _ProductCard({
    required this.asset,
    required this.title,
    required this.caption,
    required this.icon,
  });
  final String asset, title, caption;
  final IconData icon;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(7),
    decoration: BoxDecoration(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(25),
      boxShadow: [
        BoxShadow(
          color: (KarryamColors.navy.withValues(alpha: .09)).forInk(context),
          blurRadius: 25,
          offset: const Offset(0, 10),
        ),
      ],
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(19),
          child: AspectRatio(
            aspectRatio: .95,
            child: Image.asset(
              'assets/onboarding/$asset.png',
              fit: BoxFit.cover,
              excludeFromSemantics: true,
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(7, 12, 7, 9),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(
                    icon,
                    size: 15,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                  const SizedBox(width: 5),
                  Expanded(
                    child: Text(
                      title,
                      style: const TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Text(
                caption,
                style: TextStyle(
                  fontSize: 9,
                  color: KarryamColors.indigo.forInk(context),
                ),
              ),
            ],
          ),
        ),
      ],
    ),
  );
}

class _FulfilmentScene extends StatelessWidget {
  const _FulfilmentScene();
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) => Stack(
      children: [
        Positioned.fill(
          child: CustomPaint(
            painter: _OrbitPainter(
              route: true,
              surface: const Color(0xFFE7EEE5).forSurface(context),
              ink: KarryamColors.tealInk.forInk(context),
            ),
          ),
        ),
        Positioned(
          left: 6,
          top: 25,
          child: Transform.rotate(
            angle: -.06,
            child: const _SceneLabel(
              icon: Icons.shopping_bag_outlined,
              title: '01 / Receive',
              subtitle: 'Your next order starts here.',
            ),
          ),
        ),
        Positioned(
          right: 8,
          top: c.maxHeight * .38,
          child: Transform.rotate(
            angle: .055,
            child: const _SceneLabel(
              icon: Icons.inventory_2_outlined,
              title: '02 / Prepare',
              subtitle: 'Made ready with care.',
            ),
          ),
        ),
        Positioned(
          left: 8,
          bottom: 14,
          child: Transform.rotate(
            angle: -.04,
            child: const _SceneLabel(
              icon: Icons.verified_user_outlined,
              title: '03 / Confirm pickup',
              subtitle: 'A secure handover.',
            ),
          ),
        ),
        Positioned(
          right: 15,
          top: 14,
          child: Container(
            width: 76,
            height: 76,
            padding: const EdgeInsets.all(5),
            decoration: BoxDecoration(
              color: Colors.white.forSurface(context),
              shape: BoxShape.circle,
            ),
            child: ClipOval(
              child: Image.asset(
                'assets/onboarding/food-editorial.png',
                fit: BoxFit.cover,
                excludeFromSemantics: true,
              ),
            ),
          ),
        ),
        const Positioned(right: 22, bottom: 19, child: _Spark(size: 40)),
      ],
    ),
  );
}

class _SceneLabel extends StatelessWidget {
  const _SceneLabel({
    required this.icon,
    required this.title,
    required this.subtitle,
  });
  final IconData icon;
  final String title, subtitle;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(13),
    decoration: BoxDecoration(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(19),
      boxShadow: [
        BoxShadow(
          color: (KarryamColors.navy.withValues(alpha: .07)).forInk(context),
          blurRadius: 24,
          offset: const Offset(0, 8),
        ),
      ],
    ),
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 36,
          height: 36,
          decoration: BoxDecoration(
            color: KarryamColors.mint.forSurface(context),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Icon(
            icon,
            size: 19,
            color: KarryamColors.tealInk.forInk(context),
          ),
        ),
        const SizedBox(width: 10),
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              title,
              style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 4),
            Text(
              subtitle,
              style: TextStyle(
                fontSize: 9,
                color: KarryamColors.indigo.forInk(context),
              ),
            ),
          ],
        ),
      ],
    ),
  );
}

class _Spark extends StatelessWidget {
  const _Spark({required this.size});
  final double size;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: Icon(
      Icons.auto_awesome_rounded,
      size: size,
      color: KarryamColors.tealInk.forInk(context),
    ),
  );
}

class _OrbitPainter extends CustomPainter {
  _OrbitPainter({this.route = false, required this.surface, required this.ink});
  final Color surface, ink;
  final bool route;
  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width * .5, size.height * .53);
    canvas.drawOval(
      Rect.fromCenter(
        center: center,
        width: size.width * .93,
        height: size.height * .8,
      ),
      Paint()..color = surface,
    );
    canvas.save();
    canvas.translate(center.dx, center.dy);
    canvas.rotate(-.28);
    canvas.drawOval(
      Rect.fromCenter(
        center: Offset.zero,
        width: size.width * 1.05,
        height: size.height * .63,
      ),
      Paint()
        ..color = KarryamColors.teal.withValues(alpha: .45)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1,
    );
    canvas.restore();
    for (var i = 0; i < 4; i++) {
      final angle = i * math.pi / 2 + .35;
      canvas.drawCircle(
        center +
            Offset(
              math.cos(angle) * size.width * .45,
              math.sin(angle) * size.height * .37,
            ),
        i == 1 ? 4 : 2,
        Paint()..color = ink,
      );
    }
    if (route) {
      final path = Path()
        ..moveTo(40, 75)
        ..cubicTo(
          size.width + 20,
          40,
          size.width - 20,
          size.height * .6,
          size.width * .7,
          size.height * .6,
        )
        ..cubicTo(
          30,
          size.height * .5,
          15,
          size.height * .7,
          65,
          size.height - 35,
        );
      final paint = Paint()
        ..color = ink
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5;
      for (final metric in path.computeMetrics()) {
        for (double offset = 0; offset < metric.length; offset += 10) {
          canvas.drawPath(
            metric.extractPath(offset, math.min(offset + 4, metric.length)),
            paint,
          );
        }
      }
    }
  }

  @override
  bool shouldRepaint(covariant _OrbitPainter oldDelegate) =>
      oldDelegate.route != route ||
      oldDelegate.surface != surface ||
      oldDelegate.ink != ink;
}
