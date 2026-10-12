import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';

class DashboardQueue extends StatelessWidget {
  const DashboardQueue({
    super.key,
    required this.counts,
    required this.onSelect,
  });
  final List<int> counts;
  final void Function(int) onSelect;
  @override
  Widget build(BuildContext context) => Container(
    decoration: BoxDecoration(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(24),
      border: Border.all(color: Color(0xFFE4ECE7).forOutline(context)),
    ),
    child: IntrinsicHeight(
      child: Row(
        children: [
          for (var i = 0; i < 3; i++) ...[
            if (i > 0)
              Padding(
                padding: EdgeInsets.symmetric(vertical: 24),
                child: VerticalDivider(
                  width: 1,
                  color: Color(0xFFE4ECE7).forOutline(context),
                ),
              ),
            Expanded(
              child: _QueueCell(
                index: i,
                count: counts[i],
                onTap: () => onSelect(i),
              ),
            ),
          ],
        ],
      ),
    ),
  );
}

class _QueueCell extends StatelessWidget {
  const _QueueCell({
    required this.index,
    required this.count,
    required this.onTap,
  });
  final int index, count;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final color = [
      const Color(0xFF3475D5),
      const Color(0xFFB36A08),
      KarryamColors.tealInk,
    ][index];
    return Material(
      color: Colors.transparent.forSurface(context),
      borderRadius: BorderRadius.circular(24),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(24),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              Container(
                width: 32,
                height: 32,
                decoration: BoxDecoration(
                  color: (color.withValues(alpha: .10)).forSurface(context),
                  shape: BoxShape.circle,
                ),
                child: Icon(
                  [
                    Icons.call_received_rounded,
                    Icons.timelapse_rounded,
                    Icons.task_alt_rounded,
                  ][index],
                  color: (color).forInk(context),
                  size: 18,
                ),
              ),
              const SizedBox(height: 13),
              Text(
                '$count',
                style: const TextStyle(
                  fontSize: 27,
                  fontWeight: FontWeight.w700,
                  letterSpacing: -1,
                ),
              ),
              const SizedBox(height: 5),
              Text(
                ['Incoming', 'In progress', 'Ready'][index],
                style: TextStyle(
                  fontSize: 11,
                  color: KarryamColors.indigo.forInk(context),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class DashboardAction extends StatelessWidget {
  const DashboardAction({
    super.key,
    required this.title,
    required this.subtitle,
    required this.icon,
    required this.onTap,
    this.iconColor = KarryamColors.tealInk,
  });
  final String title, subtitle;
  final IconData icon;
  final Color iconColor;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => Material(
    color: Colors.white.forSurface(context),
    borderRadius: BorderRadius.circular(20),
    child: InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(20),
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Row(
          children: [
            Container(
              width: 46,
              height: 46,
              decoration: BoxDecoration(
                color: iconColor.withValues(alpha: .10).forSurface(context),
                borderRadius: BorderRadius.circular(15),
              ),
              child: Icon(icon, color: iconColor.forInk(context), size: 23),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 15,
                    ),
                  ),
                  const SizedBox(height: 5),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 12,
                      color: KarryamColors.indigo.forInk(context),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Icon(
              Icons.arrow_forward_rounded,
              color: KarryamColors.tealInk.forInk(context),
              size: 18,
            ),
          ],
        ),
      ),
    ),
  );
}

class DashboardBalance extends StatelessWidget {
  const DashboardBalance({
    super.key,
    required this.available,
    required this.pending,
    this.visible = true,
    this.onToggleVisibility,
    this.onTap,
  });
  final String available, pending;
  final bool visible;
  final VoidCallback? onToggleVisibility;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => Container(
    clipBehavior: Clip.antiAlias,
    decoration: BoxDecoration(
      borderRadius: BorderRadius.circular(28),
      gradient: LinearGradient(
        begin: Alignment.topLeft,
        end: Alignment.bottomRight,
        colors: [
          Color(0xFFDEFAF2).forSurface(context),
          Color(0xFFC7F2E8).forSurface(context),
        ],
      ),
    ),
    child: Stack(
      children: [
        Positioned(
          right: -48,
          top: -48,
          child: Transform.rotate(
            angle: -.35,
            child: Container(
              width: 155,
              height: 210,
              decoration: BoxDecoration(
                color: (KarryamColors.teal.withValues(
                  alpha: .11,
                )).forSurface(context),
                borderRadius: BorderRadius.circular(70),
              ),
            ),
          ),
        ),
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(22, 22, 18, 24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          'Available balance',
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w700,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                        ),
                      ),
                      if (onToggleVisibility != null)
                        IconButton(
                          onPressed: onToggleVisibility,
                          tooltip: visible ? 'Hide balances' : 'Show balances',
                          color: KarryamColors.tealInk.forInk(context),
                          icon: Icon(
                            visible
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                            size: 21,
                          ),
                        ),
                      if (onTap != null)
                        IconButton(
                          onPressed: onTap,
                          tooltip: 'View finance',
                          style: IconButton.styleFrom(
                            backgroundColor: (Colors.white.withValues(
                              alpha: .7,
                            )).forSurface(context),
                            foregroundColor: KarryamColors.tealInk.forInk(
                              context,
                            ),
                          ),
                          icon: const Icon(Icons.north_east_rounded, size: 19),
                        )
                      else
                        Icon(
                          Icons.account_balance_wallet_outlined,
                          color: KarryamColors.tealInk.forInk(context),
                        ),
                    ],
                  ),
                  const SizedBox(height: 16),
                  FittedBox(
                    fit: BoxFit.scaleDown,
                    alignment: Alignment.centerLeft,
                    child: Text(
                      visible
                          ? available
                          : '\u2022\u2022\u2022\u2022\u2022\u2022',
                      style: TextStyle(
                        fontSize: 34,
                        fontWeight: FontWeight.w700,
                        letterSpacing: -1.6,
                        color: KarryamColors.navy.forInk(context),
                      ),
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Business wallet',
                    style: TextStyle(
                      color: KarryamColors.tealInk.forInk(context),
                      fontSize: 12,
                    ),
                  ),
                ],
              ),
            ),
            Container(
              color: (Colors.white.withValues(alpha: .58)).forSurface(context),
              padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 17),
              child: Row(
                children: [
                  Icon(
                    Icons.schedule_rounded,
                    color: KarryamColors.tealInk.forInk(context),
                    size: 17,
                  ),
                  const SizedBox(width: 8),
                  Text(
                    'Pending',
                    style: TextStyle(
                      fontSize: 12,
                      color: KarryamColors.indigo.forInk(context),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      visible
                          ? pending
                          : '\u2022\u2022\u2022\u2022\u2022\u2022',
                      textAlign: TextAlign.right,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 13,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ],
    ),
  );
}
