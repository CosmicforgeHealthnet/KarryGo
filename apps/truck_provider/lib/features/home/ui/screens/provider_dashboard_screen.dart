import 'package:flutter/material.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../auth/models/provider_auth_models.dart';
import '../../../auth/state/provider_auth_controller.dart';
import '../../../earnings/state/provider_earnings_controller.dart';
import '../../state/provider_home_controller.dart';
import '../screens/provider_request_detail_screen.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_home_map.dart';
import '../widgets/provider_request_card.dart';

/// Home tab: the live map with a pull-up sheet holding the online switch,
/// incoming requests and today's numbers.
class ProviderDashboardScreen extends StatefulWidget {
  const ProviderDashboardScreen({
    super.key,
    required this.authController,
    required this.homeController,
    required this.state,
    this.earningsController,
    this.onNotificationsTap,
    this.onViewRequests,
    this.onViewEarnings,
  });

  final ProviderAuthController authController;
  final ProviderHomeController homeController;
  final ProviderHomeState state;
  final ProviderEarningsController? earningsController;
  final VoidCallback? onNotificationsTap;
  final VoidCallback? onViewRequests;
  final VoidCallback? onViewEarnings;

  @override
  State<ProviderDashboardScreen> createState() =>
      _ProviderDashboardScreenState();
}

class _ProviderDashboardScreenState extends State<ProviderDashboardScreen> {
  @override
  void initState() {
    super.initState();
    final earnings = widget.earningsController;
    if (earnings != null && !earnings.hasLoaded && !earnings.isLoading) {
      WidgetsBinding.instance.addPostFrameCallback((_) => earnings.load());
    }
  }

  void _openRequest(ProviderBooking booking) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderRequestDetailScreen(
          booking: booking,
          homeController: widget.homeController,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final topPadding = MediaQuery.of(context).padding.top;
    final provider = widget.authController.state.session?.provider;

    return Stack(
      children: [
        const Positioned.fill(child: ProviderHomeMap()),

        // Top overlay: who you are, whether you're live, and alerts.
        Positioned(
          top: topPadding + 12,
          left: 16,
          right: 16,
          child: Row(
            children: [
              DecoratedBox(
                decoration: const BoxDecoration(
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: Color(0x220B1F44),
                      blurRadius: 12,
                      offset: Offset(0, 4),
                    ),
                  ],
                ),
                child: KAvatar(
                  name: provider?.displayName ?? '',
                  photoUrl: provider?.profilePhotoUrl,
                  size: 44,
                ),
              ),
              const Spacer(),
              _LivePill(isOnline: state.isOnline),
              const Spacer(),
              DecoratedBox(
                decoration: const BoxDecoration(
                  shape: BoxShape.circle,
                  boxShadow: [
                    BoxShadow(
                      color: Color(0x220B1F44),
                      blurRadius: 12,
                      offset: Offset(0, 4),
                    ),
                  ],
                ),
                child: KCircleButton(
                  icon: Icons.notifications_none_rounded,
                  tooltip: 'Notifications',
                  onTap: widget.onNotificationsTap,
                ),
              ),
            ],
          ),
        ),

        DraggableScrollableSheet(
          initialChildSize: state.isOnline && state.pendingRequests.isNotEmpty
              ? 0.72
              : 0.52,
          minChildSize: 0.34,
          maxChildSize: 0.94,
          snap: true,
          builder: (context, scroll) {
            return DecoratedBox(
              decoration: BoxDecoration(
                color: kCanvas,
                borderRadius: BorderRadius.vertical(top: Radius.circular(26)),
                boxShadow: [
                  BoxShadow(
                    color: Color(0x1F0B1F44),
                    blurRadius: 24,
                    offset: Offset(0, -6),
                  ),
                ],
              ),
              child: ListView(
                controller: scroll,
                padding: const EdgeInsets.fromLTRB(20, 10, 20, 24),
                children: [
                  Center(
                    child: Container(
                      width: 40,
                      height: 4,
                      decoration: BoxDecoration(
                        color: kProviderGreenSoft,
                        borderRadius: BorderRadius.circular(4),
                      ),
                    ),
                  ),
                  const SizedBox(height: 18),
                  KHeading(
                    eyebrow: 'Your road. Your pace.',
                    title: state.isOnline
                        ? "You're online."
                        : "Let's get moving.",
                    subtitle: _subtitle(state),
                    trailing: state.isOnline
                        ? const KStatusChip('Available')
                        : const KStatusChip('Offline', tone: KTone.neutral),
                  ),
                  const SizedBox(height: 18),
                  if (state.isOnline)
                    KSecondaryButton(
                      label: 'Go offline',
                      icon: Icons.power_settings_new_rounded,
                      loading: state.isLoading && state.pendingRequests.isEmpty,
                      onPressed: widget.homeController.goOffline,
                    )
                  else
                    KPrimaryButton(
                      label: 'Go online',
                      icon: Icons.bolt_rounded,
                      loading: state.isLoading,
                      onPressed: widget.homeController.goOnline,
                    ),
                  if (state.error != null) ...[
                    const SizedBox(height: 12),
                    KNotice(state.error!, tone: KTone.danger),
                  ],
                  if (state.isOnline && state.pendingRequests.isNotEmpty) ...[
                    const SizedBox(height: 26),
                    KSectionHeader(
                      'Incoming requests',
                      action: state.pendingRequests.length > 1
                          ? 'View all'
                          : null,
                      onAction: widget.onViewRequests,
                    ),
                    const SizedBox(height: 12),
                    for (final booking in state.pendingRequests.take(2)) ...[
                      ProviderRequestCard(
                        booking: booking,
                        isLoading: state.isLoading,
                        onOpen: () => _openRequest(booking),
                        onReject: () =>
                            widget.homeController.rejectBooking(booking.id),
                        onAccept: () =>
                            widget.homeController.acceptBooking(booking.id),
                      ),
                      const SizedBox(height: 12),
                    ],
                  ],
                  const SizedBox(height: 22),
                  KSectionHeader(
                    'Today, at a glance',
                    action: 'Earnings',
                    onAction: widget.onViewEarnings,
                  ),
                  const SizedBox(height: 12),
                  _TodayTiles(
                    earningsController: widget.earningsController,
                    provider: provider,
                  ),
                  const SizedBox(height: 12),
                  KGroup(
                    children: [
                      KListRow(
                        icon: Icons.north_east_rounded,
                        title: 'Your next opportunity',
                        subtitle: state.isOnline
                            ? 'See every request waiting for you'
                            : 'Go online to start receiving loads',
                        onTap: widget.onViewRequests,
                      ),
                    ],
                  ),
                ],
              ),
            );
          },
        ),
      ],
    );
  }

  String _subtitle(ProviderHomeState state) {
    if (!state.isOnline) {
      return 'Go online to start receiving haulage requests near you.';
    }
    final n = state.pendingRequests.length;
    if (n == 0) return 'Ready for a load that fits your truck.';
    return n == 1
        ? 'A request is waiting for you.'
        : '$n requests are waiting for you.';
  }
}

