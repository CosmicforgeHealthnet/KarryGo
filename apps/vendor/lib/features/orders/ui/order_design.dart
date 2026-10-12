import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';

Color orderStatusColor(Object? status) => switch (status) {
  'CANCELLED' || 'REJECTED' || 'FAILED' => KarryamColors.error,
  'READY' ||
  'READY_FOR_PICKUP' ||
  'COMPLETED' ||
  'DELIVERED' => KarryamColors.tealInk,
  'PREPARING' ||
  'PICKING' ||
  'PACKING' ||
  'AWAITING_CUSTOMER' ||
  'SUBSTITUTION_REQUIRED' => const Color(0xFF986013),
  _ => const Color(0xFF285EB0),
};
IconData orderActionIcon(String action) => switch (action) {
  'accept' => Icons.receipt_long_rounded,
  'preparing' => Icons.soup_kitchen_outlined,
  'picking' => Icons.shopping_basket_outlined,
  'pack' => Icons.inventory_2_outlined,
  'ready' || 'ready-for-pickup' => Icons.takeout_dining_outlined,
  'pickup' || 'handover' => Icons.handshake_outlined,
  'delivering' => Icons.local_shipping_outlined,
  'delivered' || 'complete' => Icons.task_alt_rounded,
  'reject' => Icons.do_not_disturb_alt_rounded,
  'cancel' => Icons.cancel_outlined,
  _ => Icons.arrow_forward_rounded,
};
String orderActionHint(String action) => switch (action) {
  'accept' => 'Confirm you can fulfil the items in this order.',
  'preparing' => 'Let the order move into preparation.',
  'picking' => 'Begin collecting the ordered products.',
  'pack' => 'Confirm the items are checked and packed.',
  'ready' ||
  'ready-for-pickup' => 'Confirm the order is ready to be collected.',
  'pickup' || 'handover' => 'Confirm the goods have been collected.',
  'delivering' => 'Confirm your delivery has started.',
  'delivered' => 'Confirm the goods have reached their destination.',
  'complete' => 'Finish this delivered order.',
  'cancel' => 'End this vendor order and record your reason.',
  'reject' => 'Decline this vendor order and record your reason.',
  _ => 'Update this order.',
};

class OrderIllustration extends StatelessWidget {
  const OrderIllustration({
    super.key,
    this.shop = false,
    this.color = KarryamColors.tealInk,
    this.size = 112,
  });
  final bool shop;
  final Color color;
  final double size;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: CustomPaint(
      size: Size(size, size * 1.08),
      painter: _OrderPainter(
        shop,
        color.forInk(context),
        Theme.of(context).colorScheme.surfaceContainerHighest,
        color,
      ),
    ),
  );
}

class _OrderPainter extends CustomPainter {
  const _OrderPainter(this.shop, this.color, this.paper, this.badge);
  final Color paper, badge;
  final bool shop;
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / 120, size.height / 130);
    canvas.drawOval(
      const Rect.fromLTWH(10, 106, 105, 17),
      Paint()..color = color.withValues(alpha: .11),
    );
    canvas.save();
    canvas.translate(12, 5);
    canvas.rotate(-.10);
    final receipt = Path()
      ..moveTo(22, 10)
      ..lineTo(85, 10)
      ..quadraticBezierTo(93, 10, 93, 18)
      ..lineTo(93, 105);
    for (var x = 93.0; x >= 22; x -= 10) {
      receipt.lineTo(x - 5, 100);
      receipt.lineTo(x - 10, 105);
    }
    receipt
      ..lineTo(22, 18)
      ..quadraticBezierTo(22, 10, 30, 10)
      ..close();
    canvas.drawShadow(receipt, color.withValues(alpha: .22), 7, true);
    canvas.drawPath(receipt, Paint()..color = paper);
    final ink = Paint()
      ..color = color
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;
    canvas.drawLine(const Offset(35, 25), const Offset(62, 25), ink);
    canvas.drawLine(
      const Offset(35, 34),
      const Offset(79, 34),
      ink..color = color.withValues(alpha: .18),
    );
    for (var y = 78.0; y <= 91; y += 7) {
      canvas.drawLine(Offset(36, y), Offset(y == 78 ? 75 : 62, y), ink);
    }
    ink.color = color;
    if (shop) {
      final bag = Path()
        ..moveTo(41, 47)
        ..lineTo(75, 47)
        ..lineTo(79, 70)
        ..lineTo(37, 70)
        ..close();
      canvas.drawPath(bag, Paint()..color = color.withValues(alpha: .12));
      canvas.drawPath(bag, ink);
      canvas.drawArc(
        const Rect.fromLTWH(48, 39, 20, 20),
        3.14,
        3.14,
        false,
        ink,
      );
    } else {
      canvas.drawArc(
        const Rect.fromLTWH(38, 43, 41, 33),
        3.14,
        3.14,
        false,
        ink,
      );
      canvas.drawLine(const Offset(34, 62), const Offset(83, 62), ink);
      canvas.drawCircle(const Offset(58, 43), 2.5, Paint()..color = color);
      canvas.drawLine(
        const Offset(40, 68),
        const Offset(77, 68),
        ink..color = color.withValues(alpha: .3),
      );
    }
    canvas.restore();
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        const Rect.fromLTWH(74, 88, 35, 27),
        const Radius.circular(9),
      ),
      Paint()..color = badge,
    );
    canvas.drawLine(
      const Offset(83, 96),
      const Offset(99, 96),
      Paint()
        ..color = Colors.white
        ..strokeWidth = 2.5
        ..strokeCap = StrokeCap.round,
    );
    canvas.drawLine(
      const Offset(83, 103),
      const Offset(94, 103),
      Paint()
        ..color = Colors.white.withValues(alpha: .65)
        ..strokeWidth = 2.5
        ..strokeCap = StrokeCap.round,
    );
    canvas.drawCircle(
      const Offset(104, 25),
      3,
      Paint()..color = color.withValues(alpha: .5),
    );
    canvas.drawLine(
      const Offset(5, 53),
      const Offset(13, 53),
      ink
        ..color = color.withValues(alpha: .35)
        ..strokeWidth = 2,
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(_OrderPainter old) =>
      old.shop != shop ||
      old.color != color ||
      old.paper != paper ||
      old.badge != badge;
}

