import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

class OfflineSheet extends StatelessWidget {
  const OfflineSheet({super.key, required this.onGoOnline});
  final VoidCallback onGoOnline;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 20),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(20),
        boxShadow: colors.cardShadow,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Go online to get opportunities',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w800,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            'Be available to receive dispatch requests in your area.',
            style: TextStyle(
              fontSize: 13,
              color: colors.textMuted,
              height: 1.4,
            ),
          ),
          const SizedBox(height: 18),
          SizedBox(
            width: double.infinity,
            height: 52,
            child: FilledButton(
              onPressed: onGoOnline,
              child: const Text('Go online'),
            ),
          ),
        ],
      ),
    );
  }
}