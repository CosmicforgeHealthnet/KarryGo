import 'package:flutter/material.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_trips_controller.dart';
import 'provider_trip_detail_screen.dart';
import 'widgets/trip_list_card.dart';

/// "My Trips" — Completed / Ongoing / Cancelled tabs over the provider's
/// bookings (Figma screens 1–3). Embedded as a bottom-nav tab in the home shell.
class ProviderTripsScreen extends StatefulWidget {
  const ProviderTripsScreen({super.key, required this.controller});

  final ProviderTripsController controller;

  @override
  State<ProviderTripsScreen> createState() => _ProviderTripsScreenState();
}

class _ProviderTripsScreenState extends State<ProviderTripsScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 3, vsync: this);
    WidgetsBinding.instance.addPostFrameCallback((_) => widget.controller.load());
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  void _openDetail(ProviderBooking booking) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderTripDetailScreen(
          booking: booking,
          controller: widget.controller,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: kProviderSurface,
      body: SafeArea(
        bottom: false,
        child: AnimatedBuilder(
          animation: widget.controller,
          builder: (context, _) {
            final c = widget.controller;
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // ─── Header ─────────────────────────────────────────
                const Padding(
                  padding: EdgeInsets.fromLTRB(20, 16, 20, 0),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'My Trips',
                        style: TextStyle(
                          color: kProviderText,
                          fontSize: 22,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                      SizedBox(height: 4),
                      Text(
                        'Manage all your past, present and future rides in one place',
                        style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.4),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 16),

                // ─── Tabs ───────────────────────────────────────────
                TabBar(
                  controller: _tabController,
                  isScrollable: true,
                  tabAlignment: TabAlignment.start,
                  labelColor: kProviderGreen,
                  unselectedLabelColor: kProviderText,
                  labelStyle: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14),
                  unselectedLabelStyle: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
                  indicatorColor: kProviderGreen,
                  indicatorSize: TabBarIndicatorSize.label,
                  dividerColor: kProviderBorder,
                  tabs: const [
                    Tab(text: 'Completed'),
                    Tab(text: 'Ongoing'),
                    Tab(text: 'Cancelled'),
                  ],
                ),

                // ─── Tab content ────────────────────────────────────
                Expanded(
                  child: TabBarView(
                    controller: _tabController,
                    children: [
                      _TripList(
                        bookings: c.completed,
                        isLoading: c.isLoading,
                        error: c.error,
                        emptyLabel: 'No completed trips yet.',
                        onRefresh: c.load,
                        onTap: _openDetail,
                      ),
                      _TripList(
                        bookings: c.ongoing,
                        isLoading: c.isLoading,
                        error: c.error,
                        emptyLabel: 'No ongoing trips.',
                        onRefresh: c.load,
                        onTap: _openDetail,
                      ),
                      _TripList(
                        bookings: c.cancelled,
                        isLoading: c.isLoading,
                        error: c.error,
                        emptyLabel: 'No cancelled trips.',
                        onRefresh: c.load,
                        onTap: _openDetail,
                      ),
                    ],
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
  }
}

class _TripList extends StatelessWidget {
  const _TripList({
    required this.bookings,
    required this.isLoading,
    required this.error,
    required this.emptyLabel,
    required this.onRefresh,
    required this.onTap,
  });

  final List<ProviderBooking> bookings;
  final bool isLoading;
  final String? error;
  final String emptyLabel;
  final Future<void> Function() onRefresh;
  final void Function(ProviderBooking) onTap;

  @override
  Widget build(BuildContext context) {
    if (isLoading && bookings.isEmpty) {
      return const Center(
        child: CircularProgressIndicator(color: kProviderGreen),
      );
    }
    if (error != null && bookings.isEmpty) {
      return _Centered(
        icon: Icons.error_outline_rounded,
        label: 'Could not load trips.',
        action: TextButton(onPressed: onRefresh, child: const Text('Retry')),
      );
    }
    if (bookings.isEmpty) {
      return RefreshIndicator(
        color: kProviderGreen,
        onRefresh: onRefresh,
        child: ListView(
          children: [
            const SizedBox(height: 120),
            _Centered(icon: Icons.local_shipping_outlined, label: emptyLabel),
          ],
        ),
      );
    }
    return RefreshIndicator(
      color: kProviderGreen,
      onRefresh: onRefresh,
      child: ListView.builder(
        padding: const EdgeInsets.fromLTRB(20, 20, 20, 24),
        itemCount: bookings.length,
        itemBuilder: (context, i) => TripListCard(
          booking: bookings[i],
          onTap: () => onTap(bookings[i]),
        ),
      ),
    );
  }
}

class _Centered extends StatelessWidget {
  const _Centered({required this.icon, required this.label, this.action});

  final IconData icon;
  final String label;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 48, color: kProviderMuted),
          const SizedBox(height: 12),
          Text(label, style: const TextStyle(color: kProviderMuted, fontSize: 14)),
          if (action != null) ...[const SizedBox(height: 8), action!],
        ],
      ),
    );
  }
}
