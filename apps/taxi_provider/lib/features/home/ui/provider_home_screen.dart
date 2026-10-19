import '../../../core/theme/karryam_theme.dart';
import 'package:flutter/material.dart';

import '../../../app/driver_services.dart';
import '../../auth/data/provider_auth_api.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../earnings/ui/provider_earnings_screen.dart';
import '../../notifications/ui/notifications_screen.dart';
import '../../support/ui/sos_sheet.dart';
import '../../trips/ui/trips_screen.dart';
import '../../profile/ui/provider_profile_screen.dart';
import '../data/provider_api.dart';
import '../state/provider_home_controller.dart';
import 'screens/provider_active_trip_screen.dart';
import 'screens/provider_dashboard_screen.dart';

/// Signed-in shell. An active trip takes over the whole screen — a driver
/// mid-job should not be able to wander off into earnings and lose the one
/// button that matters.
class ProviderHomeScreen extends StatefulWidget {
  const ProviderHomeScreen({
    super.key,
    required this.controller,
    required this.authController,
    required this.api,
    required this.authApi,
    required this.services,
  });

  final ProviderHomeController controller;
  final ProviderAuthController authController;
  final ProviderApi api;
  final ProviderAuthApi authApi;
  final DriverServices services;

  @override
  State<ProviderHomeScreen> createState() => _ProviderHomeScreenState();
}

class _ProviderHomeScreenState extends State<ProviderHomeScreen>
    with SingleTickerProviderStateMixin, WidgetsBindingObserver {
  late final AnimationController _transition = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 380),
    value: 1,
  );
  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _transition.dispose();
    super.dispose();
  }

  int _tab = 0;
  final _trips = GlobalKey<TripsScreenState>();

  void _openNotifications() => Navigator.of(context).push(
    MaterialPageRoute(
      builder: (_) => NotificationsScreen(taxi: widget.services.taxi),
    ),
  );

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.controller.initialize();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // The server expires a driver's online status if heartbeats stop while
    // the app is in the background; reconcile as soon as it comes back.
    if (state == AppLifecycleState.resumed) widget.controller.resync();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: Listenable.merge([widget.controller, widget.authController]),
      builder: (context, _) {
        final active = widget.controller.state.activeBooking;
        if (active != null) {
          return ProviderActiveTripScreen(
            controller: widget.controller,
            booking: active,
            onSos: () => showSosSheet(
              context,
              api: widget.services.support,
              tripId: active.id,
            ),
          );
        }

        return Scaffold(
          backgroundColor: KarryamColors.of(context).surface,
          body: FadeTransition(
            opacity: CurvedAnimation(
              parent: _transition,
              curve: Curves.easeOutCubic,
            ),
            child: IndexedStack(
              index: _tab,
              children: [
                ProviderDashboardScreen(
                  controller: widget.controller,
                  authController: widget.authController,
                  onOpenNotifications: _openNotifications,
                ),
                TripsScreen(key: _trips, api: widget.services.trips),
                ProviderEarningsScreen(
                  api: widget.api,
                  authController: widget.authController,
                ),
                ProviderProfileScreen(
                  authApi: widget.authApi,
                  authController: widget.authController,
                  homeController: widget.controller,
                  services: widget.services,
                ),
              ],
            ),
          ),
          bottomNavigationBar: NavigationBar(
            selectedIndex: _tab,
            onDestinationSelected: (i) {
              setState(() => _tab = i);
              // A ride just finished should show up in Trips.
              if (i == 1) _trips.currentState?.reload();
              if (!MediaQuery.disableAnimationsOf(context)) {
                _transition.forward(from: .35);
              }
            },
            backgroundColor: KarryamColors.of(context).card,
            indicatorColor: KarryamColors.of(context).primaryTint,
            destinations: [
              NavigationDestination(
                icon: Icon(Icons.home_outlined),
                selectedIcon: Icon(Icons.home_rounded),
                label: 'Home',
              ),
              NavigationDestination(
                icon: Icon(Icons.receipt_long_outlined),
                selectedIcon: Icon(Icons.receipt_long_rounded),
                label: 'Trips',
              ),
              NavigationDestination(
                icon: Icon(Icons.bar_chart_outlined),
                selectedIcon: Icon(Icons.bar_chart_rounded),
                label: 'Earnings',
              ),
              NavigationDestination(
                icon: Icon(Icons.person_outline_rounded),
                selectedIcon: Icon(Icons.person_rounded),
                label: 'Profile',
              ),
            ],
          ),
        );
      },
    );
  }
}
