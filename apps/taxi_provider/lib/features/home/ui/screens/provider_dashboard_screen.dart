import '../../../../core/widgets/driver_map.dart';
import '../../../../core/widgets/driver_state_art.dart';
import '../../../../core/theme/karryam_theme.dart';
import 'package:flutter/material.dart';

import '../../../auth/state/provider_auth_controller.dart';
import '../../models/provider_booking.dart';
import '../../state/provider_home_controller.dart';
import '../widgets/provider_widgets.dart';

/// The driver's home: go online, then answer offers as they arrive.
class ProviderDashboardScreen extends StatelessWidget {
  const ProviderDashboardScreen({
    super.key,
    required this.controller,
    required this.authController,
  });

  final ProviderHomeController controller;
  final ProviderAuthController authController;

  @override
  Widget build(BuildContext context) {
    final state = controller.state;
    final colors = KarryamColors.of(context);
    return Scaffold(
      body: SafeArea(
        child: LayoutBuilder(
          builder: (context, constraints) {
            return Stack(
              children: [
                Positioned.fill(
                  child: DriverMap(
                    booking: state.offers.firstOrNull,
                    online: state.online,
                  ),
                ),
                Positioned(
                  top: 16,
                  left: 20,
                  right: 20,
                  child: Row(
                    children: [
                      CircleAvatar(
                        backgroundColor: colors.card,
                        foregroundColor: colors.text,
                        child: Text(
                          authController.state.provider?.initial ?? 'D',
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 18,
                            vertical: 12,
                          ),
                          decoration: BoxDecoration(
                            color: colors.card,
                            borderRadius: BorderRadius.circular(28),
                            boxShadow: [
                              BoxShadow(
                                color: Colors.black.withValues(alpha: .08),
                                blurRadius: 16,
                              ),
                            ],
                          ),
                          child: Text(
                            'Karryam Driver',
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              color: colors.text,
                              fontWeight: FontWeight.w600,
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                DraggableScrollableSheet(
                  initialChildSize: .43,
                  minChildSize: .30,
                  maxChildSize: .87,
                  builder: (context, scrollController) => Container(
                    decoration: BoxDecoration(
                      color: colors.card,
                      borderRadius: const BorderRadius.vertical(
                        top: Radius.circular(28),
                      ),
                      boxShadow: [
                        BoxShadow(
                          color: Colors.black.withValues(alpha: .12),
                          blurRadius: 30,
                          offset: const Offset(0, -5),
                        ),
                      ],
                    ),
                    child: RefreshIndicator(
                      onRefresh: controller.refreshBookings,
                      child: ListView(
                        controller: scrollController,
                        physics: const AlwaysScrollableScrollPhysics(),
                        padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                        children: [
                          Center(
                            child: Container(
                              width: 36,
                              height: 4,
                              margin: const EdgeInsets.only(bottom: 22),
                              decoration: BoxDecoration(
                                color: colors.border,
                                borderRadius: BorderRadius.circular(4),
                              ),
                            ),
                          ),
                          Row(
                            children: [
                              Container(
                                padding: const EdgeInsets.all(12),
                                decoration: BoxDecoration(
                                  color: colors.primaryTint,
                                  borderRadius: BorderRadius.circular(16),
                                ),
                                child: Icon(
                                  state.online
                                      ? Icons.route
                                      : Icons.power_settings_new,
                                  color: colors.primarySoft,
                                ),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      state.online
                                          ? 'You’re online'
                                          : 'You’re offline',
                                      style: const TextStyle(
                                        fontSize: 22,
                                        fontWeight: FontWeight.w600,
                                      ),
                                    ),
                                    Text(
                                      state.online
                                          ? 'Ready for your next opportunity.'
                                          : 'Your next opportunity is out there.',
                                      style: TextStyle(
                                        color: colors.textMuted,
                                        fontSize: 12,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 20),
                          FilledButton.icon(
                            onPressed: state.isTogglingOnline
                                ? null
                                : () => controller.setOnline(!state.online),
                            icon: state.isTogglingOnline
                                ? const SizedBox(
                                    width: 18,
                                    height: 18,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                    ),
                                  )
                                : const Icon(Icons.power_settings_new),
                            label: Text(
                              state.online ? 'Go offline' : 'Go online',
                            ),
                          ),
                          if (state.error != null)
                            Padding(
                              padding: const EdgeInsets.only(top: 16),
                              child: InlineBanner(
                                message: state.error!,
                                onDismiss: controller.clearError,
                              ),
                            ),
                          if (state.notice != null)
                            Padding(
                              padding: const EdgeInsets.only(top: 16),
                              child: InlineBanner(
                                message: state.notice!,
                                isError: false,
                                onDismiss: controller.clearNotice,
                              ),
                            ),
                          const SizedBox(height: 20),
                          if (state.offers.isEmpty) ...[
                            DriverStateArt(
                              kind: state.online
                                  ? DriverArt.searching
                                  : DriverArt.offline,
                              height: 100,
                            ),
                            Text(
                              state.online
                                  ? 'Looking for your next trip'
                                  : 'Ready when you are',
                              textAlign: TextAlign.center,
                              style: const TextStyle(
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            const SizedBox(height: 8),
                            Text(
                              state.online
                                  ? 'New requests will appear here.'
                                  : 'Go online to receive nearby requests.',
                              textAlign: TextAlign.center,
                              style: TextStyle(color: colors.textMuted),
                            ),
                          ] else
                            for (final offer in state.offers)
                              Padding(
                                padding: const EdgeInsets.only(bottom: 16),
                                child: KarryamEntrance(
                                  key: ValueKey(offer.id),
                                  child: _OfferCard(
                                    booking: offer,
                                    busy: state.isActionLoading,
                                    onAccept: () =>
                                        controller.acceptOffer(offer.id),
                                    onReject: () =>
                                        controller.rejectOffer(offer.id),
                                  ),
                                ),
                              ),
                        ],
                      ),
                    ),
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
  }
}

class _OfferCard extends StatelessWidget {
  const _OfferCard({
    required this.booking,
    required this.busy,
    required this.onAccept,
    required this.onReject,
  });

  final ProviderBooking booking;
  final bool busy;
  final VoidCallback onAccept;
  final VoidCallback onReject;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: KarryamColors.of(context).card,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: KarryamColors.of(context).border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  '#${booking.shortId}',
                  style: TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                    color: KarryamColors.of(context).textMuted,
                  ),
                ),
              ),
              JobTypeBadge(isParcel: booking.isParcel),
            ],
          ),
          SizedBox(height: 14),
          RouteSummary(booking: booking),
          SizedBox(height: 14),
          JobStatsRow(booking: booking),

          if (booking.isParcel && booking.packageDesc.isNotEmpty) ...[
            SizedBox(height: 10),
            Text(
              'Package: ${booking.packageDesc}'
              '${booking.isFragile ? ' · fragile' : ''}',
              style: TextStyle(
                color: KarryamColors.of(context).textMuted,
                fontSize: 12,
              ),
            ),
          ],
          if (booking.rideNotes.isNotEmpty) ...[
            SizedBox(height: 8),
            Text(
              'Note: ${booking.rideNotes}',
              style: TextStyle(
                color: KarryamColors.of(context).textMuted,
                fontSize: 12,
              ),
            ),
          ],

          SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: busy ? null : onReject,
                  style: OutlinedButton.styleFrom(
                    minimumSize: Size(0, 46),
                    side: BorderSide(color: KarryamColors.of(context).border),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: Text(
                    'Decline',
                    style: TextStyle(
                      color: KarryamColors.of(context).danger,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ),
              SizedBox(width: 10),
              Expanded(
                flex: 2,
                child: FilledButton(
                  onPressed: busy ? null : onAccept,
                  style: FilledButton.styleFrom(
                    minimumSize: Size(0, 46),
                    backgroundColor: KarryamColors.of(context).primary,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: Text(
                    'Accept',
                    style: TextStyle(
                      color: KarryamColors.of(context).onPrimary,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
