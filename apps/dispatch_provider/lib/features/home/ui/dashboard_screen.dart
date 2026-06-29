import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import 'home_screen.dart';
import '../../requests/ui/requests_screen.dart';
import '../../requests/state/requests_controller.dart';
import '../../trips/ui/trips_screen.dart';
import '../../trips/state/trips_controller.dart';
import '../../trips/data/trip_model.dart';
import '../../wallet/ui/wallet_screen.dart';
import '../../wallet/state/wallet_controller.dart';
import '../../profile/ui/profile_screen.dart';
import 'widgets/trip_completed_screen.dart';
import '../../profile/state/provider_profile_controller.dart';
import '../../verification/state/verification_controller.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../availability/state/availability_controller.dart';

class DashboardScreen extends StatefulWidget {
  final ProviderProfileController profileController;
  final VerificationController verificationController;
  final VehicleController vehicleController;
  final AvailabilityController availabilityController;
  final RequestsController requestsController;
  final TripsController tripsController;
  final WalletController walletController;
  final VoidCallback? onLogout;
  final VoidCallback? onAccountDeleted;

  const DashboardScreen({
    super.key,
    required this.profileController,
    required this.verificationController,
    required this.vehicleController,
    required this.availabilityController,
    required this.requestsController,
    required this.tripsController,
    required this.walletController,
    this.onLogout,
    this.onAccountDeleted,
  });

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  int _index = 0;

  final _tripStateNotifier = ValueNotifier<TripState>(TripState.offline);
  final _activeRequestNotifier = ValueNotifier<RequestModel?>(null);

  String? _activeTripId;

  late final List<Widget> _screens;

  @override
  void initState() {
    super.initState();
    _screens = [
      HomeScreen(
        tripStateNotifier: _tripStateNotifier,
        activeRequestNotifier: _activeRequestNotifier,
        availabilityController: widget.availabilityController,
      ),
      RequestsScreen(requestsController: widget.requestsController),
      TripsScreen(tripsController: widget.tripsController),
      WalletScreen(walletController: widget.walletController),
      ProfileScreen(
        profileController: widget.profileController,
        verificationController: widget.verificationController,
        vehicleController: widget.vehicleController,
        onLogout: widget.onLogout,
        onAccountDeleted: widget.onAccountDeleted,
      ),
    ];
    _loadActiveTrip();
  }

  @override
  void dispose() {
    _tripStateNotifier.dispose();
    _activeRequestNotifier.dispose();
    super.dispose();
  }

  // ── Resume active trip on startup ─────────────────────────────────────────

  Future<void> _loadActiveTrip() async {
    final result = await widget.tripsController.loadActiveTrip();
    if (!mounted) return;
    result.when(
      success: (trip) {
        if (trip == null || !trip.statusCode.isActive) return;
        _activeTripId = trip.id;
        _activeRequestNotifier.value = _requestFromTrip(trip);
        final tripState = _tripStateFromStatus(trip.statusCode);
        if (tripState != null) {
          _tripStateNotifier.value = tripState;
        }
      },
      failure: (_) {},
    );
  }

  static RequestModel _requestFromTrip(TripModel trip) {
    return RequestModel(
      id: trip.id,
      bookingId: trip.bookingId,
      pickupAddress: trip.pickupAddress,
      dropoffAddress: trip.dropoffAddress,
      receiverName: trip.receiverName,
      receiverPhone: trip.receiverPhone,
      distanceKm: trip.distanceKm,
      estimatedFareKobo: trip.estimatedFareKobo,
      status: trip.rawStatus,
      createdAt: trip.createdAt,
      customerName: trip.customerName,
      customerPhoto: trip.customerPhoto,
      notes: trip.notes,
    );
  }

  static TripState? _tripStateFromStatus(TripStatusCode code) => switch (code) {
    TripStatusCode.assigned => TripState.arriving,
    TripStatusCode.arrivedPickup => TripState.arrivedAtPickup,
    TripStatusCode.inProgress => TripState.tripStarted,
    TripStatusCode.proofSubmitted => TripState.tripCompleted,
    _ => null,
  };

  // ── Accept request ────────────────────────────────────────────────────────

