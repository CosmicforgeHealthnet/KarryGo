import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../dispatch/models/dispatch_models.dart' as dispatch;
import '../../../dispatch/state/dispatch_booking_controller.dart';
import '../../../dispatch/ui/dispatch_flow_screen.dart';
import '../../../dispatch/ui/views/dispatch_trip_detail_screen.dart';
import '../../../dispatch/ui/widgets/dispatch_trip_widgets.dart';
import '../../../hauling/data/places_api.dart';
import '../../../hauling/models/hauling_models.dart';
import '../../../hauling/state/hauling_booking_controller.dart';
import '../../../hauling/ui/hauling_flow_screen.dart';
import '../../../hauling/ui/views/hauling_trip_detail_screen.dart';
import '../../../hauling/ui/widgets/hauling_trip_widgets.dart';

/// "My Trips" — lists the customer's truck bookings AND bike deliveries in a
/// single feed split across four tabs (Past / Ongoing / Upcoming / Cancelled).
/// Backed by both controllers' `loadHistory()` / `bookingHistory`.
class CustomerTripsTab extends StatefulWidget {
  const CustomerTripsTab({
    super.key,
    required this.controller,
    required this.dispatchController,
    required this.placesApi,
  });

  final HaulingBookingController controller;
  final DispatchBookingController dispatchController;
  final PlacesApi placesApi;

  @override
  State<CustomerTripsTab> createState() => _CustomerTripsTabState();
}

// ─── Merged trip entry (service discriminator) ────────────────────────────────

sealed class _TripEntry {
  DateTime get sortDate;
}

class _HaulingEntry extends _TripEntry {
  _HaulingEntry(this.booking);
  final HaulageBooking booking;
  @override
  DateTime get sortDate => booking.createdAt;
}

class _DispatchEntry extends _TripEntry {
  _DispatchEntry(this.booking);
  final dispatch.DispatchBooking booking;
  @override
  DateTime get sortDate => booking.createdAt;
}

