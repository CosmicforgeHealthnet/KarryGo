import 'package:flutter/material.dart';

import '../../../../core/format/date_format.dart';
import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../auth/models/provider_auth_models.dart';
import '../../state/provider_home_controller.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_request_card.dart';

/// One request in full: fare, route, load and receiver, then Decline / Accept.
class ProviderRequestDetailScreen extends StatelessWidget {
  const ProviderRequestDetailScreen({
    super.key,
    required this.booking,
    required this.homeController,
  });

  final ProviderBooking booking;
  final ProviderHomeController homeController;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: KTopBar(title: 'Request details'),
      body: AnimatedBuilder(
        animation: homeController,
        builder: (context, _) {
          final state = homeController.state;
          return Column(
            children: [
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                  children: [
                    Row(
                      children: [
                        const KStatusChip('New request'),
                        const Spacer(),
                        Text(
                          '#${booking.shortId}',
                          style: TextStyle(fontSize: 12.5, color: kProviderMuted),
                        ),
                      ],
                    ),
                    const SizedBox(height: 14),
                    Text(
                      formatNairaShort(booking.fareNaira),
                      style: TextStyle(
                        fontSize: 36,
                        fontWeight: FontWeight.w700,
                        letterSpacing: -1,
                        color: kProviderText,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      [
                        'Estimated trip fare',
                        if (booking.distanceKm != null) '${booking.distanceKm!.toStringAsFixed(1)} km',
                        if (booking.estimatedMinutes != null) '~${booking.estimatedMinutes} min',
                      ].join(' · '),
                      style: TextStyle(fontSize: 13.5, color: kProviderMuted),
                    ),
                    const SizedBox(height: 20),
                    KCard(
                      child: BookingRoute(
                        full: true,
                        pickupAddress: booking.pickupAddress,
                        dropoffAddress: booking.dropoffAddress,
                      ),
                    ),
                    const SizedBox(height: 12),
                    KCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              KIconTile(Icons.inventory_2_outlined),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      booking.cargoTitle,
                                      style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
                                    ),
                                    const SizedBox(height: 2),
                                    Text(
                                      booking.cargoSummary,
                                      style: TextStyle(fontSize: 12.5, color: kProviderMuted),
                                    ),
                                  ],
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 10),
                          const Divider(),
                          const SizedBox(height: 4),
                          KInfoRow(
                            'Truck required',
                            booking.preferredTruckType.isNotEmpty
                                ? truckTypeLabel(booking.preferredTruckType)
                                : 'Any truck',
                          ),
                          KInfoRow('Load weight', booking.weightCategory.isNotEmpty
                              ? booking.weightCategory
                              : '${booking.cargoWeightKg} kg'),
                          KInfoRow(
                            'Loaders needed',
                            booking.requiresHelpers ? '${booking.helperCount}' : 'None',
                          ),
                          if (booking.packageSize.isNotEmpty) KInfoRow('Package size', booking.packageSize),
                          KInfoRow('Requested', formatDateTime(booking.createdAt)),
                          if (booking.isFragile) ...[
                            const SizedBox(height: 8),
                            const KNotice(
                              'Fragile load — handle with care when loading and unloading.',
                              tone: KTone.warning,
                            ),
                          ],
                        ],
                      ),
                    ),
                    if (booking.receiverName.trim().isNotEmpty) ...[
                      const SizedBox(height: 12),
                      KCard(
                        child: Row(
                          children: [
                            KAvatar(name: booking.receiverName),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    booking.receiverName.trim(),
                                    style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600),
                                  ),
                                  const SizedBox(height: 2),
                                  Text(
                                    'Receiver at delivery',
                                    style: TextStyle(fontSize: 12.5, color: kProviderMuted),
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                    if (state.error != null) ...[
                      const SizedBox(height: 14),
                      KNotice(state.error!, tone: KTone.danger),
                    ],
                  ],
                ),
              ),
              _BottomActions(
                children: [
                  Expanded(
                    child: KSecondaryButton(
                      label: 'Decline',
                      destructive: true,
                      onPressed: state.isLoading
                          ? null
                          : () async {
                              final ok = await homeController.rejectBooking(booking.id);
                              if (ok && context.mounted) Navigator.of(context).pop();
                            },
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: KPrimaryButton(
                      label: 'Accept',
                      loading: state.isLoading,
                      onPressed: () async {
                        final ok = await homeController.acceptBooking(booking.id);
                        if (ok && context.mounted) Navigator.of(context).pop();
                      },
                    ),
                  ),
                ],
              ),
            ],
          );
        },
      ),
    );
  }
}

/// White footer that keeps the decision buttons in reach.
class _BottomActions extends StatelessWidget {
  const _BottomActions({required this.children});
  final List<Widget> children;

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
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
          child: Row(children: children),
        ),
      ),
    );
  }
}