/// Teal "● Online" / grey "Offline" pill floating over the map.
class _LivePill extends StatelessWidget {
  const _LivePill({required this.isOnline});
  final bool isOnline;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
      decoration: BoxDecoration(
        color: kCard,
        borderRadius: BorderRadius.circular(999),
        boxShadow: const [
          BoxShadow(
            color: Color(0x220B1F44),
            blurRadius: 12,
            offset: Offset(0, 4),
          ),
        ],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          AnimatedContainer(
            duration: const Duration(milliseconds: 250),
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              color: isOnline ? kTeal : kProviderGreenSoft,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 8),
          Text(
            isOnline ? 'Online' : 'Offline',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: kProviderText,
            ),
          ),
        ],
      ),
    );
  }
}

/// Earned today (from the earnings report) and the driver's rating.
class _TodayTiles extends StatelessWidget {
  const _TodayTiles({required this.earningsController, required this.provider});
  final ProviderEarningsController? earningsController;
  final TruckProvider? provider;

  @override
  Widget build(BuildContext context) {
    final controller = earningsController;
    Widget build(String earned, String caption) {
      final rating = provider?.rating ?? 0;
      final trips = provider?.totalTrips ?? 0;
      return IntrinsicHeight(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Expanded(
              child: KStatTile(
                icon: Icons.trending_up_rounded,
                label: 'Earned today',
                value: earned,
                caption: caption,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: KStatTile(
                icon: Icons.star_rounded,
                label: 'Your rating',
                value: rating > 0 ? rating.toStringAsFixed(1) : 'New',
                caption: '$trips trip${trips == 1 ? '' : 's'} in total',
              ),
            ),
          ],
        ),
      );
    }

    if (controller == null) return build('—', 'Open Earnings for details');
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        if (!controller.hasLoaded) {
          return build(
            controller.isLoading ? '…' : '—',
            controller.error ?? 'Loading today',
          );
        }
        final e = controller.earnings;
        final n = e.tripsCompletedToday;
        return build(
          formatNairaShort(e.todayEarningsKobo / 100),
          n == 0
              ? 'No trips completed yet'
              : 'From $n completed trip${n == 1 ? '' : 's'}',
        );
      },
    );
  }
}
