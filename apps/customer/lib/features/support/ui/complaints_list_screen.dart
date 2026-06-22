import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/models/customer_auth_models.dart';
import '../data/support_api.dart';
import '../models/support_models.dart';
import 'new_complaint_screen.dart';
import 'complaint_detail_screen.dart';

class ComplaintsListScreen extends StatefulWidget {
  const ComplaintsListScreen({
    super.key,
    required this.session,
    required this.supportApi,
  });

  final CustomerSession session;
  final SupportApi supportApi;

  @override
  State<ComplaintsListScreen> createState() => _ComplaintsListScreenState();
}

class _ComplaintsListScreenState extends State<ComplaintsListScreen> {
  List<Complaint> _complaints = [];
  bool _loading = true;
  ApiException? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final result = await widget.supportApi.listMyComplaints(
        accessToken: widget.session.accessToken,
      );
      setState(() {
        _complaints = result;
        _loading = false;
      });
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _openNew() async {
    final created = await Navigator.of(context).push<Complaint>(
      MaterialPageRoute(
        builder: (_) => NewComplaintScreen(
          session: widget.session,
          supportApi: widget.supportApi,
        ),
      ),
    );
    if (created != null) {
      setState(() => _complaints.insert(0, created));
    }
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
          'Support',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            icon: const Icon(Icons.refresh_rounded,
                color: CustomerFigmaColors.primary),
            onPressed: _load,
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _openNew,
        backgroundColor: CustomerFigmaColors.primary,
        foregroundColor: Colors.white,
        icon: const Icon(Icons.add_rounded),
        label: const Text('New complaint',
            style: TextStyle(fontWeight: FontWeight.w700)),
      ),
      body: _loading
          ? const Center(
              child: CircularProgressIndicator(
                  color: CustomerFigmaColors.primary))
          : _error != null
              ? _ErrorView(error: _error!, onRetry: _load)
              : _complaints.isEmpty
                  ? const _EmptyView()
                  : RefreshIndicator(
                      color: CustomerFigmaColors.primary,
                      onRefresh: _load,
                      child: ListView.separated(
                        padding: const EdgeInsets.fromLTRB(16, 12, 16, 100),
                        itemCount: _complaints.length,
                        separatorBuilder: (_, __) => const SizedBox(height: 10),
                        itemBuilder: (context, index) {
                          final c = _complaints[index];
                          return _ComplaintTile(
                            complaint: c,
                            onTap: () async {
                              await Navigator.of(context).push(
                                MaterialPageRoute(
                                  builder: (_) => ComplaintDetailScreen(
                                    complaint: c,
                                    session: widget.session,
                                    supportApi: widget.supportApi,
                                  ),
                                ),
                              );
                              _load();
                            },
                          );
                        },
                      ),
                    ),
    );
  }
}

class _ComplaintTile extends StatelessWidget {
  const _ComplaintTile({required this.complaint, required this.onTap});

  final Complaint complaint;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(16),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(16),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              _StatusDot(status: complaint.status),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      complaint.subject,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        _ServiceChip(serviceType: complaint.serviceType),
                        const SizedBox(width: 8),
                        Text(
                          _formatDate(complaint.createdAt),
                          style: const TextStyle(
                            color: CustomerFigmaColors.muted,
                            fontSize: 11,
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              _StatusBadge(label: complaint.statusLabel, status: complaint.status),
            ],
          ),
        ),
      ),
    );
  }

  String _formatDate(DateTime dt) {
    final months = [
      'Jan','Feb','Mar','Apr','May','Jun',
      'Jul','Aug','Sep','Oct','Nov','Dec',
    ];
    return '${months[dt.month - 1]} ${dt.day}';
  }
}

class _StatusDot extends StatelessWidget {
  const _StatusDot({required this.status});
  final String status;

  @override
  Widget build(BuildContext context) {
    final color = _colorFor(status);
    return Container(
      width: 10,
      height: 10,
      decoration: BoxDecoration(shape: BoxShape.circle, color: color),
    );
  }

  Color _colorFor(String s) => switch (s) {
        'open' => const Color(0xFFF59E0B),
        'under_review' => const Color(0xFF2563EB),
        'resolved' || 'closed' => CustomerFigmaColors.primary,
        'escalated' => const Color(0xFFE53935),
        _ => CustomerFigmaColors.muted,
      };
}

class _StatusBadge extends StatelessWidget {
  const _StatusBadge({required this.label, required this.status});
  final String label;
  final String status;

  @override
  Widget build(BuildContext context) {
    final color = _colorFor(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w700,
        ),
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
}

class _ServiceChip extends StatelessWidget {
  const _ServiceChip({required this.serviceType});
  final String serviceType;

  @override
  Widget build(BuildContext context) {
    final label = switch (serviceType) {
      'taxi' => 'Taxi',
      'dispatch' => 'Dispatch',
      'hauling' => 'Hauling',
      _ => 'Platform',
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.primaryPale,
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        label,
        style: const TextStyle(
          color: CustomerFigmaColors.darkGreen,
          fontSize: 10,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

class _EmptyView extends StatelessWidget {
  const _EmptyView();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 80,
            height: 80,
            decoration: BoxDecoration(
              color: CustomerFigmaColors.primaryPale,
              shape: BoxShape.circle,
            ),
            child: const Icon(Icons.support_agent_rounded,
                size: 38, color: CustomerFigmaColors.primary),
          ),
          const SizedBox(height: 20),
          const Text(
            'No complaints yet',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 17,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          const Text(
            'Tap the button below to raise a support request.',
            textAlign: TextAlign.center,
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
          ),
        ],
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.error, required this.onRetry});
  final ApiException error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded,
                color: Color(0xFFE53935), size: 40),
            const SizedBox(height: 16),
            Text(error.message,
                textAlign: TextAlign.center,
                style: const TextStyle(
                    color: CustomerFigmaColors.text, fontSize: 14)),
            const SizedBox(height: 20),
            FigmaPrimaryButton(label: 'Try again', onPressed: onRetry),
          ],
        ),
      ),
    );
  }
}
