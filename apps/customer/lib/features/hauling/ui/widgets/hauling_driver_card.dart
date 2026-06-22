import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';

class HaulingDriverCard extends StatelessWidget {
  const HaulingDriverCard({
    super.key,
    required this.provider,
    this.truck,
    this.fareKobo,
    this.distanceKm,
    this.compact = false,
  });

  final ProviderSnapshot provider;
  final TruckSnapshot? truck;
  final int? fareKobo;
  final double? distanceKm;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        boxShadow: [BoxShadow(color: Colors.black.withAlpha(18), blurRadius: 8)],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              _DriverAvatar(photoUrl: provider.profilePhotoUrl, initials: _initials),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      provider.displayName,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Row(
                      children: [
                        const Icon(Icons.star_rounded, color: Color(0xFFFFC107), size: 14),
                        const SizedBox(width: 3),
                        Text(
                          provider.rating.toStringAsFixed(1),
                          style: const TextStyle(
                            color: CustomerFigmaColors.text,
                            fontSize: 12,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        const SizedBox(width: 8),
                        Text(
                          '${provider.totalTrips} trips',
                          style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
          if (truck != null) ...[
            const SizedBox(height: 10),
            _TruckRow(truck: truck!),
          ],
          if (!compact && (fareKobo != null || distanceKm != null)) ...[
            const SizedBox(height: 10),
            const Divider(height: 1),
            const SizedBox(height: 10),
            Row(
              children: [
                if (distanceKm != null)
                  _InfoChip(
                    icon: Icons.straighten_rounded,
                    label: '${distanceKm!.toStringAsFixed(1)} km',
                  ),
                if (fareKobo != null) ...[
                  const SizedBox(width: 12),
                  _InfoChip(
                    icon: Icons.payments_outlined,
                    label: '₦${(fareKobo! / 100).toStringAsFixed(0)}',
                  ),
                ],
              ],
            ),
          ],
        ],
      ),
    );
  }

  String get _initials {
    final f = provider.firstName.isNotEmpty ? provider.firstName[0] : '';
    final l = provider.lastName.isNotEmpty ? provider.lastName[0] : '';
    return '$f$l'.toUpperCase();
  }
}

class _DriverAvatar extends StatelessWidget {
  const _DriverAvatar({this.photoUrl, required this.initials});

  final String? photoUrl;
  final String initials;

  @override
  Widget build(BuildContext context) {
    return CircleAvatar(
      radius: 24,
      backgroundColor: CustomerFigmaColors.primaryTint,
      backgroundImage: photoUrl != null && photoUrl!.isNotEmpty
          ? NetworkImage(photoUrl!)
          : null,
      child: (photoUrl == null || photoUrl!.isEmpty)
          ? Text(
              initials,
              style: const TextStyle(
                color: CustomerFigmaColors.darkGreen,
                fontWeight: FontWeight.w700,
                fontSize: 14,
              ),
            )
          : null,
    );
  }
}

class _TruckRow extends StatelessWidget {
  const _TruckRow({required this.truck});

  final TruckSnapshot truck;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Icon(Icons.local_shipping_outlined, color: CustomerFigmaColors.muted, size: 16),
        const SizedBox(width: 6),
        Expanded(
          child: Text(
            '${truck.displayInfo} · ${truck.plateNumber}',
            style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }
}

class _InfoChip extends StatelessWidget {
  const _InfoChip({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, color: CustomerFigmaColors.primary, size: 14),
        const SizedBox(width: 4),
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 12,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    );
  }
}

class HaulingDriverCardShimmer extends StatelessWidget {
  const HaulingDriverCardShimmer({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        boxShadow: [BoxShadow(color: Colors.black.withAlpha(18), blurRadius: 8)],
      ),
      child: Row(
        children: [
          Container(
            width: 48, height: 48,
            decoration: BoxDecoration(
              color: Colors.grey[200],
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Container(height: 12, width: 120, color: Colors.grey[200]),
                const SizedBox(height: 6),
                Container(height: 10, width: 80, color: Colors.grey[200]),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
