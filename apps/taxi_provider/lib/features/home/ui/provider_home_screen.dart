import '../../../core/theme/karryam_theme.dart';
import 'package:flutter/material.dart';

import '../../auth/data/provider_auth_api.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../earnings/ui/provider_earnings_screen.dart';
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
  });

  final ProviderHomeController controller;
  final ProviderAuthController authController;
  final ProviderApi api;
  final ProviderAuthApi authApi;

  @override
  State<ProviderHomeScreen> createState() => _ProviderHomeScreenState();
}

class _ProviderHomeScreenState extends State<ProviderHomeScreen>
    with SingleTickerProviderStateMixin {
  late final AnimationController _transition = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 380),
    value: 1,
  );
  @override
  void dispose() {
    _transition.dispose();
    super.dispose();
  }

  int _tab = 0;

  @override
  void initState() {
    super.initState();
    widget.controller.initialize();
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
                ),
                ProviderEarningsScreen(
                  api: widget.api,
                  authController: widget.authController,
                ),
                ProviderProfileScreen(
                  authApi: widget.authApi,
                  authController: widget.authController,
                  homeController: widget.controller,
                ),
              ],
            ),
          ),
          bottomNavigationBar: NavigationBar(
            selectedIndex: _tab,
            onDestinationSelected: (i) {
              setState(() => _tab = i);
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
