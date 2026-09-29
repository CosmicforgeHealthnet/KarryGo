import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:latlong2/latlong.dart' hide Path;

import '../../availability/state/availability_controller.dart';
import '../../requests/data/request_model.dart';
import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';

import 'widgets/header_bar.dart';
import 'widgets/driver_pin.dart';
import 'widgets/map_area_label.dart';
import 'widgets/locate_button.dart';
import 'sheets/offline_sheet.dart';
import 'sheets/incoming_requests_sheet.dart';
import 'sheets/trip_sheet.dart';
import 'sheets/trip_started_sheet.dart';

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
  static const _center = LatLng(6.5355, 3.2910);
  static const _ikoyiLabelPos = LatLng(6.5390, 3.2965);
  static const _victoriaIslandLabelPos = LatLng(6.5280, 3.2830);
  static const _lekkiLabelPos = LatLng(6.5300, 3.3060);

  TripState get _tripState => widget.tripStateNotifier.value;

  String get _todayEarnings {
    if (AppModeConfig.instance.isMock) return 'N24,350';
    return widget.todayEarningsDisplay;
  }

  String get _acceptanceRate {
    if (AppModeConfig.instance.isMock) return '92%';
    return widget.acceptanceRateDisplay;
  }

  @override
  void initState() {
    super.initState();
    widget.tripStateNotifier.addListener(_onNotifierChanged);
    widget.activeRequestNotifier.addListener(_onNotifierChanged);
    widget.availabilityController.addListener(_onNotifierChanged);
    _loadInitialAvailability();
  }

  @override
  void dispose() {
    widget.tripStateNotifier.removeListener(_onNotifierChanged);
    widget.activeRequestNotifier.removeListener(_onNotifierChanged);
    widget.availabilityController.removeListener(_onNotifierChanged);
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
      failure: (_) {},
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return AnimatedBuilder(
      animation: AppModeConfig.instance,
      builder: (context, _) => ValueListenableBuilder<TripState>(
        valueListenable: widget.tripStateNotifier,
        builder: (context, tripState, _) {
          final isChanging = widget.availabilityController.isChangingStatus ||
              widget.availabilityController.isLoading;

          return Column(
            children: [
              SafeArea(
                bottom: false,
                child: Container(
                  width: double.infinity,
                  color: colors.surface,
                  padding: const EdgeInsets.fromLTRB(16, 12, 16, 14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      HomeHeaderBar(
                        avatarImageProvider: widget.avatarImageProvider,
                        onOpenNotifications: widget.onOpenNotifications,
                      ),
                      const SizedBox(height: 14),
                      _StatusCard(
                        tripState: tripState,
                        isChanging: isChanging,
                        onToggleStatus: widget.onToggleStatus,
                        todayEarningsDisplay: _todayEarnings,
                        acceptanceRateDisplay: _acceptanceRate,
                        // Option C — hide the mock earnings/rate row until a
                        // real endpoint exists. Only shown in MOCK mode.
                        showStatsRow: AppModeConfig.instance.isMock,
                      ),
                    ],
                  ),
                ),
              ),
              Expanded(
                child: Stack(
                  children: [
                    Positioned.fill(
                      child: FlutterMap(
                        options: const MapOptions(
                          initialCenter: _center,
                          initialZoom: 14,
                        ),
                        children: [
                          TileLayer(
                            urlTemplate:
                                'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
                            userAgentPackageName:
                                'com.cosmicforge.logistics.dispatchprovider',
                          ),
                          if (tripState == TripState.arriving ||
                              tripState == TripState.arrivedAtPickup ||
                              tripState == TripState.tripStarted)
                            PolylineLayer(
                              polylines: [
                                Polyline(
                                  points: [
                                    const LatLng(6.5420, 3.2850),
                                    const LatLng(6.5390, 3.2870),
                                    const LatLng(6.5370, 3.2900),
                                    const LatLng(6.5355, 3.2910),
                                    const LatLng(6.5330, 3.2940),
                                    const LatLng(6.5300, 3.2980),
                                  ],
                                  color: tripState == TripState.tripStarted
                                      ? colors.brandNavy900
                                      : Colors.black54,
                                  strokeWidth: 4,
                                  pattern: tripState == TripState.arriving
                                      ? StrokePattern.dashed(
                                          segments: const [10, 6])
                                      : StrokePattern.solid(),
                                ),
                              ],
                            ),
                          MarkerLayer(
                            markers: [
                              const Marker(
                                point: _center,
                                width: 44,
                                height: 58,
                                child: DriverPin(),
                              ),
                              const Marker(
                                point: _ikoyiLabelPos,
                                width: 90,
                                height: 24,
                                child: MapAreaLabel(label: 'Ikoyi'),
                              ),
                              const Marker(
                                point: _victoriaIslandLabelPos,
                                width: 130,
                                height: 24,
                                child: MapAreaLabel(label: 'Victoria Island'),
                              ),
                              const Marker(
                                point: _lekkiLabelPos,
                                width: 90,
                                height: 24,
                                child: MapAreaLabel(label: 'Lekki'),
                              ),
                              if (tripState == TripState.arriving ||
                                  tripState == TripState.arrivedAtPickup ||
                                  tripState == TripState.tripStarted)
                                Marker(
                                  point: const LatLng(6.5420, 3.2850),
                                  width: 80,
                                  height: 30,
                                  child: const _MapLabel(label: 'Pick up'),
                                ),
                              if (tripState == TripState.arriving ||
                                  tripState == TripState.arrivedAtPickup ||
                                  tripState == TripState.tripStarted)
                                Marker(
                                  point: const LatLng(6.5300, 3.2980),
                                  width: 96,
                                  height: 30,
                                  child:
                                      const _MapLabel(label: 'Destination'),
                                ),
                            ],
                          ),
                        ],
                      ),
                    ),
                    Positioned(
                      right: 16,
                      bottom: tripState == TripState.offline ? 150 : 16,
                      child: LocateButton(onPressed: widget.onLocateMe),
                    ),
                  ],
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

// ── Private widgets that stay in home_screen ─────────────────────────────

class _StatusCard extends StatelessWidget {
  const _StatusCard({
    required this.tripState,
    required this.isChanging,
    required this.todayEarningsDisplay,
    required this.acceptanceRateDisplay,
    this.showStatsRow = true,
    this.onToggleStatus,
  });

  final TripState tripState;
  final bool isChanging;
  final String todayEarningsDisplay;
  final String acceptanceRateDisplay;

  /// When false, only the status pill is drawn — the earnings/rate columns
  /// are hidden. Used in LIVE mode until the earnings endpoint ships.
  final bool showStatsRow;

  final VoidCallback? onToggleStatus;

  bool get _isOnMission =>
      tripState == TripState.arriving ||
      tripState == TripState.arrivedAtPickup ||
      tripState == TripState.tripStarted;

  bool get _isTogglable =>
      tripState == TripState.offline || tripState == TripState.online;

  String get _statusLabel {
    if (_isOnMission) return 'On a mission';
    if (tripState == TripState.offline) return 'Offline';
    return 'Online';
  }

  String get _statusSubtext {
    if (_isOnMission) return 'System controlled';
    if (tripState == TripState.offline) {
      return "You're not available for dispatch";
    }
    return "You're visible to nearby customers";
  }

  Color _statusColor(BuildContext context) {
    final colors = context.palette;
    if (_isOnMission) return colors.warning;
    if (tripState == TripState.offline) return colors.danger;
    return colors.success;
  }

  Color _statusSoftBg(BuildContext context) {
    final colors = context.palette;
    if (_isOnMission) return colors.warningSoft;
    if (tripState == TripState.offline) return colors.dangerSoft;
    return colors.successSoft;
  }

  IconData get _statusIcon {
    if (_isOnMission) return Icons.route;
    if (tripState == TripState.offline) return Icons.power_settings_new;
    return Icons.check;
  }

  @override
  Widget build(BuildContext context) {
    final statusColor = _statusColor(context);
    final statusBg = _statusSoftBg(context);

    final pill = GestureDetector(
      onTap: (_isTogglable && !isChanging) ? onToggleStatus : null,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: statusBg,
          borderRadius: BorderRadius.circular(14),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (isChanging)
                  const SizedBox(
                    width: 14,
                    height: 14,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                else
                  Container(
                    width: 14,
                    height: 14,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: statusColor,
                      shape: BoxShape.circle,
                    ),
                    child: Icon(_statusIcon, size: 9, color: Colors.white),
                  ),
                const SizedBox(width: 6),
                Text(
                  _statusLabel,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                    color: statusColor,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 2),
            Text(
              _statusSubtext,
              style: TextStyle(fontSize: 10, color: statusColor),
            ),
          ],
        ),
      ),
    );

    if (!showStatsRow) {
      return SizedBox(width: double.infinity, child: pill);
    }

    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        pill,
        const SizedBox(width: 16),
        Expanded(
          child: _StatColumn(
            value: todayEarningsDisplay,
            label: "Today's earnings",
          ),
        ),
        Expanded(
          child: _StatColumn(
            value: acceptanceRateDisplay,
            label: 'Acceptance rate',
          ),
        ),
      ],
    );
  }
}

class _StatColumn extends StatelessWidget {
  const _StatColumn({required this.value, required this.label});
  final String value;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          value,
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w800,
            color: colors.text,
          ),
        ),
        const SizedBox(height: 3),
        Text(
          label,
          style: TextStyle(fontSize: 10, color: colors.textMuted),
        ),
      ],
    );
  }
}

class _MapLabel extends StatelessWidget {
  const _MapLabel({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      alignment: Alignment.center,
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: Colors.black,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        label,
        style: const TextStyle(color: Colors.white, fontSize: 11),
      ),
    );
  }
}