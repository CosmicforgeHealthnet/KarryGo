import 'dart:math' as math;
import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../state/promotion_rules.dart';

Color promotionStatusColor(Object? status) => switch (status) {
  'ACTIVE' => KarryamColors.tealInk,
  'SCHEDULED' => const Color(0xFF346BA6),
  'PAUSED' => const Color(0xFF986013),
  'ENDED' => const Color(0xFFB05B55),
  _ => const Color(0xFF64748B),
};
IconData promotionIcon(Object? type) => switch (type) {
  'FREE_DELIVERY' ||
  'DELIVERY_PERCENTAGE_DISCOUNT' ||
  'DELIVERY_FLAT_DISCOUNT' => Icons.local_shipping_outlined,
  'BUNDLE' => Icons.widgets_outlined,
  'QUANTITY_BREAK' => Icons.stacked_bar_chart_rounded,
  'COUPON' => Icons.confirmation_number_outlined,
  'FIXED_DISCOUNT' => Icons.sell_outlined,
  _ => Icons.percent_rounded,
};

class PromotionBadge extends StatelessWidget {
  const PromotionBadge(this.status, {super.key});
  final Object? status;
  @override
  Widget build(BuildContext context) {
    final color = promotionStatusColor(status).forInk(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 7),
      decoration: BoxDecoration(
        color: color.withValues(alpha: .1),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        dashboardLabel(status),
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

class PromotionSurface extends StatelessWidget {
  const PromotionSurface({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(20),
  });
  final Widget child;
  final EdgeInsets padding;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: double.infinity,
    child: Material(
      color: Theme.of(context).colorScheme.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(26),
        side: BorderSide(color: Theme.of(context).colorScheme.outlineVariant),
      ),
      clipBehavior: Clip.antiAlias,
      child: Padding(padding: padding, child: child),
    ),
  );
}

class PromotionTicket extends StatelessWidget {
  const PromotionTicket({
    super.key,
    required this.row,
    this.onTap,
    this.hero = false,
  });
  final JsonObject row;
  final VoidCallback? onTap;
  final bool hero;
  @override
  Widget build(BuildContext context) {
    final ink = KarryamColors.tealInk.forInk(context);
    return Material(
      color: Theme.of(context).colorScheme.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(26),
        side: BorderSide(color: Theme.of(context).colorScheme.outlineVariant),
      ),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(22),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Container(
                    width: 52,
                    height: 52,
                    decoration: BoxDecoration(
                      color: ink.withValues(alpha: .09),
                      borderRadius: BorderRadius.circular(18),
                    ),
                    child: Icon(
                      promotionIcon(row['promotion_type']),
                      color: ink,
                      size: 28,
                    ),
                  ),
                  const Spacer(),
                  PromotionBadge(row['status']),
                ],
              ),
              const SizedBox(height: 18),
              Text(
                promotionBenefit(row),
                style: TextStyle(
                  fontSize: hero ? 32 : 26,
                  letterSpacing: -1,
                  fontWeight: FontWeight.w800,
                  color: Theme.of(context).colorScheme.onSurface,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                row['name'].toString(),
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 19),
              CustomPaint(
                size: const Size(double.infinity, 1),
                painter: _TicketRule(
                  Theme.of(context).colorScheme.outlineVariant,
                ),
              ),
              const SizedBox(height: 16),
              Row(
                children: [
                  Icon(
                    Icons.event_outlined,
                    size: 17,
                    color: Theme.of(context).colorScheme.onSurfaceVariant,
                  ),
                  const SizedBox(width: 7),
                  Expanded(
                    child: Text(
                      'Ends ${promotionDate(row['ends_at'])}',
                      style: const TextStyle(fontSize: 12),
                    ),
                  ),
                  if (onTap != null)
                    Icon(Icons.arrow_forward_rounded, size: 21, color: ink),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _TicketRule extends CustomPainter {
  const _TicketRule(this.color);
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    for (double x = 0; x < size.width; x += 8) {
      canvas.drawLine(
        Offset(x, 0),
        Offset(math.min(x + 4, size.width), 0),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(_TicketRule oldDelegate) => color != oldDelegate.color;
}

String promotionDate(Object? raw, {bool time = false}) {
  final at = DateTime.tryParse(raw?.toString() ?? '')?.toLocal();
  if (at == null) return 'Unavailable';
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  final date = '${at.day} ${months[at.month - 1]} ${at.year}';
  return time
      ? '$date · ${at.hour.toString().padLeft(2, '0')}:${at.minute.toString().padLeft(2, '0')}'
      : date;
}

class PromotionFact extends StatelessWidget {
  const PromotionFact(this.label, this.value, {super.key});
  final String label, value;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 10),
    child: LayoutBuilder(
      builder: (context, constraints) {
        final children = [
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
          ),
          Text(
            value,
            style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
          ),
        ];
        return constraints.maxWidth < 270
            ? Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [children[0], const SizedBox(height: 6), children[1]],
              )
            : Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(flex: 4, child: children[0]),
                  const SizedBox(width: 12),
                  Expanded(flex: 6, child: children[1]),
                ],
              );
      },
    ),
  );
}

class PromotionBusyLabel extends StatelessWidget {
  const PromotionBusyLabel(this.label, this.busy, {super.key});
  final String label;
  final bool busy;
  @override
  Widget build(BuildContext context) => busy
      ? const SizedBox(
          width: 20,
          height: 20,
          child: CircularProgressIndicator(strokeWidth: 2),
        )
      : Text(label);
}