  Future<void> _handleAccept(RequestModel req) async {
    final result = await widget.requestsController.accept(req.id);
    if (!mounted) return;
    result.when(
      success: (_) {
        _activeRequestNotifier.value = req;
        _tripStateNotifier.value = TripState.arriving;
        _loadActiveTrip();
      },
      failure: (error) {
        final message = switch (error.code) {
          ApiErrorCode.forbidden => 'This request is no longer available.',
          ApiErrorCode.network => 'Cannot connect to Cosmicforge Logistics server.',
          _ =>
            error.message.isNotEmpty
                ? error.message
                : 'Failed to accept request. Please try again.',
        };
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(message),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 5),
          ),
        );
      },
    );
  }

  // ── Reject request ────────────────────────────────────────────────────────

  Future<void> _handleReject(RequestModel req) async {
    final result = await widget.requestsController.reject(
      req.id,
      'Provider rejected',
    );
    if (!mounted) return;
    result.when(
      success: (_) {},
      failure: (error) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              error.code == ApiErrorCode.network
                  ? 'Cannot connect to Cosmicforge Logistics server.'
                  : error.message.isNotEmpty
                  ? error.message
                  : 'Failed to reject request.',
            ),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 4),
          ),
        );
      },
    );
  }

  // ── Go online ─────────────────────────────────────────────────────────────

  Future<void> _handleGoOnline() async {
    final result = await widget.availabilityController.goOnline();
    if (!mounted) return;
    result.when(
      success: (_) {
        _tripStateNotifier.value = TripState.online;
      },
      failure: (error) {
        // GPS / location permission errors arrive as unknown code with a
        // human-readable message already set by the controller.
        // For forbidden, the backend sends the specific gate reason in
        // error.message (e.g. "Complete all verification steps before going
        // online." vs "Register and get your bike approved before going online.")
        final message = error.code == ApiErrorCode.forbidden
            ? error.message.isNotEmpty
                ? error.message
                : 'You cannot go online until your profile, verification, and vehicle are approved.'
            : error.code == ApiErrorCode.network
            ? 'Cannot connect to Cosmicforge Logistics server.'
            : error.message.isNotEmpty
            ? error.message
            : 'Failed to go online. Please try again.';
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(message),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 5),
          ),
        );
      },
    );
  }

  // ── Mark arrived ──────────────────────────────────────────────────────────

  Future<void> _handleMarkArrived() async {
    final id = _activeTripId;
    if (id == null) {
      _tripStateNotifier.value = TripState.arrivedAtPickup;
      return;
    }
    final result = await widget.tripsController.markArrived(id);
    if (!mounted) return;
    result.when(
      success: (_) => _tripStateNotifier.value = TripState.arrivedAtPickup,
      failure: (err) => ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            err.message.isNotEmpty ? err.message : 'Failed to mark arrived.',
          ),
          backgroundColor: Colors.red.shade800,
          duration: const Duration(seconds: 4),
        ),
      ),
    );
  }

  // ── Start trip ────────────────────────────────────────────────────────────

  Future<void> _handleStartTrip() async {
    final id = _activeTripId;
    if (id == null) {
      _tripStateNotifier.value = TripState.tripStarted;
      return;
    }
    final result = await widget.tripsController.startTrip(id);
    if (!mounted) return;
    result.when(
      success: (_) => _tripStateNotifier.value = TripState.tripStarted,
      failure: (err) => ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            err.message.isNotEmpty ? err.message : 'Failed to start trip.',
          ),
          backgroundColor: Colors.red.shade800,
          duration: const Duration(seconds: 4),
        ),
      ),
    );
  }

  // ── Cancel trip ───────────────────────────────────────────────────────────

  Future<void> _handleCancelTrip() async {
    final id = _activeTripId;
    if (id != null) {
      await widget.tripsController.cancelTrip(id);
    }
    if (!mounted) return;
    _activeTripId = null;
    _activeRequestNotifier.value = null;
    widget.requestsController.clearActiveRequest();
    _tripStateNotifier.value = TripState.online;
  }

  // ── Sheet builder ─────────────────────────────────────────────────────────

  Widget _buildSheet(TripState tripState, RequestModel? activeRequest) {
    return KeyedSubtree(
      key: ValueKey(tripState),
      child: switch (tripState) {
        TripState.offline => OfflineSheet(onGoOnline: () => _handleGoOnline()),
        TripState.online => ListenableBuilder(
          listenable: widget.requestsController,
          builder: (context, _) => IncomingRequestsSheet(
            requests: widget.requestsController.requests,
            isLoading: widget.requestsController.isLoading,
            onAccept: _handleAccept,
            onReject: _handleReject,
          ),
        ),
        TripState.arriving => TripSheet(
          request: activeRequest!,
          title: 'Arriving at pick up',
          subtitle: 'You are on your way to pick up the package.',
          showCall: true,
          primaryLabel: 'Start Trip',
          primaryEnabled: false,
          secondaryLabel: 'Cancel Trip',
          onPrimary: () {},
          onSecondary: _handleCancelTrip,
          onSimulate: _handleMarkArrived,
        ),
        TripState.arrivedAtPickup => TripSheet(
          request: activeRequest!,
          title: 'Arrived at Pickup',
          subtitle: 'You have gotten to the pickup point.',
          showCall: true,
          primaryLabel: 'Start Trip',
          primaryEnabled: true,
          secondaryLabel: 'Cancel Trip',
          onPrimary: _handleStartTrip,
          onSecondary: _handleCancelTrip,
        ),
        TripState.tripStarted => TripStartedSheet(
          request: activeRequest!,
          onEndTrip: () {
            _tripStateNotifier.value = TripState.tripCompleted;
          },
        ),
        TripState.tripCompleted => TripCompletedScreen(
          request: activeRequest!,
          tripId: _activeTripId,
          tripsController: widget.tripsController,
          onConfirm: () {
            _activeTripId = null;
            _activeRequestNotifier.value = null;
            widget.requestsController.clearActiveRequest();
            _tripStateNotifier.value = TripState.online;
          },
        ),
      },
    );
  }

  static const _icons = [
    Icons.home_rounded,
    Icons.swap_vert,
    Icons.calendar_month_outlined,
    Icons.credit_card_outlined,
    Icons.person_outline,
  ];

  static const _labels = ['Home', 'Requests', 'Trips', 'Wallet', 'Profile'];

  @override
  Widget build(BuildContext context) {
    final isHome = _index == 0;

    return Scaffold(
      extendBody: true,
      bottomSheet: isHome && _tripStateNotifier.value != TripState.tripCompleted
          ? ValueListenableBuilder<TripState>(
              valueListenable: _tripStateNotifier,
              builder: (context, tripState, _) {
                return ValueListenableBuilder<RequestModel?>(
                  valueListenable: _activeRequestNotifier,
                  builder: (context, activeRequest, _) =>
                      _buildSheet(tripState, activeRequest),
                );
              },
            )
          : null,
      body: ValueListenableBuilder<TripState>(
        valueListenable: _tripStateNotifier,
        builder: (context, tripState, _) {
          if (tripState == TripState.tripCompleted) {
            return Scaffold(
              backgroundColor: Colors.white,
              body: TripCompletedScreen(
                request: _activeRequestNotifier.value!,
                tripId: _activeTripId,
                tripsController: widget.tripsController,
                onConfirm: () {
                  _activeTripId = null;
                  _activeRequestNotifier.value = null;
                  widget.requestsController.clearActiveRequest();
                  _tripStateNotifier.value = TripState.online;
                },
              ),
            );
          }
          return IndexedStack(index: _index, children: _screens);
        },
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
          child: Container(
            height: 70,
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(999),
              boxShadow: [
                BoxShadow(
                  color: Colors.black.withValues(alpha: 0.10),
                  blurRadius: 20,
                  offset: const Offset(0, 4),
                ),
              ],
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceAround,
              children: List.generate(5, (i) {
                final isActive = _index == i;
                return GestureDetector(
                  onTap: () => setState(() => _index = i),
                  behavior: HitTestBehavior.opaque,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 200),
                    padding: isActive
                        ? const EdgeInsets.symmetric(
                            horizontal: 20,
                            vertical: 12,
                          )
                        : const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: isActive
                          ? const Color(0xFF4CAF50)
                          : Colors.transparent,
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          _icons[i],
                          size: 22,
                          color: isActive
                              ? Colors.white
                              : const Color(0xFF1A1A1A),
                        ),
                        if (isActive) ...[
                          const SizedBox(width: 8),
                          Text(
                            _labels[i],
                            style: const TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                );
              }),
            ),
          ),
        ),
      ),
    );
  }
}
