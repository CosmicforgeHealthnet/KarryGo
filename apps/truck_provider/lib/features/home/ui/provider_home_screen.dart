import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../../auth/state/provider_auth_controller.dart';
import '../../earnings/state/provider_earnings_controller.dart';
import '../../earnings/ui/provider_earnings_screen.dart';
import '../../profile/state/provider_profile_controller.dart';
import '../../profile/ui/provider_profile_screen.dart';
import '../../disputes/state/provider_dispute_controller.dart';
import '../../disputes/ui/provider_log_disputes_screen.dart';
import '../../trips/state/provider_trips_controller.dart';
import '../../trips/ui/provider_trips_screen.dart';
import '../../wallet/state/provider_withdrawal_controller.dart';
import '../../wallet/ui/provider_withdrawal_form_screen.dart';
import '../state/provider_home_controller.dart';
import 'screens/provider_active_trip_screen.dart';
import 'screens/provider_dashboard_screen.dart';
import 'screens/provider_notifications_screen.dart';
import 'screens/provider_requests_screen.dart';
import 'widgets/provider_app_colors.dart';

/// Entry point for the provider home experience.
/// 5-tab bottom nav shell; active-trip takes over the full screen.
class ProviderHomeScreen extends StatefulWidget {
  const ProviderHomeScreen({
    super.key,
    required this.authController,
    required this.homeController,
    required this.profileController,
    required this.earningsController,
    required this.withdrawalController,
    required this.disputeController,
    required this.tripsController,
  });

  final ProviderAuthController authController;
  final ProviderHomeController homeController;
  final ProviderProfileController profileController;
  final ProviderEarningsController earningsController;
  final ProviderWithdrawalController withdrawalController;
  final ProviderDisputeController disputeController;
  final ProviderTripsController tripsController;

  @override
  State<ProviderHomeScreen> createState() => _ProviderHomeScreenState();
}

class _ProviderHomeScreenState extends State<ProviderHomeScreen> {
  int _tabIndex = 0;

  void _onTabTap(int index) {
    // Refresh the relevant tab's data when it's opened.
    if (index == 2) {
      widget.tripsController.load();
    }
    if (index == 3) {
      widget.earningsController.load();
    }
    setState(() => _tabIndex = index);
  }

  void _openNotifications() {
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => const ProviderNotificationsScreen()),
    );
  }

  void _openWithdrawal() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderWithdrawalFormScreen(controller: widget.withdrawalController),
      ),
    );
  }

  void _openDisputes() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderLogDisputesScreen(
          disputeController: widget.disputeController,
          earningsController: widget.earningsController,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.homeController,
      builder: (context, _) {
        final state = widget.homeController.state;

        // Active trip takes over the full screen
        if (state.status == ProviderHomeStatus.activeTrip && state.activeBooking != null) {
          return ProviderActiveTripScreen(
            controller: widget.homeController,
            booking: state.activeBooking!,
          );
        }

        final pendingCount = state.pendingRequests.length;

        return Scaffold(
          backgroundColor: Colors.white,
          body: IndexedStack(
            index: _tabIndex,
            children: [
              ProviderDashboardScreen(
                authController: widget.authController,
                homeController: widget.homeController,
                state: state,
                onNotificationsTap: _openNotifications,
              ),
              ProviderRequestsScreen(
                homeController: widget.homeController,
                state: state,
              ),
              ProviderTripsScreen(controller: widget.tripsController),
              ProviderEarningsScreen(
                controller: widget.earningsController,
                onWithdraw: _openWithdrawal,
                onDispute: _openDisputes,
              ),
              ProviderProfileScreen(
                authController: widget.authController,
                profileController: widget.profileController,
              ),
            ],
          ),
          bottomNavigationBar: _ProviderBottomNav(
            selectedIndex: _tabIndex,
            pendingCount: pendingCount,
            onTap: _onTabTap,
          ),
        );
      },
    );
  }
}

// ─── Bottom navigation bar ────────────────────────────────────────────────────

class _ProviderBottomNav extends StatelessWidget {
  const _ProviderBottomNav({
    required this.selectedIndex,
    required this.pendingCount,
    required this.onTap,
  });

  final int selectedIndex;
  final int pendingCount;
  final ValueChanged<int> onTap;

  static final _items = <(_NavIcon, String)>[
    (_NavIcon.svg('assets/figma/House_01.svg'), 'Home'),
    (_NavIcon.svg('assets/figma/Group 1000004752.svg'), 'Requests'),
    (_NavIcon.icon(Icons.calendar_month_rounded), 'Trips'),
    (_NavIcon.icon(Icons.account_balance_wallet_rounded), 'Earnings'),
    (_NavIcon.svg('assets/figma/user.svg'), 'Profile'),
  ];

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 12),
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(40),
        boxShadow: const [BoxShadow(color: Color(0x1A000000), blurRadius: 20, offset: Offset(0, 6))],
      ),
      child: SafeArea(
        top: false,
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            for (var i = 0; i < _items.length; i++)
              _NavItem(
                icon: _items[i].$1,
                label: _items[i].$2,
                selected: selectedIndex == i,
                badge: i == 1 && pendingCount > 0 ? pendingCount : null,
                onTap: () => onTap(i),
              ),
          ],
        ),
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.icon,
    required this.label,
    required this.selected,
    required this.onTap,
    this.badge,
  });

  final _NavIcon icon;
  final String label;
  final bool selected;
  final VoidCallback onTap;
  final int? badge;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        padding: EdgeInsets.symmetric(horizontal: selected ? 18 : 14, vertical: 12),
        decoration: BoxDecoration(
          color: selected ? kProviderGreen : Colors.transparent,
          borderRadius: BorderRadius.circular(30),
        ),
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                icon.build(selected ? Colors.white : kProviderMuted),
                if (selected) ...[
                  const SizedBox(width: 8),
                  Text(
                    label,
                    style: const TextStyle(
                      color: Colors.white,
                      fontWeight: FontWeight.w700,
                      fontSize: 13,
                    ),
                  ),
                ],
              ],
            ),
            if (badge != null && !selected)
              Positioned(
                top: -6,
                right: -6,
                child: Container(
                  width: 16,
                  height: 16,
                  decoration: const BoxDecoration(color: Colors.red, shape: BoxShape.circle),
                  alignment: Alignment.center,
                  child: Text(
                    badge! > 9 ? '9+' : '$badge',
                    style: const TextStyle(color: Colors.white, fontSize: 9, fontWeight: FontWeight.w800),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

// ─── Nav icon descriptor (asset SVG or Material icon) ─────────────────────────

class _NavIcon {
  const _NavIcon._({this.assetPath, this.iconData});
  factory _NavIcon.svg(String path) => _NavIcon._(assetPath: path);
  factory _NavIcon.icon(IconData data) => _NavIcon._(iconData: data);

  final String? assetPath;
  final IconData? iconData;

  Widget build(Color color) {
    final path = assetPath;
    if (path != null) {
      return SvgPicture.asset(
        path,
        width: 22,
        height: 22,
        colorFilter: ColorFilter.mode(color, BlendMode.srcIn),
      );
    }
    return Icon(iconData, color: color, size: 22);
  }
}
