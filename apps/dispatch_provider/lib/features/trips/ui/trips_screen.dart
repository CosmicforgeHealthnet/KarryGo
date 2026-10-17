import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../home/ui/widgets/route_info.dart';
import '../data/trip_model.dart';
import '../state/trips_controller.dart';
import 'trip_detail_screen.dart';
import 'activity_filters_sheet.dart';

class TripsScreen extends StatefulWidget {
  const TripsScreen({super.key, required this.tripsController});
  final TripsController tripsController;
  @override
  State<TripsScreen> createState() => _TripsScreenState();
}

class _TripsScreenState extends State<TripsScreen> {
  ActivityTab _tab = ActivityTab.all;
  String _dateRange = 'All time';
  TripsController get _ctrl => widget.tripsController;
  @override
  void initState() {
    super.initState();
    _ctrl.addListener(_changed);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _ctrl.loadTrips(applyFilters: true);
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

  void _reload() {
    final now = DateTime.now();
    final from = switch (_dateRange) {
      'Today' => DateTime(now.year, now.month, now.day),
      'Last 7 days' => now.subtract(const Duration(days: 7)),
      'Last 30 days' => now.subtract(const Duration(days: 30)),
      'This year' => DateTime(now.year),
      _ => null,
    };
    _ctrl.loadTrips(
      applyFilters: true,
      status: _tab == ActivityTab.all ? null : _tab.name,
      createdFrom: from,
    );
  }

  Future<void> _filters() async {
    final result = await showActivityFiltersSheet(
      context,
      initial: ActivityFilterResult(tab: _tab, dateRange: _dateRange),
    );
    if (result == null || !mounted) return;
    setState(() {
      _tab = result.tab;
      _dateRange = result.dateRange;
    });
    _reload();
  }

  void _reset() {
    setState(() {
      _tab = ActivityTab.all;
      _dateRange = 'All time';
    });
    _reload();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: context.palette.background,
    body: SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          DeliveryCollectionHeader(
            title: 'Activity',
            subtitle: 'Your deliveries, clearly recorded.',
            action: IconButton(
              tooltip: 'Filter activity',
              onPressed: _filters,
              icon: const Icon(Icons.tune_rounded),
            ),
          ),
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Row(
              children: [
                for (final entry in const {
                  ActivityTab.all: 'All',
                  ActivityTab.active: 'Active',
                  ActivityTab.completed: 'Completed',
                  ActivityTab.cancelled: 'Cancelled',
                  ActivityTab.failed: 'Failed',
                }.entries)
                  Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: DeliveryFilter(
                      label: entry.value,
                      selected: _tab == entry.key,
                      onTap: () {
                        setState(() => _tab = entry.key);
                        _reload();
                      },
                    ),
                  ),
              ],
            ),
          ),
          if (_dateRange != 'All time')
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
              child: Row(
                children: [
                  const Icon(Icons.calendar_today_outlined, size: 15),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      _dateRange,
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ),
                  TextButton(
                    onPressed: _reset,
                    child: const Text('Reset filters'),
                  ),
                ],
              ),
            ),
          const SizedBox(height: 12),
          if (_ctrl.error != null && _ctrl.trips.isNotEmpty)
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
              child: ActionNotice(
                message: _ctrl.error!,
                isError: true,
                onRetry: _ctrl.loadTrips,
              ),
            ),
          Expanded(
            child: RefreshIndicator(
              onRefresh: () async {
                await _ctrl.loadTrips();
              },
              child: _body(context),
            ),
          ),
        ],
      ),
    ),
  );
  Widget _body(BuildContext context) {
    if (_ctrl.isLoading && _ctrl.trips.isEmpty)
      return const ContentSkeleton(rows: 3);
    if (_ctrl.error != null && _ctrl.trips.isEmpty)
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.all(24),
        children: [
          const SizedBox(height: 40),
          const JourneyIcon(JourneySymbol.review, size: 64),
          const SizedBox(height: 24),
          ActionNotice(
            message: _ctrl.error!,
            isError: true,
            onRetry: _ctrl.loadTrips,
          ),
        ],
      );
    final items = _ctrl.trips;
    if (items.isEmpty) {
      final filtered = _tab != ActivityTab.all || _dateRange != 'All time';
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        children: [
          DispatchEmptyState(
            story: filtered ? EmptyStory.search : EmptyStory.activity,
            title: filtered ? 'No matching deliveries' : 'No deliveries yet',
            message: filtered
                ? 'Try another status or date range.'
                : 'Your delivery records will appear here.',
            actionLabel: filtered ? 'Reset filters' : 'Refresh',
            onAction: filtered ? _reset : () => _ctrl.loadTrips(),
          ),
        ],
      );
    }
    return ListView.builder(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
      itemCount: items.length + 1,
      itemBuilder: (context, i) {
        if (i == items.length)
          return _ctrl.hasMore
              ? Padding(
                  padding: const EdgeInsets.symmetric(vertical: 16),
                  child: OutlinedButton.icon(
                    onPressed: _ctrl.loadingMore ? null : _ctrl.loadMore,
                    icon: _ctrl.loadingMore
                        ? const SizedBox.square(
                            dimension: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.expand_more_rounded),
                    label: Text(
                      _ctrl.loadingMore
                          ? 'Loading...'
                          : 'Load earlier deliveries',
                    ),
                  ),
                )
              : const SizedBox(height: 8);
        final trip = items[i];
        final heading = _group(trip.createdAt);
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (i == 0 || _group(items[i - 1].createdAt) != heading)
              Padding(
                padding: const EdgeInsets.fromLTRB(4, 12, 4, 12),
                child: Text(
                  heading,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
              ),
            Padding(
              padding: const EdgeInsets.only(bottom: 14),
              child: _ActivityCard(
                trip: trip,
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) =>
                        TripDetailScreen(trip: trip, controller: _ctrl),
                  ),
                ),
              ),
            ),
          ],
        );
      },
    );
  }

  String _group(DateTime date) {
    final d = date.toLocal(), now = DateTime.now();
    final days = DateTime(
      now.year,
      now.month,
      now.day,
    ).difference(DateTime(d.year, d.month, d.day)).inDays;
    return days == 0
        ? 'Today'
        : days == 1
        ? 'Yesterday'
        : deliveryDate(d);
  }
}

