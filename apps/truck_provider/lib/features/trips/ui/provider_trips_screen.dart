import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
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
      body: SafeArea(
        bottom: false,
        child: AnimatedBuilder(
          animation: widget.controller,
          builder: (context, _) {
            final c = widget.controller;
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Padding(
                  padding: EdgeInsets.fromLTRB(20, 24, 20, 0),
                  child: KHeading(
                    eyebrow: 'Your journeys',
                    title: 'My Trips',
                    subtitle: 'Every load you have carried, in one place.',
                  ),
                ),
                const SizedBox(height: 18),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  child: Container(
                    height: 44,
                    padding: const EdgeInsets.all(4),
                    decoration: BoxDecoration(
                      color: kProviderGreenTint,
                      borderRadius: BorderRadius.circular(14),
                    ),
                    child: TabBar(
                      controller: _tabController,
                      indicatorSize: TabBarIndicatorSize.tab,
                      indicator: BoxDecoration(
                        color: kCard,
                        borderRadius: BorderRadius.circular(11),
                        boxShadow: const [BoxShadow(color: Color(0x140B1F44), blurRadius: 6, offset: Offset(0, 2))],
                      ),
                      labelColor: kProviderText,
                      unselectedLabelColor: kProviderMuted,
                      labelStyle: const TextStyle(fontFamily: 'Inter', fontWeight: FontWeight.w700, fontSize: 13),
                      unselectedLabelStyle: const TextStyle(fontFamily: 'Inter', fontWeight: FontWeight.w500, fontSize: 13),
                      splashBorderRadius: BorderRadius.circular(11),
                      tabs: [
                        Tab(text: 'Completed${_count(c.completed.length)}'),
                        Tab(text: 'Ongoing${_count(c.ongoing.length)}'),
                        Tab(text: 'Cancelled${_count(c.cancelled.length)}'),
                      ],
                    ),
                  ),
                ),
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

  /// Tab labels stay bare when empty so the count never reads "(0)".
  String _count(int n) => n == 0 ? '' : ' · $n';
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
      return const Center(child: CircularProgressIndicator());
    }
    if (error != null && bookings.isEmpty) {
      return Center(
        child: KEmptyState(
          icon: Icons.cloud_off_rounded,
          title: 'Could not load trips.',
          message: error,
          actionLabel: 'Retry',
          onAction: onRefresh,
        ),
      );
    }
    if (bookings.isEmpty) {
      return RefreshIndicator(
        onRefresh: onRefresh,
        child: ListView(
          children: [
            const SizedBox(height: 40),
            KEmptyState(icon: Icons.local_shipping_outlined, title: emptyLabel),
          ],
        ),
      );
    }
    return RefreshIndicator(
      onRefresh: onRefresh,
      child: ListView.builder(
        padding: const EdgeInsets.fromLTRB(20, 18, 20, 24),
        itemCount: bookings.length,
        itemBuilder: (context, i) => TripListCard(
          booking: bookings[i],
          onTap: () => onTap(bookings[i]),
        ),
      ),
    );
  }
}
