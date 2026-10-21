import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../core/network/provider_session.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../../../shared/widgets/async_content.dart';
import '../../home/ui/widgets/home_feedback.dart';
import '../data/request_model.dart';
import '../state/requests_controller.dart';
import 'widgets/opportunity_card.dart';
import 'request_detail_screen.dart';
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
  bool _working = false;
  RequestsController get _ctrl => widget.requestsController;
  @override
  void initState() {
    super.initState();
    _ctrl.addListener(_changed);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _load();
    });
  }

  @override
  void dispose() {
    _ctrl.removeListener(_changed);
    super.dispose();
  }

  void _changed() {
    if (mounted) setState(() {});
  }

  Future<void> _load() => _ctrl.loadRequests();
  List<RequestModel> get _visible => _ctrl.requests
      .where(
        (r) => switch (_tab) {
          _OpportunityTab.all => true,
          _OpportunityTab.nearby => r.hasDistance && r.distanceKm < 5,
          _OpportunityTab.assignments => r.source == RequestSource.assignment,
          _OpportunityTab.commerce => r.source == RequestSource.commerce,
        },
      )
      .toList();
  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: context.palette.background,
    body: SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          DeliveryCollectionHeader(
            title: 'Opportunities',
            subtitle: 'Choose your next delivery.',
            action: IconButton(
              tooltip: 'Refresh opportunities',
              onPressed: _ctrl.isLoading ? null : _load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ),
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Row(
              children: [
                for (final entry in const {
                  _OpportunityTab.all: 'All',
                  _OpportunityTab.nearby: 'Under 5 km',
                  _OpportunityTab.assignments: 'Assignments',
                  _OpportunityTab.commerce: 'Commerce',
                }.entries)
                  Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: DeliveryFilter(
                      label: entry.value,
                      selected: _tab == entry.key,
                      onTap: () => setState(() => _tab = entry.key),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(height: 14),
          if (_ctrl.error != null && _ctrl.requests.isNotEmpty)
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
              child: ActionNotice(
                message: _ctrl.error!,
                isError: true,
                onRetry: _load,
              ),
            ),
          Expanded(
            child: RefreshIndicator(onRefresh: _load, child: _body(context)),
          ),
        ],
      ),
    ),
  );
  Widget _body(BuildContext context) {
    if (_ctrl.isLoading && _ctrl.requests.isEmpty)
      return const ContentSkeleton(rows: 3);
    if (_ctrl.error != null && _ctrl.requests.isEmpty)
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.all(24),
        children: [
          const SizedBox(height: 40),
          const JourneyIcon(JourneySymbol.route, size: 64),
          const SizedBox(height: 24),
          ActionNotice(message: _ctrl.error!, isError: true, onRetry: _load),
        ],
      );
    final items = _visible;
    if (items.isEmpty)
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        children: [
          DispatchEmptyState(
            story: _tab == _OpportunityTab.all
                ? EmptyStory.opportunities
                : EmptyStory.search,
            title: _tab == _OpportunityTab.all
                ? 'No requests right now'
                : 'No matching opportunities',
            message: _tab == _OpportunityTab.all
                ? 'Available delivery requests will appear here.'
                : 'Try another category or refresh the list.',
            actionLabel: _tab == _OpportunityTab.all ? 'Refresh' : 'Show all',
            onAction: _tab == _OpportunityTab.all
                ? _load
                : () => setState(() => _tab = _OpportunityTab.all),
          ),
        ],
      );
    return ListView.separated(
      key: PageStorageKey(_tab),
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
      itemCount: items.length,
      separatorBuilder: (_, _) => const SizedBox(height: 14),
      itemBuilder: (context, i) => OpportunityCard(
        request: items[i],
        busy: _working,
        actionsAvailable: _ctrl.canAcceptOffers,
        onTap: () => _detail(items[i]),
        onAccept: () => _accept(items[i]),
        onDecline: () => _decline(items[i]),
      ),
    );
  }

  void _detail(RequestModel request) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (screen) => RequestDetailScreen(
          request: request,
          controller: _ctrl,
          onAccept: () => _accept(request, detail: screen),
          onDecline: () => _decline(request, detail: screen),
          onBrowseNew: () => Navigator.pop(screen),
          onBackToOpportunities: () => Navigator.pop(screen),
        ),
      ),
    );
  }

  Future<void> _accept(RequestModel request, {BuildContext? detail}) async {
    if (_working) return;
    setState(() => _working = true);
    try {
      final current =
          _ctrl.requests
              .where((r) => r.id == request.id && r.kind == request.kind)
              .firstOrNull ??
          request;
      final result = await _ctrl.accept(current);
      if (!mounted) return;
      await result.when(
        success: (_) async {
          if (detail != null && detail.mounted) Navigator.pop(detail);
          final session = context
              .dependOnInheritedWidgetOfExactType<ProviderSession>();
          await showHomeFeedback(
            context,
            title: 'Delivery accepted',
            message:
                'The request has been assigned to you. Review its delivery instructions.',
            action: session?.openCurrentDelivery == null
                ? 'Done'
                : 'Open delivery',
          );
          if (mounted) session?.openCurrentDelivery?.call();
        },
        failure: (error) async {
          await showHomeFeedback(
            context,
            title: 'Could not accept request',
            message: error.message,
            error: true,
            action: 'Back to request',
          );
        },
      );
    } catch (_) {
      if (mounted)
        await showHomeFeedback(
          context,
          title: 'Could not confirm acceptance',
          message: 'Check the current request status before trying again.',
          error: true,
        );
    } finally {
      if (mounted) setState(() => _working = false);
    }
  }

  Future<void> _decline(RequestModel request, {BuildContext? detail}) async {
    if (_working) return;
    final reason = await pushDeclineRequestScreen(context);
    if (reason == null || !mounted) return;
    setState(() => _working = true);
    try {
      final result = await _ctrl.reject(request, reason.reason.backendCode);
      if (!mounted) return;
      await result.when(
        success: (_) async {
          if (detail != null && detail.mounted) Navigator.pop(detail);
          await showHomeFeedback(
            context,
            title: 'Request declined',
            message: 'You can review the next available opportunity.',
          );
        },
        failure: (error) async {
          await showHomeFeedback(
            context,
            title: 'Could not decline request',
            message: error.message,
            error: true,
          );
        },
      );
    } catch (_) {
      if (mounted)
        await showHomeFeedback(
          context,
          title: 'Could not confirm decline',
          message: 'Check your connection and refresh the request.',
          error: true,
        );
    } finally {
      if (mounted) setState(() => _working = false);
    }
  }
}
