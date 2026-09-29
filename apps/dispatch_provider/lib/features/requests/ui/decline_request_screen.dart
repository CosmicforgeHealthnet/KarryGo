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
Future<DeclineResult?> pushDeclineRequestScreen(BuildContext context) {
  return Navigator.of(context).push<DeclineResult>(
    MaterialPageRoute(builder: (_) => const DeclineRequestScreen()),
  );
}

class DeclineRequestScreen extends StatefulWidget {
  const DeclineRequestScreen({super.key});

  @override
  State<DeclineRequestScreen> createState() => _DeclineRequestScreenState();
}

class _DeclineRequestScreenState extends State<DeclineRequestScreen> {
  DeclineReason? _selected;
  final _commentController = TextEditingController();
  final _commentFocus = FocusNode();

  @override
  void dispose() {
    _commentController.dispose();
    _commentFocus.dispose();
    super.dispose();
  }

  bool get _canSubmit => _selected != null;

  void _submit() {
    if (!_canSubmit) return;
    Navigator.of(context).pop(
      DeclineResult(
        reason: _selected!,
        comment: _commentController.text.trim(),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      'Help us improve',
                      style: TextStyle(
                        fontSize: 24,
                        fontWeight: FontWeight.w800,
                        color: colors.text,
                        letterSpacing: -0.6,
                        height: 1.15,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      'Why are you declining this request?',
                      style: TextStyle(
                        fontSize: 14,
                        color: colors.textMuted,
                        height: 1.45,
                      ),
                    ),

                    const SizedBox(height: 24),

                    for (final r in DeclineReason.values) ...[
                      _ReasonRadio(
                        label: r.label,
                        selected: _selected == r,
                        onTap: () => setState(() => _selected = r),
                      ),
                      const SizedBox(height: 8),
                    ],

                    const SizedBox(height: 20),

                    Text(
                      'Add a comment (optional)',
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        color: colors.textMuted,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Container(
                      decoration: BoxDecoration(
                        color: colors.surface,
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(color: colors.divider),
                      ),
                      child: TextField(
                        controller: _commentController,
                        focusNode: _commentFocus,
                        maxLines: 4,
                        maxLength: 200,
                        textCapitalization: TextCapitalization.sentences,
                        style: TextStyle(
                          fontSize: 14,
                          color: colors.text,
                          height: 1.45,
                        ),
                        decoration: InputDecoration(
                          hintText: 'Anything we should know?',
                          hintStyle: TextStyle(
                            fontSize: 14,
                            color: colors.textDisabled,
                          ),
                          border: InputBorder.none,
                          counterText: '',
                          contentPadding: const EdgeInsets.all(14),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            _buildBottomButton(context),
          ],
        ),
      ),
    );
  }

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Row(
        children: [
          GestureDetector(
            onTap: () => Navigator.of(context).pop(),
            behavior: HitTestBehavior.opaque,
            child: Padding(
              padding: const EdgeInsets.all(8),
              child: Icon(
                Icons.arrow_back_rounded,
                size: 22,
                color: colors.text,
              ),
            ),
          ),
          const Spacer(),
        ],
      ),
    );
  }

  Widget _buildBottomButton(BuildContext context) {
    final colors = context.palette;
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
        child: SizedBox(
          width: double.infinity,
          height: 54,
          child: FilledButton(
            onPressed: _canSubmit ? _submit : null,
            style: FilledButton.styleFrom(
              backgroundColor: colors.danger,
              foregroundColor: Colors.white,
              disabledBackgroundColor:
                  colors.danger.withValues(alpha: 0.35),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
              textStyle: const TextStyle(
                fontSize: 15.5,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: const Text('Decline request'),
          ),
        ),
      ),
    );
  }
}

class _ReasonRadio extends StatelessWidget {
  const _ReasonRadio({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 140),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        decoration: BoxDecoration(
          color: selected ? colors.teal10 : colors.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected
                ? colors.brandTeal500.withValues(alpha: 0.5)
                : colors.divider,
            width: selected ? 1.4 : 1,
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 20,
              height: 20,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(
                  color: selected ? colors.brandTeal500 : colors.navy40,
                  width: 2,
                ),
              ),
              child: selected
                  ? Container(
                      width: 10,
                      height: 10,
                      decoration: BoxDecoration(
                        color: colors.brandTeal500,
                        shape: BoxShape.circle,
                      ),
                    )
                  : null,
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                  color: colors.text,
                  letterSpacing: -0.1,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}