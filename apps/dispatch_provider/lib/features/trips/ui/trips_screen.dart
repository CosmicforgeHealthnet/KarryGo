import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
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

  @override
  void initState() {
    super.initState();
    widget.tripsController.addListener(_onChanged);
    widget.tripsController.loadTrips();
  }

  @override
  void dispose() {
    widget.tripsController.removeListener(_onChanged);
    super.dispose();
  }

  void _onChanged() {
    if (mounted) setState(() {});
  }

  List<TripModel> get _visible {
    final all = widget.tripsController.trips;
    switch (_tab) {
      case ActivityTab.all:
        return all;
      case ActivityTab.active:
        return all.where((t) => t.statusCode.isActive).toList();
      case ActivityTab.completed:
        return all.where((t) => t.statusCode.isCompleted).toList();
      case ActivityTab.cancelled:
        return all.where((t) => t.statusCode.isCancelled).toList();
      case ActivityTab.failed:
        return all.where((t) => t.statusCode.isFailed).toList();
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final ctrl = widget.tripsController;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildHeader(context),
            _buildTabs(context),
            const SizedBox(height: 12),
            Expanded(
              child: ctrl.isLoading && ctrl.trips.isEmpty
                  ? const Center(child: CircularProgressIndicator())
                  : ctrl.error != null && ctrl.trips.isEmpty
                      ? _buildError(context)
                      : _buildList(context),
            ),
          ],
        ),
      ),
    );
  }

  // ── Header ────────────────────────────────────────────────────────

  Widget _buildHeader(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 14),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Activity',
                  style: TextStyle(
                    fontSize: 22,
                    fontWeight: FontWeight.w800,
                    color: colors.text,
                    letterSpacing: -0.6,
                    height: 1.15,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  'Your work history, proof and performance in one place.',
                  style: TextStyle(
                    fontSize: 13,
                    color: colors.textMuted,
                    height: 1.4,
                  ),
                ),
              ],
            ),
          ),
          GestureDetector(
            onTap: () => _openFilters(context),
            behavior: HitTestBehavior.opaque,
            child: Container(
              width: 40,
              height: 40,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: colors.surface,
                shape: BoxShape.circle,
                border: Border.all(color: colors.divider),
              ),
              child: Icon(
                Icons.tune_rounded,
                size: 20,
                color: colors.text,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _openFilters(BuildContext context) async {
    final result = await showActivityFiltersSheet(context);
    if (result == null || !mounted) return;
    setState(() => _tab = result.tab);
  }

  // ── Tabs ──────────────────────────────────────────────────────────

  Widget _buildTabs(BuildContext context) {
    const labels = {
      ActivityTab.all: 'All',
      ActivityTab.active: 'Active',
      ActivityTab.completed: 'Completed',
      ActivityTab.cancelled: 'Cancelled',
      ActivityTab.failed: 'Failed',
    };

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Row(
        children: [
          for (final entry in labels.entries) ...[
            _TabPill(
              label: entry.value,
              active: _tab == entry.key,
              onTap: () => setState(() => _tab = entry.key),
            ),
            if (entry.key != ActivityTab.failed) const SizedBox(width: 8),
          ],
        ],
      ),
    );
  }

  // ── List ──────────────────────────────────────────────────────────

  Widget _buildList(BuildContext context) {
    final items = _visible;
    if (items.isEmpty) return _buildEmpty(context);

    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 100),
      itemCount: items.length,
      separatorBuilder: (_, __) => const SizedBox(height: 12),
      itemBuilder: (context, i) => _ActivityCard(
        trip: items[i],
        onTap: () => Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) => TripDetailScreen(trip: items[i]),
          ),
        ),
      ),
    );
  }

  // ── Empty state ───────────────────────────────────────────────────

  Widget _buildEmpty(BuildContext context) {
    final colors = context.palette;
    final (String title, String body) = switch (_tab) {
      ActivityTab.all => (
          'No activity yet',
          'Your completed deliveries, cancellations and other activity will appear here.',
        ),
      ActivityTab.active => (
          'No active deliveries',
          'Accept an opportunity to see it here in real time.',
        ),
      ActivityTab.completed => (
          'No completed deliveries yet',
          'Finished deliveries will show up here.',
        ),
      ActivityTab.cancelled => (
          'No cancelled deliveries',
          'Cancelled requests will show up here.',
        ),
      ActivityTab.failed => (
          'No failed deliveries',
          'Failed attempts will show up here.',
        ),
    };

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
                Icons.assignment_outlined,
                size: 40,
                color: colors.navy40,
              ),
            ),
            const SizedBox(height: 18),
            Text(
              title,
              style: TextStyle(
                fontSize: 17,
                fontWeight: FontWeight.w700,
                color: colors.text,
              ),
            ),
            const SizedBox(height: 6),
            Text(
              body,
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 14,
                color: colors.textMuted,
                height: 1.5,
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Error state ───────────────────────────────────────────────────

  Widget _buildError(BuildContext context) {
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
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: colors.danger,
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.priority_high_rounded,
                size: 40,
                color: Colors.white,
              ),
            ),
            const SizedBox(height: 20),
            Text(
              'Something went wrong',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w800,
                color: colors.text,
                letterSpacing: -0.3,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              "We couldn't load your activity at the moment. Please check your connection and try again.",
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 13.5,
                color: colors.textMuted,
                height: 1.5,
              ),
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: widget.tripsController.loadTrips,
                style: FilledButton.styleFrom(
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(14),
                  ),
                  textStyle: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                child: const Text('Try again'),
              ),
            ),
            const SizedBox(height: 8),
            SizedBox(
              width: double.infinity,
              height: 48,
              child: OutlinedButton(
                onPressed: () {},
                style: OutlinedButton.styleFrom(
                  foregroundColor: colors.text,
                  side: BorderSide(color: colors.divider, width: 1.2),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(14),
                  ),
                  textStyle: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                child: const Text('Go offline'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Tab pill ────────────────────────────────────────────────────────

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

// ── Activity card ───────────────────────────────────────────────────

class _ActivityCard extends StatelessWidget {
  const _ActivityCard({required this.trip, required this.onTap});
  final TripModel trip;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final isActive = trip.statusCode.isActive;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: colors.divider),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(
                  '#${trip.bookingId}',
                  style: TextStyle(
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                    color: colors.textMuted,
                    letterSpacing: 0.1,
                  ),
                ),
                const Spacer(),
                _StatusChip(status: trip.statusCode),
              ],
            ),

            const SizedBox(height: 12),

            _RouteLine(
              icon: Icons.radio_button_checked,
              color: colors.brandNavy900,
              label: trip.pickupAddress,
            ),
            const SizedBox(height: 6),
            _RouteLine(
              icon: Icons.place_outlined,
              color: colors.brandTeal500,
              label: trip.dropoffAddress,
            ),

            const SizedBox(height: 14),

            if (isActive)
              Row(
                children: [
                  _Metric(
                    icon: Icons.schedule_rounded,
                    label: trip.etaDisplay,
                  ),
                  const SizedBox(width: 14),
                  _Metric(
                    icon: Icons.straighten_rounded,
                    label: trip.distanceDisplay,
                  ),
                  const Spacer(),
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 10,
                      vertical: 5,
                    ),
                    decoration: BoxDecoration(
                      color: colors.brandTeal500,
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: const Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          Icons.my_location_rounded,
                          size: 11,
                          color: Colors.white,
                        ),
                        SizedBox(width: 4),
                        Text(
                          'Live tracking',
                          style: TextStyle(
                            fontSize: 11,
                            fontWeight: FontWeight.w700,
                            color: Colors.white,
                          ),
                        ),
                      ],
                    ),
                  ),
                ],
              )
            else
              Row(
                children: [
                  _Metric(
                    icon: Icons.attach_money_rounded,
                    label: trip.earningsDisplay,
                  ),
                  const Spacer(),
                  Text(
                    _formatShortDate(
                      trip.completedAt ??
                          trip.cancelledAt ??
                          trip.failedAt ??
                          trip.createdAt,
                    ),
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
          ],
        ),
      ),
    );
  }

  static String _formatShortDate(DateTime dt) {
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    return '${dt.day} ${months[dt.month - 1]} ${dt.year}';
  }
}

