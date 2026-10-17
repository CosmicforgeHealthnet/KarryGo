import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

/// Reasons the rider can pick when declining (design 06).
///
/// `code` is our internal id (kept for UI keys/logging).
/// `backendCode` is what actually goes to the server — the backend whitelist
/// is strictly {too_far, busy, other}; anything else returns 400.
enum DeclineReason {
  notOnMyRoute('not_on_my_route', 'Not on my route', 'too_far'),
  vehicleNotSuitable('vehicle_not_suitable', 'Vehicle not suitable', 'other'),
  rateTooLow('rate_too_low', 'Rate too low', 'other'),
  scheduleConflict('schedule_conflict', 'Schedule conflict', 'busy'),
  notAvailableNow('not_available_now', 'Not available now', 'busy'),
  other('other', 'Other', 'other');

  const DeclineReason(this.code, this.label, this.backendCode);

  final String code;
  final String label;

  /// The value sent to `POST /provider/requests/:id/reject`.
  /// Must be one of: too_far | busy | other.
  final String backendCode;
}

/// Result of the decline flow: reason + optional free-text comment.
///
/// NOTE: `comment` is captured for UX only — the backend's reject endpoint
/// accepts just a whitelisted reason code and has no field for free text.
/// It is deliberately not sent over the wire.
class DeclineResult {
  const DeclineResult({required this.reason, this.comment = ''});
  final DeclineReason reason;
  final String comment;
}

/// Full-screen decline flow. Returns the picked [DeclineResult] on submit, or
/// null if the rider backed out.
Future<DeclineResult?> pushDeclineRequestScreen(BuildContext context) =>
    showModalBottomSheet<DeclineResult>(
      context: context,
      useSafeArea: true,
      isScrollControlled: true,
      showDragHandle: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (_) => const DeclineRequestScreen(),
    );

class DeclineRequestScreen extends StatefulWidget {
  const DeclineRequestScreen({super.key});
  @override
  State<DeclineRequestScreen> createState() => _DeclineRequestScreenState();
}

class _DeclineRequestScreenState extends State<DeclineRequestScreen> {
  DeclineReason? _selected;
  @override
  Widget build(BuildContext context) => SingleChildScrollView(
    padding: EdgeInsets.fromLTRB(
      24,
      8,
      24,
      24 + MediaQuery.paddingOf(context).bottom,
    ),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          'Decline request',
          style: Theme.of(context).textTheme.headlineSmall,
        ),
        const SizedBox(height: 8),
        Text(
          'Choose a reason for declining this delivery.',
          style: Theme.of(context).textTheme.bodyLarge,
        ),
        const SizedBox(height: 20),
        for (final reason in [
          DeclineReason.notOnMyRoute,
          DeclineReason.scheduleConflict,
          DeclineReason.other,
        ])
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: RadioListTile<DeclineReason>(
              value: reason,
              groupValue: _selected,
              onChanged: (value) => setState(() => _selected = value),
              title: Text(reason.label),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(16),
              ),
              tileColor: _selected == reason
                  ? context.palette.surfaceElevated
                  : null,
            ),
          ),
        const SizedBox(height: 16),
        FilledButton(
          onPressed: _selected == null
              ? null
              : () => Navigator.pop(context, DeclineResult(reason: _selected!)),
          style: FilledButton.styleFrom(
            backgroundColor: context.palette.dangerSoft,
            foregroundColor: context.palette.danger,
          ),
          child: const Text('Decline request'),
        ),
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Keep request'),
        ),
      ],
    ),
  );
}
