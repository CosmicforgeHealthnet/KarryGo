import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';
import '../../../shared/widgets/async_content.dart';
import '../../offline/ui/offline_updates_screen.dart';

import '../../availability/state/availability_controller.dart';
import '../../requests/data/request_model.dart';
import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';

import 'widgets/header_bar.dart';
import 'widgets/live_dispatch_map.dart';
import '../../missions/ui/delivery_operations_screen.dart';
import '../../trips/state/trips_controller.dart';

export '../../requests/data/request_model.dart' show RequestModel;
export 'sheets/offline_sheet.dart' show OfflineSheet;
export 'sheets/incoming_requests_sheet.dart' show IncomingRequestsSheet;
export 'sheets/trip_sheet.dart' show TripSheet;
export 'sheets/trip_started_sheet.dart' show TripStartedSheet;

enum TripState {
  offline,
  online,
  arriving,
  arrivedAtPickup,
  tripStarted,
  tripCompleted,
}

class HomeScreen extends StatefulWidget {
  const HomeScreen({
    super.key,
    required this.tripStateNotifier,
    required this.activeRequestNotifier,
    required this.availabilityController,
    this.onOpenNotifications,
    this.onLocateMe,
    this.onOpenAccount,
    this.onOpenTrip,
    this.tripsController,
    this.onToggleStatus,
    this.todayEarningsDisplay = '—',
    this.acceptanceRateDisplay = '—',
    this.avatarImageProvider,
  });

  final ValueNotifier<TripState> tripStateNotifier;
  final ValueNotifier<RequestModel?> activeRequestNotifier;
  final AvailabilityController availabilityController;
  final VoidCallback? onOpenNotifications;
  final VoidCallback? onLocateMe;
  final VoidCallback? onOpenAccount;
  final VoidCallback? onOpenTrip;
  final TripsController? tripsController;

  /// Called when the status pill in the header is tapped. Parent decides
  /// whether that means "go online" or "go offline" based on current state.
  final VoidCallback? onToggleStatus;

  final String todayEarningsDisplay;
  final String acceptanceRateDisplay;
  final ImageProvider? avatarImageProvider;

  @override
  HomeScreenState createState() => HomeScreenState();
}

class HomeScreenState extends State<HomeScreen> {
  String? _availabilityError;
  TripState get _tripState => widget.tripStateNotifier.value;

  @override
  void initState() {
    super.initState();
    widget.tripStateNotifier.addListener(_onNotifierChanged);
    widget.activeRequestNotifier.addListener(_onNotifierChanged);
    widget.availabilityController.addListener(_onNotifierChanged);
    widget.tripsController?.addListener(_onNotifierChanged);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _loadInitialAvailability();
    });
  }

  @override
  void dispose() {
    widget.tripStateNotifier.removeListener(_onNotifierChanged);
    widget.activeRequestNotifier.removeListener(_onNotifierChanged);
    widget.availabilityController.removeListener(_onNotifierChanged);
    widget.tripsController?.removeListener(_onNotifierChanged);
    super.dispose();
  }

  void _onNotifierChanged() => setState(() {});

  void _update(
    TripState newState, {
    RequestModel? request,
    bool clearRequest = false,
  }) {
    if (clearRequest) widget.activeRequestNotifier.value = null;
    if (request != null) widget.activeRequestNotifier.value = request;
    widget.tripStateNotifier.value = newState;
  }

  Future<void> _loadInitialAvailability() async {
    setState(() => _availabilityError = null);
    final result = await widget.availabilityController.loadAvailability();
    if (!mounted) return;
    result.when(
      success: (avail) {
        if (avail.isOnline && _tripState == TripState.offline) {
          _update(TripState.online);
        }
        if (avail.isOffline && _tripState != TripState.offline) {
          _update(TripState.offline, clearRequest: true);
        }
      },
      failure: (error) => setState(() => _availabilityError = error.message),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final state = _tripState;
    final online = state != TripState.offline;
    final onDelivery = state != TripState.offline && state != TripState.online;
    final changing = widget.availabilityController.isChangingStatus;
    return Stack(
      fit: StackFit.expand,
      children: [
        Positioned.fill(
          child: LiveDispatchMap(
            demo: AppModeConfig.instance.isMock,
            online: online,
            location: widget.availabilityController.lastLocation,
            trip: widget.tripsController?.activeTrip,
            stale: widget.availabilityController.hasStaleLocation,
          ),
        ),
        SafeArea(
          bottom: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  padding: const EdgeInsets.fromLTRB(16, 10, 8, 10),
                  decoration: BoxDecoration(
                    color: c.surface,
                    borderRadius: BorderRadius.circular(24),
                    border: Border.all(color: c.divider),
                    boxShadow: c.cardShadow,
                  ),
                  child: HomeHeaderBar(
                    onOpenAccount: widget.onOpenAccount,
                    onOpenNotifications: widget.onOpenNotifications,
                    avatarImageProvider: widget.avatarImageProvider,
                    onOpenDelivery: widget.tripsController?.activeTrip == null
                        ? null
                        : widget.onOpenTrip ??
                              () async {
                                await Navigator.of(context).push(
                                  MaterialPageRoute(
                                    builder: (_) => DeliveryOperationsScreen(
                                      tripId: widget
                                          .tripsController!
                                          .activeTrip!
                                          .id,
                                    ),
                                  ),
                                );
                                if (mounted)
                                  await widget.tripsController
                                      ?.loadActiveTrip();
                              },
                  ),
                ),
                const SizedBox(height: 12),
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Expanded(
                      child: AnimatedContainer(
                        duration: const Duration(milliseconds: 250),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 16,
                          vertical: 12,
                        ),
                        decoration: BoxDecoration(
                          color: c.surface,
                          borderRadius: BorderRadius.circular(20),
                          border: Border.all(color: c.divider),
                        ),
                        child: Row(
                          children: [
                            JourneyIcon(
                              JourneySymbol.route,
                              size: 24,
                              color: online ? c.success : c.textMuted,
                            ),
                            const SizedBox(width: 10),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    onDelivery
                                        ? 'Delivery in progress'
                                        : online
                                        ? 'On the move'
                                        : 'Your time. Your pace.',
                                    style: Theme.of(
                                      context,
                                    ).textTheme.titleSmall,
                                  ),
                                  const SizedBox(height: 3),
                                  Text(
                                    onDelivery
                                        ? 'Your next handover, in view'
                                        : online
                                        ? 'Available for deliveries'
                                        : 'Go online when you are ready',
                                    style: Theme.of(
                                      context,
                                    ).textTheme.bodySmall,
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                    if (state == TripState.online) ...[
                      const SizedBox(width: 8),
                      Material(
                        color: c.dangerSoft,
                        borderRadius: BorderRadius.circular(20),
                        child: IconButton(
                          tooltip: 'Go offline',
                          onPressed: changing ? null : widget.onToggleStatus,
                          icon: Icon(
                            Icons.power_settings_new_rounded,
                            color: c.danger,
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
                if (widget.tripsController?.activeTripError != null) ...[
                  const SizedBox(height: 8),
                  ActionNotice(
                    message: widget.tripsController!.activeTripError!,
                    isError: true,
                    onRetry: () => widget.tripsController!.loadActiveTrip(),
                  ),
                ],
                if (_availabilityError != null)
                  ActionNotice(
                    message: _availabilityError!,
                    isError: true,
                    onRetry: _loadInitialAvailability,
                  ),
                const OfflineSyncNotice(),
              ],
            ),
          ),
        ),
      ],
    );
  }
}
