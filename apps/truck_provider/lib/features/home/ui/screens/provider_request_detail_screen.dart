import 'package:flutter/material.dart';

import '../../../auth/models/provider_auth_models.dart';
import '../../state/provider_home_controller.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_request_card.dart';

/// Request Detail screen (Figma 2045).
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
    final distanceKm = booking.distanceKm;
    final estMinutes = distanceKm != null ? (distanceKm / 30 * 60).round() : null;

    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        leading: Padding(
          padding: const EdgeInsets.only(left: 12),
          child: _CircleBackButton(onTap: () => Navigator.of(context).pop()),
        ),
        title: const Text(
          'Request Detail',
          style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18),
        ),
        centerTitle: true,
      ),
      body: AnimatedBuilder(
        animation: homeController,
        builder: (context, _) {
          final state = homeController.state;
          return SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // ─── Customer header ───────────────────────────────
                Center(
                  child: Column(
                    children: [
                      ProviderAvatar(name: booking.displayName, size: 72),
                      const SizedBox(height: 12),
                      Text(
                        booking.displayName,
                        style: const TextStyle(
                          color: kProviderText,
                          fontWeight: FontWeight.w800,
                          fontSize: 24,
                        ),
                      ),
                      const SizedBox(height: 8),
                      BookingMetricsRow(
                        distanceKm: distanceKm,
                        minutes: estMinutes,
                        fareNaira: booking.fareEstimateNaira,
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 24),

                // ─── Trip reference + date ─────────────────────────
                _LabelValueRow(label: 'Trip Reference:', value: booking.shortId),
                const SizedBox(height: 14),
                _LabelValueRow(label: 'Date:', value: _formatDate(booking.createdAt)),
                const SizedBox(height: 16),

                // ─── Route card with fee ───────────────────────────
                _RouteCard(booking: booking),
                const SizedBox(height: 24),

                // ─── Truck Haul Information ────────────────────────
                const Text(
                  'Truck Haul Information',
                  style: TextStyle(
                    color: kProviderText,
                    fontWeight: FontWeight.w800,
                    fontSize: 18,
                  ),
                ),
                const SizedBox(height: 16),

                _HaulField(
                  question: 'What are you moving?',
                  answer: booking.packageContent.isNotEmpty
                      ? booking.packageContent
                      : (booking.cargoDescription.isNotEmpty ? booking.cargoDescription : '—'),
                ),
                _HaulField(
                  question: 'Load weight category',
                  helper: 'Let us know how heavy the item is.',
                  answer: booking.weightCategory.isNotEmpty
                      ? booking.weightCategory
                      : '${booking.cargoWeightKg} kg',
                ),
                _HaulField(
                  question: 'Truck Type',
                  answer: booking.preferredTruckType.isNotEmpty
                      ? _capitalize(booking.preferredTruckType)
                      : 'Select truck type',
                  answerMuted: booking.preferredTruckType.isEmpty,
                ),
                _HaulField(
                  question: 'Do you need loaders?',
                  helper: 'Let us know if you need extra hands to help you load the truck.',
                  answer: booking.requiresHelpers ? 'Yes ( ${booking.helperCount} )' : 'No',
                ),
                const SizedBox(height: 8),

                const Text(
                  'Do you want to Accept this trip request?',
                  style: TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14),
                ),
                const SizedBox(height: 16),

                if (state.error != null) ...[
                  Text(
                    state.error!,
                    style: const TextStyle(color: Colors.red, fontSize: 13),
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 12),
                ],

                // ─── Action buttons (Figma 2045) ───────────────────
                Row(
                  children: [
                    Expanded(
                      child: RejectRequestButton(
                        onPressed: state.isLoading
                            ? null
                            : () async {
                                await homeController.rejectBooking(booking.id);
                                if (context.mounted) Navigator.of(context).pop();
                              },
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: AcceptRequestButton(
                        onPressed: state.isLoading
                            ? null
                            : () async {
                                await homeController.acceptBooking(booking.id);
                                if (context.mounted) Navigator.of(context).pop();
                              },
                      ),
                    ),
                  ],
                ),
              ],
            ),
          );
        },
      ),
    );
  }

  String _formatDate(DateTime dt) {
    const days = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    return '${days[dt.weekday - 1]}, ${dt.day} ${months[dt.month - 1]} ${dt.year}';
  }

  String _capitalize(String s) => s.isEmpty ? s : s[0].toUpperCase() + s.substring(1);
}

// ─── Circle back button ───────────────────────────────────────────────────────

class _CircleBackButton extends StatelessWidget {
  const _CircleBackButton({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: 40,
        height: 40,
        decoration: const BoxDecoration(color: Color(0xFFF7F8F7), shape: BoxShape.circle),
        child: const Icon(Icons.arrow_back_rounded, color: kProviderText, size: 20),
      ),
    );
  }
}

// ─── Label / value row ────────────────────────────────────────────────────────

class _LabelValueRow extends StatelessWidget {
  const _LabelValueRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 13),
        ),
        const SizedBox(width: 16),
        Expanded(
          child: Text(
            value,
            style: const TextStyle(color: kProviderMuted, fontSize: 13),
            textAlign: TextAlign.end,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }
}

// ─── Route card with Trip Fee ─────────────────────────────────────────────────

class _RouteCard extends StatelessWidget {
  const _RouteCard({required this.booking});
  final ProviderBooking booking;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: kProviderBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          BookingRoute(
            pickupAddress: booking.pickupAddress,
            dropoffAddress: booking.dropoffAddress,
          ),
          const SizedBox(height: 14),
          const Divider(height: 1),
          const SizedBox(height: 14),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'Trip Fee:',
                style: TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14),
              ),
              Text(
                '₦${booking.fareEstimateNaira.toStringAsFixed(2)}',
                style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 15),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

// ─── Haul info field ──────────────────────────────────────────────────────────

class _HaulField extends StatelessWidget {
  const _HaulField({
    required this.question,
    required this.answer,
    this.helper,
    this.answerMuted = false,
  });

  final String question;
  final String answer;
  final String? helper;
  final bool answerMuted;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            question,
            style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14),
          ),
          if (helper != null) ...[
            const SizedBox(height: 2),
            Text(helper!, style: const TextStyle(color: kProviderMuted, fontSize: 12, height: 1.3)),
          ],
          const SizedBox(height: 4),
          Text(
            answer,
            style: TextStyle(
              color: answerMuted ? kProviderMuted : kProviderText,
              fontWeight: answerMuted ? FontWeight.w400 : FontWeight.w700,
              fontSize: 14,
            ),
          ),
        ],
      ),
    );
  }
}
