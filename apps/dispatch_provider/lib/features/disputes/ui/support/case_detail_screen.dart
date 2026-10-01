import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../models/dispute_models.dart';
import '../../state/rider_dispute_controller.dart';
import 'create_case_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 06 — Case Detail / In progress (stage < 2)
// Figma 07 — Case Resolved (stage == 2)
// ═══════════════════════════════════════════════════════════════════════════

class CaseDetailScreen extends StatelessWidget {
  const CaseDetailScreen({
    super.key,
    required this.complaint,
    required this.disputeController,
  });

  final Complaint complaint;
  final RiderDisputeController disputeController;

  bool get _isResolved => complaint.stage >= 2;

  Future<void> _openNewCase(BuildContext context) async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => CreateCaseScreen(
          disputeController: disputeController,
        ),
      ),
    );
    if (!context.mounted) return;
    if (r == true) Navigator.of(context).pop();
  }

  void _reopen(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Case reopened')),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final status = complaint.status.toLowerCase();

    final (label, fg, bg) = switch (status) {
      'resolved' || 'closed' => ('Resolved', c.success, c.successSoft),
      'under_review' || 'awaiting_evidence' || 'escalated' => (
          'In progress',
          c.info,
          c.info.withValues(alpha: 0.12),
        ),
      _ => ('Open', c.warning, c.warningSoft),
    };

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
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
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Case details',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  // ── Status chip ──
                  Row(
                    children: [
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 5,
                        ),
                        decoration: BoxDecoration(
                          color: bg,
                          borderRadius: BorderRadius.circular(999),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              _isResolved
                                  ? Icons.check_circle_rounded
                                  : Icons.schedule_rounded,
                              size: 13,
                              color: fg,
                            ),
                            const SizedBox(width: 5),
                            Text(
                              label,
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w700,
                                color: fg,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 16),

                  // ── Case meta ──
                  Text(
                    '#${complaint.id}',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    complaint.subject,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: c.textMuted,
                    ),
                  ),
                  const SizedBox(height: 20),

                  Container(
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: c.surface,
                      borderRadius: BorderRadius.circular(14),
                      border: Border.all(color: c.divider),
                    ),
                    child: Column(
                      children: [
                        _MetaRow(
                          label: 'Created',
                          value: _fmt(complaint.createdAt),
                        ),
                        const SizedBox(height: 12),
                        _MetaRow(
                          label: 'Last update',
                          value: _fmt(
                            complaint.resolvedAt ?? complaint.createdAt,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 24),

                  // ── Body ──
                  if (_isResolved) ...[
                    Center(
                      child: Column(
                        children: [
                          Container(
                            width: 88,
                            height: 88,
                            alignment: Alignment.center,
                            decoration: BoxDecoration(
                              shape: BoxShape.circle,
                              color: c.successSoft,
                            ),
                            child: Icon(
                              Icons.check_rounded,
                              size: 52,
                              color: c.success,
                            ),
                          ),
                          const SizedBox(height: 20),
                          Text(
                            'This case has been resolved',
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              fontSize: 20,
                              fontWeight: FontWeight.w800,
                              color: c.text,
                              letterSpacing: -0.3,
                            ),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            "We've completed your request. If you still need help, you can reopen this case or create a new one.",
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w500,
                              color: c.textMuted,
                              height: 1.5,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ] else ...[
                    Text(
                      complaint.description,
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                        color: c.text,
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 24),
                    Text(
                      'Progress',
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                      ),
                    ),
                    const SizedBox(height: 16),
                    _TimelineStep(
                      label: 'Case submitted',
                      time: _fmt(complaint.createdAt),
                      done: complaint.stage >= 0,
                    ),
                    _TimelineStep(
                      label: 'Under review',
                      time: complaint.stage >= 1 ? _fmt(complaint.createdAt) : '',
                      done: complaint.stage >= 1,
                      active: complaint.stage == 1,
                    ),
                    _TimelineStep(
                      label: 'Awaiting information',
                      time: '',
                      done: complaint.stage >= 1,
                      active: false,
                    ),
                    _TimelineStep(
                      label: 'Resolved',
                      time: complaint.resolvedAt != null
                          ? _fmt(complaint.resolvedAt!)
                          : '',
                      done: complaint.stage >= 2,
                      isLast: true,
                    ),
                  ],
                ],
              ),
            ),

            // ── CTAs ──
            if (_isResolved) ...[
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 8),
                child: SizedBox(
                  height: 52,
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: () => _reopen(context),
                    style: FilledButton.styleFrom(
                      textStyle: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    child: const Text('Reopen case'),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
                child: SizedBox(
                  height: 52,
                  width: double.infinity,
                  child: OutlinedButton(
                    onPressed: () => _openNewCase(context),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: c.text,
                      side: BorderSide(color: c.border, width: 1.2),
                      shape: const StadiumBorder(),
                      textStyle: const TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    child: const Text('Create new case'),
                  ),
                ),
              ),
            ] else
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
                child: SizedBox(
                  height: 52,
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: () => Navigator.of(context).pop(),
                    style: FilledButton.styleFrom(
                      textStyle: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    child: const Text('Back to cases'),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }

  static String _fmt(DateTime t) {
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    final h12 = t.hour % 12 == 0 ? 12 : t.hour % 12;
    final m = t.minute.toString().padLeft(2, '0');
    final p = t.hour < 12 ? 'AM' : 'PM';
    return '${t.day} ${months[t.month - 1]} ${t.year} · $h12:$m $p';
  }
}

class _MetaRow extends StatelessWidget {
  const _MetaRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Expanded(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: c.textMuted,
            ),
          ),
        ),
        Text(
          value,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: c.text,
          ),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Timeline row
// ═══════════════════════════════════════════════════════════════════════════

class _TimelineStep extends StatelessWidget {
  const _TimelineStep({
    required this.label,
    required this.time,
    required this.done,
    this.active = false,
    this.isLast = false,
  });

  final String label;
  final String time;
  final bool done;
  final bool active;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final dotColor = done ? c.success : (active ? c.info : c.navy20);

    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 22,
            child: Column(
              children: [
                Container(
                  width: 16,
                  height: 16,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: done || active ? dotColor : Colors.transparent,
                    shape: BoxShape.circle,
                    border: done || active
                        ? null
                        : Border.all(color: c.navy20, width: 2),
                  ),
                  child: done
                      ? const Icon(
                          Icons.check_rounded,
                          size: 10,
                          color: Colors.white,
                        )
                      : active
                          ? const Icon(
                              Icons.sync_rounded,
                              size: 10,
                              color: Colors.white,
                            )
                          : null,
                ),
                if (!isLast)
                  Expanded(
                    child: Center(
                      child: Container(
                        width: 2,
                        color: done ? c.success : c.navy20,
                      ),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Padding(
              padding: EdgeInsets.only(bottom: isLast ? 0 : 18),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    label,
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                      color: done || active ? c.text : c.textMuted,
                    ),
                  ),
                  if (time.isNotEmpty) ...[
                    const SizedBox(height: 3),
                    Text(
                      time,
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}