class _ActivityCard extends StatelessWidget {
  const _ActivityCard({required this.trip, required this.onTap});
  final TripModel trip;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return DeliveryPanel(
      onTap: onTap,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                width: 42,
                height: 42,
                decoration: BoxDecoration(
                  color: c.surfaceElevated,
                  borderRadius: BorderRadius.circular(14),
                ),
                child: Center(
                  child: JourneyIcon(
                    trip.statusCode.isCompleted
                        ? JourneySymbol.shield
                        : JourneySymbol.parcel,
                    size: 26,
                    color: trip.statusCode.isCompleted
                        ? c.success
                        : trip.statusCode.isCancelled ||
                              trip.statusCode.isFailed
                        ? c.danger
                        : c.brandTeal600,
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  trip.customerName.isNotEmpty ? trip.customerName : 'Delivery',
                  style: Theme.of(context).textTheme.titleSmall,
                ),
              ),
              const Icon(Icons.chevron_right_rounded),
            ],
          ),
          const SizedBox(height: 14),
          Wrap(
            spacing: 12,
            runSpacing: 8,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              TripStatusBadge(status: trip.statusCode),
              if (trip.moneyVisible)
                Text(
                  trip.fareDisplay,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
            ],
          ),
          if (trip.pickupAddress.isNotEmpty &&
              trip.dropoffAddress.isNotEmpty) ...[
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 16),
              child: Divider(height: 1),
            ),
            RouteInfo(
              pickup: trip.pickupAddress,
              dropoff: trip.dropoffAddress,
              compact: true,
            ),
          ],
          const SizedBox(height: 14),
          Wrap(
            spacing: 12,
            runSpacing: 6,
            children: [
              Text(
                deliveryDate(trip.createdAt, time: true),
                style: Theme.of(context).textTheme.bodySmall,
              ),
              if (trip.distanceKm > 0)
                Text(
                  trip.distanceDisplay,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
            ],
          ),
        ],
      ),
    );
  }
}
