import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';

enum EmptyStory {
  orders,
  filtered,
  activity,
  menu,
  products,
  finance,
  setup,
  restricted,
  unavailable,
}

class DashboardEmptyState extends StatelessWidget {
  const DashboardEmptyState({
    super.key,
    required this.story,
    required this.title,
    required this.message,
    this.action,
    this.onAction,
    this.iconOverride,
    this.detailIconOverride,
  });
  final EmptyStory story;
  final String title, message;
  final String? action;
  final VoidCallback? onAction;
  final IconData? iconOverride, detailIconOverride;

  @override
  Widget build(BuildContext context) {
    final color = switch (story) {
      EmptyStory.restricted => const Color(0xFF9A650D),
      EmptyStory.unavailable => KarryamColors.error,
      EmptyStory.menu => const Color(0xFFAF591C),
      EmptyStory.products => const Color(0xFF346BA6),
      _ => KarryamColors.tealInk,
    };
    final icon =
        iconOverride ??
        switch (story) {
          EmptyStory.orders => Icons.receipt_long_outlined,
          EmptyStory.filtered => Icons.manage_search_rounded,
          EmptyStory.activity => Icons.notifications_none_rounded,
          EmptyStory.menu => Icons.menu_book_rounded,
          EmptyStory.products => Icons.shopping_bag_outlined,
          EmptyStory.finance => Icons.account_balance_wallet_outlined,
          EmptyStory.setup => Icons.storefront_outlined,
          EmptyStory.restricted => Icons.lock_outline_rounded,
          EmptyStory.unavailable => Icons.wifi_off_rounded,
        };
    final detail =
        detailIconOverride ??
        switch (story) {
          EmptyStory.orders || EmptyStory.activity => Icons.schedule_rounded,
          EmptyStory.filtered => Icons.filter_list_rounded,
          EmptyStory.menu => Icons.restaurant_rounded,
          EmptyStory.products => Icons.inventory_2_outlined,
          EmptyStory.finance => Icons.swap_horiz_rounded,
          EmptyStory.setup => Icons.tune_rounded,
          EmptyStory.restricted => Icons.key_rounded,
          EmptyStory.unavailable => Icons.refresh_rounded,
        };
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.fromLTRB(22, 24, 22, 22),
      decoration: BoxDecoration(
        color: Colors.white.forSurface(context),
        borderRadius: BorderRadius.circular(26),
        border: Border.all(
          color: (color.withValues(alpha: .10)).forOutline(context),
        ),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          ExcludeSemantics(
            child: SizedBox(
              width: 144,
              height: 116,
              child: Stack(
                alignment: Alignment.center,
                children: [
                  CustomPaint(
                    size: const Size(144, 116),
                    painter: _StoryBackdrop(color.forInk(context)),
                  ),
                  Transform.rotate(
                    angle: -.09,
                    child: Container(
                      width: 72,
                      height: 78,
                      decoration: BoxDecoration(
                        color: Colors.white.forSurface(context),
                        borderRadius: BorderRadius.circular(18),
                        border: Border.all(
                          color: (color.withValues(
                            alpha: .24,
                          )).forOutline(context),
                          width: 1.5,
                        ),
                        boxShadow: [
                          BoxShadow(
                            color: (color.withValues(
                              alpha: .08,
                            )).forInk(context),
                            blurRadius: 14,
                            offset: const Offset(0, 6),
                          ),
                        ],
                      ),
                      child: Icon(
                        icon,
                        size: 42,
                        color: (color).forInk(context),
                      ),
                    ),
                  ),
                  Positioned(
                    right: 16,
                    bottom: 10,
                    child: Container(
                      width: 36,
                      height: 36,
                      decoration: BoxDecoration(
                        color: (color).forSurface(context),
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(
                          color: Colors.white.forOutline(context),
                          width: 3,
                        ),
                      ),
                      child: Icon(
                        detail,
                        size: 18,
                        color: Colors.white.forInk(context),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 14),
          Text(
            title,
            textAlign: TextAlign.center,
            style: TextStyle(
              color: KarryamColors.navy.forInk(context),
              fontSize: 19,
              fontWeight: FontWeight.w700,
              letterSpacing: -.4,
            ),
          ),
          const SizedBox(height: 8),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 280),
            child: Text(
              message,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: KarryamColors.indigo.forInk(context),
                fontSize: 13,
                height: 1.6,
              ),
            ),
          ),
          if (onAction != null && action != null) ...[
            const SizedBox(height: 16),
            TextButton(
              onPressed: onAction,
              style: TextButton.styleFrom(
                foregroundColor: KarryamColors.tealInk.forInk(context),
                backgroundColor: KarryamColors.mint.forSurface(context),
                padding: const EdgeInsets.symmetric(
                  horizontal: 20,
                  vertical: 12,
                ),
              ),
              child: Text(
                action!,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _StoryBackdrop extends CustomPainter {
  const _StoryBackdrop(this.color);
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    final fill = Paint()..color = color.withValues(alpha: .07);
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(14, 13, 116, 90),
        const Radius.circular(34),
      ),
      fill,
    );
    final line = Paint()
      ..color = color.withValues(alpha: .22)
      ..strokeWidth = 1.5
      ..strokeCap = StrokeCap.round;
    canvas.drawLine(const Offset(10, 80), const Offset(24, 80), line);
    canvas.drawLine(const Offset(117, 28), const Offset(132, 28), line);
    canvas.drawLine(const Offset(124, 21), const Offset(124, 35), line);
    canvas.drawCircle(
      const Offset(26, 24),
      3,
      Paint()..color = color.withValues(alpha: .28),
    );
  }

  @override
  bool shouldRepaint(_StoryBackdrop oldDelegate) => color != oldDelegate.color;
}
