import '../../../core/app_mode.dart';
import '../../missions/ui/delivery_operations_screen.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'widgets/home_feedback.dart';
import 'widgets/trip_peek_bar.dart';
import '../../../shared/widgets/async_content.dart';
import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/theme/theme_extensions.dart';
import 'home_screen.dart';
import 'sheets/online_idle_sheet.dart';
import 'sheets/go_online_processing.dart';
import 'sheets/readiness_blocker.dart';
import 'sheets/location_permission_blocker.dart';
import 'sheets/stale_gps_warning.dart';
import 'widgets/pickup_code_card.dart';
import 'widgets/trip_completed_screen.dart';
import '../../requests/data/request_model.dart';
import '../../requests/ui/requests_screen.dart';
import '../../requests/state/requests_controller.dart';
import '../../trips/ui/trips_screen.dart';
import '../../trips/state/trips_controller.dart';
import '../../trips/data/trip_model.dart';
import '../../wallet/ui/wallet_screen.dart';
import '../../wallet/state/wallet_controller.dart';
import '../../profile/ui/profile_screen.dart';
import '../../profile/state/provider_profile_controller.dart';
import '../../verification/state/verification_controller.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../availability/state/availability_controller.dart';
import '../../disputes/state/rider_dispute_controller.dart';

class DashboardScreen extends StatefulWidget {
  final ProviderProfileController profileController;
  final VerificationController verificationController;
  final VehicleController vehicleController;
  final AvailabilityController availabilityController;
  final RequestsController requestsController;
  final TripsController tripsController;
  final WalletController walletController;
  final RiderDisputeController? disputeController;
  final VoidCallback? onLogout;
  final VoidCallback? onAccountDeleted;
  final VoidCallback? onOpenNotifications;

