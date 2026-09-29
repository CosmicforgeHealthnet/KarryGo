import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../models/dispute_models.dart';
import '../../state/rider_dispute_controller.dart';
import 'case_detail_screen.dart';
import 'create_case_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 03 — My Support Cases (list)
// Figma 08 — Empty state
// ═══════════════════════════════════════════════════════════════════════════

class CaseListScreen extends StatefulWidget {
  const CaseListScreen({super.key, required this.disputeController});

  final RiderDisputeController disputeController;

  @override
  State<CaseListScreen> createState() => _CaseListScreenState();
}

enum _CaseTab { all, open, resolved }

class _CaseListScreenState extends State<CaseListScreen> {
  _CaseTab _tab = _CaseTab.all;

  @override
  void initState() {
    super.initState();
    widget.disputeController.loadComplaints();
  }

  List<Complaint> _filter(List<Complaint> all) {
    switch (_tab) {
      case _CaseTab.all:
        return all;
      case _CaseTab.open:
        return all
            .where((c) =>
                c.status != 'resolved' && c.status != 'closed')
            .toList();
      case _CaseTab.resolved:
        return all
            .where((c) =>
                c.status == 'resolved' || c.status == 'closed')
            .toList();
    }
  }

  Future<void> _openCreate() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => CreateCaseScreen(
          disputeController: widget.disputeController,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) widget.disputeController.loadComplaints();
  }

  Future<void> _openDetail(Complaint c) async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => CaseDetailScreen(
          complaint: c,
          disputeController: widget.disputeController,
        ),
      ),
    );
    if (mounted) widget.disputeController.loadComplaints();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

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
                      'My support cases',
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

            // ── Tabs ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 12),
              child: Row(
                children: [
                  _TabPill(
                    label: 'All',
                    active: _tab == _CaseTab.all,
                    onTap: () => setState(() => _tab = _CaseTab.all),
                  ),
                  const SizedBox(width: 8),
                  _TabPill(
                    label: 'Open',
                    active: _tab == _CaseTab.open,
                    onTap: () => setState(() => _tab = _CaseTab.open),
                  ),
                  const SizedBox(width: 8),
                  _TabPill(
                    label: 'Resolved',
                    active: _tab == _CaseTab.resolved,
                    onTap: () => setState(() => _tab = _CaseTab.resolved),
                  ),
                ],
              ),
            ),

            // ── List ──
            Expanded(
              child: AnimatedBuilder(
                animation: widget.disputeController,
                builder: (context, _) {
                  final all = widget.disputeController.complaints;
                  final items = _filter(all);

                  if (widget.disputeController.loading && all.isEmpty) {
                    return Center(
                      child: CircularProgressIndicator(
                        color: c.brandTeal500,
                      ),
                    );
                  }

                  if (items.isEmpty) {
                    return _EmptyState(onCreate: _openCreate);
                  }

                  return RefreshIndicator(
                    color: c.brandTeal500,
                    onRefresh: widget.disputeController.loadComplaints,
                    child: ListView.separated(
                      physics: const AlwaysScrollableScrollPhysics(),
                      padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
                      itemCount: items.length,
                      separatorBuilder: (_, __) =>
                          const SizedBox(height: 12),
                      itemBuilder: (context, i) => _CaseCard(
                        complaint: items[i],
                        onTap: () => _openDetail(items[i]),
                      ),
                    ),
                  );
                },
              ),
            ),

            // ── Create new case ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _openCreate,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Create new case'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Tab pill
// ═══════════════════════════════════════════════════════════════════════════

class _TabPill extends StatelessWidget {
  const _TabPill({
    required this.label,
    required this.active,
    required this.onTap,
  });

  final String label;
  final bool active;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        decoration: BoxDecoration(
          color: active ? c.brandTeal500 : c.navy10,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: active ? c.brandWhite : c.textMuted,
          ),
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Case card
// ═══════════════════════════════════════════════════════════════════════════

class _CaseCard extends StatelessWidget {
  const _CaseCard({required this.complaint, required this.onTap});
  final Complaint complaint;
  final VoidCallback onTap;

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

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: c.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: c.divider),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    '#${complaint.id}',
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.1,
                    ),
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: bg,
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    label,
                    style: TextStyle(
                      fontSize: 11.5,
                      fontWeight: FontWeight.w700,
                      color: fg,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              complaint.subject,
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w600,
                color: c.text,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              _relativeTime(complaint.createdAt),
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
                color: c.textMuted,
              ),
            ),
          ],
        ),
      ),
    );
  }

  static String _relativeTime(DateTime t) {
    final now = DateTime.now();
    final diff = now.difference(t);
    if (diff.inMinutes < 60) return '${diff.inMinutes} mins ago';
    if (diff.inHours < 24) return '${diff.inHours} hours ago';
    if (diff.inDays == 1) return '1 day ago';
    return '${diff.inDays} days ago';
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Empty state — Figma 08
// ═══════════════════════════════════════════════════════════════════════════

class _EmptyState extends StatelessWidget {
  const _EmptyState({required this.onCreate});
  final VoidCallback onCreate;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 120,
              height: 120,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: c.navy10,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.description_outlined,
                size: 58,
                color: c.textMuted,
              ),
            ),
            const SizedBox(height: 28),
            Text(
              'No support cases yet',
              style: TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w800,
                color: c.text,
                letterSpacing: -0.4,
              ),
            ),
            const SizedBox(height: 10),
            Text(
              "Need help with something? Our support team is here for you.",
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w500,
                color: c.textMuted,
                height: 1.5,
              ),
            ),
            const SizedBox(height: 24),
            SizedBox(
              height: 48,
              width: double.infinity,
              child: FilledButton(
                onPressed: onCreate,
                style: FilledButton.styleFrom(
                  textStyle: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                child: const Text('Create your first case'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}