// ── Status chip ─────────────────────────────────────────────────────

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.status});
  final TripStatusCode status;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    final (Color fg, Color bg) = switch (status) {
      TripStatusCode.completed => (colors.success, colors.successSoft),
      TripStatusCode.inProgress => (colors.brandTeal600, colors.teal10),
      TripStatusCode.enRoutePickup => (colors.brandTeal600, colors.teal10),
      TripStatusCode.assigned => (colors.brandNavy900, colors.navy10),
      TripStatusCode.arrivedPickup => (colors.warning, colors.warningSoft),
      TripStatusCode.proofSubmitted => (colors.brandTeal600, colors.teal10),
      TripStatusCode.cancelled => (colors.danger, colors.dangerSoft),
      TripStatusCode.failed => (colors.danger, colors.dangerSoft),
    };

    final icon = switch (status) {
      TripStatusCode.completed => Icons.check_circle_rounded,
      TripStatusCode.inProgress => Icons.local_shipping_rounded,
      TripStatusCode.enRoutePickup => Icons.navigation_rounded,
      TripStatusCode.assigned => Icons.assignment_turned_in_outlined,
      TripStatusCode.arrivedPickup => Icons.pin_drop_outlined,
      TripStatusCode.proofSubmitted => Icons.photo_camera_outlined,
      TripStatusCode.cancelled => Icons.cancel_outlined,
      TripStatusCode.failed => Icons.error_outline_rounded,
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 12, color: fg),
          const SizedBox(width: 4),
          Text(
            status.shortLabel,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: fg,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Small helpers ───────────────────────────────────────────────────

class _RouteLine extends StatelessWidget {
  const _RouteLine({
    required this.icon,
    required this.color,
    required this.label,
  });

  final IconData icon;
  final Color color;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Icon(icon, size: 14, color: color),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: colors.text,
              height: 1.3,
            ),
          ),
        ),
      ],
    );
  }
}

class _Metric extends StatelessWidget {
  const _Metric({required this.icon, required this.label});
  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 14, color: colors.textMuted),
        const SizedBox(width: 4),
        Text(
          label,
          style: TextStyle(
            fontSize: 12.5,
            fontWeight: FontWeight.w600,
            color: colors.textMuted,
          ),
        ),
      ],
    );
  }
}