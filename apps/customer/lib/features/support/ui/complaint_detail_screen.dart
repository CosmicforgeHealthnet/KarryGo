import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/models/customer_auth_models.dart';
import '../data/support_api.dart';
import '../models/support_models.dart';

class ComplaintDetailScreen extends StatefulWidget {
  const ComplaintDetailScreen({
    super.key,
    required this.complaint,
    required this.session,
    required this.supportApi,
  });

  final Complaint complaint;
  final CustomerSession session;
  final SupportApi supportApi;

  @override
  State<ComplaintDetailScreen> createState() => _ComplaintDetailScreenState();
}

class _ComplaintDetailScreenState extends State<ComplaintDetailScreen> {
  late Complaint _complaint;

  @override
  void initState() {
    super.initState();
    _complaint = widget.complaint;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: CustomerFigmaColors.surface,
        elevation: 0,
        leading:
            FigmaBackButton(onPressed: () => Navigator.of(context).pop()),
        title: const Text(
          'Complaint',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            _StatusHeader(complaint: _complaint),
            const SizedBox(height: 20),
            _DetailSection(
              title: 'Subject',
              body: _complaint.subject,
            ),
            const SizedBox(height: 16),
            _DetailSection(
              title: 'Description',
              body: _complaint.description,
            ),
            if (_complaint.bookingReference != null) ...[
              const SizedBox(height: 16),
              _DetailSection(
                title: 'Booking Reference',
                body: _complaint.bookingReference!,
              ),
            ],
            const SizedBox(height: 16),
            _MetaRow(label: 'Service', value: _serviceLabel(_complaint.serviceType)),
            const SizedBox(height: 8),
            _MetaRow(
              label: 'Submitted',
              value: _formatDate(_complaint.createdAt),
            ),
            if (_complaint.resolvedAt != null) ...[
              const SizedBox(height: 8),
              _MetaRow(
                label: 'Resolved',
                value: _formatDate(_complaint.resolvedAt!),
              ),
            ],
            if (_complaint.resolutionNote != null) ...[
              const SizedBox(height: 20),
              _ResolutionNoteCard(note: _complaint.resolutionNote!),
            ],
          ],
        ),
      ),
    );
  }

  String _serviceLabel(String s) => switch (s) {
        'taxi' => 'Taxi ride',
        'dispatch' => 'Dispatch delivery',
        'hauling' => 'Truck haulage',
        _ => 'Platform',
      };

  String _formatDate(DateTime dt) {
    final months = [
      'Jan','Feb','Mar','Apr','May','Jun',
      'Jul','Aug','Sep','Oct','Nov','Dec',
    ];
    return '${months[dt.month - 1]} ${dt.day}, ${dt.year}';
  }
}

class _StatusHeader extends StatelessWidget {
  const _StatusHeader({required this.complaint});
  final Complaint complaint;

  @override
  Widget build(BuildContext context) {
    final color = _colorFor(complaint.status);
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: color.withValues(alpha: 0.2)),
      ),
      child: Row(
        children: [
          Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: color.withValues(alpha: 0.15),
              shape: BoxShape.circle,
            ),
            child: Icon(_iconFor(complaint.status), color: color, size: 22),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  complaint.statusLabel,
                  style: TextStyle(
                    color: color,
                    fontSize: 15,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  _subtitleFor(complaint.status),
                  style: const TextStyle(
                    color: CustomerFigmaColors.muted,
                    fontSize: 12,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Color _colorFor(String s) => switch (s) {
        'open' => const Color(0xFFF59E0B),
        'under_review' => const Color(0xFF2563EB),
        'resolved' || 'closed' => CustomerFigmaColors.primary,
        'escalated' => const Color(0xFFE53935),
        _ => CustomerFigmaColors.muted,
      };

  IconData _iconFor(String s) => switch (s) {
        'open' => Icons.pending_outlined,
        'under_review' => Icons.manage_search_rounded,
        'resolved' => Icons.check_circle_outline_rounded,
        'closed' => Icons.lock_outline_rounded,
        'escalated' => Icons.warning_amber_rounded,
        _ => Icons.help_outline_rounded,
      };

  String _subtitleFor(String s) => switch (s) {
        'open' => 'Your complaint has been received.',
        'under_review' => 'Our team is reviewing this.',
        'awaiting_evidence' => 'We need more information from you.',
        'resolved' => 'This complaint has been resolved.',
        'closed' => 'This complaint is now closed.',
        'escalated' => 'This has been escalated to a dispute.',
        _ => '',
      };
}

class _DetailSection extends StatelessWidget {
  const _DetailSection({required this.title, required this.body});
  final String title;
  final String body;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            color: CustomerFigmaColors.muted,
            fontSize: 12,
            fontWeight: FontWeight.w700,
          ),
        ),
        const SizedBox(height: 6),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(12),
          ),
          child: Text(
            body,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 14,
              height: 1.5,
            ),
          ),
        ),
      ],
    );
  }
}

class _MetaRow extends StatelessWidget {
  const _MetaRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        SizedBox(
          width: 80,
          child: Text(
            label,
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 12,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
        Expanded(
          child: Text(
            value,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 13,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
      ],
    );
  }
}

class _ResolutionNoteCard extends StatelessWidget {
  const _ResolutionNoteCard({required this.note});
  final String note;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.primaryTint,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: CustomerFigmaColors.primarySoft),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Row(
            children: [
              Icon(Icons.check_circle_rounded,
                  color: CustomerFigmaColors.primary, size: 16),
              SizedBox(width: 6),
              Text(
                'Resolution note',
                style: TextStyle(
                  color: CustomerFigmaColors.primary,
                  fontSize: 13,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            note,
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 13,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }
}
