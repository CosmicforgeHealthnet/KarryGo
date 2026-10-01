import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
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
    final provider = authController.state.provider;

    return Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      body: SafeArea(
        child: RefreshIndicator(
          color: CosmicforgeLogisticsColors.primary,
          onRefresh: controller.refreshBookings,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 32),
            children: [
              Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Hi, ${provider?.firstName.isNotEmpty == true ? provider!.firstName : 'Driver'}',
                          style: const TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w800,
                            color: CosmicforgeLogisticsColors.text,
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          state.online
                              ? 'You are online and can receive trips.'
                              : 'You are offline.',
                          style: const TextStyle(
                            fontSize: 13,
                            color: CosmicforgeLogisticsColors.textMuted,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (provider != null)
                    CircleAvatar(
                      radius: 22,
                      backgroundColor: CosmicforgeLogisticsColors.primaryTint,
                      child: Text(
                        provider.initial,
                        style: const TextStyle(
                          color: CosmicforgeLogisticsColors.primary,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 20),

              _OnlineCard(controller: controller, state: state),

              if (state.error != null) ...[
                const SizedBox(height: 14),
                InlineBanner(
                  message: state.error!,
                  onDismiss: controller.clearError,
                ),
              ],
              if (state.notice != null) ...[
                const SizedBox(height: 14),
                InlineBanner(
                  message: state.notice!,
                  isError: false,
                  onDismiss: controller.clearNotice,
                ),
              ],

              const SizedBox(height: 24),
              Row(
                children: [
                  const Text(
                    'Trip requests',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w800,
                      color: CosmicforgeLogisticsColors.text,
                    ),
                  ),
                  const SizedBox(width: 8),
                  if (state.offers.isNotEmpty)
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: CosmicforgeLogisticsColors.primary,
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: Text(
                        '${state.offers.length}',
                        style: const TextStyle(
                          color: Colors.white,
                          fontSize: 11,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 12),

              if (state.offers.isEmpty)
                _EmptyOffers(online: state.online)
              else
                for (final offer in state.offers)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: _OfferCard(
                      booking: offer,
                      busy: state.isActionLoading,
                      onAccept: () => controller.acceptOffer(offer.id),
                      onReject: () => controller.rejectOffer(offer.id),
                    ),
                  ),
            ],
          ),
        ),
      ),
    );
  }
}

class _OnlineCard extends StatelessWidget {
  const _OnlineCard({required this.controller, required this.state});

  final ProviderHomeController controller;
  final ProviderHomeState state;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        gradient: CosmicforgeLogisticsColors.radialBlue,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  state.online ? 'Online' : 'Offline',
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 18,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  state.online
                      ? 'We are sending you nearby trips.'
                      : 'Go online to start receiving trips.',
                  style: TextStyle(
                    color: Colors.white.withValues(alpha: 0.85),
                    fontSize: 12.5,
                  ),
                ),
              ],
            ),
          ),
          if (state.isTogglingOnline)
            const SizedBox(
              width: 24,
              height: 24,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: Colors.white,
              ),
            )
          else
            Switch.adaptive(
              value: state.online,
              onChanged: controller.setOnline,
              activeThumbColor: Colors.white,
            ),
        ],
      ),
    );
  }
}

class _EmptyOffers extends StatelessWidget {
  const _EmptyOffers({required this.online});

  final bool online;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 36, horizontal: 20),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.card,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: CosmicforgeLogisticsColors.border),
      ),
      child: Column(
        children: [
          Icon(
            online ? Icons.hourglass_empty_rounded : Icons.power_settings_new_rounded,
            size: 34,
            color: CosmicforgeLogisticsColors.primarySoft,
          ),
          const SizedBox(height: 12),
          Text(
            online
                ? 'No requests yet. We will show them here as they come in.'
                : 'You are offline, so no requests will reach you.',
            textAlign: TextAlign.center,
            style: const TextStyle(
              color: CosmicforgeLogisticsColors.textMuted,
              fontSize: 13,
            ),
          ),
        ],
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
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.card,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: CosmicforgeLogisticsColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  '#${booking.shortId}',
                  style: const TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                    color: CosmicforgeLogisticsColors.textMuted,
                  ),
                ),
              ),
              JobTypeBadge(isParcel: booking.isParcel),
            ],
          ),
          const SizedBox(height: 14),
          RouteSummary(booking: booking),
          const SizedBox(height: 14),
          JobStatsRow(booking: booking),

          if (booking.isParcel && booking.packageDesc.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(
              'Package: ${booking.packageDesc}'
              '${booking.isFragile ? ' · fragile' : ''}',
              style: const TextStyle(
                color: CosmicforgeLogisticsColors.textMuted,
                fontSize: 12,
              ),
            ),
          ],
          if (booking.rideNotes.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              'Note: ${booking.rideNotes}',
              style: const TextStyle(
                color: CosmicforgeLogisticsColors.textMuted,
                fontSize: 12,
              ),
            ),
          ],

          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: busy ? null : onReject,
                  style: OutlinedButton.styleFrom(
                    minimumSize: const Size(0, 46),
                    side: const BorderSide(
                      color: CosmicforgeLogisticsColors.border,
                    ),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(24),
                    ),
                  ),
                  child: const Text(
                    'Decline',
                    style: TextStyle(
                      color: CosmicforgeLogisticsColors.danger,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                flex: 2,
                child: FilledButton(
                  onPressed: busy ? null : onAccept,
                  style: FilledButton.styleFrom(
                    minimumSize: const Size(0, 46),
                    backgroundColor: CosmicforgeLogisticsColors.success,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(24),
                    ),
                  ),
                  child: const Text(
                    'Accept',
                    style: TextStyle(
                      color: Colors.white,
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
