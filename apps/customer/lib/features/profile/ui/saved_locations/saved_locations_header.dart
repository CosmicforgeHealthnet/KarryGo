import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Figma 1777/1778 header: floating white circular back button with the
/// "Saved Locations" title + subtitle beside it.
class SavedLocationsHeader extends StatelessWidget {
  const SavedLocationsHeader({super.key, this.trailing});

  /// Optional widget aligned to the far right of the header row
  /// (e.g. the delete action on the edit form).
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 10, 20, 0),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Material(
            color: Colors.white,
            shape: const CircleBorder(),
            elevation: 3,
            shadowColor: const Color(0x33000000),
            child: InkWell(
              customBorder: const CircleBorder(),
              onTap: () => Navigator.of(context).pop(),
              child: const Padding(
                padding: EdgeInsets.all(12),
                child: Icon(Icons.arrow_back, color: CustomerFigmaColors.text, size: 20),
              ),
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: const [
                Text(
                  'Saved Locations',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 21,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                SizedBox(height: 4),
                Text(
                  'Manage your saved locations to easily book your trips.',
                  style: TextStyle(
                    color: CustomerFigmaColors.muted,
                    fontSize: 13,
                    height: 1.35,
                  ),
                ),
              ],
            ),
          ),
          ?trailing,
        ],
      ),
    );
  }
}