class _CustomerTripsTabState extends State<CustomerTripsTab>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;
  bool _loadedOnce = false;

  // Lazily-loaded provider snapshots, keyed by provider id, so trip cards can
  // show the driver/rider name without an N+1 fetch blocking the first render.
  // Hauling and dispatch providers live in different services, so two caches.
  final Map<String, ProviderSnapshot> _providers = {};
  final Set<String> _providerInFlight = {};
  final Map<String, ProviderSnapshot> _riders = {};
  final Set<String> _riderInFlight = {};

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 4, vsync: this);
    WidgetsBinding.instance.addPostFrameCallback((_) => _load());
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    await Future.wait([
      widget.controller.loadHistory(),
      widget.dispatchController.loadHistory(),
    ]);
    if (!mounted) return;
    setState(() => _loadedOnce = true);
    _prefetchProviders();
    _prefetchRiders();
  }

  void _prefetchProviders() {
    final token = widget.controller.accessTokenForWallet;
    if (token == null) return;
    for (final b in widget.controller.state.bookingHistory) {
      final id = b.providerId;
      if (id == null || id.isEmpty) continue;
      if (_providers.containsKey(id) || _providerInFlight.contains(id)) continue;
      _providerInFlight.add(id);
      widget.controller.api
          .getProvider(accessToken: token, providerId: id)
          .then((p) {
        if (!mounted) return;
        setState(() {
          _providers[id] = p;
          _providerInFlight.remove(id);
        });
      }).catchError((_) {
        _providerInFlight.remove(id);
      });
    }
  }

  void _prefetchRiders() {
    final token = widget.dispatchController.accessTokenForWallet;
    if (token == null) return;
    for (final b in widget.dispatchController.state.bookingHistory) {
      final id = b.providerId;
      if (id == null || id.isEmpty) continue;
      if (_riders.containsKey(id) || _riderInFlight.contains(id)) continue;
      _riderInFlight.add(id);
      widget.dispatchController.api
          .getProvider(accessToken: token, providerId: id)
          .then((p) {
        if (!mounted) return;
        setState(() {
          _riders[id] = p;
          _riderInFlight.remove(id);
        });
      }).catchError((_) {
        _riderInFlight.remove(id);
      });
    }
  }

  void _onHaulingTap(HaulageBooking booking) {
    final status = booking.status;
    if (status.isActive || status.isSearching) {
      widget.controller.openActiveBooking(booking);
      Navigator.of(context).push(
        MaterialPageRoute(
          fullscreenDialog: true,
          builder: (_) => HaulingFlowScreen(
            controller: widget.controller,
            placesApi: widget.placesApi,
          ),
        ),
      );
    } else {
      Navigator.of(context)
          .push(
            MaterialPageRoute(
              builder: (_) => HaulingTripDetailScreen(
                booking: booking,
                controller: widget.controller,
                placesApi: widget.placesApi,
              ),
            ),
          )
          .then((_) => _load());
    }
  }

  void _onDispatchTap(dispatch.DispatchBooking booking) {
    final status = booking.status;
    if (status.isActive ||
        status.isSearching ||
        status == dispatch.DispatchBookingStatus.delivered) {
      widget.dispatchController.openActiveBooking(booking);
      Navigator.of(context).push(
        MaterialPageRoute(
          fullscreenDialog: true,
          builder: (_) => DispatchFlowScreen(
            controller: widget.dispatchController,
            placesApi: widget.placesApi,
          ),
        ),
      );
    } else {
      Navigator.of(context)
          .push(
            MaterialPageRoute(
              builder: (_) => DispatchTripDetailScreen(
                booking: booking,
                controller: widget.dispatchController,
                placesApi: widget.placesApi,
              ),
            ),
          )
          .then((_) => _load());
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        automaticallyImplyLeading: false,
        titleSpacing: 16,
        toolbarHeight: 92,
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                Container(
                  width: 40,
                  height: 40,
                  decoration: const BoxDecoration(
                    color: CustomerFigmaColors.surface,
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(Icons.menu,
                      color: CustomerFigmaColors.text, size: 20),
                ),
                const SizedBox(width: 12),
                const Text(
                  'My Trips',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontWeight: FontWeight.w800,
                    fontSize: 22,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 6),
            const Padding(
              padding: EdgeInsets.only(left: 52),
              child: Text(
                'Manage all your past, present and future rides in one place',
                style: TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 12.5,
                  height: 1.3,
                ),
              ),
            ),
          ],
        ),
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(48),
          child: Align(
            alignment: Alignment.centerLeft,
            child: TabBar(
              controller: _tabController,
              isScrollable: true,
              tabAlignment: TabAlignment.start,
              labelColor: CustomerFigmaColors.primary,
              unselectedLabelColor: CustomerFigmaColors.muted,
              indicatorColor: CustomerFigmaColors.primary,
              indicatorWeight: 2.5,
              indicatorSize: TabBarIndicatorSize.label,
              dividerColor: CustomerFigmaColors.border,
              labelStyle:
                  const TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
              unselectedLabelStyle:
                  const TextStyle(fontSize: 14, fontWeight: FontWeight.w500),
              tabs: const [
                Tab(text: 'Past'),
                Tab(text: 'Ongoing'),
                Tab(text: 'Upcoming'),
                Tab(text: 'Cancelled'),
              ],
            ),
          ),
        ),
      ),
      body: AnimatedBuilder(
        animation: Listenable.merge([
          widget.controller,
          widget.dispatchController,
        ]),
        builder: (context, _) {
          final hauling = widget.controller.state.bookingHistory;
          final dispatchAll =
              widget.dispatchController.state.bookingHistory;

          // A delivered trip is treated as completed here: the driver/rider has
          // dropped off the cargo; it only awaits the customer review (or the
          // provider-side completion) to flip to `completed` server-side.
          final past = <_TripEntry>[
            ...hauling
                .where((b) =>
                    b.status == HaulingBookingStatus.completed ||
                    b.status == HaulingBookingStatus.delivered)
                .map(_HaulingEntry.new),
            ...dispatchAll
                .where((b) =>
                    b.status == dispatch.DispatchBookingStatus.completed ||
                    b.status == dispatch.DispatchBookingStatus.delivered)
                .map(_DispatchEntry.new),
          ];
          final ongoing = <_TripEntry>[
            ...hauling
                .where((b) =>
                    !b.isUpcoming &&
                    (b.status.isSearching || b.status.isActive))
                .map(_HaulingEntry.new),
            ...dispatchAll
                .where((b) => b.status.isSearching || b.status.isActive)
                .map(_DispatchEntry.new),
          ];
          // Dispatch deliveries have no scheduling, so only hauling can be
          // "Upcoming".
          final upcoming = <_TripEntry>[
            ...hauling.where((b) => b.isUpcoming).map(_HaulingEntry.new),
          ];
          final cancelled = <_TripEntry>[
            ...hauling
                .where((b) =>
                    b.status == HaulingBookingStatus.cancelled ||
                    b.status == HaulingBookingStatus.unmatched)
                .map(_HaulingEntry.new),
            ...dispatchAll
                .where((b) =>
                    b.status == dispatch.DispatchBookingStatus.cancelled ||
                    b.status == dispatch.DispatchBookingStatus.unmatched)
                .map(_DispatchEntry.new),
          ];
          for (final list in [past, ongoing, upcoming, cancelled]) {
            list.sort((a, b) => b.sortDate.compareTo(a.sortDate));
          }

          return TabBarView(
            controller: _tabController,
            children: [
              _tripList(past, 'No completed trips yet.'),
              _tripList(ongoing, 'No ongoing trips.'),
              _tripList(upcoming, 'No upcoming trips.'),
              _tripList(cancelled, 'No cancelled trips.'),
            ],
          );
        },
      ),
    );
  }

  Widget _tripList(List<_TripEntry> entries, String emptyText) {
    return RefreshIndicator(
      color: CustomerFigmaColors.primary,
      onRefresh: _load,
      child: entries.isEmpty
          ? _EmptyOrLoading(loaded: _loadedOnce, message: emptyText)
          : ListView.builder(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
              itemCount: entries.length,
              itemBuilder: (context, i) {
                return switch (entries[i]) {
                  _HaulingEntry(booking: final b) => TripCard(
                      booking: b,
                      provider:
                          b.providerId == null ? null : _providers[b.providerId],
                      onTap: () => _onHaulingTap(b),
                    ),
                  _DispatchEntry(booking: final b) => DispatchTripCard(
                      booking: b,
                      rider: b.providerId == null ? null : _riders[b.providerId],
                      onTap: () => _onDispatchTap(b),
                    ),
                };
              },
            ),
    );
  }
}

class _EmptyOrLoading extends StatelessWidget {
  const _EmptyOrLoading({required this.loaded, required this.message});

  final bool loaded;
  final String message;

  @override
  Widget build(BuildContext context) {
    if (!loaded) {
      return const Center(
        child: CircularProgressIndicator(color: CustomerFigmaColors.primary),
      );
    }
    return ListView(
      children: [
        SizedBox(height: MediaQuery.sizeOf(context).height * 0.16),
        Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 80,
                height: 80,
                decoration: const BoxDecoration(
                  color: CustomerFigmaColors.primaryPale,
                  shape: BoxShape.circle,
                ),
                child: const Icon(
                  Icons.receipt_long_rounded,
                  size: 38,
                  color: CustomerFigmaColors.primary,
                ),
              ),
              const SizedBox(height: 20),
              Text(
                message,
                style: const TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 14,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
