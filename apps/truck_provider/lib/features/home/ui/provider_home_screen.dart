import 'package:flutter/material.dart';

import '../../fuel/fuel_prompt.dart';
import '../../referrals/state/referral_controller.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../earnings/state/provider_earnings_controller.dart';
import '../../earnings/ui/provider_earnings_screen.dart';
import '../../notifications/state/provider_notification_controller.dart';
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
import 'screens/provider_delivery_complete_screen.dart';
import 'screens/provider_notifications_screen.dart';
import 'screens/provider_requests_screen.dart';
import 'widgets/provider_sos_sheet.dart';
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
    this.referralController,
    required this.tripsController,
    required this.notificationController,
    this.tabRequests,
  });

  final ProviderAuthController authController;
  final ProviderHomeController homeController;
  final ProviderProfileController profileController;
  final ProviderEarningsController earningsController;
  final ProviderWithdrawalController withdrawalController;
  final ProviderDisputeController disputeController;

  /// The referral dashboard; hidden from Profile when null.
  final ProviderReferralController? referralController;
  final ProviderTripsController tripsController;
  final ProviderNotificationController notificationController;

  /// Tabs requested from outside the shell (a tapped push notification).
  final ValueNotifier<int?>? tabRequests;

  @override
  State<ProviderHomeScreen> createState() => _ProviderHomeScreenState();
}

class _ProviderHomeScreenState extends State<ProviderHomeScreen> {
  int _tabIndex = 0;

  @override
  void initState() {
    super.initState();
    widget.tabRequests?.addListener(_onTabRequested);
    WidgetsBinding.instance.addPostFrameCallback((_) => _onTabRequested());
    widget.homeController.onWentOnline = _promptFuelPrice;
  }

  @override
  void dispose() {
    if (widget.homeController.onWentOnline == _promptFuelPrice) {
      widget.homeController.onWentOnline = null;
    }
    widget.tabRequests?.removeListener(_onTabRequested);
    super.dispose();
  }

  void _onTabRequested() {
    final index = widget.tabRequests?.value;
    if (index == null || !mounted) return;
    widget.tabRequests!.value = null;
    _onTabTap(index);
  }

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

  /// The first time each day the driver goes online, ask what they paid for
  /// fuel. Skipped while a trip is on screen.
  Future<void> _promptFuelPrice() async {
    if (!mounted || widget.homeController.state.activeBooking != null) return;
    if (!await FuelPromptSchedule.claimToday() || !mounted) return;
    await showFuelPriceSheet(context, report: widget.homeController.reportFuelPrice, prompted: true);
  }

  void _openNotifications() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderNotificationsScreen(
          controller: widget.notificationController,
        ),
      ),
    );
  }

  void _openWithdrawal() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderWithdrawalFormScreen(controller: widget.withdrawalController),
      ),
    );
  }

  /// "Go to Trips" from anywhere in Earnings: close what is open over the
  /// shell and switch to the Trips tab.
  void _goToTrips() {
    Navigator.of(context).popUntil((route) => route.isFirst);
    _onTabTap(2);
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
          final active = state.activeBooking!;
          return ProviderActiveTripScreen(
            controller: widget.homeController,
            booking: active,
            onSos: () => showProviderSosSheet(
              context,
              support: widget.disputeController,
              bookingId: active.id,
            ),
          );
        }

        final delivered = state.justDelivered;
        if (delivered != null) {
          return ProviderDeliveryCompleteScreen(
            booking: delivered,
            onDone: () {
              widget.homeController.dismissDeliveryComplete();
              _onTabTap(0);
            },
            onViewEarnings: () {
              widget.homeController.dismissDeliveryComplete();
              _onTabTap(3);
            },
          );
        }

        final pendingCount = state.pendingRequests.length;

        return Scaffold(
          body: IndexedStack(
            index: _tabIndex,
            children: [
              ProviderDashboardScreen(
                authController: widget.authController,
                homeController: widget.homeController,
                state: state,
                earningsController: widget.earningsController,
                onNotificationsTap: _openNotifications,
                onViewRequests: () => _onTabTap(1),
                onViewEarnings: () => _onTabTap(3),
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
                onGoToTrips: _goToTrips,
              ),
              ProviderProfileScreen(
                authController: widget.authController,
                profileController: widget.profileController,
                disputeController: widget.disputeController,
                withdrawalController: widget.withdrawalController,
                referralController: widget.referralController,
                onReportFuel: () => showFuelPriceSheet(context, report: widget.homeController.reportFuelPrice),
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
    (_NavIcon.icon(Icons.home_rounded), 'Home'),
    (_NavIcon.icon(Icons.inbox_rounded), 'Requests'),
    (_NavIcon.icon(Icons.local_shipping_rounded), 'Trips'),
    (_NavIcon.icon(Icons.account_balance_wallet_rounded), 'Earnings'),
    (_NavIcon.icon(Icons.person_rounded), 'Profile'),
  ];

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(
        color: kCard,
        border: Border(top: BorderSide(color: kProviderBorder)),
      ),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(8, 8, 8, 6),
          child: Row(
            children: [
              for (var i = 0; i < _items.length; i++)
                Expanded(
                  child: _NavItem(
                    icon: _items[i].$1,
                    label: _items[i].$2,
                    selected: selectedIndex == i,
                    badge: i == 1 && pendingCount > 0 ? pendingCount : null,
                    onTap: () => onTap(i),
                  ),
                ),
            ],
          ),
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
    final color = selected ? kProviderText : kProviderMuted;
    return Semantics(
      button: true,
      selected: selected,
      label: badge == null ? label : '$label, $badge new',
      excludeSemantics: true,
      onTap: onTap,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(14),
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              AnimatedContainer(
                duration: const Duration(milliseconds: 200),
                width: 52,
                height: 30,
                decoration: BoxDecoration(
                  color: selected ? kTealTint : Colors.transparent,
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Stack(
                  clipBehavior: Clip.none,
                  alignment: Alignment.center,
                  children: [
                    icon.build(selected ? kTealText : kProviderMuted),
                    if (badge != null)
                      Positioned(
                        top: -2,
                        right: 8,
                        child: Container(
                          constraints: const BoxConstraints(minWidth: 16),
                          height: 16,
                          padding: const EdgeInsets.symmetric(horizontal: 4),
                          decoration: BoxDecoration(
                            color: const Color(0xFFC62838),
                            borderRadius: BorderRadius.circular(8),
                            border: Border.all(color: kCard, width: 1.5),
                          ),
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
              const SizedBox(height: 4),
              Text(
                label,
                maxLines: 1,
                style: TextStyle(
                  color: color,
                  fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                  fontSize: 11.5,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Nav icon ─────────────────────────────────────────────────────────────────

class _NavIcon {
  const _NavIcon._(this.iconData);
  factory _NavIcon.icon(IconData data) => _NavIcon._(data);

  final IconData iconData;

  Widget build(Color color) => Icon(iconData, color: color, size: 22);
}
