import 'package:flutter/material.dart';

/// Area name overlay on the map. Kept dark regardless of theme since the
/// map tiles (OpenStreetMap) stay light in both modes.
class MapAreaLabel extends StatelessWidget {
  const MapAreaLabel({super.key, required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Text(
        label,
        style: const TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w600,
          color: Color(0xFF3C4C69),
          shadows: [
            Shadow(color: Colors.white, blurRadius: 4),
            Shadow(color: Colors.white, blurRadius: 4),
          ],
        ),
      ),
    );
  }
}
