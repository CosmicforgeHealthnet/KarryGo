import 'package:flutter/material.dart';

import '../../auth/state/provider_auth_controller.dart';
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
  });

  final ProviderAuthController authController;
  final ProviderHomeController homeController;

  @override
  State<ProviderHomeScreen> createState() => _ProviderHomeScreenState();
}

class _ProviderHomeScreenState extends State<ProviderHomeScreen> {
  int _tabIndex = 0;

  void _onTabTap(int index) {
    if (index == 2 || index == 3) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Coming soon'),
          duration: Duration(seconds: 1),
          behavior: SnackBarBehavior.floating,
        ),
      );
      return;
    }
    setState(() => _tabIndex = index);
  }

  void _openNotifications() {
    Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => const ProviderNotificationsScreen()),
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
              const SizedBox.shrink(),
              const SizedBox.shrink(),
              _ProfileTab(authController: widget.authController),
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

  static const _items = [
    (Icons.home_rounded, 'Home'),
    (Icons.swap_vert_rounded, 'Requests'),
    (Icons.calendar_today_rounded, 'Calendar'),
    (Icons.credit_card_rounded, 'Card'),
    (Icons.person_rounded, 'Profile'),
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

  final IconData icon;
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
                Icon(icon, color: selected ? Colors.white : kProviderMuted, size: 22),
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

// ─── Profile tab ──────────────────────────────────────────────────────────────

class _ProfileTab extends StatelessWidget {
  const _ProfileTab({required this.authController});

  final ProviderAuthController authController;

  @override
  Widget build(BuildContext context) {
    final provider = authController.state.session?.provider;
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        title: const Text(
          'Profile',
          style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 17),
        ),
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(20),
        child: Column(
          children: [
            const SizedBox(height: 16),
            Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                color: kProviderGreenTint,
                shape: BoxShape.circle,
                border: Border.all(color: kProviderGreen, width: 2),
              ),
              child: provider?.profilePhotoUrl != null
                  ? ClipOval(child: Image.network(provider!.profilePhotoUrl!, fit: BoxFit.cover))
                  : const Icon(Icons.person_rounded, color: kProviderGreen, size: 40),
            ),
            const SizedBox(height: 12),
            Text(
              provider?.displayName ?? 'Provider',
              style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18),
            ),
            Text(provider?.phone ?? '', style: const TextStyle(color: kProviderMuted, fontSize: 13)),
            const SizedBox(height: 32),
            _MenuItem(icon: Icons.local_shipping_rounded, label: 'My Trucks', onTap: () {}),
            _MenuItem(icon: Icons.history_rounded, label: 'Trip History', onTap: () {}),
            _MenuItem(icon: Icons.support_agent_rounded, label: 'Support', onTap: () {}),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              child: OutlinedButton.icon(
                onPressed: () => authController.logout(),
                icon: const Icon(Icons.logout_rounded),
                label: const Text('Sign out'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: Colors.red,
                  side: const BorderSide(color: Colors.red),
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                  padding: const EdgeInsets.symmetric(vertical: 14),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MenuItem extends StatelessWidget {
  const _MenuItem({required this.icon, required this.label, required this.onTap});

  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(12),
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: kProviderSurface,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            Icon(icon, color: kProviderGreen, size: 20),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w600, fontSize: 14),
              ),
            ),
            const Icon(Icons.chevron_right_rounded, color: kProviderMuted, size: 20),
          ],
        ),
      ),
    );
  }
}
