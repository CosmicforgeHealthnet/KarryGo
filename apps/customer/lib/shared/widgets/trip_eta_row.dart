import 'package:flutter/material.dart';

import 'figma_customer_widgets.dart';

/// When the driver should reach the pickup and the drop-off, side by side.
/// A missing time is left out; with neither, nothing is shown.
class TripEtaRow extends StatelessWidget {
  const TripEtaRow({super.key, this.pickupEta, this.dropoffEta, this.pickupLabel = 'Pickup'});

  final DateTime? pickupEta;
  final DateTime? dropoffEta;
  final String pickupLabel;

  @override
  Widget build(BuildContext context) {
    final tiles = <Widget>[
      if (pickupEta != null) _EtaTile(label: pickupLabel, at: pickupEta!, icon: Icons.radio_button_checked),
      if (dropoffEta != null) _EtaTile(label: 'Drop-off', at: dropoffEta!, icon: Icons.location_on),
    ];
    if (tiles.isEmpty) return const SizedBox.shrink();
    return Row(
      children: [
        for (var i = 0; i < tiles.length; i++) ...[
          if (i > 0) const SizedBox(width: 10),
          Expanded(child: tiles[i]),
        ],
      ],
    );
  }
}

class _EtaTile extends StatelessWidget {
  const _EtaTile({required this.label, required this.at, required this.icon});
  final String label;
  final DateTime at;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    final minutes = at.difference(DateTime.now()).inSeconds / 60;
    final countdown = minutes < 1 ? 'any moment now' : 'in ${minutes.ceil()} min';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: CustomerFigmaColors.border),
      ),
      child: Row(
        children: [
          Icon(icon, size: 18, color: CustomerFigmaColors.primary),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(label, style: const TextStyle(fontSize: 11.5, color: CustomerFigmaColors.muted)),
                Text(
                  formatClockTime(at),
                  style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w800, color: CustomerFigmaColors.text),
                ),
                Text(countdown, style: const TextStyle(fontSize: 11.5, color: CustomerFigmaColors.muted)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// "10:42 AM".
String formatClockTime(DateTime t) {
  final local = t.toLocal();
  final hour = local.hour % 12 == 0 ? 12 : local.hour % 12;
  final minute = local.minute.toString().padLeft(2, '0');
  return '$hour:$minute ${local.hour < 12 ? 'AM' : 'PM'}';
}
