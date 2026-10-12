import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';

class DashboardCatalogueBanner extends StatelessWidget {
  const DashboardCatalogueBanner({
    super.key,
    required this.food,
    required this.onTap,
  });
  final bool food;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final dark = Theme.of(context).brightness == Brightness.dark;
    final surface = dark ? KarryamColors.darkRaised : KarryamColors.navy;
    return Container(
      margin: const EdgeInsets.only(top: 22),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(26),
        border: Border.all(color: dark ? KarryamColors.darkBorder : surface),
      ),
      clipBehavior: Clip.antiAlias,
      child: LayoutBuilder(
        builder: (context, constraints) {
          final compact =
              constraints.maxWidth < 300 ||
              MediaQuery.textScalerOf(context).scale(14) > 19;
          return Stack(
            children: [
              Positioned.fill(child: ColoredBox(color: surface)),
              Positioned.fill(
                left: constraints.maxWidth * .38,
                child: Image.asset(
                  'assets/onboarding/${food ? 'food' : 'shop'}-editorial.png',
                  fit: BoxFit.cover,
                  alignment: Alignment.centerRight,
                  excludeFromSemantics: true,
                ),
              ),
              Positioned.fill(
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      colors: [
                        surface,
                        surface.withValues(alpha: .97),
                        surface.withValues(alpha: compact ? .72 : .30),
                      ],
                      stops: const [0, .46, 1],
                    ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(22),
                child: SizedBox(
                  width: compact
                      ? double.infinity
                      : (constraints.maxWidth - 44) * .72,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        food
                            ? 'Your menu,\nready for orders.'
                            : 'Your catalogue,\nready for orders.',
                        style: const TextStyle(
                          color: Colors.white,
                          fontSize: 23,
                          height: 1.16,
                          letterSpacing: -.7,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 10),
                      const Text(
                        'Keep items and prices up to date.',
                        style: TextStyle(
                          color: Color(0xFFD4DFE5),
                          fontSize: 12,
                          height: 1.5,
                        ),
                      ),
                      const SizedBox(height: 18),
                      FilledButton(
                        style: FilledButton.styleFrom(
                          minimumSize: const Size(0, 44),
                          padding: const EdgeInsets.symmetric(
                            horizontal: 16,
                            vertical: 12,
                          ),
                          textStyle: const TextStyle(
                            fontFamily: 'Inter',
                            fontSize: 12,
                            fontWeight: FontWeight.w700,
                          ),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(14),
                          ),
                        ),
                        onPressed: onTap,
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Flexible(
                              child: Text(
                                food ? 'Manage menus' : 'Manage products',
                              ),
                            ),
                            const SizedBox(width: 8),
                            const Icon(Icons.arrow_forward_rounded, size: 17),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