  const DashboardScreen({
    super.key,
    required this.profileController,
    required this.verificationController,
    required this.vehicleController,
    required this.availabilityController,
    required this.requestsController,
    required this.tripsController,
    required this.walletController,
    this.disputeController,
    this.onLogout,
    this.onAccountDeleted,
    this.onOpenNotifications,
  });

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen>
    with WidgetsBindingObserver {
  final _actionBusy = ValueNotifier<bool>(false);
  bool _workspaceOpen = false;
  int _index = 0;
  final _visitedTabs = <int>{0};

  final _tripStateNotifier = ValueNotifier<TripState>(TripState.offline);
  final _activeRequestNotifier = ValueNotifier<RequestModel?>(null);

  final _sheetCollapsedNotifier = ValueNotifier<bool>(false);

  String? _activeTripId;

  bool _activityRefreshInFlight = false;

  late final List<Widget> _screens;

  static const int _activityTabIndex = 2;

  /// Backend-whitelisted reason codes for POST /trips/:id/fail. The backend
  /// derives retry eligibility from the code — never sent from the client.
  static const _failReasons = <String, String>{
    'recipient_unavailable': 'Recipient not available',
    'recipient_refused': 'Recipient refused the package',
    'address_unreachable': 'Address is wrong or unreachable',
    'package_damaged': 'Package was damaged in transit',
    'safety_concern': 'Safety concern',
    'other': 'Other',
  };

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _screens = [
      HomeScreen(
        tripsController: widget.tripsController,
        tripStateNotifier: _tripStateNotifier,
        activeRequestNotifier: _activeRequestNotifier,
        availabilityController: widget.availabilityController,
        onOpenNotifications: widget.onOpenNotifications,
        onToggleStatus: _handleToggleStatus,
        onOpenAccount: () => _onTabChanged(4),
        onOpenTrip: _openTripWorkspace,
      ),
      RequestsScreen(requestsController: widget.requestsController),
      TripsScreen(tripsController: widget.tripsController),
      WalletScreen(
        onViewOpportunities: () => _onTabChanged(1),
        walletController: widget.walletController,
        disputeController: widget.disputeController,
        tripsController: widget.tripsController,
      ),
      ProfileScreen(
        profileController: widget.profileController,
        verificationController: widget.verificationController,
        vehicleController: widget.vehicleController,
        disputeController: widget.disputeController,
        walletController: widget.walletController,
        onLogout: widget.onLogout,
        onAccountDeleted: widget.onAccountDeleted,
      ),
    ];
    _loadActiveTrip();
    widget.requestsController.startPolling();
    widget.tripsController.startActivePolling();
    widget.tripsController.addListener(_syncTripStateFromController);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.requestsController.stopPolling();
    widget.tripsController.stopActivePolling();
    widget.tripsController.removeListener(_syncTripStateFromController);
    _actionBusy.dispose();
    _tripStateNotifier.dispose();
    _activeRequestNotifier.dispose();
    _sheetCollapsedNotifier.dispose();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      widget.tripsController.loadActiveTrip();
      widget.requestsController.loadRequests();
    }
  }

  void _refreshTripsList() {
    if (_activityRefreshInFlight) return;
    _activityRefreshInFlight = true;
    widget.tripsController.loadTrips().whenComplete(() {
      if (mounted) _activityRefreshInFlight = false;
    });
  }

  void _onTabChanged(int i) {
    if (_index == i) return;
    setState(() {
      _index = i;
      _visitedTabs.add(i);
    });
    if (i == _activityTabIndex) _refreshTripsList();
  }

  void _syncTripStateFromController() {
    if (!mounted) return;
    final trip = widget.tripsController.activeTrip;
    if (trip == null || !trip.statusCode.isActive) {
      if (widget.tripsController.activeTripError == null &&
          _activeTripId != null) {
        _activeTripId = null;
        _activeRequestNotifier.value = null;
        _tripStateNotifier.value = widget.availabilityController.isOnline
            ? TripState.online
            : TripState.offline;
      }
      return;
    }
    if (widget.tripsController.isActionLoading) return;
    final tripState = _displayStateForTrip(trip);
    if (tripState == null) return;
    _activeTripId = trip.id;
    _activeRequestNotifier.value = _requestFromTrip(trip);
    _tripStateNotifier.value = tripState;
    _ensurePickupCredential(trip);
  }

  Future<void> _loadActiveTrip() async {
    final result = await widget.tripsController.loadActiveTrip();
    if (!mounted) return;
    result.when(
      success: (trip) {
        if (trip == null || !trip.statusCode.isActive) return;
        final tripState = _displayStateForTrip(trip);
        _activeTripId = trip.id;
        _activeRequestNotifier.value = _requestFromTrip(trip);
        if (tripState != null) {
          _tripStateNotifier.value = tripState;
        }
        _ensurePickupCredential(trip);
      },
      failure: (_) {},
    );
  }

  void _ensurePickupCredential(TripModel trip) {
    if (trip.statusCode != TripStatusCode.arrivedPickup) return;
    widget.tripsController.fetchPickupCredential(trip.id);
  }

  static RequestModel _requestFromTrip(TripModel trip) {
    return RequestModel(
      id: trip.id,
      kind: RequestServerKind.broadcast,
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

  TripState? _displayStateForTrip(TripModel trip) {
    final completingSameTrip =
        _tripStateNotifier.value == TripState.tripCompleted &&
        _activeTripId == trip.id;
    if (completingSameTrip &&
        (trip.statusCode == TripStatusCode.inProgress ||
            trip.statusCode == TripStatusCode.proofSubmitted)) {
      return TripState.tripCompleted;
    }
    return _tripStateFromStatus(trip.statusCode);
  }

  static TripState? _tripStateFromStatus(TripStatusCode code) => switch (code) {
    TripStatusCode.assigned => TripState.arriving,
    TripStatusCode.enRoutePickup => TripState.arriving,
    TripStatusCode.arrivedPickup => TripState.arrivedAtPickup,
    TripStatusCode.inProgress => TripState.tripStarted,
    TripStatusCode.proofSubmitted => TripState.tripCompleted,
    _ => null,
  };

  Future<void> _runTripAction(Future<void> Function() task) async {
    if (_actionBusy.value) return;
    _actionBusy.value = true;
    try {
      await task();
    } finally {
      if (mounted) _actionBusy.value = false;
    }
  }

  Future<void> _openTripWorkspace() async {
    if (_workspaceOpen || _activeRequestNotifier.value == null) return;
    _workspaceOpen = true;
    _sheetCollapsedNotifier.value = false;
    try {
      await Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (screen) => ListenableBuilder(
            listenable: Listenable.merge([
              _tripStateNotifier,
              _activeRequestNotifier,
              widget.tripsController,
              _actionBusy,
            ]),
            builder: (screen, _) {
              final state = _tripStateNotifier.value;
              final request = _activeRequestNotifier.value;
              if (request == null ||
                  state == TripState.online ||
                  state == TripState.offline ||
                  state == TripState.tripCompleted) {
                final route = ModalRoute.of(screen);
                WidgetsBinding.instance.addPostFrameCallback((_) {
                  if (screen.mounted && route != null && route.isActive)
                    Navigator.of(screen).removeRoute(route);
                });
                return const Scaffold(body: SizedBox.shrink());
              }
              return Scaffold(
                appBar: AppBar(
                  leading: const DispatchBackButton(),
                  title: const Text('Current delivery'),
                  actions: [
                    if (_activeTripId != null)
                      TextButton.icon(
                        onPressed: () => Navigator.of(screen).push(
                          MaterialPageRoute(
                            builder: (_) => DeliveryOperationsScreen(
                              tripId: _activeTripId!,
                            ),
                          ),
                        ),
                        icon: const JourneyIcon(JourneySymbol.review, size: 20),
                        label: const Text('Tasks'),
                      ),
                  ],
                ),
                body: SafeArea(
                  top: false,
                  child: SingleChildScrollView(
                    key: const Key('trip-workspace-scroll'),
                    padding: EdgeInsets.only(
                      bottom: MediaQuery.paddingOf(screen).bottom,
                    ),
                    child: Center(
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 600),
                        child: _buildSheet(state, request),
                      ),
                    ),
                  ),
                ),
              );
            },
          ),
        ),
      );
    } finally {
      _workspaceOpen = false;
    }
  }

  Future<void> _handleAccept(RequestModel req) async {
    if (_actionBusy.value) return;
    _actionBusy.value = true;
    final result = await widget.requestsController.accept(req);
    if (mounted) _actionBusy.value = false;
    if (!mounted) return;
    result.when(
      success: (_) async {
        _activeRequestNotifier.value = req;
        _tripStateNotifier.value = TripState.arriving;
        _sheetCollapsedNotifier.value = false;
        await _loadActiveTrip();
        _refreshTripsList();
        if (!mounted) return;
        await showHomeFeedback(
          context,
          title: 'Delivery accepted',
          message:
              'The request is assigned to you. Review the pickup instructions before setting off.',
          action: 'View pickup',
        );
        if (mounted) _openTripWorkspace();
      },
      failure: (error) {
        final message = switch (error.code) {
          ApiErrorCode.forbidden => 'This request is no longer available.',
          ApiErrorCode.network =>
            'Could not connect. Check your connection and try again.',
          _ =>
            error.message.isNotEmpty
                ? error.message
                : 'Failed to accept request. Please try again.',
        };
        showHomeFeedback(
          context,
          title: 'Could not accept delivery',
          message: message,
          error: true,
          action: 'Back to requests',
        );
      },
    );
  }

  Future<void> _handleReject(RequestModel req) async {
    if (_actionBusy.value) return;
    _actionBusy.value = true;
    final result = await widget.requestsController.reject(req, 'other');
    if (mounted) _actionBusy.value = false;
    if (!mounted) return;
    result.when(
      success: (_) => showHomeFeedback(
        context,
        title: 'Request declined',
        message: 'You can review the next available opportunity.',
      ),
      failure: (error) {
        showHomeFeedback(
          context,
          title: 'Could not complete action',
          message: error.code == ApiErrorCode.network
              ? 'Could not connect. Check your connection and try again.'
              : error.message.isNotEmpty
              ? error.message
              : 'Failed to reject request.',
          error: true,
          action: 'Back to delivery',
        );
      },
    );
  }

  Future<void> _handleGoOnline() async {
    if (widget.availabilityController.isChangingStatus) return;
    final result = await showGoOnlineProcessing(
      context,
      task: widget.availabilityController.goOnline(),
    );
    if (!mounted) return;
    result.when(
      success: (_) {
        _tripStateNotifier.value = TripState.online;
        widget.requestsController.loadRequests();
      },
      failure: (error) async {
        final isLocationError =
            error.code == ApiErrorCode.unknown &&
            (error.message.contains('location') ||
                error.message.contains('Location'));
        if (isLocationError) {
          final allowed = await showLocationPermissionBlocker(context);
          if (allowed && mounted) {
            _handleGoOnline();
          }
          return;
        }

        if (error.code == ApiErrorCode.forbidden) {
          await _showReadinessBlocker();
          return;
        }

        final message = error.code == ApiErrorCode.network
            ? 'Could not connect. Check your connection and try again.'
            : error.message.isNotEmpty
            ? error.message
            : 'Failed to go online. Please try again.';
        if (!mounted) return;
        showDispatchError(context, message);
      },
    );
  }

  Future<void> _handleGoOffline() async {
    final result = await widget.availabilityController.goOffline();
    if (!mounted) return;
    result.when(
      success: (_) {
        _activeTripId = null;
        _activeRequestNotifier.value = null;
        widget.requestsController.clearActiveRequest();
        _tripStateNotifier.value = TripState.offline;
        _sheetCollapsedNotifier.value = false;
      },
      failure: (error) {
        showHomeFeedback(
          context,
          title: 'Could not complete action',
          message: error.message.isNotEmpty
              ? error.message
              : 'Failed to go offline. Please try again.',
          error: true,
          action: 'Back to delivery',
        );
      },
    );
  }

  void _handleToggleStatus() {
    final state = _tripStateNotifier.value;
    if (state == TripState.offline) {
      _handleGoOnline();
    } else if (state == TripState.online) {
      _confirmGoOffline();
    }
  }

  Future<void> _confirmGoOffline() async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (sheet) => SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Go offline?', style: Theme.of(sheet).textTheme.headlineSmall),
            const SizedBox(height: 10),
            Text(
              'You will stop receiving new delivery requests.',
              style: Theme.of(sheet).textTheme.bodyLarge,
            ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: () => Navigator.pop(sheet, true),
              style: FilledButton.styleFrom(
                backgroundColor: sheet.palette.dangerSoft,
                foregroundColor: sheet.palette.danger,
              ),
              child: const Text('Go offline'),
            ),
            TextButton(
              onPressed: () => Navigator.pop(sheet, false),
              child: const Text('Stay online'),
            ),
          ],
        ),
      ),
    );
    if (confirmed == true && mounted) await _handleGoOffline();
  }

  void _toggleSheetCollapsed() {
    if (_workspaceOpen) {
      Navigator.of(context).pop();
      return;
    }
    _openTripWorkspace();
  }

  String _peekStatusFor(TripState state) {
    switch (state) {
      case TripState.arriving:
        final eta = _etaMinutes(_activeRequestNotifier.value?.distanceKm);
        return '$eta min · on the way to pickup';
      case TripState.arrivedAtPickup:
        return 'Arrived · show the customer your code';
      case TripState.tripStarted:
        final eta = _etaMinutes(_activeRequestNotifier.value?.distanceKm);
        return '$eta min · on the way to drop-off';
      default:
        return '';
    }
  }

  int _etaMinutes(double? distanceKm) {
    if (distanceKm == null || distanceKm <= 0) return 0;
    final eta = (distanceKm * 2.5).round();
    return eta.clamp(1, 999);
  }

  Future<void> _showReadinessBlocker() async {
    widget.verificationController.refreshStatus();

    final items = _buildReadinessItems();
    if (items.isEmpty) {
      showHomeFeedback(
        context,
        title: 'Could not complete action',
        message: 'You cannot go online until your profile is approved.',
        error: true,
        action: 'Back to delivery',
      );
      return;
    }

    await showReadinessBlocker(
      context,
      items: items,
      onCompleteSetup: () {
        setState(() {
          _index = 4;
          _visitedTabs.add(4);
        });
      },
    );
  }

  List<ReadinessItem> _buildReadinessItems() {
    final items = <ReadinessItem>[];
    final profile = widget.profileController.profile;
    final verif = widget.verificationController.latestStatus;

    final licenceStep = verif?.steps
        .where((s) => s.step == 'licence' || s.step == 'vehicle')
        .firstOrNull;
    final vehicleApproved = licenceStep?.status == 'approved';
    if (!vehicleApproved) {
      final isPending =
          licenceStep?.status == 'pending' ||
          licenceStep?.status == 'submitted';
      items.add(
        ReadinessItem(
          title: 'Vehicle details',
          subtitle: isPending
              ? 'Your vehicle is under review'
              : 'Add your vehicle information',
          status: isPending
              ? ReadinessItemStatus.pending
              : ReadinessItemStatus.missing,
        ),
      );
    }

    final photoUrl = profile?.profilePhotoUrl;
    if (photoUrl == null || photoUrl.isEmpty) {
      items.add(
        const ReadinessItem(
          title: 'Profile photo',
          subtitle: 'Add a clear profile photo',
          status: ReadinessItemStatus.missing,
        ),
      );
    }

    final identityStep = verif?.steps
        .where((s) => s.step == 'identity')
        .firstOrNull;
    if (identityStep == null || identityStep.status != 'approved') {
      final isPending =
          identityStep?.status == 'pending' ||
          identityStep?.status == 'submitted';
      final isRejected = identityStep?.status == 'rejected';
      items.add(
        ReadinessItem(
          title: 'Identity verification',
          subtitle: isRejected
              ? (identityStep?.rejectionReason ??
                    'Your ID was rejected. Please resubmit.')
              : isPending
              ? 'Under review'
              : 'Complete ID verification',
          status: isPending
              ? ReadinessItemStatus.pending
              : ReadinessItemStatus.missing,
        ),
      );
    }

    return items;
  }

  Future<void> _showStaleGpsHelp() async {
    final colors = context.palette;
    await showDialog<void>(
      context: context,
      barrierColor: Colors.black.withValues(alpha: 0.72),
      builder: (context) => Dialog(
        backgroundColor: colors.surface,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 24, 24, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Why is this happening?',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                "We haven't received a GPS signal in a while. This usually "
                'means:\n\n'
                '• You are indoors or underground\n'
                '• Location services are turned off\n'
                '• The app was backgrounded for too long\n\n'
                'Move to an open area or check your device location settings.',
                style: TextStyle(
                  fontSize: 13,
                  height: 1.5,
                  color: colors.textMuted,
                ),
              ),
              const SizedBox(height: 20),
              SizedBox(
                width: double.infinity,
                height: 48,
                child: FilledButton(
                  onPressed: () => Navigator.of(context).pop(),
                  child: const Text('Got it'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<String?> _resolveActiveTripId() async {
    if (_activeTripId != null) return _activeTripId;
    await _loadActiveTrip();
    return _activeTripId;
  }

  void _showTripNotReadySnackBar() {
    showHomeFeedback(
      context,
      title: 'Could not complete action',
      message: 'Trip is still being set up. Please try again.',
      error: true,
      action: 'Back to delivery',
    );
  }

  Future<void> _handleMarkArrived() async {
    final id = await _resolveActiveTripId();
    if (!mounted) return;
    if (id == null) {
      _showTripNotReadySnackBar();
      return;
    }
    final result = await widget.tripsController.markArrived(id);
    if (!mounted) return;
    result.when(
      success: (_) {
        _tripStateNotifier.value = TripState.arrivedAtPickup;
        showHomeFeedback(
          context,
          title: 'Pickup reached',
          message: 'Show the pickup code to the customer for verification.',
          action: 'View pickup code',
        );
      },
      failure: (err) => showHomeFeedback(
        context,
        title: 'Could not complete action',
        message: err.message.isNotEmpty
            ? err.message
            : 'Failed to mark arrived.',
        error: true,
        action: 'Back to delivery',
      ),
    );
  }

  Future<void> _handleStartTrip() async {
    final id = await _resolveActiveTripId();
    if (!mounted) return;
    if (id == null) {
      _showTripNotReadySnackBar();
      return;
    }
    final verified = widget.tripsController.pickupCredential?.verified ?? false;
    if (!verified) {
      showHomeFeedback(
        context,
        title: 'Pickup verification needed',
        message: 'Waiting for the customer to verify your pickup code.',
        warning: true,
        action: 'View pickup',
      );
      return;
    }
    final result = await widget.tripsController.startTrip(id);
    if (!mounted) return;
    result.when(
      success: (_) {
        _tripStateNotifier.value = TripState.tripStarted;
        showHomeFeedback(
          context,
          title: 'Delivery started',
          message:
              'Your pickup is recorded. Continue to the delivery location.',
          action: 'View delivery',
        );
      },
      failure: (err) => showHomeFeedback(
        context,
        title: 'Could not complete action',
        message: err.message.isNotEmpty ? err.message : 'Failed to start trip.',
        error: true,
        action: 'Back to delivery',
      ),
    );
  }

  Future<String?> _pickCancelReason() {
    final colors = context.palette;
    return showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      backgroundColor: colors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (context) => SafeArea(
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
                child: Text(
                  'Why are you cancelling?',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                  ),
                ),
              ),
              for (final entry in tripCancelReasons.entries)
                ListTile(
                  title: Text(
                    entry.value,
                    style: TextStyle(color: colors.text),
                  ),
                  onTap: () => Navigator.of(context).pop(entry.key),
                ),
              const SizedBox(height: 8),
            ],
          ),
        ),
      ),
    );
  }

  Future<String?> _pickFailReason() {
    final colors = context.palette;
    return showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      backgroundColor: colors.surface,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (context) => SafeArea(
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
                child: Text(
                  'Why did the delivery fail?',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                  ),
                ),
              ),
              for (final entry in _failReasons.entries)
                ListTile(
                  title: Text(
                    entry.value,
                    style: TextStyle(color: colors.text),
                  ),
                  onTap: () => Navigator.of(context).pop(entry.key),
                ),
              const SizedBox(height: 8),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _handleCancelTrip() async {
    final id = await _resolveActiveTripId();
    if (!mounted) return;
    if (id == null) {
      _showTripNotReadySnackBar();
      return;
    }
    final reasonCode = await _pickCancelReason();
    if (!mounted || reasonCode == null) return;

    final result = await widget.tripsController.cancelTrip(
      id,
      reasonCode: reasonCode,
    );
    if (!mounted) return;
    result.when(
      success: (_) {
        _activeTripId = null;
        _activeRequestNotifier.value = null;
        widget.requestsController.clearActiveRequest();
        _tripStateNotifier.value = TripState.online;
        _sheetCollapsedNotifier.value = false;
        _refreshTripsList();
        showHomeFeedback(
          context,
          title: 'Delivery cancelled',
          message: 'The cancellation has been recorded.',
        );
      },
      failure: (err) => showHomeFeedback(
        context,
        title: 'Could not complete action',
        message: err.message.isNotEmpty
            ? err.message
            : 'Failed to cancel trip.',
        error: true,
        action: 'Back to delivery',
      ),
    );
  }

  Future<void> _handleFailDelivery() async {
    final id = await _resolveActiveTripId();
    if (!mounted) return;
    if (id == null) {
      _showTripNotReadySnackBar();
      return;
    }
    final reasonCode = await _pickFailReason();
    if (!mounted || reasonCode == null) return;

    final result = await widget.tripsController.failDelivery(
      id: id,
      reasonCode: reasonCode,
      expectedSourceVersion: 0,
      idempotencyKey: 'fail-$id-${DateTime.now().microsecondsSinceEpoch}',
    );
    if (!mounted) return;
    result.when(
      success: (_) {
        _activeTripId = null;
        _activeRequestNotifier.value = null;
        widget.requestsController.clearActiveRequest();
        _tripStateNotifier.value = TripState.online;
        _sheetCollapsedNotifier.value = false;
        _refreshTripsList();
        showHomeFeedback(
          context,
          title: 'Delivery issue recorded',
          message:
              'Your report has been saved. Review available opportunities when you are ready.',
        );
      },
      failure: (err) => showHomeFeedback(
        context,
        title: 'Could not complete action',
        message: err.message.isNotEmpty
            ? err.message
            : 'Failed to report delivery failure.',
        error: true,
        action: 'Back to delivery',
      ),
    );
  }

  Future<void> _confirmLeaveTrip() async {
    final colors = context.palette;
    final confirmed = await showDialog<bool>(
      context: context,
      barrierColor: Colors.black.withValues(alpha: 0.72),
      builder: (context) => Dialog(
        backgroundColor: colors.surface,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 24, 24, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Leave this trip?',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                "You'll stay online but this delivery won't be completed.",
                style: TextStyle(
                  fontSize: 13,
                  height: 1.5,
                  color: colors.textMuted,
                ),
              ),
              const SizedBox(height: 20),
              Row(
                children: [
                  Expanded(
                    child: SizedBox(
                      height: 48,
                      child: OutlinedButton(
                        onPressed: () => Navigator.of(context).pop(false),
                        style: OutlinedButton.styleFrom(
                          foregroundColor: colors.text,
                          side: BorderSide(color: colors.border),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(14),
                          ),
                        ),
                        child: const Text(
                          'Stay',
                          style: TextStyle(fontWeight: FontWeight.w700),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: SizedBox(
                      height: 48,
                      child: FilledButton(
                        onPressed: () => Navigator.of(context).pop(true),
                        child: const Text('Leave'),
                      ),
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );

    if (confirmed == true && mounted) {
      await _handleCancelTrip();
    }
  }

  void _handleTripCompleted() {
    showHomeFeedback(
      context,
      title: 'Delivery completed',
      message:
          'The delivery has been recorded. You can return to available opportunities.',
      action: 'Back to Home',
    );
    _activeTripId = null;
    _activeRequestNotifier.value = null;
    widget.requestsController.clearActiveRequest();
    _tripStateNotifier.value = TripState.online;
    _sheetCollapsedNotifier.value = false;
    _refreshTripsList();
  }

  Widget _buildSheet(TripState tripState, RequestModel? activeRequest) {
    return KeyedSubtree(
      key: ValueKey(tripState),
      child: switch (tripState) {
        TripState.offline => OfflineSheet(
          onGoOnline: () => _handleGoOnline(),
          busy: widget.availabilityController.isChangingStatus,
        ),

        TripState.online => ListenableBuilder(
          listenable: Listenable.merge([
            widget.requestsController,
            widget.availabilityController,
            _actionBusy,
          ]),
          builder: (context, _) {
            if (widget.availabilityController.hasStaleLocation) {
              return StaleGpsWarningSheet(
                onLearnMore: _showStaleGpsHelp,
                onKeepAppOpen: () => _handleGoOffline(),
              );
            }

            final requests = widget.requestsController.requests;
            final isLoading = widget.requestsController.isLoading;

            if (!isLoading && requests.isEmpty) {
              return const OnlineIdleSheet();
            }

            return IncomingRequestsSheet(
              requests: requests,
              busy: _actionBusy.value,
              isLoading: isLoading,
              onAccept: _handleAccept,
              onReject: _handleReject,
            );
          },
        ),

        TripState.arriving => TripSheet(
          request: activeRequest!,
          title: 'Arriving at pick up',
          subtitle: 'You are on your way to pick up the package.',
          showCall: true,
          primaryLabel: "I've Arrived",
          primaryEnabled: true,
          secondaryLabel: 'Cancel Trip',
          onPrimary: () => _runTripAction(_handleMarkArrived),
          busy: _actionBusy.value,
          onSecondary: () => _runTripAction(_handleCancelTrip),
          collapsed: _sheetCollapsedNotifier.value,
          onToggleCollapse: _toggleSheetCollapsed,
          peekStatusText: _peekStatusFor(tripState),
        ),

        TripState.arrivedAtPickup => ListenableBuilder(
          listenable: widget.tripsController,
          builder: (context, _) {
            final verified =
                widget.tripsController.pickupCredential?.verified ?? false;

            return TripSheet(
              request: activeRequest!,
              title: 'Arrived at Pickup',
              subtitle: verified
                  ? 'Customer verified you. You can start the trip.'
                  : 'Show the code below to the customer. Start Trip unlocks '
                        'once they verify you.',
              showCall: true,
              primaryLabel: verified ? 'Start Trip' : 'Awaiting verification',
              primaryEnabled: verified,
              secondaryLabel: 'Cancel Trip',
              onPrimary: () => _runTripAction(_handleStartTrip),
              busy: _actionBusy.value,
              onSecondary: () => _runTripAction(_handleCancelTrip),
              collapsed: _sheetCollapsedNotifier.value,
              onToggleCollapse: _toggleSheetCollapsed,
              peekStatusText: _peekStatusFor(tripState),
              extra: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  PickupCodeCard(
                    title: 'Show this code to the customer',
                    subtitle:
                        'They will scan or enter it to verify you before pickup.',
                    code: widget.tripsController.pickupCredential?.code,
                    expiresAt:
                        widget.tripsController.pickupCredential?.expiresAt,
                    onResend: () {
                      final id = _activeTripId;
                      if (id != null) {
                        widget.tripsController.resendPickupCredential(id);
                      }
                    },
                    canResend:
                        !widget.tripsController.isResendingPickupCredential,
                    isResending:
                        widget.tripsController.isResendingPickupCredential,
                  ),
                  if (AppModeConfig.instance.isMock && !verified) ...[
                    const SizedBox(height: 12),
                    OutlinedButton.icon(
                      onPressed: () =>
                          widget.tripsController.simulatePickupVerification(),
                      icon: const Icon(Icons.science_outlined),
                      label: const Text('Simulate customer scan'),
                    ),
                    Text(
                      'Mock only: marks the pickup code as verified.',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ],
              ),
            );
          },
        ),

        TripState.tripStarted => TripStartedSheet(
          request: activeRequest!,
          onEndTrip: () {
            _tripStateNotifier.value = TripState.tripCompleted;
          },
          onReportFailure: () => _runTripAction(_handleFailDelivery),
          busy: _actionBusy.value,
          collapsed: _sheetCollapsedNotifier.value,
          onToggleCollapse: _toggleSheetCollapsed,
          peekStatusText: _peekStatusFor(tripState),
        ),

        TripState.tripCompleted => TripCompletedScreen(
          request: activeRequest!,
          tripId: _activeTripId,
          tripsController: widget.tripsController,
          onConfirm: _handleTripCompleted,
          onBack: () {
            _tripStateNotifier.value = TripState.tripStarted;
            _openTripWorkspace();
          },
        ),
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final isHome = _index == 0;
    final colors = context.palette;

    return PopScope(
      canPop: _index == 0 && _tripStateNotifier.value == TripState.offline,
      onPopInvokedWithResult: (didPop, _) {
        if (didPop) return;
        if (_index != 0) {
          setState(() => _index = 0);
          return;
        }
        final state = _tripStateNotifier.value;
        if (state == TripState.arriving ||
            state == TripState.arrivedAtPickup ||
            state == TripState.tripStarted) {
          _confirmLeaveTrip();
        }
      },
      child: Scaffold(
        extendBody: false,
        body: ValueListenableBuilder<TripState>(
          valueListenable: _tripStateNotifier,
          builder: (context, tripState, _) {
            if (tripState == TripState.tripCompleted) {
              return Scaffold(
                backgroundColor: colors.surface,
                body: TripCompletedScreen(
                  request: _activeRequestNotifier.value!,
                  tripId: _activeTripId,
                  tripsController: widget.tripsController,
                  onConfirm: _handleTripCompleted,
                  onBack: () {
                    _tripStateNotifier.value = TripState.tripStarted;
                    _openTripWorkspace();
                  },
                ),
              );
            }
            return LayoutBuilder(
              builder: (context, constraints) => Stack(
                fit: StackFit.expand,
                children: [
                  IndexedStack(
                    sizing: StackFit.expand,
                    index: _index,
                    children: [
                      for (var i = 0; i < _screens.length; i++)
                        _visitedTabs.contains(i)
                            ? _screens[i]
                            : const SizedBox.shrink(),
                    ],
                  ),
                  if (isHome)
                    Positioned(
                      left: 0,
                      right: 0,
                      bottom: 0,
                      child: ConstrainedBox(
                        constraints: BoxConstraints(
                          maxHeight: constraints.maxHeight * .7,
                        ),
                        child: SingleChildScrollView(
                          child: ValueListenableBuilder<bool>(
                            valueListenable: _sheetCollapsedNotifier,
                            builder: (context, collapsed, _) =>
                                ValueListenableBuilder<RequestModel?>(
                                  valueListenable: _activeRequestNotifier,
                                  builder: (context, request, _) =>
                                      (tripState == TripState.arriving ||
                                          tripState ==
                                              TripState.arrivedAtPickup ||
                                          tripState == TripState.tripStarted)
                                      ? TripPeekBar(
                                          stage:
                                              tripState == TripState.tripStarted
                                              ? 3
                                              : tripState ==
                                                    TripState.arrivedAtPickup
                                              ? 2
                                              : 1,
                                          title:
                                              tripState == TripState.tripStarted
                                              ? 'Delivery in progress'
                                              : tripState ==
                                                    TripState.arrivedAtPickup
                                              ? 'Pickup verification'
                                              : 'Head to pickup',
                                          statusText:
                                              'Open instructions and delivery actions',
                                          onExpand: _openTripWorkspace,
                                        )
                                      : _buildSheet(tripState, request),
                                ),
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            );
          },
        ),
        bottomNavigationBar: ValueListenableBuilder<TripState>(
          valueListenable: _tripStateNotifier,
          builder: (context, state, _) => state == TripState.tripCompleted
              ? const SizedBox.shrink()
              : _BottomNav(index: _index, onChanged: _onTabChanged),
        ),
      ),
    );
  }
}

// ─── Bottom navigation ──────────────────────────────────────────────────────

class _BottomNav extends StatelessWidget {
  const _BottomNav({required this.index, required this.onChanged});

  final int index;
  final ValueChanged<int> onChanged;

  static const _items = [
    _NavItem(icon: Icons.home_rounded, label: 'Home'),
    _NavItem(icon: Icons.swap_vert_rounded, label: 'Opportunities'),
    _NavItem(icon: Icons.calendar_month_outlined, label: 'Activity'),
    _NavItem(icon: Icons.account_balance_wallet_outlined, label: 'Wallet'),
    _NavItem(icon: Icons.person_outline_rounded, label: 'Account'),
  ];

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return SafeArea(
      top: false,
      child: Container(
        decoration: BoxDecoration(
          color: colors.surface,
          border: Border(top: BorderSide(color: colors.divider)),
        ),
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceAround,
          children: List.generate(_items.length, (i) {
            final isActive = i == index;
            final item = _items[i];
            final color = isActive ? colors.brandTeal600 : colors.textMuted;
            return Semantics(
              selected: isActive,
              button: true,
              label: item.label,
              child: InkWell(
                onTap: () => onChanged(i),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    SizedBox(
                      height: 28,
                      width: 48,
                      child: Icon(item.icon, size: 24, color: color),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      item.label,
                      style: TextStyle(
                        fontSize: 11,
                        fontWeight: isActive
                            ? FontWeight.w700
                            : FontWeight.w500,
                        color: color,
                      ),
                    ),
                  ],
                ),
              ),
            );
          }),
        ),
      ),
    );
  }
}

class _NavItem {
  const _NavItem({required this.icon, required this.label});
  final IconData icon;
  final String label;
}
