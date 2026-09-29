import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 04 — Delayed realtime / reconnect
// ═══════════════════════════════════════════════════════════════════════════

enum ReconnectStepState { done, active, pending }

class ReconnectScreen extends StatelessWidget {
  const ReconnectScreen({
    super.key,
    this.step = ReconnectStepState.active,
    this.onDone,
  });

  final ReconnectStepState step;
  final VoidCallback? onDone;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
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
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
                children: [
                  // ── Hero ──
                  Center(
                    child: Container(
                      width: 120,
                      height: 120,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: c.info.withValues(alpha: 0.10),
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        Icons.wifi_tethering_rounded,
                        size: 60,
                        color: c.info,
                      ),
                    ),
                  ),
                  const SizedBox(height: 28),

                  Text(
                    'Reconnecting to get the latest updates',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                      height: 1.3,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    "You're back online. We're syncing your latest notifications and updates from Karryam.",
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.5,
                    ),
                  ),
                  const SizedBox(height: 36),

                  _ProgressRow(
                    state: ReconnectStepState.done,
                    text: 'Checking network connection',
                  ),
                  const SizedBox(height: 18),
                  _ProgressRow(
                    state: step == ReconnectStepState.done
                        ? ReconnectStepState.done
                        : ReconnectStepState.active,
                    text: 'Syncing notifications...',
                  ),
                  const SizedBox(height: 18),
                  _ProgressRow(
                    state: step == ReconnectStepState.done
                        ? ReconnectStepState.done
                        : ReconnectStepState.pending,
                    text: 'Updating mission status',
                  ),
                  const SizedBox(height: 18),
                  _ProgressRow(
                    state: step == ReconnectStepState.done
                        ? ReconnectStepState.done
                        : ReconnectStepState.pending,
                    text: 'Almost there...',
                  ),
                  const SizedBox(height: 32),

                  // ── Info chip ──
                  Container(
                    padding: const EdgeInsets.all(14),
                    decoration: BoxDecoration(
                      color: c.info.withValues(alpha: 0.10),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Icon(
                          Icons.info_outline_rounded,
                          size: 18,
                          color: c.info,
                        ),
                        const SizedBox(width: 10),
                        Expanded(
                          child: Text(
                            "This may take a few seconds. We'll notify you as soon as everything is up to date.",
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w500,
                              color: c.info,
                              height: 1.45,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),

            if (onDone != null)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                child: SizedBox(
                  height: 52,
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: onDone,
                    style: FilledButton.styleFrom(
                      textStyle: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    child: const Text('Continue'),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _ProgressRow extends StatelessWidget {
  const _ProgressRow({required this.state, required this.text});
  final ReconnectStepState state;
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    final (IconData icon, Color fg) = switch (state) {
      ReconnectStepState.done => (Icons.check_circle_rounded, c.success),
      ReconnectStepState.active => (Icons.sync_rounded, c.info),
      ReconnectStepState.pending =>
        (Icons.radio_button_unchecked, c.textDisabled),
    };

    return Row(
      children: [
        Icon(icon, size: 22, color: fg),
        const SizedBox(width: 14),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 15,
              fontWeight: state == ReconnectStepState.pending
                  ? FontWeight.w500
                  : FontWeight.w600,
              color: state == ReconnectStepState.pending
                  ? c.textMuted
                  : c.text,
            ),
          ),
        ),
      ],
    );
  }
}