class OrderHero extends StatelessWidget {
  const OrderHero({super.key, required this.order});
  final JsonObject order;
  @override
  Widget build(BuildContext context) {
    final color = orderStatusColor(order['status']);
    final hint = switch (order['status']) {
      'CREATED' ||
      'PAID' ||
      'SENT_TO_VENDOR' => 'Review the items and choose how to proceed.',
      'ACCEPTED' => 'Your order is accepted. Preparation is next.',
      'PREPARING' => 'Keep this order moving through preparation.',
      'PICKING' => 'Check availability as you collect each item.',
      'PACKING' => 'The order is packed. Prepare it for collection.',
      'AWAITING_CUSTOMER' || 'SUBSTITUTION_REQUIRED' =>
        'A replacement decision is needed before proceeding.',
      'READY' || 'READY_FOR_PICKUP' => 'This order is ready for collection.',
      'HANDED_OVER' ||
      'PICKED_UP' ||
      'DELIVERING' => 'The order has moved into fulfilment.',
      'DELIVERED' => 'Delivery is recorded. This order can be completed.',
      'COMPLETED' => 'This order is complete.',
      'CANCELLED' => 'This vendor order has been cancelled.',
      'REJECTED' => 'This vendor order has been declined.',
      _ => 'View the latest order information below.',
    };
    return AnimatedContainer(
      duration: MediaQuery.disableAnimationsOf(context)
          ? Duration.zero
          : const Duration(milliseconds: 300),
      padding: const EdgeInsets.all(22),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(30),
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            (color.withValues(alpha: .13)).forSurface(context),
            (color.withValues(alpha: .035)).forSurface(context),
          ],
        ),
        border: Border.all(
          color: (color.withValues(alpha: .12)).forOutline(context),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      order['domain'] == 'SHOP' ? 'SHOP ORDER' : 'FOOD ORDER',
                      style: TextStyle(
                        color: (color).forInk(context),
                        fontSize: 10,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 1.8,
                      ),
                    ),
                    const SizedBox(height: 10),
                    AnimatedSwitcher(
                      duration: MediaQuery.disableAnimationsOf(context)
                          ? Duration.zero
                          : const Duration(milliseconds: 260),
                      child: Text(
                        dashboardLabel(order['status']),
                        key: ValueKey(order['status']),
                        style: TextStyle(
                          color: KarryamColors.navy.forInk(context),
                          fontSize: 27,
                          height: 1.12,
                          fontWeight: FontWeight.w800,
                          letterSpacing: -.9,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 10),
              OrderIllustration(
                shop: order['domain'] == 'SHOP',
                color: (color).forInk(context),
                size: 94,
              ),
            ],
          ),
          const SizedBox(height: 10),
          Text(hint, style: const TextStyle(fontSize: 13, height: 1.6)),
          const SizedBox(height: 22),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: (Colors.white.withValues(alpha: .8)).forSurface(context),
              borderRadius: BorderRadius.circular(20),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Vendor payable',
                  style: TextStyle(fontSize: 11, fontWeight: FontWeight.w500),
                ),
                const SizedBox(height: 5),
                Text(
                  dashboardMoney(
                    order['vendor_payable_minor'],
                    order['currency'],
                  ),
                  style: const TextStyle(
                    fontSize: 26,
                    fontWeight: FontWeight.w800,
                    letterSpacing: -.8,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class OrderSection extends StatelessWidget {
  const OrderSection({
    super.key,
    required this.title,
    required this.icon,
    required this.child,
    this.color = KarryamColors.tealInk,
  });
  final String title;
  final IconData icon;
  final Color color;
  final Widget child;
  @override
  Widget build(BuildContext context) => Container(
    margin: const EdgeInsets.only(top: 18),
    padding: const EdgeInsets.all(20),
    decoration: BoxDecoration(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(26),
      border: Border.all(color: Color(0xFFE7ECEF).forOutline(context)),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Container(
              padding: const EdgeInsets.all(9),
              decoration: BoxDecoration(
                color: (color.withValues(alpha: .10)).forSurface(context),
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(icon, size: 20, color: (color).forInk(context)),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                title,
                style: const TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 18),
        child,
      ],
    ),
  );
}

Future<T?> orderPage<T>(BuildContext context, Widget page) =>
    Navigator.of(context).push<T>(
      PageRouteBuilder<T>(
        pageBuilder: (_, animation, secondaryAnimation) => page,
        transitionDuration: MediaQuery.disableAnimationsOf(context)
            ? Duration.zero
            : const Duration(milliseconds: 320),
        reverseTransitionDuration: MediaQuery.disableAnimationsOf(context)
            ? Duration.zero
            : const Duration(milliseconds: 240),
        transitionsBuilder: (_, animation, secondaryAnimation, child) {
          final curve = CurvedAnimation(
            parent: animation,
            curve: Curves.easeOutCubic,
            reverseCurve: Curves.easeInCubic,
          );
          return FadeTransition(
            opacity: curve,
            child: SlideTransition(
              position: Tween(
                begin: const Offset(.08, 0),
                end: Offset.zero,
              ).animate(curve),
              child: child,
            ),
          );
        },
      ),
    );
