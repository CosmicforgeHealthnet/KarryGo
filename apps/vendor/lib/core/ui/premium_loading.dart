import 'package:flutter/material.dart';
import '../theme/vendor_theme.dart';

class PremiumSkeleton extends StatefulWidget {
  const PremiumSkeleton({super.key, this.rows = 3, this.detail = false});
  final int rows;
  final bool detail;
  @override
  State<PremiumSkeleton> createState() => _PremiumSkeletonState();
}

class _PremiumSkeletonState extends State<PremiumSkeleton>
    with SingleTickerProviderStateMixin {
  late final motion = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1500),
  );
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      motion.stop();
    } else {
      motion.repeat(reverse: true);
    }
  }

  @override
  void dispose() {
    motion.dispose();
    super.dispose();
  }

  Widget block({
    double width = double.infinity,
    double height = 12,
    double radius = 8,
  }) => Container(
    width: width,
    height: height,
    decoration: BoxDecoration(
      color: Color(0xFFE6ECEE).forSurface(context),
      borderRadius: BorderRadius.circular(radius),
    ),
  );
  Widget card({bool hero = false}) => Container(
    width: double.infinity,
    margin: const EdgeInsets.only(bottom: 12),
    padding: const EdgeInsets.all(22),
    decoration: BoxDecoration(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(24),
      border: Border.all(color: Color(0xFFF0F3F4).forOutline(context)),
    ),
    child: hero
        ? Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              block(width: 94),
              const SizedBox(height: 18),
              block(width: 156, height: 42, radius: 12),
              const SizedBox(height: 18),
              block(width: 190),
            ],
          )
        : Row(
            children: [
              block(width: 48, height: 48, radius: 16),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    FractionallySizedBox(widthFactor: .72, child: block()),
                    const SizedBox(height: 12),
                    FractionallySizedBox(
                      widthFactor: .48,
                      child: block(height: 9),
                    ),
                  ],
                ),
              ),
            ],
          ),
  );
  @override
  Widget build(BuildContext context) => Semantics(
    label: 'Loading content',
    liveRegion: true,
    child: ExcludeSemantics(
      child: AnimatedBuilder(
        animation: motion,
        builder: (context, _) => Opacity(
          opacity: .6 + motion.value * .4,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (widget.detail) card(hero: true),
              for (var i = 0; i < widget.rows; i++) card(),
            ],
          ),
        ),
      ),
    ),
  );
}

class ArchiveFilter extends StatelessWidget {
  const ArchiveFilter({super.key, required this.selected, this.onChanged});
  final bool selected;
  final ValueChanged<bool>? onChanged;
  @override
  Widget build(BuildContext context) => PopupMenuButton<bool>(
    tooltip: selected ? 'Filters: archived included' : 'Filter records',
    enabled: onChanged != null,
    initialValue: selected,
    onSelected: onChanged,
    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
    color: Colors.white.forSurface(context),
    icon: Badge(
      isLabelVisible: selected,
      smallSize: 7,
      backgroundColor: KarryamColors.tealInk.forSurface(context),
      child: Icon(
        Icons.filter_list_rounded,
        color: KarryamColors.tealInk.forInk(context),
        size: 23,
      ),
    ),
    itemBuilder: (_) => [
      CheckedPopupMenuItem(
        value: false,
        checked: !selected,
        child: const Text('Current records'),
      ),
      CheckedPopupMenuItem(
        value: true,
        checked: selected,
        child: const Text('Include archived'),
      ),
    ],
  );
}

class ContentRefresh extends StatelessWidget {
  const ContentRefresh({super.key});
  @override
  Widget build(BuildContext context) => Padding(
    padding: EdgeInsets.symmetric(vertical: 12),
    child: Row(
      children: [
        SizedBox(
          width: 14,
          height: 14,
          child: CircularProgressIndicator(
            strokeWidth: 1.6,
            color: KarryamColors.tealInk.forInk(context),
          ),
        ),
        SizedBox(width: 10),
        Text(
          'Refreshing',
          style: TextStyle(
            fontSize: 12,
            color: Color(0xFF64768A).forInk(context),
          ),
        ),
      ],
    ),
  );
}
