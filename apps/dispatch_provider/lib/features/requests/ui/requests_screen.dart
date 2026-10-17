import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../data/request_model.dart';
import '../state/requests_controller.dart';
import 'widgets/opportunity_card.dart';
import 'request_detail_screen.dart';
import 'accept_confirmation_screen.dart';
import 'decline_request_screen.dart';

class RequestsScreen extends StatefulWidget {
  const RequestsScreen({super.key, required this.requestsController});

  final RequestsController requestsController;

  @override
  State<RequestsScreen> createState() => _RequestsScreenState();
}

enum _OpportunityTab { all, nearby, assignments, commerce }

class _RequestsScreenState extends State<RequestsScreen> {
  _OpportunityTab _tab = _OpportunityTab.all;

  RequestsController get _ctrl => widget.requestsController;

  @override
  void initState() {
    super.initState();
    _ctrl.addListener(_onControllerChanged);
    _load();
  }

  @override
  void dispose() {
    _ctrl.removeListener(_onControllerChanged);
    super.dispose();
  }

  void _onControllerChanged() => setState(() {});

  Future<void> _load() => _ctrl.loadRequests();

  List<RequestModel> get _visible {
    final all = _ctrl.requests;
    switch (_tab) {
      case _OpportunityTab.all:
        return all;
      case _OpportunityTab.nearby:
        return all.where((r) => r.hasDistance && r.distanceKm <= 5.0).toList();
      case _OpportunityTab.assignments:
        return all
            .where((r) => r.source == RequestSource.assignment)
            .toList();
      case _OpportunityTab.commerce:
        return all.where((r) => r.source == RequestSource.commerce).toList();
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildHeader(context),
            const SizedBox(height: 4),
            _buildTabs(context),
            const SizedBox(height: 12),
            Expanded(child: _buildBody(context)),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Text(
                'Opportunities',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: colors.text,
                  letterSpacing: -0.6,
                  height: 1.15,
                ),
              ),
              const Spacer(),
              if (_ctrl.isLoading)
                SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: colors.brandTeal500,
                  ),
                ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            'New dispatch requests near you',
            style: TextStyle(
              fontSize: 13.5,
              color: colors.textMuted,
              height: 1.4,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildTabs(BuildContext context) {
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Row(
        children: [
          _TabChip(
            label: 'All',
            active: _tab == _OpportunityTab.all,
            onTap: () => setState(() => _tab = _OpportunityTab.all),
          ),
          const SizedBox(width: 8),
          _TabChip(
            label: 'Nearby',
            active: _tab == _OpportunityTab.nearby,
            onTap: () => setState(() => _tab = _OpportunityTab.nearby),
          ),
          const SizedBox(width: 8),
          _TabChip(
            label: 'Assignments',
            active: _tab == _OpportunityTab.assignments,
            onTap: () => setState(() => _tab = _OpportunityTab.assignments),
          ),
          const SizedBox(width: 8),
          _TabChip(
            label: 'Commerce',
            active: _tab == _OpportunityTab.commerce,
            onTap: () => setState(() => _tab = _OpportunityTab.commerce),
          ),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context) {
    final colors = context.palette;

    if (_ctrl.error != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                _ctrl.error!,
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 14, color: colors.textMuted),
              ),
              const SizedBox(height: 16),
              FilledButton(
                onPressed: () {
                  _ctrl.clearError();
                  _load();
                },
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      );
    }

    final items = _visible;
    if (items.isEmpty) {
      return _buildEmptyState(context);
    }

    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 100),
      itemCount: items.length,
      separatorBuilder: (_, __) => const SizedBox(height: 12),
      itemBuilder: (context, i) {
        final req = items[i];
        return OpportunityCard(
          request: req,
          onTap: () => _openDetail(req),
          onAccept: () => _onAccept(req),
          onDecline: () => _onReject(req),
        );
      },
    );
  }

  Widget _buildEmptyState(BuildContext context) {
    final colors = context.palette;
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 88,
              height: 88,
              decoration: BoxDecoration(
                color: colors.navy10,
                shape: BoxShape.circle,
              ),
              child: Icon(
                Icons.location_off_outlined,
                size: 40,
                color: colors.navy40,
              ),
            ),
            const SizedBox(height: 18),
            Text(
              'No opportunities nearby',
              style: TextStyle(
                fontSize: 17,
                fontWeight: FontWeight.w700,
                color: colors.text,
              ),
            ),
            const SizedBox(height: 6),
            Text(
              'New requests will appear here when available.',
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 14, color: colors.textMuted),
            ),
            const SizedBox(height: 26),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton.icon(
                onPressed: _ctrl.isLoading ? null : _load,
                icon: const Icon(Icons.refresh, size: 18),
                label: const Text(
                  'Refresh opportunities',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                style: FilledButton.styleFrom(
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(14),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  void _openDetail(RequestModel req) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => RequestDetailScreen(
          request: req,
          onAccept: () => _onAccept(req),
          onDecline: () => _onReject(req),
          onBrowseNew: () => Navigator.of(context).pop(),
          onBackToOpportunities: () => Navigator.of(context).pop(),
        ),
      ),
    );
  }

  Future<void> _onAccept(RequestModel req) async {
   final result = await _ctrl.accept(req);   
    if (!mounted) return;
    result.when(
      success: (_) {
        Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) => AcceptConfirmationScreen(
              request: req,
              onViewTrips: () {
                Navigator.of(context).popUntil((r) => r.isFirst);
              },
              onBrowseMore: () => Navigator.of(context).pop(),
            ),
          ),
        );
      },
      failure: (err) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              err.message.isNotEmpty
                  ? err.message
                  : 'Failed to accept request.',
            ),
            backgroundColor: context.palette.danger,
          ),
        );
      },
    );
  }

  Future<void> _onReject(RequestModel req) async {
    final picked = await pushDeclineRequestScreen(context);
    if (picked == null || !mounted) return;

    // Backend whitelist is strictly {too_far, busy, other}. The picker's
    // free-text comment is intentionally NOT sent — the reject endpoint has
    // no field for it.
    final result = await _ctrl.reject(req, picked.reason.backendCode); 
    if (!mounted) return;
    result.when(
      success: (_) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Request declined.')),
        );
      },
      failure: (err) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              err.message.isNotEmpty
                  ? err.message
                  : 'Failed to decline request.',
            ),
            backgroundColor: context.palette.danger,
          ),
        );
      },
    );
  }
}

class _TabChip extends StatelessWidget {
  const _TabChip({
    required this.label,
    required this.active,
    required this.onTap,
  });

  final String label;
  final bool active;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 9),
        decoration: BoxDecoration(
          color: active ? colors.teal10 : colors.surface,
          borderRadius: BorderRadius.circular(999),
          border: Border.all(
            color: active
                ? colors.brandTeal500.withValues(alpha: 0.5)
                : colors.divider,
            width: 1.2,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: active ? colors.brandTeal600 : colors.text,
          ),
        ),
      ),
    );
  }
}