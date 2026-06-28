import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

class CustomerHomeBottomNav extends StatelessWidget {
  const CustomerHomeBottomNav({
    super.key,
    required this.selectedIndex,
    required this.onTap,
  });

  final int selectedIndex;
  final ValueChanged<int> onTap;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        border: Border(top: BorderSide(color: CustomerFigmaColors.border, width: 0.5)),
      ),
      child: SafeArea(
        top: false,
        child: SizedBox(
          height: 64,
          child: Row(
            children: [
              _NavItem(
                icon: _NavIcon.svg('assets/figma/House_01.svg'),
                label: 'Home',
                selected: selectedIndex == 0,
                onTap: () => onTap(0),
              ),
              _NavItem(
                icon: _NavIcon.svg('assets/figma/Group 1000004752.svg'),
                label: 'Trips',
                selected: selectedIndex == 1,
                onTap: () => onTap(1),
              ),
              _NavItem(
                icon: _NavIcon.png('assets/figma/notification_bell.png'),
                label: 'Alerts',
                selected: selectedIndex == 2,
                onTap: () => onTap(2),
              ),
              _NavItem(
                icon: _NavIcon.svg('assets/figma/user.svg'),
                label: 'Profile',
                selected: selectedIndex == 3,
                onTap: () => onTap(3),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Icon descriptor ──────────────────────────────────────────────────────────

class _NavIcon {
  const _NavIcon._({required this.path, required this.isSvg});
  factory _NavIcon.svg(String path) => _NavIcon._(path: path, isSvg: true);
  factory _NavIcon.png(String path) => _NavIcon._(path: path, isSvg: false);

  final String path;
  final bool isSvg;

  Widget build(Color color) {
    if (isSvg) {
      return SvgPicture.asset(
        path,
        width: 22,
        height: 22,
        colorFilter: ColorFilter.mode(color, BlendMode.srcIn),
      );
    }
    return Image.asset(
      path,
      width: 22,
      height: 22,
      color: color,
    );
  }
}

// ─── Nav item ─────────────────────────────────────────────────────────────────

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.icon,
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final _NavIcon icon;
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: GestureDetector(
        onTap: onTap,
        behavior: HitTestBehavior.opaque,
        child: Center(
          child: selected
              ? Container(
                  padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
                  decoration: BoxDecoration(
                    color: CustomerFigmaColors.primary,
                    borderRadius: BorderRadius.circular(24),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      icon.build(Colors.white),
                      const SizedBox(width: 6),
                      Text(
                        label,
                        style: const TextStyle(
                          color: Colors.white,
                          fontSize: 12,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ],
                  ),
                )
              : icon.build(CustomerFigmaColors.muted),
        ),
      ),
    );
  }
}
